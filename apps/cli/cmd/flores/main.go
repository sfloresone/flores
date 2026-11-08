// Command flores is the flores coding agent.
//
// This file only wires things together: flags and environment → provider →
// agent loop → output (text or a trajectory/v1 JSONL stream). The
// interesting code lives in internal/ (see ARCHITECTURE.md).
package main

import (
	"cmp"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"iter"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/sfloresone/flores/apps/cli/internal/agent"
	"github.com/sfloresone/flores/apps/cli/internal/provider"
	"github.com/sfloresone/flores/apps/cli/internal/provider/anthropic"
	"github.com/sfloresone/flores/apps/cli/internal/provider/openai"
	"github.com/sfloresone/flores/apps/cli/internal/tool"
	"github.com/sfloresone/flores/apps/cli/internal/trajectory"
)

// version is overridden at release time with -ldflags "-X main.version=…".
var version = "0.0.0-dev"

// basePrompt is deliberately tiny: every byte is sent on every step.
const basePrompt = "You are flores, a coding agent. Be concise."

// systemPrompt adds what the tools need to know: paths are relative to the
// working directory, and commands depend on the OS.
func systemPrompt(cwd string, tools bool) string {
	if !tools {
		return basePrompt
	}
	return basePrompt + "\nWorking directory: " + cwd + "\nOS: " + runtime.GOOS
}

// Exit codes (ADR 0003). The benchmark contract relies on them.
const (
	exitOK        = 0
	exitUsage     = 2
	exitProvider  = 3
	exitBrake     = 4
	exitCancelled = 130
)

const usageText = `flores - a small, provider-agnostic coding agent

Usage:
  flores -p PROMPT [flags]    run one turn and exit (prompt from stdin if omitted)
  flores --version            print the version

Flags:
  -p, --prompt TEXT     the task
  --output MODE         text (default) or jsonl (trajectory/v1 events on stdout)
  --provider NAME       anthropic (default) or openai; openai also covers
                        compatible servers (Ollama, vLLM, OpenRouter...)
                                                 (env FLORES_PROVIDER)
  --model ID            model id                 (env FLORES_MODEL)
  --base-url URL        provider base URL        (env FLORES_BASE_URL)
                        e.g. http://localhost:11434/v1 for Ollama
  --api-key KEY         API key                  (env FLORES_API_KEY, else
                        ANTHROPIC_API_KEY or OPENAI_API_KEY)
  --max-steps N         stop after N model calls; 0 = no limit (default 0)
  --max-retries N       resend a call that failed transiently up to N times;
                        0 = never (default 6, env FLORES_MAX_RETRIES)
  --timeout DUR         overall timeout, e.g. 10m; 0 = none (default 0)
  --no-tools            give the model no tools. By default it has bash, read,
                        edit, write, grep and glob in the current directory,
                        and they run without asking for permission

Exit codes: 0 success, 2 usage, 3 provider error or timeout,
            4 stopped by a brake (step limit or repeated tool call), 130 cancelled.
`

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

type options struct {
	prompt, output, model, baseURL, apiKey, providerName string
	maxSteps, maxRetries                                 int
	timeout                                              time.Duration
	version, noTools                                     bool
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		// Bare `flores` will start the interactive mode (REPL); not built yet.
		fmt.Fprint(stderr, usageText)
		return exitUsage
	}
	opts, code, ok := parse(args, stdin, stderr)
	if !ok {
		return code
	}
	if opts.version {
		fmt.Fprintf(stdout, "flores %s\n", version)
		return exitOK
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if opts.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, opts.timeout)
		defer cancel()
	}

	policy := provider.DefaultRetryPolicy()
	policy.MaxRetries = opts.maxRetries
	vendor := providers[opts.providerName]
	p := provider.WithRetry(vendor.new(opts), policy)
	cwd, _ := os.Getwd()
	cfg := agent.Config{Model: opts.model, System: systemPrompt(cwd, !opts.noTools), MaxSteps: opts.maxSteps}
	if !opts.noTools {
		// Full bash outputs too long to send are saved here. os.Remove only
		// removes an empty directory, so saved outputs outlive the run.
		tmp, err := os.MkdirTemp("", "flores-")
		if err == nil {
			defer os.Remove(tmp)
		}
		cfg.Tools, cfg.Env = tool.Defaults(), tool.Env{Dir: cwd, TempDir: tmp}
	}
	a := agent.New(p, cfg)

	var res *agent.Result
	if opts.output == "jsonl" {
		em := trajectory.New(stdout, trajectory.Session{
			AgentName: "flores", AgentVersion: version, Model: opts.model,
			ProviderName: opts.providerName, Protocol: vendor.protocol,
			BaseURL: cmp.Or(opts.baseURL, vendor.baseURL), Cwd: cwd,
		})
		for ev := range a.Run(ctx, opts.prompt) {
			em.Record(ev)
			if ev.Kind == agent.EventDone {
				res = ev.Result
			}
		}
	} else {
		res = printText(a.Run(ctx, opts.prompt), stdout, stderr, opts.maxRetries)
	}
	return report(res, opts, stderr)
}

