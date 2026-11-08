package openai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/sfloresone/flores/apps/cli/internal/provider"
)

// chunk is one "chat.completion.chunk" of the stream. Only the fields flores
// uses are decoded.
type chunk struct {
	Choices []struct {
		Delta struct {
			Content   string          `json:"content"`
			Refusal   string          `json:"refusal"`
			ToolCalls []toolCallDelta `json:"tool_calls"`
		} `json:"delta"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage *wireUsage      `json:"usage"`
	Error json.RawMessage `json:"error"`
}

type toolCallDelta struct {
	Index    *int   `json:"index"`
	ID       string `json:"id"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type wireUsage struct {
	PromptTokens        int `json:"prompt_tokens"`
	CompletionTokens    int `json:"completion_tokens"`
	PromptTokensDetails *struct {
		CachedTokens int `json:"cached_tokens"`
	} `json:"prompt_tokens_details"`
}

// pendingCall collects one tool call while its arguments stream in.
type pendingCall struct {
	id, name string
	args     strings.Builder
}

// calls assembles tool calls from their deltas. A call is keyed by its
// "index"; a different id on an index already in use starts a new call
// (some compatible servers send every parallel call with index 0), and a
// delta without an index continues the last call.
type calls struct {
	list    []*pendingCall
	byIndex map[int]*pendingCall
	last    *pendingCall
}

func (cs *calls) add(d toolCallDelta) {
	var cur *pendingCall
	if d.Index != nil {
		cur = cs.byIndex[*d.Index]
	} else {
		cur = cs.last
	}
	if cur == nil || (d.ID != "" && cur.id != "" && d.ID != cur.id) {
		cur = &pendingCall{}
		cs.list = append(cs.list, cur)
		if d.Index != nil {
			if cs.byIndex == nil {
				cs.byIndex = map[int]*pendingCall{}
			}
			cs.byIndex[*d.Index] = cur
		}
	}
	cs.last = cur
	if cur.id == "" {
		cur.id = d.ID
	}
	if cur.name == "" {
		cur.name = d.Function.Name
	}
	cur.args.WriteString(d.Function.Arguments)
}

// readStream turns the SSE body into provider events. The stream ends with
// "data: [DONE]"; a body that ends after a finish_reason without it is
// accepted too (the answer is complete), one that ends before is truncated.
func readStream(ctx context.Context, body io.Reader, yield func(provider.Event, error) bool) {
	var (
		usage   provider.Usage
		rawStop string
		pending calls
	)
	finish := func() {
		for i, c := range pending.list {
			args := strings.TrimSpace(c.args.String())
			if args == "" { // a tool with no arguments
				args = "{}"
			}
			id := c.id
			if id == "" {
				id = fmt.Sprintf("call_%d", i)
			}
			call := &provider.ToolCall{ID: id, Name: c.name, Input: json.RawMessage(args)}
			if !yield(provider.Event{Kind: provider.EventToolCall, ToolCall: call}, nil) {
				return
			}
		}
		yield(provider.Event{Kind: provider.EventDone, Usage: usage, Stop: stopReason(rawStop, len(pending.list) > 0), RawStop: rawStop}, nil)
	}

	err := provider.ReadSSE(body, func(data []byte) (bool, error) {
		if string(data) == "[DONE]" {
			finish()
			return false, nil
		}
		var c chunk
		if err := json.Unmarshal(data, &c); err != nil {
			return false, &provider.Error{Kind: provider.ErrInvalidResponse, Err: err}
		}
		if apiErr, ok := parseAPIError(c.Error); ok {
			return false, streamError(apiErr)
		}
		if c.Usage != nil { // the last chunk, with empty choices; keep the latest
			usage = usageOf(*c.Usage)
		}
		if len(c.Choices) == 0 {
			return true, nil
		}
		ch := c.Choices[0] // flores never asks for more than one (n=1)
		// A refusal is the model's answer: show it like any other text.
		for _, text := range []string{ch.Delta.Content, ch.Delta.Refusal} {
			if text != "" && !yield(provider.Event{Kind: provider.EventTextDelta, Text: text}, nil) {
				return false, nil
			}
		}
		for _, d := range ch.Delta.ToolCalls {
			pending.add(d)
		}
		if ch.FinishReason != "" {
			rawStop = ch.FinishReason
		}
		return true, nil
	})
	switch {
	case err == nil:
		return
	case ctx.Err() != nil:
		yield(provider.Event{}, ctx.Err())
	case errors.Is(err, provider.ErrStreamEnded) && rawStop != "":
		finish() // complete answer, only [DONE] is missing
	default:
		var pe *provider.Error
		if !errors.As(err, &pe) {
			pe = &provider.Error{Kind: provider.ErrTruncated, Err: err}
		}
		yield(provider.Event{}, pe)
	}
}

// usageOf normalizes usage: prompt_tokens includes the cached input, while
// provider.Usage keeps it apart (ADR 0005).
func usageOf(u wireUsage) provider.Usage {
	cached := 0
	if u.PromptTokensDetails != nil {
		cached = u.PromptTokensDetails.CachedTokens
	}
	return provider.Usage{
		InputTokens:     max(0, u.PromptTokens-cached),
		OutputTokens:    u.CompletionTokens,
		CacheReadTokens: cached,
	}
}

// stopReason maps finish_reason. Some servers say "stop" even when the
// answer has tool calls; the calls are what matters.
func stopReason(raw string, hasCalls bool) provider.StopReason {
	switch raw {
	case "length":
		return provider.StopMaxTokens
	case "tool_calls", "function_call":
		return provider.StopToolUse
	case "stop", "end", "":
		if hasCalls {
			return provider.StopToolUse
		}
		return provider.StopEndTurn
	}
	return provider.StopOther // content_filter and anything new
}
