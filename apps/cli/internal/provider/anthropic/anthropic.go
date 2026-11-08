package anthropic

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
	// DefaultBaseURL is Anthropic's API. Gateways and the benchmark mock use
	// their own base URL; the client always appends /v1/messages.
	DefaultBaseURL = "https://api.anthropic.com"
	apiVersion     = "2023-06-01"
	// defaultMaxTokens applies when the request does not set MaxTokens: the
	// Messages API requires the field.
	defaultMaxTokens = 8192
	// maxErrorBody caps how much of an error response we read.
	maxErrorBody = 64 << 10
)

// Config holds what the client needs to reach the API.
type Config struct {
	BaseURL   string // DefaultBaseURL when empty
	APIKey    string
	UserAgent string
	// HTTPClient is reused for every call so the TCP/TLS connection stays
	// open between steps. http.DefaultClient when nil.
	HTTPClient *http.Client
}

// Client implements provider.Provider for the Messages API.
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
		endpoint:  strings.TrimRight(base, "/") + "/v1/messages",
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
		h.Set("Anthropic-Version", apiVersion)
		if c.apiKey != "" {
			h.Set("X-Api-Key", c.apiKey)
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
	Model      string          `json:"model"`
	MaxTokens  int             `json:"max_tokens"`
	System     string          `json:"system,omitempty"`
	Messages   []wireMessage   `json:"messages"`
	Tools      []wireTool      `json:"tools,omitempty"`
	ToolChoice *wireToolChoice `json:"tool_choice,omitempty"`
	Stream     bool            `json:"stream"`
}

type wireMessage struct {
	Role    string      `json:"role"`
	Content []wireBlock `json:"content"`
}

// wireBlock is one content block; which fields are set depends on Type.
type wireBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text,omitempty"`
	ID        string          `json:"id,omitempty"`
	Name      string          `json:"name,omitempty"`
	Input     json.RawMessage `json:"input,omitempty"`
	ToolUseID string          `json:"tool_use_id,omitempty"`
	Content   string          `json:"content,omitempty"`
	IsError   bool            `json:"is_error,omitempty"`
}

type wireTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"input_schema"`
}

type wireToolChoice struct {
	Type string `json:"type"`
}

func encode(req provider.Request) ([]byte, error) {
	w := wireRequest{
		Model:     req.Model,
		MaxTokens: req.MaxTokens,
		System:    req.System,
		Messages:  make([]wireMessage, 0, len(req.Messages)),
		Stream:    true,
	}
	if w.MaxTokens <= 0 {
		w.MaxTokens = defaultMaxTokens
	}
	for _, m := range req.Messages {
		blocks := toBlocks(m.Parts)
		if len(blocks) == 0 {
			continue
		}
		// The API wants alternating roles: merge consecutive messages from the
		// same author (e.g. tool results followed by the summary request).
		if n := len(w.Messages); n > 0 && w.Messages[n-1].Role == string(m.Role) {
			w.Messages[n-1].Content = append(w.Messages[n-1].Content, blocks...)
			continue
		}
		w.Messages = append(w.Messages, wireMessage{Role: string(m.Role), Content: blocks})
	}
	if len(req.Tools) > 0 {
		tools := slices.Clone(req.Tools)
		slices.SortFunc(tools, func(a, b provider.ToolDef) int { return cmp.Compare(a.Name, b.Name) })
		for _, t := range tools {
			w.Tools = append(w.Tools, wireTool{Name: t.Name, Description: t.Description, InputSchema: t.InputSchema})
		}
		// tool_choice is only valid when tools are declared.
		if req.ToolChoice == provider.ToolChoiceNone {
			w.ToolChoice = &wireToolChoice{Type: "none"}
		}
	}
	return json.Marshal(w)
}

func toBlocks(parts []provider.Part) []wireBlock {
	blocks := make([]wireBlock, 0, len(parts))
	for _, p := range parts {
		switch p := p.(type) {
		case provider.Text:
			if p.Text != "" { // the API rejects empty text blocks
				blocks = append(blocks, wireBlock{Type: "text", Text: p.Text})
			}
		case provider.ToolCall:
			input := p.Input
			if !json.Valid(input) {
				// The API requires an object. Invalid JSON from the model is
				// replayed as {} (the tool already answered with an error).
				input = json.RawMessage("{}")
			}
			blocks = append(blocks, wireBlock{Type: "tool_use", ID: p.ID, Name: p.Name, Input: input})
		case provider.ToolResult:
			blocks = append(blocks, wireBlock{Type: "tool_result", ToolUseID: p.CallID, Content: p.Content, IsError: p.IsError})
		}
	}
	return blocks
}

// ------------------------------------------------------------------ errors

func httpError(status int, header http.Header, body []byte) *provider.Error {
	e := &provider.Error{
		Kind: provider.KindForStatus(status), Status: status,
		RetryAfter: provider.RetryAfter(header), Retry: provider.ShouldRetry(header),
	}
	var parsed struct {
		Error struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal(body, &parsed) == nil && parsed.Error.Message != "" {
		e.Message = parsed.Error.Message
	} else {
		e.Message = strings.TrimSpace(string(body))
	}
	// An empty account is a 400 here (not a 429 as in OpenAI). It was never
	// retried; naming it lets the CLI say what to do about it.
	if e.Kind == provider.ErrBadRequest && strings.Contains(strings.ToLower(e.Message), "credit balance") {
		e.Kind = provider.ErrQuota
	}
	return e
}
