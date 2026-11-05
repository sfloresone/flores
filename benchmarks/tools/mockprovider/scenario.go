package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
)

// Scenario is a deterministic script of model responses. The mock picks the
// step by counting how many assistant turns the incoming request already
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
	Text      string     `json:"text,omitempty"`
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
	Error     *StepError `json:"error,omitempty"`
	Usage     *Usage     `json:"usage,omitempty"`
}

type ToolCall struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

type StepError struct {
	Status  int    `json:"status"`
	Type    string `json:"type,omitempty"`
	Message string `json:"message"`
}

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
		for j, tc := range st.ToolCalls {
			if tc.Name == "" {
				return fmt.Errorf("step %d tool_call %d: name is required", i, j)
			}
			if len(tc.Arguments) == 0 {
				s.Steps[i].ToolCalls[j].Arguments = json.RawMessage("{}")
			} else if !json.Valid(tc.Arguments) {
				return fmt.Errorf("step %d tool_call %d: arguments must be JSON", i, j)
			}
		}
	}
	return nil
}

// pick returns the step for a conversation that already has `turn` assistant
// turns, or nil when the scenario is exhausted and after_last is "error".
func (s *Scenario) pick(turn int) *Step {
	if turn < len(s.Steps) {
		return &s.Steps[turn]
	}
	if s.AfterLast == "error" {
		return nil
	}
	return &s.Steps[len(s.Steps)-1]
}
