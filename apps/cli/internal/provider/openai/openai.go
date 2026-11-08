package openai

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"io"
	"iter"
	"net/http"
	"slices"
	"strings"

	"github.com/sfloresone/flores/apps/cli/internal/provider"
)

const (
	// DefaultBaseURL is OpenAI's API. Compatible servers use their own base
	// URL, version included (e.g. http://localhost:11434/v1 for Ollama); the
	// client always appends /chat/completions.
	DefaultBaseURL = "https://api.openai.com/v1"
	// maxErrorBody caps how much of an error response we read.
	maxErrorBody = 64 << 10
)

// Config holds what the client needs to reach the API.
type Config struct {
	BaseURL   string // DefaultBaseURL when empty
	APIKey    string // sent as a bearer token; local servers may need none
	UserAgent string
	// HTTPClient is reused for every call so the TCP/TLS connection stays
	// open between steps. http.DefaultClient when nil.
	HTTPClient *http.Client
}

// Client implements provider.Provider for Chat Completions.
type Client struct {
	endpoint  string
	apiKey    string
	userAgent string
	http      *http.Client
}

// New returns a client for the given configuration.
func New(cfg Config) *Client {
	base := cmp.Or(cfg.BaseURL, DefaultBaseURL)
	return &Client{
		endpoint:  strings.TrimRight(base, "/") + "/chat/completions",
		apiKey:    cfg.APIKey,
		userAgent: cfg.UserAgent,
		http:      cmp.Or(cfg.HTTPClient, http.DefaultClient),
	}
}

// Stream sends one request and yields its events (see provider.Provider).
func (c *Client) Stream(ctx context.Context, req provider.Request) iter.Seq2[provider.Event, error] {
	return func(yield func(provider.Event, error) bool) {
		body, err := encode(req)
		if err != nil {
			yield(provider.Event{}, &provider.Error{Kind: provider.ErrBadRequest, Err: err})
			return
		}
		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
		if err != nil {
			yield(provider.Event{}, &provider.Error{Kind: provider.ErrBadRequest, Err: err})
			return
		}
		h := httpReq.Header
		h.Set("Content-Type", "application/json")
		h.Set("Accept", "text/event-stream")
		if c.apiKey != "" {
			h.Set("Authorization", "Bearer "+c.apiKey)
		}
		if c.userAgent != "" {
			h.Set("User-Agent", c.userAgent)
		}

		if !yield(provider.Event{Kind: provider.EventSent, Bytes: len(body)}, nil) {
			return
		}
		resp, err := c.http.Do(httpReq)
		if err != nil {
			yield(provider.Event{}, provider.TransportError(ctx, err))
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			b, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorBody))
			yield(provider.Event{}, httpError(resp.StatusCode, resp.Header, b))
			return
		}
		readStream(ctx, resp.Body, yield)
	}
}

// ------------------------------------------------------------ request body

type wireRequest struct {
	Model    string        `json:"model"`
	Messages []wireMessage `json:"messages"`
	Tools    []wireTool    `json:"tools,omitempty"`
	// ToolChoice is only sent as "none" (the summary step, ADR 0003); left
	// out, the model may call tools ("auto").
	ToolChoice string `json:"tool_choice,omitempty"`
	// MaxCompletionTokens is OpenAI's current name for the output limit
	// (reasoning models reject the old max_tokens). Left out when 0, the
	// default, so compatible servers that only know max_tokens never see it.
	MaxCompletionTokens int            `json:"max_completion_tokens,omitempty"`
	Stream              bool           `json:"stream"`
	StreamOptions       *streamOptions `json:"stream_options,omitempty"`
}

type streamOptions struct {
	IncludeUsage bool `json:"include_usage"`
}

type wireMessage struct {
	Role string `json:"role"`
	// Content is null for an assistant message that only calls tools.
	Content    *string        `json:"content"`
	ToolCalls  []wireToolCall `json:"tool_calls,omitempty"`
	ToolCallID string         `json:"tool_call_id,omitempty"`
}

type wireToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function wireFunction `json:"function"`
}

type wireFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"` // a JSON document inside a string
}

type wireTool struct {
	Type     string      `json:"type"`
	Function wireToolDef `json:"function"`
}

type wireToolDef struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters"`
}

// emptyResult replaces an empty tool result: some compatible servers reject
// a tool message with empty content.
const emptyResult = "(no output)"

