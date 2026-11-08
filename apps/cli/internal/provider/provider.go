package provider

import (
	"context"
	"encoding/json"
	"iter"
	"time"
)

// Provider streams one model call. It is the only thing the agent loop needs
// from a model vendor.
//
// The iterator yields events as they arrive and stops after a Done event or
// an error. An error before any content is an HTTP or connection problem; an
// error after content means the stream broke. Both are *Error, except
// context cancellation, which is returned as ctx.Err().
type Provider interface {
	Stream(ctx context.Context, req Request) iter.Seq2[Event, error]
}

// Request is everything one model call needs. The model travels with each
// request, so a provider holds no per-conversation state.
type Request struct {
	Model    string
	System   string
	Messages []Message
	// Tools are sent sorted by name so the bytes stay stable between steps,
	// which is what prompt caching needs.
	Tools      []ToolDef
	ToolChoice ToolChoice
	MaxTokens  int
}

// ToolChoice tells the model whether it may call tools in this step.
type ToolChoice int

const (
	// ToolChoiceAuto lets the model decide (the default).
	ToolChoiceAuto ToolChoice = iota
	// ToolChoiceNone forbids tool calls; used by the summary step (ADR 0003).
	ToolChoiceNone
)

// ToolDef describes a tool to the model.
type ToolDef struct {
	Name        string
	Description string
	InputSchema json.RawMessage // a JSON Schema object
}

// Role is who wrote a message.
type Role string

const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
)

// Message is one entry of the conversation. Its parts are typed values
// (Text, ToolCall, ToolResult) read with a type switch.
type Message struct {
	Role  Role
	Parts []Part
}

// Part is one piece of a message: Text, ToolCall or ToolResult.
type Part interface{ isPart() }

// Text is plain text written by the user or the model.
type Text struct{ Text string }

// ToolCall is the model asking to run a tool. Input is the raw JSON the model
// produced; it may be invalid, and validating it is the tool's job.
type ToolCall struct {
	ID    string
	Name  string
	Input json.RawMessage
}

// ToolResult answers the ToolCall with the same ID.
type ToolResult struct {
	CallID  string
	Content string
	IsError bool
}

func (Text) isPart()       {}
func (ToolCall) isPart()   {}
func (ToolResult) isPart() {}

// EventKind says which fields of an Event are set.
type EventKind int

const (
	// EventSent: the request body is about to go out (Bytes is set).
	EventSent EventKind = iota + 1
	// EventTextDelta: a chunk of assistant text (Text is set).
	EventTextDelta
	// EventToolCall: a complete tool call; the adapter assembles the
	// streamed fragments (ToolCall is set).
	EventToolCall
	// EventDone: the call finished (Usage, Stop and RawStop are set).
	EventDone
	// EventRetry: the attempt failed and the same request will be sent
	// again after Delay (Err, Attempt and Delay are set). Everything
	// streamed since the last EventSent belongs to the failed attempt and
	// must be discarded. Only WithRetry emits it (ADR 0004).
	EventRetry
)

// Event is one thing that happened during a model call.
type Event struct {
	Kind     EventKind
	Bytes    int
	Text     string
	ToolCall *ToolCall
	Usage    Usage
	Stop     StopReason
	// RawStop is the provider's own stop reason, kept for the trajectory.
	RawStop string

	// Retry fields (EventRetry).
	Err     error         // why the attempt failed
	Attempt int           // number of the attempt that comes next (2, 3…)
	Delay   time.Duration // wait before it
}

// StopReason is why the model stopped, normalized across providers.
type StopReason int

const (
	StopEndTurn   StopReason = iota + 1 // finished its answer
	StopToolUse                         // wants tools to run
	StopMaxTokens                       // hit the output token limit
	StopOther                           // anything else (refusal, stop sequence…)
)

// Usage is the token count of one model call. InputTokens counts only the
// input that was not served from the prompt cache; cached input is in
// CacheReadTokens (and CacheWriteTokens for what was written to the cache).
// That is how the Messages API reports it; adapters for APIs that include
// cached tokens in the input total (Chat Completions) subtract them, so cost
// is computed the same way for every provider (ADR 0005).
type Usage struct {
	InputTokens      int `json:"input_tokens"`
	OutputTokens     int `json:"output_tokens"`
	CacheReadTokens  int `json:"cache_read_tokens"`
	CacheWriteTokens int `json:"cache_write_tokens"`
}

// Add accumulates another call's usage.
func (u *Usage) Add(o Usage) {
	u.InputTokens += o.InputTokens
	u.OutputTokens += o.OutputTokens
	u.CacheReadTokens += o.CacheReadTokens
	u.CacheWriteTokens += o.CacheWriteTokens
}
