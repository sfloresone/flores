package agent

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"strings"
	"time"

	"github.com/sfloresone/flores/apps/cli/internal/provider"
)

// Config is fixed for the whole conversation.
type Config struct {
	Model     string
	System    string
	MaxSteps  int // per turn; 0 means no limit (ADR 0003)
	MaxTokens int
}

// Agent is one conversation with a model.
type Agent struct {
	provider provider.Provider
	cfg      Config
	history  []provider.Message
	steps    int // model calls in the whole conversation; numbers the steps
}

// New returns an agent with an empty conversation.
func New(p provider.Provider, cfg Config) *Agent {
	return &Agent{provider: p, cfg: cfg}
}

// Run executes one turn and streams what happens. The last event is always
// EventDone, whose Result says how the turn ended.
func (a *Agent) Run(ctx context.Context, prompt string) iter.Seq[Event] {
	return func(yield func(Event) bool) {
		t := &turn{agent: a, yield: yield, guard: newGuard(a.cfg.MaxSteps)}
		a.history = append(a.history, provider.Message{
			Role: provider.RoleUser, Parts: []provider.Part{provider.Text{Text: prompt}},
		})
		res := t.loop(ctx)
		if !t.stopped {
			yield(Event{Kind: EventDone, Result: &res})
		}
	}
}

// turn holds the state of one Run.
type turn struct {
	agent   *Agent
	yield   func(Event) bool
	stopped bool // the consumer stopped iterating
	guard   *guard
	res     Result
}

// emit sends an event; it returns false once the consumer has stopped.
func (t *turn) emit(ev Event) bool {
	if t.stopped {
		return false
	}
	t.stopped = !t.yield(ev)
	return !t.stopped
}

// loop is the agent loop itself.
func (t *turn) loop(ctx context.Context) Result {
	for {
		out, err := t.step(ctx, provider.ToolChoiceAuto)
		if err != nil {
			return t.end(statusOf(err), err)
		}
		t.res.FinalText = out.text
		if len(out.calls) == 0 {
			return t.end(StatusSuccess, nil)
		}
		results := t.runTools(out.calls)
		if t.stopped {
			return t.res
		}
		if brake := t.guard.afterStep(out.calls, results); brake != "" {
			return t.summarize(ctx, brake)
		}
	}
}

// stepOutput is what one model call produced.
type stepOutput struct {
	text  string
	calls []provider.ToolCall
}

// step makes one model call, streams its events and appends the assistant
// message to the history.
func (t *turn) step(ctx context.Context, choice provider.ToolChoice) (stepOutput, error) {
	a := t.agent
	a.steps++
	t.res.Steps++
	n := a.steps
	if !t.emit(Event{Kind: EventStepStart, Step: n}) {
		return stepOutput{}, errStopped
	}

	req := provider.Request{
		Model:      a.cfg.Model,
		System:     a.cfg.System,
		Messages:   a.history,
		ToolChoice: choice,
		MaxTokens:  a.cfg.MaxTokens,
	}
	var (
		out     stepOutput
		text    strings.Builder
		done    *provider.Event
		sent    bool
		attempt = 1
		start   = time.Now()
		stats   = ResponseStats{}
	)
	requestEvent := func(bytes int) bool {
		sent = true
		return t.emit(Event{Kind: EventRequest, Step: n, Request: &RequestStats{
			Attempt: attempt, Bytes: bytes, Messages: len(a.history), Tools: len(req.Tools), SystemBytes: len(req.System),
		}})
	}
	for ev, err := range a.provider.Stream(ctx, req) {
		if err != nil {
			if !sent {
				requestEvent(0)
			}
			stats.Duration, stats.Err = time.Since(start), err
			var pe *provider.Error
			if errors.As(err, &pe) {
				stats.HTTPStatus = pe.Status
			}
			t.emit(Event{Kind: EventResponse, Step: n, Response: &stats})
			return out, err
		}
		switch ev.Kind {
		case provider.EventSent:
			start = time.Now() // each attempt is timed on its own
			if !requestEvent(ev.Bytes) {
				return out, errStopped
			}
		case provider.EventRetry:
			// The attempt failed and the same request goes out again:
			// report it, then forget everything it streamed (ADR 0004).
			if !sent {
				requestEvent(0)
			}
			failed := stats
			failed.Duration, failed.Err = time.Since(start), ev.Err
			var pe *provider.Error
			if errors.As(ev.Err, &pe) {
				failed.HTTPStatus = pe.Status
			}
			if !t.emit(Event{Kind: EventResponse, Step: n, Response: &failed}) {
				return out, errStopped
			}
			partial := text.Len() > 0 || len(out.calls) > 0
			t.res.Retries++
			if !t.emit(Event{Kind: EventRetry, Step: n, Retry: &RetryInfo{
				Attempt: ev.Attempt, Delay: ev.Delay, Err: ev.Err, Partial: partial,
			}}) {
				return out, errStopped
			}
			attempt, sent, stats = ev.Attempt, false, ResponseStats{}
			text.Reset()
			out.calls = nil
		case provider.EventTextDelta:
			stats.markFirstContent(start)
			text.WriteString(ev.Text)
			if !t.emit(Event{Kind: EventTextDelta, Step: n, Text: ev.Text}) {
				return out, errStopped
			}
		case provider.EventToolCall:
			stats.markFirstContent(start)
			out.calls = append(out.calls, *ev.ToolCall)
		case provider.EventDone:
			done = &ev
		}
	}
	if done == nil { // a well-behaved provider never ends without Done or an error
		err := &provider.Error{Kind: provider.ErrTruncated, Message: "stream ended without a final event"}
		stats.Duration, stats.Err = time.Since(start), err
		t.emit(Event{Kind: EventResponse, Step: n, Response: &stats})
		return out, err
	}

	out.text = text.String()
	stats.Duration, stats.Usage, stats.Stop, stats.Text = time.Since(start), done.Usage, done.RawStop, out.text
	stats.HTTPStatus = 200
	t.res.Usage.Add(done.Usage)

	parts := make([]provider.Part, 0, 1+len(out.calls))
	parts = append(parts, provider.Text{Text: out.text})
	for _, c := range out.calls {
		parts = append(parts, c)
	}
	a.history = append(a.history, provider.Message{Role: provider.RoleAssistant, Parts: parts})
	if !t.emit(Event{Kind: EventResponse, Step: n, Response: &stats}) {
		return out, errStopped
	}
	return out, nil
}

