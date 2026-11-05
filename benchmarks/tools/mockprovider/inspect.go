package main

import (
	"encoding/json"
	"strings"
)

// Protocol identifies the wire API a request used.
type Protocol string

const (
	ProtoChat      Protocol = "openai-chat"
	ProtoResponses Protocol = "openai-responses"
	ProtoAnthropic Protocol = "anthropic-messages"
)

// RequestInfo is what the mock learns from a request body. It is both used to
// pick the scenario step and recorded so benchmarks can compare how much each
// harness sends to the model (system prompt size, tool count, history).
type RequestInfo struct {
	Protocol     Protocol `json:"protocol"`
	Model        string   `json:"model"`
	Stream       bool     `json:"stream"`
	IncludeUsage bool     `json:"-"`
	RequestBytes int      `json:"request_bytes"`
	SystemBytes  int      `json:"system_bytes"`
	Messages     int      `json:"messages"`
	Tools        int      `json:"tools"`
	ToolsBytes   int      `json:"tools_bytes"`
	// Turn is the number of assistant turns already present in the request.
	Turn int `json:"turn"`
}

func inspect(proto Protocol, body []byte) (RequestInfo, error) {
	info := RequestInfo{Protocol: proto, RequestBytes: len(body)}
	var req map[string]json.RawMessage
	if err := json.Unmarshal(body, &req); err != nil {
		return info, err
	}
	_ = json.Unmarshal(req["model"], &info.Model)
	_ = json.Unmarshal(req["stream"], &info.Stream)
	if raw, ok := req["tools"]; ok {
		var tools []json.RawMessage
		if json.Unmarshal(raw, &tools) == nil {
			info.Tools = len(tools)
			info.ToolsBytes = len(raw)
		}
	}

	switch proto {
	case ProtoChat:
		var opts struct {
			IncludeUsage bool `json:"include_usage"`
		}
		_ = json.Unmarshal(req["stream_options"], &opts)
		info.IncludeUsage = opts.IncludeUsage
		var msgs []message
		_ = json.Unmarshal(req["messages"], &msgs)
		info.Messages = len(msgs)
		flags := make([]bool, len(msgs))
		for i, m := range msgs {
			if m.Role == "system" || m.Role == "developer" {
				info.SystemBytes += contentBytes(m.Content)
			}
			flags[i] = m.Role == "assistant"
		}
		info.Turn = countRuns(flags)

	case ProtoAnthropic:
		info.SystemBytes = contentBytes(req["system"])
		var msgs []message
		_ = json.Unmarshal(req["messages"], &msgs)
		info.Messages = len(msgs)
		flags := make([]bool, len(msgs))
		for i, m := range msgs {
			flags[i] = m.Role == "assistant"
		}
		info.Turn = countRuns(flags)

	case ProtoResponses:
		info.SystemBytes = contentBytes(req["instructions"])
		var input string
		if json.Unmarshal(req["input"], &input) == nil {
			info.Messages = 1
			break
		}
		var items []struct {
			Type    string          `json:"type"`
			Role    string          `json:"role"`
			Content json.RawMessage `json:"content"`
		}
		_ = json.Unmarshal(req["input"], &items)
		info.Messages = len(items)
		flags := make([]bool, len(items))
		for i, it := range items {
			if it.Role == "system" || it.Role == "developer" {
				info.SystemBytes += contentBytes(it.Content)
			}
			switch {
			case it.Role == "assistant":
				flags[i] = true
			case strings.HasSuffix(it.Type, "_call") || it.Type == "reasoning":
				// function_call, custom_tool_call, local_shell_call, ...
				flags[i] = true
			}
		}
		info.Turn = countRuns(flags)
	}
	return info, nil
}

type message struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

// contentBytes measures the text in a content field that is either a plain
// string or an array of parts with a "text" field.
func contentBytes(raw json.RawMessage) int {
	if len(raw) == 0 {
		return 0
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return len(s)
	}
	var parts []struct {
		Text string `json:"text"`
	}
	if json.Unmarshal(raw, &parts) == nil {
		n := 0
		for _, p := range parts {
			n += len(p.Text)
		}
		return n
	}
	return 0
}

// countRuns counts maximal runs of true values. An assistant turn that emits
// several parallel tool calls appears as consecutive items, so it counts once.
func countRuns(flags []bool) int {
	n, prev := 0, false
	for _, f := range flags {
		if f && !prev {
			n++
		}
		prev = f
	}
	return n
}
