package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
	"unicode/utf8"
)

// reply is everything a protocol writer needs to render one scripted step.
type reply struct {
	id         string
	model      string
	step       *Step
	usage      Usage
	chunkRunes int
	ttft       time.Duration
	chunkDelay time.Duration
}

func (r *reply) callID(i int) string { return fmt.Sprintf("call_%s_%d", r.id, i) }

func (r *reply) chunks() []string { return splitRunes(r.step.Text, r.chunkRunes) }

func splitRunes(s string, n int) []string {
	if s == "" {
		return nil
	}
	var out []string
	for len(s) > 0 {
		i, count := 0, 0
		for i < len(s) && count < n {
			_, size := utf8.DecodeRuneInString(s[i:])
			i += size
			count++
		}
		out = append(out, s[:i])
		s = s[i:]
	}
	return out
}

// sse writes Server-Sent Events and flushes after each one so clients see
// real streaming timing.
type sse struct {
	w       http.ResponseWriter
	f       http.Flusher
	started bool
	delay   time.Duration
}

func newSSE(w http.ResponseWriter, delay time.Duration) *sse {
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	h.Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	f, _ := w.(http.Flusher)
	if f != nil {
		f.Flush()
	}
	return &sse{w: w, f: f, delay: delay}
}

func (s *sse) send(event string, v any) {
	if s.started && s.delay > 0 {
		time.Sleep(s.delay)
	}
	s.started = true
	data, _ := json.Marshal(v)
	if event != "" {
		fmt.Fprintf(s.w, "event: %s\n", event)
	}
	fmt.Fprintf(s.w, "data: %s\n\n", data)
	if s.f != nil {
		s.f.Flush()
	}
}

