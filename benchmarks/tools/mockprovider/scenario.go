package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// Scenario is a deterministic script of model responses. The mock picks the
// step by counting how many assistant messages the incoming request already
// contains, so it is stateless and safe for concurrent clients: a fresh
// conversation always gets step 0, its second request step 1, and so on.
type Scenario struct {
	Name        string  `json:"name"`
	Description string  `json:"description,omitempty"`
	Model       string  `json:"model,omitempty"`
	Latency     Latency `json:"latency"`
	// ChunkRunes is how many characters each streamed text delta carries.
	ChunkRunes int    `json:"chunk_runes,omitempty"`
	Steps      []Step `json:"steps"`
	// AfterLast decides what happens when a conversation goes past the last
	// step: "repeat_last" (default) or "error".
	AfterLast string `json:"after_last,omitempty"`
}

type Latency struct {
	TTFTMs       int `json:"ttft_ms,omitempty"`
	ChunkDelayMs int `json:"chunk_delay_ms,omitempty"`
}

type Step struct {
	Text string `json:"text,omitempty"`
	// Repeat multiplies Text, to produce huge outputs without huge files.
	Repeat    int        `json:"repeat,omitempty"`
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
	// Error answers every attempt of this step with an HTTP error.
	Error *StepError `json:"error,omitempty"`
	// FailFirst answers the first Times attempts with an error, then succeeds:
	// the agent is expected to retry.
	FailFirst *FailFirst `json:"fail_first,omitempty"`
	// Truncate cuts the connection after AfterChunks text chunks on the first
	// Times attempts (0 = every attempt), simulating a dropped stream.
	Truncate *Truncate `json:"truncate,omitempty"`
	// TTFTMs overrides the scenario latency for this step.
	TTFTMs *int   `json:"ttft_ms,omitempty"`
	Usage  *Usage `json:"usage,omitempty"`
}

type FailFirst struct {
	Times       int    `json:"times"`
	Status      int    `json:"status"`
	Type        string `json:"type,omitempty"`
	Message     string `json:"message,omitempty"`
	RetryAfterS int    `json:"retry_after_s,omitempty"`
}

type Truncate struct {
	AfterChunks int `json:"after_chunks"`
	Times       int `json:"times,omitempty"`
}

type ToolCall struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
	// RawArguments is sent verbatim instead of Arguments. Use it to send
	// malformed JSON and check the agent survives it.
	RawArguments string `json:"raw_arguments,omitempty"`
}

// args is the argument string as it goes on the wire.
func (tc ToolCall) args() string {
	if tc.RawArguments != "" {
		return tc.RawArguments
	}
	return string(tc.Arguments)
}

// argsObject is a valid JSON value for protocols that embed arguments as an
// object (Anthropic non-streaming); malformed raw arguments degrade to {}.
func (tc ToolCall) argsObject() json.RawMessage {
	if json.Valid([]byte(tc.args())) {
		return json.RawMessage(tc.args())
	}
	return json.RawMessage("{}")
}

type StepError struct {
	Status  int    `json:"status"`
	Type    string `json:"type,omitempty"`
	Message string `json:"message"`
	// Code is sent as error.code in the OpenAI shapes (null when empty).
	Code string `json:"code,omitempty"`
	// Anthropic, when set, is how the same failure looks in the Messages
	// API. Some failures differ by vendor: no credit left is a 429
	// insufficient_quota in OpenAI and a 400 "credit balance is too low" in
	// Anthropic.
	Anthropic *StepError `json:"anthropic,omitempty"`
}

// Usage is what a step reports. InputTokens is the input that was not
// cached, as the Messages API reports it; the OpenAI shapes add
// CacheReadTokens to their input total, as the real APIs do.
type Usage struct {
	InputTokens     int `json:"input_tokens,omitempty"`
	OutputTokens    int `json:"output_tokens,omitempty"`
	CacheReadTokens int `json:"cache_read_tokens,omitempty"`
}

// defaultScenario mirrors the trivial workload fx uses for its competitive
// libfx benchmark: one short greeting, so the measurement is dominated by
// harness overhead instead of model behaviour.
func defaultScenario() *Scenario {
	return &Scenario{
		Name:        "hello",
		Description: "Single short greeting. Measures pure harness overhead.",
		Model:       "mock-model",
		ChunkRunes:  8,
		Steps:       []Step{{Text: "Hello! Nice to meet you."}},
	}
}

func loadScenario(path string) (*Scenario, error) {
	if path == "" {
		return defaultScenario(), nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var s Scenario
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&s); err != nil {
		return nil, fmt.Errorf("scenario %s: %w", path, err)
	}
	if err := s.validate(); err != nil {
		return nil, fmt.Errorf("scenario %s: %w", path, err)
	}
	return &s, nil
}

func (s *Scenario) validate() error {
	if s.Name == "" {
		return fmt.Errorf("name is required")
	}
	if len(s.Steps) == 0 {
		return fmt.Errorf("at least one step is required")
	}
	if s.Model == "" {
		s.Model = "mock-model"
	}
	if s.ChunkRunes <= 0 {
		s.ChunkRunes = 8
	}
	switch s.AfterLast {
	case "":
		s.AfterLast = "repeat_last"
	case "repeat_last", "error":
	default:
		return fmt.Errorf("after_last must be repeat_last or error, got %q", s.AfterLast)
	}
	for i, st := range s.Steps {
		if st.Error == nil && st.Text == "" && len(st.ToolCalls) == 0 {
			return fmt.Errorf("step %d: needs text, tool_calls or error", i)
		}
		if st.Error != nil && st.Error.Status < 400 {
			return fmt.Errorf("step %d: error.status must be >= 400", i)
		}
		if f := st.FailFirst; f != nil && (f.Times < 1 || f.Status < 400) {
			return fmt.Errorf("step %d: fail_first needs times >= 1 and status >= 400", i)
		}
		if t := st.Truncate; t != nil && (t.AfterChunks < 0 || t.Times < 0) {
			return fmt.Errorf("step %d: truncate values must be >= 0", i)
		}
		if st.Repeat > 1 {
			s.Steps[i].Text = strings.Repeat(st.Text, st.Repeat)
			s.Steps[i].Repeat = 0
		}
		for j, tc := range st.ToolCalls {
			if tc.Name == "" {
				return fmt.Errorf("step %d tool_call %d: name is required", i, j)
			}
			switch {
			case tc.RawArguments != "":
			case len(tc.Arguments) == 0:
				s.Steps[i].ToolCalls[j].Arguments = json.RawMessage("{}")
			case !json.Valid(tc.Arguments):
				return fmt.Errorf("step %d tool_call %d: arguments must be JSON", i, j)
			}
		}
	}
	return nil
}

// pick returns the scenario step for request number `step` (0-based) of a
// conversation, or nil when the scenario is exhausted and after_last is "error".
func (s *Scenario) pick(step int) *Step {
	if step < len(s.Steps) {
		return &s.Steps[step]
	}
	if s.AfterLast == "error" {
		return nil
	}
	return &s.Steps[len(s.Steps)-1]
}