// vendor is one provider flores can talk to. Adding a provider is a new
// adapter package and one entry here (ADR 0002).
type vendor struct {
	protocol string // trajectory session.start "protocol"
	baseURL  string // default base URL
	keyEnv   string // the vendor's own API key variable
	new      func(options) provider.Provider
}

var providers = map[string]vendor{
	"anthropic": {
		protocol: "anthropic-messages", baseURL: anthropic.DefaultBaseURL, keyEnv: "ANTHROPIC_API_KEY",
		new: func(o options) provider.Provider {
			return anthropic.New(anthropic.Config{
				BaseURL: o.baseURL, APIKey: o.apiKey, UserAgent: "flores/" + version,
				HTTPClient: &http.Client{}, // one client: the connection is reused across steps
			})
		},
	},
	"openai": {
		protocol: "openai-chat", baseURL: openai.DefaultBaseURL, keyEnv: "OPENAI_API_KEY",
		new: func(o options) provider.Provider {
			return openai.New(openai.Config{
				BaseURL: o.baseURL, APIKey: o.apiKey, UserAgent: "flores/" + version,
				HTTPClient: &http.Client{},
			})
		},
	},
}

func parse(args []string, stdin io.Reader, stderr io.Writer) (options, int, bool) {
	var o options
	fs := flag.NewFlagSet("flores", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&o.prompt, "p", "", "")
	fs.StringVar(&o.prompt, "prompt", "", "")
	fs.StringVar(&o.output, "output", "text", "")
	fs.StringVar(&o.model, "model", os.Getenv("FLORES_MODEL"), "")
	fs.StringVar(&o.baseURL, "base-url", os.Getenv("FLORES_BASE_URL"), "")
	fs.StringVar(&o.apiKey, "api-key", "", "")
	fs.StringVar(&o.providerName, "provider", cmp.Or(os.Getenv("FLORES_PROVIDER"), "anthropic"), "")
	fs.IntVar(&o.maxSteps, "max-steps", 0, "")
	defaultRetries := provider.DefaultRetryPolicy().MaxRetries
	if v := os.Getenv("FLORES_MAX_RETRIES"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			defaultRetries = n
		} else {
			defaultRetries = -1 // reported below
		}
	}
	fs.IntVar(&o.maxRetries, "max-retries", defaultRetries, "")
	fs.DurationVar(&o.timeout, "timeout", 0, "")
	fs.BoolVar(&o.noTools, "no-tools", false, "")
	fs.BoolVar(&o.version, "version", false, "")
	fs.BoolVar(&o.version, "v", false, "")
	help := fs.Bool("help", false, "")
	fs.BoolVar(help, "h", false, "")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(stderr, "error: %v\n\n%s", err, usageText)
		return o, exitUsage, false
	}
	if *help {
		fmt.Fprint(stderr, usageText)
		return o, exitOK, false
	}
	if o.version {
		return o, 0, true
	}
	vendor, known := providers[o.providerName]
	if known {
		// The key is looked up once the provider is known, so an
		// ANTHROPIC_API_KEY is never sent to OpenAI or the other way round.
		o.apiKey = cmp.Or(o.apiKey, os.Getenv("FLORES_API_KEY"), os.Getenv(vendor.keyEnv))
	}
	if o.prompt == "" && fs.NArg() > 0 {
		o.prompt = strings.Join(fs.Args(), " ")
	}
	if o.prompt == "" && stdin != nil {
		b, _ := io.ReadAll(stdin)
		o.prompt = strings.TrimSpace(string(b))
	}
	var problem string
	switch {
	case o.prompt == "":
		problem = "empty prompt: pass -p TEXT or pipe it on stdin"
	case o.output != "text" && o.output != "jsonl":
		problem = "--output must be text or jsonl"
	case !known:
		problem = fmt.Sprintf("unknown provider %q (--provider or FLORES_PROVIDER): use anthropic or openai", o.providerName)
	case o.model == "":
		problem = "no model: pass --model or set FLORES_MODEL"
	case o.apiKey == "":
		problem = fmt.Sprintf("no API key: pass --api-key or set FLORES_API_KEY / %s", vendor.keyEnv)
	case o.maxSteps < 0:
		problem = "--max-steps must be >= 0 (0 = no limit)"
	case o.maxRetries < 0:
		problem = "--max-retries (or FLORES_MAX_RETRIES) must be a number >= 0 (0 = never retry)"
	}
	if problem != "" {
		fmt.Fprintln(stderr, "error:", problem)
		return o, exitUsage, false
	}
	return o, 0, true
}

