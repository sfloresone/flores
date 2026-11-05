// Command flores-stub is a minimal agent that honours the flores headless
// contract (benchmarks/docs/trajectory-schema.md) without doing anything
// useful: it runs a plain chat loop against an OpenAI Chat Completions
// endpoint and answers every tool call with an error.
//
// It exists so the whole measurement chain (adapters, mock provider, schema
// validation, reports) can be built and tested before the real agent exists,
// and it is the floor every real flores build is compared against.
package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	version      = "0.0.0-stub"
	systemPrompt = "You are flores-stub, a placeholder coding agent used to calibrate benchmarks."

	exitOK       = 0
	exitUsage    = 2
	exitProvider = 3
	exitLimit    = 4
)

const usageText = `flores-stub - placeholder agent for the flores benchmark suite

Usage:
  flores-stub run [flags] [--prompt TEXT]   run one task (prompt from stdin if omitted)
  flores-stub version                       print the version
  flores-stub --help                        show this help

Run flags:
  --prompt TEXT      task prompt
  --output MODE      text (default) or jsonl (trajectory events on stdout)
  --base-url URL     OpenAI-compatible base URL   (env FLORES_BASE_URL)
  --model NAME       model id                     (env FLORES_MODEL, default mock-model)
  --api-key KEY      API key                      (env FLORES_API_KEY)
  --max-turns N      stop after N model turns (default 8)
  --timeout DUR      overall timeout (default 5m)

Exit codes: 0 success, 2 usage, 3 provider error, 4 turn limit reached.
`

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usageText)
		return exitUsage
	}
	switch args[0] {
	case "version", "--version", "-v":
		fmt.Fprintf(stdout, "flores-stub %s\n", version)
		return exitOK
	case "help", "--help", "-h":
		fmt.Fprint(stdout, usageText)
		return exitOK
	case "run":
		return runCmd(args[1:], stdin, stdout, stderr)
	default:
		fmt.Fprintf(stderr, "unknown command %q\n\n%s", args[0], usageText)
		return exitUsage
	}
}

