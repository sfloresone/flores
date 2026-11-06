package agent

import (
	"time"

	"github.com/sfloresone/flores/apps/cli/internal/provider"
)

// EventKind says which fields of an Event are set.
type EventKind int

const (
	EventStepStart  EventKind = iota + 1 // a model call starts (Step)
	EventRequest                         // its request is sent (Request)
	EventTextDelta                       // a chunk of assistant text (Text)
	EventResponse                        // the model call ended, well or not (Response)
	EventToolCall                        // the model asked for a tool (ToolCall)
	EventToolResult                      // a tool finished (ToolResult, ToolDuration)
	EventDone                            // the turn ended (Result); always last
)

// Event is one thing that happened during a turn. Step numbers the model
// call it belongs to, counting from 1 across the whole conversation.
type Event struct {
	Kind         EventKind
	Step         int
	Text         string
	Request      *RequestStats
	Response     *ResponseStats
	ToolCall     *provider.ToolCall
	ToolResult   *provider.ToolResult
	ToolDuration time.Duration
	Result       *Result
}

// RequestStats describes what one step sent to the model.
type RequestStats struct {
	Bytes       int // body size on the wire
	Messages    int
	Tools       int
	SystemBytes int
}

// ResponseStats describes how one model call went.
type ResponseStats struct {
	Duration time.Duration
	// TTFT is the time to the first text or tool call; 0 if none arrived.
	TTFT       time.Duration
	Usage      provider.Usage
	Stop       string // the provider's own stop reason
	HTTPStatus int    // 0 when there was no HTTP response
	Text       string // the full assistant text of this step
	Err        error  // set when the call failed
}

func (s *ResponseStats) markFirstContent(start time.Time) {
	if s.TTFT == 0 {
		s.TTFT = time.Since(start)
	}
}

// Status is how a turn ended. The values are the trajectory/v1 statuses.
type Status string

const (
	StatusSuccess    Status = "success"
	StatusError      Status = "error"
	StatusMaxSteps   Status = "max_steps"
	StatusRepetition Status = "repetition"
	StatusCancelled  Status = "cancelled"
)

// Result summarizes a turn.
type Result struct {
	Status     Status
	Steps      int // model calls in this turn, the summary step included
	Usage      provider.Usage
	ToolCalls  int
	ToolErrors int
	// FinalText is the last assistant text: the answer, or the summary when
	// a brake stopped the turn.
	FinalText string
	// Err is the provider error or cancellation that ended the turn, if any.
	Err error
}