// printText streams the assistant text to stdout as it arrives. Retries
// are announced on stderr; text a failed attempt already printed stays,
// and the new attempt starts on a fresh line (ADR 0004).
func printText(events iter.Seq[agent.Event], stdout, stderr io.Writer, maxRetries int) *agent.Result {
	var res *agent.Result
	var last byte = '\n' // last byte written, to separate steps cleanly
	for ev := range events {
		switch ev.Kind {
		case agent.EventStepStart:
			if last != '\n' {
				io.WriteString(stdout, "\n")
				last = '\n'
			}
		case agent.EventRetry:
			if last != '\n' {
				io.WriteString(stdout, "\n")
				last = '\n'
			}
			r := ev.Retry
			fmt.Fprintf(stderr, "flores: %v — retrying in %s (attempt %d of %d)\n",
				r.Err, r.Delay.Round(time.Millisecond), r.Attempt, maxRetries+1)
		case agent.EventTextDelta:
			if ev.Text != "" {
				io.WriteString(stdout, ev.Text)
				last = ev.Text[len(ev.Text)-1]
			}
		case agent.EventDone:
			res = ev.Result
		}
	}
	if last != '\n' {
		io.WriteString(stdout, "\n")
	}
	return res
}

// report explains how the turn ended on stderr and picks the exit code.
func report(res *agent.Result, opts options, stderr io.Writer) int {
	switch res.Status {
	case agent.StatusSuccess:
		return exitOK
	case agent.StatusMaxSteps:
		fmt.Fprintf(stderr, "flores: stopped after reaching --max-steps=%d\n", opts.maxSteps)
		return exitBrake
	case agent.StatusRepetition:
		fmt.Fprintln(stderr, "flores: stopped because the same tool call kept returning the same result")
		return exitBrake
	case agent.StatusCancelled:
		fmt.Fprintln(stderr, "flores: cancelled")
		return exitCancelled
	}
	switch {
	case errors.Is(res.Err, context.DeadlineExceeded):
		fmt.Fprintf(stderr, "error: timed out after %s\n", opts.timeout)
	case res.Err != nil:
		fmt.Fprintln(stderr, "error:", res.Err)
	}
	var pe *provider.Error
	if errors.As(res.Err, &pe) {
		switch pe.Kind {
		case provider.ErrAuth:
			fmt.Fprintf(stderr, "hint: check the API key (FLORES_API_KEY or %s)\n", providers[opts.providerName].keyEnv)
		case provider.ErrQuota:
			fmt.Fprintln(stderr, "hint: the account has no credit or quota left; top it up, then run again")
		}
	}
	return exitProvider
}