type config struct {
	prompt   string
	output   string
	baseURL  string
	model    string
	apiKey   string
	maxTurns int
	timeout  time.Duration
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func runCmd(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var cfg config
	fs.StringVar(&cfg.prompt, "prompt", "", "")
	fs.StringVar(&cfg.output, "output", "text", "")
	fs.StringVar(&cfg.baseURL, "base-url", envOr("FLORES_BASE_URL", ""), "")
	fs.StringVar(&cfg.model, "model", envOr("FLORES_MODEL", "mock-model"), "")
	fs.StringVar(&cfg.apiKey, "api-key", envOr("FLORES_API_KEY", ""), "")
	fs.IntVar(&cfg.maxTurns, "max-turns", 8, "")
	fs.DurationVar(&cfg.timeout, "timeout", 5*time.Minute, "")
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	if cfg.prompt == "" && fs.NArg() > 0 {
		cfg.prompt = strings.Join(fs.Args(), " ")
	}
	if cfg.prompt == "" {
		b, _ := io.ReadAll(stdin)
		cfg.prompt = strings.TrimSpace(string(b))
	}
	switch {
	case cfg.prompt == "":
		fmt.Fprintln(stderr, "error: empty prompt")
		return exitUsage
	case cfg.baseURL == "":
		fmt.Fprintln(stderr, "error: --base-url or FLORES_BASE_URL is required")
		return exitUsage
	case cfg.output != "text" && cfg.output != "jsonl":
		fmt.Fprintln(stderr, "error: --output must be text or jsonl")
		return exitUsage
	case cfg.maxTurns < 1:
		fmt.Fprintln(stderr, "error: --max-turns must be >= 1")
		return exitUsage
	}

	ctx, cancel := context.WithTimeout(context.Background(), cfg.timeout)
	defer cancel()
	em := newEmitter(stdout, cfg.output == "jsonl")
	res := agentLoop(ctx, cfg, em)
	if cfg.output == "text" {
		if res.finalText != "" {
			fmt.Fprintln(stdout, res.finalText)
		}
		if res.err != nil {
			fmt.Fprintln(stderr, "error:", res.err)
		}
	}
	return res.code
}

// ------------------------------------------------------------------ loop

type chatMessage struct {
	Role       string     `json:"role"`
	Content    *string    `json:"content"`
	ToolCalls  []toolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

type toolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type usage struct {
	InputTokens     int `json:"input_tokens"`
	OutputTokens    int `json:"output_tokens"`
	CacheReadTokens int `json:"cache_read_tokens"`
}

func (u *usage) add(o usage) {
	u.InputTokens += o.InputTokens
	u.OutputTokens += o.OutputTokens
	u.CacheReadTokens += o.CacheReadTokens
}

type loopResult struct {
	code      int
	finalText string
	err       error
}

func strPtr(s string) *string { return &s }

func agentLoop(ctx context.Context, cfg config, em *emitter) loopResult {
	cwd, _ := os.Getwd()
	em.emit("session.start", map[string]any{
		"schema_version": "trajectory/v0",
		"agent":          map[string]any{"name": "flores-stub", "version": version},
		"model":          cfg.model,
		"provider":       map[string]any{"name": "openai-compatible", "protocol": "openai-chat", "base_url": cfg.baseURL},
		"cwd":            cwd,
		"started_at":     em.start.UTC().Format(time.RFC3339Nano),
	})

	msgs := []chatMessage{
		{Role: "system", Content: strPtr(systemPrompt)},
		{Role: "user", Content: strPtr(cfg.prompt)},
	}
	var total usage
	toolCalls, toolErrors, turns := 0, 0, 0
	end := func(status string, res loopResult) loopResult {
		em.emit("session.end", map[string]any{
			"status": status, "turns": turns, "duration_ms": em.elapsed(),
			"usage": total, "tool_calls": toolCalls, "tool_errors": toolErrors,
			"final_text": res.finalText,
		})
		return res
	}

	for turn := 1; turn <= cfg.maxTurns; turn++ {
		turns = turn
		em.emit("turn.start", map[string]any{"turn": turn})
		body, _ := json.Marshal(map[string]any{
			"model": cfg.model, "stream": true,
			"stream_options": map[string]any{"include_usage": true},
			"messages":       msgs,
		})
		em.emit("llm.request", map[string]any{
			"turn": turn, "request_bytes": len(body), "messages": len(msgs),
			"tools": 0, "system_bytes": len(systemPrompt), "attempt": 1,
		})

		resp, err := callChat(ctx, cfg, body)
		ev := map[string]any{
			"turn": turn, "duration_ms": resp.durationMs, "ttft_ms": resp.ttftMs,
			"http_status": resp.httpStatus, "stop_reason": resp.finishReason, "usage": resp.usage,
		}
		if err != nil {
			ev["status"] = "error"
			em.emit("llm.response", ev)
			retryable := resp.httpStatus == 429 || resp.httpStatus >= 500 || resp.httpStatus == 0
			em.emit("error", map[string]any{"kind": "provider", "message": err.Error(), "retryable": retryable})
			return end("error", loopResult{code: exitProvider, err: err})
		}
		ev["status"] = "ok"
		em.emit("llm.response", ev)
		total.add(resp.usage)

		if resp.text != "" {
			em.emit("message", map[string]any{"turn": turn, "role": "assistant", "text": resp.text})
		}
		assistant := chatMessage{Role: "assistant", Content: strPtr(resp.text), ToolCalls: resp.toolCalls}
		msgs = append(msgs, assistant)
		if len(resp.toolCalls) == 0 {
			return end("success", loopResult{code: exitOK, finalText: resp.text})
		}
		for _, tc := range resp.toolCalls {
			toolCalls++
			toolErrors++
			var args any = tc.Function.Arguments
			var parsed any
			if json.Unmarshal([]byte(tc.Function.Arguments), &parsed) == nil {
				args = parsed
			}
			em.emit("tool.call", map[string]any{"turn": turn, "id": tc.ID, "name": tc.Function.Name, "arguments": args})
			out := fmt.Sprintf("error: flores-stub has no tools (requested %q)", tc.Function.Name)
			em.emit("tool.result", map[string]any{
				"turn": turn, "id": tc.ID, "ok": false, "duration_ms": 0.0,
				"output_bytes": len(out), "error": "tool not implemented",
			})
			msgs = append(msgs, chatMessage{Role: "tool", Content: strPtr(out), ToolCallID: tc.ID})
		}
	}
	err := fmt.Errorf("reached --max-turns=%d", cfg.maxTurns)
	em.emit("error", map[string]any{"kind": "limit", "message": err.Error(), "retryable": false})
	return end("max_turns", loopResult{code: exitLimit, err: err})
}

// ------------------------------------------------------------ provider

type chatResult struct {
	text         string
	toolCalls    []toolCall
	finishReason *string
	usage        usage
	httpStatus   int
	ttftMs       *float64
	durationMs   float64
}

// callChat uses named results so the deferred duration lands in the value
// actually returned (a defer cannot modify an already-copied return value).
func callChat(ctx context.Context, cfg config, body []byte) (res chatResult, err error) {
	start := time.Now()
	defer func() { res.durationMs = msSince(start) }()

	url := strings.TrimRight(cfg.baseURL, "/") + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return res, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("User-Agent", "flores-stub/"+version)
	if cfg.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.apiKey)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return res, err
	}
	defer resp.Body.Close()
	res.httpStatus = resp.StatusCode
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return res, fmt.Errorf("provider returned HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}

	var text strings.Builder
	calls := map[int]*toolCall{}
	order := []int{}
	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 64*1024), 8<<20)
	for sc.Scan() {
		line := sc.Text()
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "[DONE]" {
			break
		}
		var chunk struct {
			Choices []struct {
				Delta struct {
					Content   string `json:"content"`
					ToolCalls []struct {
						Index    int    `json:"index"`
						ID       string `json:"id"`
						Function struct {
							Name      string `json:"name"`
							Arguments string `json:"arguments"`
						} `json:"function"`
					} `json:"tool_calls"`
				} `json:"delta"`
				FinishReason *string `json:"finish_reason"`
			} `json:"choices"`
			Usage *struct {
				PromptTokens        int `json:"prompt_tokens"`
				CompletionTokens    int `json:"completion_tokens"`
				PromptTokensDetails *struct {
					CachedTokens int `json:"cached_tokens"`
				} `json:"prompt_tokens_details"`
			} `json:"usage"`
		}
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			return res, fmt.Errorf("bad stream chunk: %w", err)
		}
		if chunk.Usage != nil {
			res.usage.InputTokens = chunk.Usage.PromptTokens
			res.usage.OutputTokens = chunk.Usage.CompletionTokens
			if chunk.Usage.PromptTokensDetails != nil {
				res.usage.CacheReadTokens = chunk.Usage.PromptTokensDetails.CachedTokens
			}
		}
		for _, ch := range chunk.Choices {
			if ch.FinishReason != nil {
				res.finishReason = ch.FinishReason
			}
			if (ch.Delta.Content != "" || len(ch.Delta.ToolCalls) > 0) && res.ttftMs == nil {
				t := msSince(start)
				res.ttftMs = &t
			}
			text.WriteString(ch.Delta.Content)
			for _, d := range ch.Delta.ToolCalls {
				tc, ok := calls[d.Index]
				if !ok {
					tc = &toolCall{Type: "function"}
					calls[d.Index] = tc
					order = append(order, d.Index)
				}
				if d.ID != "" {
					tc.ID = d.ID
				}
				tc.Function.Name += d.Function.Name
				tc.Function.Arguments += d.Function.Arguments
			}
		}
	}
	if err := sc.Err(); err != nil {
		return res, err
	}
	res.text = text.String()
	for i, idx := range order {
		tc := calls[idx]
		if tc.ID == "" {
			tc.ID = fmt.Sprintf("call_%d", i)
		}
		res.toolCalls = append(res.toolCalls, *tc)
	}
	if res.finishReason == nil {
		return res, errors.New("stream ended without finish_reason")
	}
	return res, nil
}

func msSince(t time.Time) float64 { return float64(time.Since(t).Microseconds()) / 1000 }