func (s *sse) raw(line string) {
	fmt.Fprintf(s.w, "%s\n\n", line)
	if s.f != nil {
		s.f.Flush()
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// ---------------------------------------------------------------- errors

func writeError(w http.ResponseWriter, proto Protocol, e *StepError) {
	typ := e.Type
	if typ == "" {
		typ = "api_error"
	}
	if proto == ProtoAnthropic {
		writeJSON(w, e.Status, map[string]any{
			"type":  "error",
			"error": map[string]any{"type": typ, "message": e.Message},
		})
		return
	}
	writeJSON(w, e.Status, map[string]any{
		"error": map[string]any{"message": e.Message, "type": typ, "code": nil, "param": nil},
	})
}

// ---------------------------------------------------- OpenAI Chat Completions

func chatToolCalls(r *reply) []map[string]any {
	out := make([]map[string]any, 0, len(r.step.ToolCalls))
	for i, tc := range r.step.ToolCalls {
		out = append(out, map[string]any{
			"id":       r.callID(i),
			"type":     "function",
			"function": map[string]any{"name": tc.Name, "arguments": string(tc.Arguments)},
		})
	}
	return out
}

func chatFinish(r *reply) string {
	if len(r.step.ToolCalls) > 0 {
		return "tool_calls"
	}
	return "stop"
}

func chatUsage(r *reply) map[string]any {
	return map[string]any{
		"prompt_tokens":         r.usage.InputTokens,
		"completion_tokens":     r.usage.OutputTokens,
		"total_tokens":          r.usage.InputTokens + r.usage.OutputTokens,
		"prompt_tokens_details": map[string]any{"cached_tokens": r.usage.CacheReadTokens},
	}
}

func writeChat(w http.ResponseWriter, r *reply, info RequestInfo) {
	created := time.Now().Unix()
	id := "chatcmpl-" + r.id
	if !info.Stream {
		time.Sleep(r.ttft)
		msg := map[string]any{"role": "assistant", "content": nil}
		if r.step.Text != "" {
			msg["content"] = r.step.Text
		}
		if len(r.step.ToolCalls) > 0 {
			msg["tool_calls"] = chatToolCalls(r)
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"id": id, "object": "chat.completion", "created": created, "model": r.model,
			"choices": []any{map[string]any{"index": 0, "message": msg, "finish_reason": chatFinish(r)}},
			"usage":   chatUsage(r),
		})
		return
	}

	s := newSSE(w, r.chunkDelay)
	time.Sleep(r.ttft)
	chunk := func(delta map[string]any, finish any) map[string]any {
		return map[string]any{
			"id": id, "object": "chat.completion.chunk", "created": created, "model": r.model,
			"choices": []any{map[string]any{"index": 0, "delta": delta, "finish_reason": finish}},
		}
	}
	s.send("", chunk(map[string]any{"role": "assistant", "content": ""}, nil))
	for _, c := range r.chunks() {
		s.send("", chunk(map[string]any{"content": c}, nil))
	}
	for i, tc := range chatToolCalls(r) {
		tc["index"] = i
		s.send("", chunk(map[string]any{"tool_calls": []any{tc}}, nil))
	}
	s.send("", chunk(map[string]any{}, chatFinish(r)))
	if info.IncludeUsage {
		s.send("", map[string]any{
			"id": id, "object": "chat.completion.chunk", "created": created, "model": r.model,
			"choices": []any{}, "usage": chatUsage(r),
		})
	}
	s.raw("data: [DONE]")
}

// -------------------------------------------------- Anthropic Messages API

func anthropicStop(r *reply) string {
	if len(r.step.ToolCalls) > 0 {
		return "tool_use"
	}
	return "end_turn"
}

func anthropicUsage(r *reply, output int) map[string]any {
	return map[string]any{
		"input_tokens":                r.usage.InputTokens,
		"output_tokens":               output,
		"cache_creation_input_tokens": 0,
		"cache_read_input_tokens":     r.usage.CacheReadTokens,
	}
}

func writeAnthropic(w http.ResponseWriter, r *reply, info RequestInfo) {
	id := "msg_" + r.id
	if !info.Stream {
		time.Sleep(r.ttft)
		content := []any{}
		if r.step.Text != "" {
			content = append(content, map[string]any{"type": "text", "text": r.step.Text})
		}
		for i, tc := range r.step.ToolCalls {
			content = append(content, map[string]any{
				"type": "tool_use", "id": "toolu_" + r.callID(i), "name": tc.Name, "input": tc.Arguments,
			})
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"id": id, "type": "message", "role": "assistant", "model": r.model,
			"content": content, "stop_reason": anthropicStop(r), "stop_sequence": nil,
			"usage": anthropicUsage(r, r.usage.OutputTokens),
		})
		return
	}

	s := newSSE(w, r.chunkDelay)
	time.Sleep(r.ttft)
	s.send("message_start", map[string]any{
		"type": "message_start",
		"message": map[string]any{
			"id": id, "type": "message", "role": "assistant", "model": r.model,
			"content": []any{}, "stop_reason": nil, "stop_sequence": nil,
			"usage": anthropicUsage(r, 1),
		},
	})
	index := 0
	if r.step.Text != "" {
		s.send("content_block_start", map[string]any{
			"type": "content_block_start", "index": index,
			"content_block": map[string]any{"type": "text", "text": ""},
		})
		for _, c := range r.chunks() {
			s.send("content_block_delta", map[string]any{
				"type": "content_block_delta", "index": index,
				"delta": map[string]any{"type": "text_delta", "text": c},
			})
		}
		s.send("content_block_stop", map[string]any{"type": "content_block_stop", "index": index})
		index++
	}
	for i, tc := range r.step.ToolCalls {
		s.send("content_block_start", map[string]any{
			"type": "content_block_start", "index": index,
			"content_block": map[string]any{
				"type": "tool_use", "id": "toolu_" + r.callID(i), "name": tc.Name, "input": map[string]any{},
			},
		})
		s.send("content_block_delta", map[string]any{
			"type": "content_block_delta", "index": index,
			"delta": map[string]any{"type": "input_json_delta", "partial_json": string(tc.Arguments)},
		})
		s.send("content_block_stop", map[string]any{"type": "content_block_stop", "index": index})
		index++
	}
	s.send("message_delta", map[string]any{
		"type":  "message_delta",
		"delta": map[string]any{"stop_reason": anthropicStop(r), "stop_sequence": nil},
		"usage": map[string]any{"output_tokens": r.usage.OutputTokens},
	})
	s.send("message_stop", map[string]any{"type": "message_stop"})
}