func encode(req provider.Request) ([]byte, error) {
	w := wireRequest{
		Model:               req.Model,
		Messages:            make([]wireMessage, 0, len(req.Messages)+1),
		MaxCompletionTokens: max(req.MaxTokens, 0),
		Stream:              true,
		StreamOptions:       &streamOptions{IncludeUsage: true},
	}
	if req.System != "" {
		w.Messages = append(w.Messages, wireMessage{Role: "system", Content: &req.System})
	}
	for _, m := range req.Messages {
		w.Messages = append(w.Messages, toMessages(m)...)
	}
	if len(req.Tools) > 0 {
		tools := slices.Clone(req.Tools)
		slices.SortFunc(tools, func(a, b provider.ToolDef) int { return cmp.Compare(a.Name, b.Name) })
		for _, t := range tools {
			w.Tools = append(w.Tools, wireTool{Type: "function", Function: wireToolDef{
				Name: t.Name, Description: t.Description, Parameters: t.InputSchema,
			}})
		}
		// tool_choice is only valid when tools are declared.
		if req.ToolChoice == provider.ToolChoiceNone {
			w.ToolChoice = "none"
		}
	}
	return json.Marshal(w)
}

// toMessages maps one provider message to wire messages. Chat Completions
// has no "tool result" part: each result is its own role "tool" message,
// and those must come right after the assistant message that asked for them.
func toMessages(m provider.Message) []wireMessage {
	var (
		text  strings.Builder
		calls []wireToolCall
		out   []wireMessage
	)
	for _, p := range m.Parts {
		switch p := p.(type) {
		case provider.Text:
			text.WriteString(p.Text)
		case provider.ToolCall:
			args := string(p.Input)
			if !json.Valid(p.Input) {
				// Empty or invalid JSON from the model is replayed as {} (the
				// tool already answered with an error).
				args = "{}"
			}
			calls = append(calls, wireToolCall{ID: p.ID, Type: "function", Function: wireFunction{Name: p.Name, Arguments: args}})
		case provider.ToolResult:
			content := cmp.Or(p.Content, emptyResult)
			// There is no error flag on the wire: the content says it.
			out = append(out, wireMessage{Role: "tool", Content: &content, ToolCallID: p.CallID})
		}
	}
	s := text.String()
	switch {
	case m.Role == provider.RoleAssistant && len(calls) > 0:
		msg := wireMessage{Role: "assistant", ToolCalls: calls}
		if s != "" {
			msg.Content = &s
		}
		out = append(out, msg)
	case s != "":
		out = append(out, wireMessage{Role: string(m.Role), Content: &s})
	}
	return out
}

// ------------------------------------------------------------------ errors

// apiError is the error object of the API: {"error": {...}} in an HTTP error
// body or in a stream chunk. Compatible servers vary: code may be a number,
// and some (Ollama) send "error" as a plain string.
type apiError struct {
	Type    string
	Code    string
	Message string
}

func parseAPIError(raw json.RawMessage) (apiError, bool) {
	if len(raw) == 0 || string(raw) == "null" {
		return apiError{}, false
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return apiError{Message: s}, true
	}
	var obj struct {
		Type    string          `json:"type"`
		Code    json.RawMessage `json:"code"`
		Message string          `json:"message"`
	}
	if json.Unmarshal(raw, &obj) != nil {
		return apiError{}, false
	}
	e := apiError{Type: obj.Type, Message: obj.Message}
	if json.Unmarshal(obj.Code, &e.Code) != nil && len(obj.Code) > 0 && string(obj.Code) != "null" {
		e.Code = string(obj.Code) // a number
	}
	return e, true
}

// quota reports whether the error means "no credit left". OpenAI sends it
// as a 429, the same status as "too many requests", so the status alone
// cannot tell them apart; the code can (ADR 0005).
func (e apiError) quota() bool {
	return e.Code == "insufficient_quota" || e.Type == "insufficient_quota"
}

func httpError(status int, header http.Header, body []byte) *provider.Error {
	e := &provider.Error{
		Kind: provider.KindForStatus(status), Status: status,
		RetryAfter: provider.RetryAfter(header), Retry: provider.ShouldRetry(header),
	}
	var wrapper struct {
		Error json.RawMessage `json:"error"`
	}
	apiErr, ok := apiError{}, false
	if json.Unmarshal(body, &wrapper) == nil {
		apiErr, ok = parseAPIError(wrapper.Error)
	}
	if ok && apiErr.Message != "" {
		e.Message = apiErr.Message
	} else {
		e.Message = strings.TrimSpace(string(body))
	}
	if apiErr.quota() {
		e.Kind = provider.ErrQuota
	}
	return e
}

// streamError classifies an error object sent in the middle of a stream,
// where there is no HTTP status to go by.
func streamError(apiErr apiError) *provider.Error {
	e := &provider.Error{Kind: provider.ErrOverloaded, Message: apiErr.Message}
	what := apiErr.Code + " " + apiErr.Type
	switch {
	case apiErr.quota():
		e.Kind = provider.ErrQuota
	case strings.Contains(what, "rate_limit"):
		e.Kind = provider.ErrRateLimit
	case strings.Contains(what, "invalid_api_key") || strings.Contains(what, "authentication"):
		e.Kind = provider.ErrAuth
	case strings.Contains(what, "invalid_request") || strings.Contains(what, "context_length_exceeded"):
		e.Kind = provider.ErrBadRequest
	}
	return e
}
