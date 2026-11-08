package anthropic

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"

	"github.com/sfloresone/flores/apps/cli/internal/provider"
)

// streamEvent is the union of the SSE payloads we read. Every payload carries
// its own "type", so the SSE "event:" line is not needed.
type streamEvent struct {
	Type    string `json:"type"`
	Index   int    `json:"index"`
	Message *struct {
		Usage wireUsage `json:"usage"`
	} `json:"message"`
	ContentBlock *struct {
		Type string `json:"type"`
		ID   string `json:"id"`
		Name string `json:"name"`
		Text string `json:"text"`
	} `json:"content_block"`
	Delta *struct {
		Type        string `json:"type"`
		Text        string `json:"text"`
		PartialJSON string `json:"partial_json"`
		StopReason  string `json:"stop_reason"`
	} `json:"delta"`
	Usage *wireUsage `json:"usage"`
	Error *struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

type wireUsage struct {
	InputTokens   int `json:"input_tokens"`
	OutputTokens  int `json:"output_tokens"`
	CacheCreation int `json:"cache_creation_input_tokens"`
	CacheRead     int `json:"cache_read_input_tokens"`
}

// toolBlock collects one tool_use block while its JSON input streams in.
type toolBlock struct {
	id, name string
	input    strings.Builder
}

// readStream turns the SSE body into provider events until message_stop.
func readStream(ctx context.Context, body io.Reader, yield func(provider.Event, error) bool) {
	var (
		usage   provider.Usage
		rawStop string
		tools   = map[int]*toolBlock{}
	)
	err := provider.ReadSSE(body, func(data []byte) (bool, error) {
		var ev streamEvent
		if err := json.Unmarshal(data, &ev); err != nil {
			return false, &provider.Error{Kind: provider.ErrInvalidResponse, Err: err}
		}
		switch ev.Type {
		case "message_start":
			if ev.Message != nil {
				usage = usageOf(ev.Message.Usage)
			}
		case "content_block_start":
			if ev.ContentBlock == nil {
				break
			}
			switch ev.ContentBlock.Type {
			case "text":
				if ev.ContentBlock.Text != "" && !yield(provider.Event{Kind: provider.EventTextDelta, Text: ev.ContentBlock.Text}, nil) {
					return false, nil
				}
			case "tool_use":
				tools[ev.Index] = &toolBlock{id: ev.ContentBlock.ID, name: ev.ContentBlock.Name}
			}
		case "content_block_delta":
			if ev.Delta == nil {
				break
			}
			switch ev.Delta.Type {
			case "text_delta":
				if !yield(provider.Event{Kind: provider.EventTextDelta, Text: ev.Delta.Text}, nil) {
					return false, nil
				}
			case "input_json_delta":
				if t := tools[ev.Index]; t != nil {
					t.input.WriteString(ev.Delta.PartialJSON)
				}
			}
		case "content_block_stop":
			t := tools[ev.Index]
			if t == nil {
				break
			}
			delete(tools, ev.Index)
			input := t.input.String()
			if input == "" { // a tool with no arguments streams no deltas
				input = "{}"
			}
			call := &provider.ToolCall{ID: t.id, Name: t.name, Input: json.RawMessage(input)}
			if !yield(provider.Event{Kind: provider.EventToolCall, ToolCall: call}, nil) {
				return false, nil
			}
		case "message_delta":
			if ev.Delta != nil && ev.Delta.StopReason != "" {
				rawStop = ev.Delta.StopReason
			}
			if ev.Usage != nil { // output_tokens here is the final total
				usage.OutputTokens = ev.Usage.OutputTokens
				if ev.Usage.InputTokens > 0 {
					usage.InputTokens = ev.Usage.InputTokens
				}
			}
		case "message_stop":
			yield(provider.Event{Kind: provider.EventDone, Usage: usage, Stop: stopReason(rawStop), RawStop: rawStop}, nil)
			return false, nil
		case "error":
			return false, streamError(ev)
		}
		return true, nil // "ping" and unknown types are ignored
	})
	switch {
	case err == nil:
		return
	case ctx.Err() != nil:
		yield(provider.Event{}, ctx.Err())
	default:
		var pe *provider.Error
		if !errors.As(err, &pe) {
			pe = &provider.Error{Kind: provider.ErrTruncated, Err: err}
		}
		yield(provider.Event{}, pe)
	}
}

func usageOf(u wireUsage) provider.Usage {
	return provider.Usage{
		InputTokens:      u.InputTokens,
		OutputTokens:     u.OutputTokens,
		CacheReadTokens:  u.CacheRead,
		CacheWriteTokens: u.CacheCreation,
	}
}

func stopReason(raw string) provider.StopReason {
	switch raw {
	case "end_turn":
		return provider.StopEndTurn
	case "tool_use":
		return provider.StopToolUse
	case "max_tokens":
		return provider.StopMaxTokens
	}
	return provider.StopOther
}

// streamError classifies an "error" event sent in the middle of a stream.
func streamError(ev streamEvent) *provider.Error {
	e := &provider.Error{Kind: provider.ErrOverloaded}
	if ev.Error == nil {
		return e
	}
	e.Message = ev.Error.Message
	switch ev.Error.Type {
	case "rate_limit_error":
		e.Kind = provider.ErrRateLimit
	case "authentication_error", "permission_error":
		e.Kind = provider.ErrAuth
	case "invalid_request_error", "not_found_error", "request_too_large":
		e.Kind = provider.ErrBadRequest
	}
	return e
}