// runTools answers every tool call and appends the results to the history.
func (t *turn) runTools(calls []provider.ToolCall) []provider.ToolResult {
	n := t.agent.steps
	results := make([]provider.ToolResult, 0, len(calls))
	parts := make([]provider.Part, 0, len(calls))
	for i := range calls {
		call := &calls[i]
		if !t.emit(Event{Kind: EventToolCall, Step: n, ToolCall: call}) {
			return nil
		}
		start := time.Now()
		res := runTool(*call)
		t.res.ToolCalls++
		if res.IsError {
			t.res.ToolErrors++
		}
		results = append(results, res)
		parts = append(parts, res)
		if !t.emit(Event{Kind: EventToolResult, Step: n, ToolResult: &res, ToolDuration: time.Since(start)}) {
			return nil
		}
	}
	t.agent.history = append(t.agent.history, provider.Message{Role: provider.RoleUser, Parts: parts})
	return results
}

// runTool executes one tool. M0 has no tools: every call gets an error the
// model can read, the same answer flores-stub gives. M2 adds the registry.
func runTool(call provider.ToolCall) provider.ToolResult {
	return provider.ToolResult{
		CallID:  call.ID,
		Content: fmt.Sprintf("error: unknown tool %q (no tool with that name is available)", call.Name),
		IsError: true,
	}
}

// summarize runs the extra step after a brake fires (ADR 0003): the model is
// told why the turn stops and asked, without tools, where the work stands.
func (t *turn) summarize(ctx context.Context, brake Status) Result {
	a := t.agent
	a.history = append(a.history, provider.Message{
		Role: provider.RoleUser, Parts: []provider.Part{provider.Text{Text: summaryPrompt(brake, a.cfg.MaxSteps)}},
	})
	out, err := t.step(ctx, provider.ToolChoiceNone)
	if err != nil {
		if errors.Is(err, errStopped) {
			return t.res
		}
		return t.end(brake, err)
	}
	// Tool calls in the summary are ignored. Drop them from the history too,
	// so a later turn never replays a tool call that has no result.
	if len(out.calls) > 0 {
		last := &a.history[len(a.history)-1]
		last.Parts = []provider.Part{provider.Text{Text: out.text}}
	}
	t.res.FinalText = out.text
	return t.end(brake, nil)
}

func summaryPrompt(brake Status, maxSteps int) string {
	why := fmt.Sprintf("it reached the step limit (--max-steps=%d)", maxSteps)
	if brake == StatusRepetition {
		why = fmt.Sprintf("the same tool call returned the same result %d times in the last %d steps, which looks like a loop",
			repeatLimit, repeatWindow)
	}
	return "flores stopped this turn because " + why + ". Tools are disabled for this reply. " +
		"Answer with text only: summarize what you did, what is still pending, and what you recommend as next steps."
}

func (t *turn) end(status Status, err error) Result {
	t.res.Status, t.res.Err = status, err
	return t.res
}

// errStopped is internal: the consumer stopped iterating.
var errStopped = errors.New("consumer stopped")

// statusOf maps the error that ended a turn. Only a cancellation (Ctrl-C) is
// "cancelled"; a timeout is an error like any other.
func statusOf(err error) Status {
	if errors.Is(err, context.Canceled) {
		return StatusCancelled
	}
	return StatusError
}
