package anthropic

import (
	"bufio"
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
	err := readSSE(body, func(data []byte) (bool, error) {
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

// errNoStop marks a stream that ended without message_stop.
var errNoStop = errors.New("stream ended before message_stop")

// readSSE calls handle with the data of each server-sent event until handle
// returns false or the stream ends. Lines can be any length: a single text
// delta may be megabytes long (layer B: huge-output).
func readSSE(body io.Reader, handle func(data []byte) (bool, error)) error {
	r := bufio.NewReaderSize(body, 64<<10)
	var (
		data    []byte
		pending bool // a data: line was seen since the last dispatch
	)
	for {
		line, err := r.ReadSlice('\n')
		if errors.Is(err, bufio.ErrBufferFull) {
			// Longer than the buffer: keep reading the rest of the line.
			long := append([]byte(nil), line...)
			for errors.Is(err, bufio.ErrBufferFull) {
				line, err = r.ReadSlice('\n')
				long = append(long, line...)
			}
			line = long
		}
		if err != nil && !(errors.Is(err, io.EOF) && len(line) > 0) {
			if errors.Is(err, io.EOF) {
				return errNoStop
			}
			return err
		}
		line = trimEOL(line)
		switch {
		case len(line) == 0: // a blank line ends the event
			if !pending {
				continue
			}
			more, herr := handle(data)
			data, pending = data[:0], false
			if herr != nil || !more {
				return herr
			}
		case line[0] == ':': // comment
		default:
			if v, ok := field(line, "data"); ok {
				if pending {
					data = append(data, '\n')
				}
				data, pending = append(data, v...), true
			}
			// "event:", "id:" and "retry:" are not needed.
		}
		if errors.Is(err, io.EOF) {
			return errNoStop
		}
	}
}

func trimEOL(b []byte) []byte {
	if n := len(b); n > 0 && b[n-1] == '\n' {
		b = b[:n-1]
	}
	if n := len(b); n > 0 && b[n-1] == '\r' {
		b = b[:n-1]
	}
	return b
}

// field returns the value of "name: value" (one optional space after ':').
func field(line []byte, name string) ([]byte, bool) {
	if len(line) <= len(name) || string(line[:len(name)]) != name || line[len(name)] != ':' {
		return nil, false
	}
	v := line[len(name)+1:]
	if len(v) > 0 && v[0] == ' ' {
		v = v[1:]
	}
	return v, true
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