// ---------------------------------------------------- OpenAI Responses API

func responsesItems(r *reply, status string) []map[string]any {
	var items []map[string]any
	if r.step.Text != "" {
		content := []any{}
		if status == "completed" {
			content = append(content, map[string]any{"type": "output_text", "text": r.step.Text, "annotations": []any{}})
		}
		items = append(items, map[string]any{
			"id": "msg_" + r.id, "type": "message", "status": status, "role": "assistant", "content": content,
		})
	}
	for i, tc := range r.step.ToolCalls {
		args := ""
		if status == "completed" {
			args = string(tc.Arguments)
		}
		items = append(items, map[string]any{
			"id": fmt.Sprintf("fc_%s_%d", r.id, i), "type": "function_call", "status": status,
			"call_id": r.callID(i), "name": tc.Name, "arguments": args,
		})
	}
	return items
}

func responsesUsage(r *reply) map[string]any {
	return map[string]any{
		"input_tokens":          r.usage.InputTokens,
		"input_tokens_details":  map[string]any{"cached_tokens": r.usage.CacheReadTokens},
		"output_tokens":         r.usage.OutputTokens,
		"output_tokens_details": map[string]any{"reasoning_tokens": 0},
		"total_tokens":          r.usage.InputTokens + r.usage.OutputTokens,
	}
}

func responsesObject(r *reply, status string, created int64) map[string]any {
	obj := map[string]any{
		"id": "resp_" + r.id, "object": "response", "created_at": created,
		"status": status, "model": r.model, "output": []any{},
		"error": nil, "incomplete_details": nil,
	}
	if status == "completed" {
		obj["output"] = responsesItems(r, "completed")
		obj["usage"] = responsesUsage(r)
	}
	return obj
}

func writeResponses(w http.ResponseWriter, r *reply, info RequestInfo) {
	created := time.Now().Unix()
	if !info.Stream {
		time.Sleep(r.ttft)
		writeJSON(w, http.StatusOK, responsesObject(r, "completed", created))
		return
	}

	s := newSSE(w, r.chunkDelay)
	time.Sleep(r.ttft)
	seq := 0
	send := func(typ string, fields map[string]any) {
		fields["type"] = typ
		fields["sequence_number"] = seq
		seq++
		s.send(typ, fields)
	}
	send("response.created", map[string]any{"response": responsesObject(r, "in_progress", created)})
	send("response.in_progress", map[string]any{"response": responsesObject(r, "in_progress", created)})

	done := responsesItems(r, "completed")
	pending := responsesItems(r, "in_progress")
	for idx := range done {
		item := done[idx]
		itemID := item["id"]
		send("response.output_item.added", map[string]any{"output_index": idx, "item": pending[idx]})
		switch item["type"] {
		case "message":
			part := map[string]any{"type": "output_text", "text": "", "annotations": []any{}}
			send("response.content_part.added", map[string]any{
				"item_id": itemID, "output_index": idx, "content_index": 0, "part": part,
			})
			for _, c := range r.chunks() {
				send("response.output_text.delta", map[string]any{
					"item_id": itemID, "output_index": idx, "content_index": 0, "delta": c, "logprobs": []any{},
				})
			}
			send("response.output_text.done", map[string]any{
				"item_id": itemID, "output_index": idx, "content_index": 0, "text": r.step.Text, "logprobs": []any{},
			})
			send("response.content_part.done", map[string]any{
				"item_id": itemID, "output_index": idx, "content_index": 0,
				"part": map[string]any{"type": "output_text", "text": r.step.Text, "annotations": []any{}},
			})
		case "function_call":
			send("response.function_call_arguments.delta", map[string]any{
				"item_id": itemID, "output_index": idx, "delta": item["arguments"],
			})
			send("response.function_call_arguments.done", map[string]any{
				"item_id": itemID, "output_index": idx, "arguments": item["arguments"],
			})
		}
		send("response.output_item.done", map[string]any{"output_index": idx, "item": item})
	}
	send("response.completed", map[string]any{"response": responsesObject(r, "completed", created)})
}
