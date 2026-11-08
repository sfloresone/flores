package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"iter"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/sfloresone/flores/apps/cli/internal/provider"
)

// reply is one scripted model answer.
type reply struct {
	text  string
	calls []provider.ToolCall
	err   error
	stop  provider.StopReason
}

// fake is a provider that answers from a script; after the script ends it
// repeats the last reply. With ToolChoiceNone it answers summaryReply.
type fake struct {
	script       []reply
	summaryReply reply
	requests     []provider.Request
}

func (f *fake) Stream(ctx context.Context, req provider.Request) iter.Seq2[provider.Event, error] {
	req.Messages = slices.Clone(req.Messages) // the agent keeps appending
	f.requests = append(f.requests, req)
	r := f.script[min(len(f.requests), len(f.script))-1]
	if req.ToolChoice == provider.ToolChoiceNone {
		r = f.summaryReply
	}
	return func(yield func(provider.Event, error) bool) {
		if err := ctx.Err(); err != nil {
			yield(provider.Event{}, err)
			return
		}
		if !yield(provider.Event{Kind: provider.EventSent, Bytes: 100}, nil) {
			return
		}
		if r.err != nil {
			yield(provider.Event{}, r.err)
			return
		}
		for _, chunk := range strings.SplitAfter(r.text, " ") {
			if chunk != "" && !yield(provider.Event{Kind: provider.EventTextDelta, Text: chunk}, nil) {
				return
			}
		}
		for _, c := range r.calls {
			if !yield(provider.Event{Kind: provider.EventToolCall, ToolCall: &c}, nil) {
				return
			}
		}
		yield(provider.Event{Kind: provider.EventDone, Usage: provider.Usage{InputTokens: 10, OutputTokens: 2}, Stop: r.stop, RawStop: "end_turn"}, nil)
	}
}

func call(id, name, input string) provider.ToolCall {
	return provider.ToolCall{ID: id, Name: name, Input: json.RawMessage(input)}
}

func run(t *testing.T, a *Agent, ctx context.Context, prompt string) ([]Event, Result) {
	t.Helper()
	var events []Event
	for ev := range a.Run(ctx, prompt) {
		events = append(events, ev)
	}
	last := events[len(events)-1]
	if last.Kind != EventDone || last.Result == nil {
		t.Fatalf("last event is %v, want EventDone", last.Kind)
	}
	return events, *last.Result
}

func kinds(events []Event) []EventKind {
	var ks []EventKind
	for _, ev := range events {
		if ev.Kind != EventTextDelta {
			ks = append(ks, ev.Kind)
		}
	}
	return ks
}

func TestTextOnlyTurn(t *testing.T) {
	f := &fake{script: []reply{{text: "Hello there"}}}
	events, res := run(t, New(f, Config{Model: "m", System: "sys"}), context.Background(), "hi")
	if res.Status != StatusSuccess || res.Steps != 1 || res.FinalText != "Hello there" || res.Usage.InputTokens != 10 {
		t.Fatalf("result %+v", res)
	}
	want := []EventKind{EventStepStart, EventRequest, EventResponse, EventDone}
	if got := kinds(events); !slices.Equal(got, want) {
		t.Fatalf("events %v, want %v", got, want)
	}
	resp := events[len(events)-2].Response
	if resp.Text != "Hello there" || resp.HTTPStatus != 200 || resp.TTFT <= 0 || resp.Duration < resp.TTFT {
		t.Fatalf("response stats %+v", resp)
	}
	if req := events[1].Request; req.Bytes != 100 || req.Messages != 1 || req.SystemBytes != 3 {
		t.Fatalf("request stats %+v", req)
	}
}

func TestUnknownToolGetsAnErrorResult(t *testing.T) {
	f := &fake{script: []reply{{calls: []provider.ToolCall{call("c1", "bash", `{"command":"ls"}`)}}, {text: "done"}}}
	events, res := run(t, New(f, Config{}), context.Background(), "list files")
	if res.Status != StatusSuccess || res.Steps != 2 || res.ToolCalls != 1 || res.ToolErrors != 1 || res.FinalText != "done" {
		t.Fatalf("result %+v", res)
	}
	want := []EventKind{EventStepStart, EventRequest, EventResponse, EventToolCall, EventToolResult,
		EventStepStart, EventRequest, EventResponse, EventDone}
	if got := kinds(events); !slices.Equal(got, want) {
		t.Fatalf("events %v, want %v", got, want)
	}
	// The second request carries the assistant tool call and its error result.
	msgs := f.requests[1].Messages
	tr, ok := msgs[len(msgs)-1].Parts[0].(provider.ToolResult)
	if !ok || tr.CallID != "c1" || !tr.IsError || !strings.Contains(tr.Content, `unknown tool "bash"`) {
		t.Fatalf("last message %+v", msgs[len(msgs)-1])
	}
}

func TestMaxStepsStopsWithASummary(t *testing.T) {
	// Different arguments every step, so only the step limit can stop it.
	var script []reply
	for i := range 10 {
		script = append(script, reply{calls: []provider.ToolCall{call(fmt.Sprint("c", i), "bash", fmt.Sprintf(`{"n":%d}`, i))}})
	}
	f := &fake{script: script, summaryReply: reply{text: "I listed nothing; next: add tools."}}
	_, res := run(t, New(f, Config{MaxSteps: 3}), context.Background(), "go")
	if res.Status != StatusMaxSteps || res.Steps != 4 || len(f.requests) != 4 || res.Err != nil {
		t.Fatalf("result %+v after %d requests", res, len(f.requests))
	}
	if res.FinalText != "I listed nothing; next: add tools." {
		t.Fatalf("final text %q", res.FinalText)
	}
	last := f.requests[3]
	msg := last.Messages[len(last.Messages)-1]
	text, _ := msg.Parts[0].(provider.Text)
	if last.ToolChoice != provider.ToolChoiceNone || msg.Role != provider.RoleUser || !strings.Contains(text.Text, "--max-steps=3") {
		t.Fatalf("summary request: choice %v, message %+v", last.ToolChoice, msg)
	}
}

func TestRepetitionStopsWithoutAStepLimit(t *testing.T) {
	loop := reply{calls: []provider.ToolCall{call("c", "flores_does_not_exist", `{}`)}}
	// The summary also asks for a tool (the benchmark mock does): it is ignored.
	f := &fake{script: []reply{loop}, summaryReply: reply{text: "stuck", calls: loop.calls}}
	a := New(f, Config{MaxSteps: 0})
	_, res := run(t, a, context.Background(), "do the task")
	if res.Status != StatusRepetition || res.Steps != repeatLimit+1 || len(f.requests) != repeatLimit+1 {
		t.Fatalf("result %+v after %d requests", res, len(f.requests))
	}
	last := a.history[len(a.history)-1]
	if len(last.Parts) != 1 {
		t.Fatalf("summary tool calls must not stay in the history: %+v", last)
	}
}

func TestProviderErrorEndsTheTurn(t *testing.T) {
	perr := &provider.Error{Kind: provider.ErrAuth, Status: 401}
	f := &fake{script: []reply{{err: perr}}}
	events, res := run(t, New(f, Config{}), context.Background(), "hi")
	if res.Status != StatusError || !errors.Is(res.Err, perr) || len(f.requests) != 1 {
		t.Fatalf("result %+v", res)
	}
	resp := events[len(events)-2].Response
	if resp == nil || resp.HTTPStatus != 401 || resp.Err == nil {
		t.Fatalf("response %+v", resp)
	}
}

func TestCancelledTurn(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	f := &fake{script: []reply{{text: "never"}}}
	_, res := run(t, New(f, Config{}), ctx, "hi")
	if res.Status != StatusCancelled || !errors.Is(res.Err, context.Canceled) {
		t.Fatalf("result %+v", res)
	}
}

func TestConsumerCanStopEarly(t *testing.T) {
	f := &fake{script: []reply{{calls: []provider.ToolCall{call("c", "x", `{}`)}}}}
	for ev := range New(f, Config{}).Run(context.Background(), "hi") {
		if ev.Kind == EventToolCall {
			break
		}
	}
	if len(f.requests) != 1 {
		t.Fatalf("the loop kept going after the consumer stopped: %d requests", len(f.requests))
	}
}

func TestSecondTurnKeepsHistoryAndStepNumbers(t *testing.T) {
	f := &fake{script: []reply{{text: "one"}, {text: "two"}}}
	a := New(f, Config{})
	run(t, a, context.Background(), "first")
	events, res := run(t, a, context.Background(), "second")
	if res.Steps != 1 || events[0].Step != 2 || len(f.requests[1].Messages) != 3 {
		t.Fatalf("second turn: steps %d, first step %d, %d messages", res.Steps, events[0].Step, len(f.requests[1].Messages))
	}
}

// flaky cuts the first answer after a few words, then answers in full.
type flaky struct {
	calls    int
	requests []provider.Request
}

func (f *flaky) Stream(ctx context.Context, req provider.Request) iter.Seq2[provider.Event, error] {
	f.calls++
	f.requests = append(f.requests, req)
	first := f.calls == 1
	return func(yield func(provider.Event, error) bool) {
		if !yield(provider.Event{Kind: provider.EventSent, Bytes: 100}, nil) {
			return
		}
		if first {
			if yield(provider.Event{Kind: provider.EventTextDelta, Text: "The qu"}, nil) {
				yield(provider.Event{}, &provider.Error{Kind: provider.ErrTruncated})
			}
			return
		}
		if yield(provider.Event{Kind: provider.EventTextDelta, Text: "The quick brown fox."}, nil) {
			yield(provider.Event{Kind: provider.EventDone, RawStop: "end_turn"}, nil)
		}
	}
}

func TestRetryDiscardsThePartialAttempt(t *testing.T) {
	f := &flaky{}
	policy := provider.RetryPolicy{MaxRetries: 2, Base: time.Millisecond, Max: time.Millisecond, MaxServerWait: time.Second}
	a := New(provider.WithRetry(f, policy), Config{MaxSteps: 1})
	events, res := run(t, a, context.Background(), "go")
	if res.Status != StatusSuccess || res.Steps != 1 || res.Retries != 1 || res.FinalText != "The quick brown fox." {
		t.Fatalf("result %+v", res)
	}
	want := []EventKind{EventStepStart, EventRequest, EventResponse, EventRetry, EventRequest, EventResponse, EventDone}
	if got := kinds(events); !slices.Equal(got, want) {
		t.Fatalf("events %v, want %v", got, want)
	}
	var attempts []int
	for _, ev := range events {
		switch {
		case ev.Request != nil:
			attempts = append(attempts, ev.Request.Attempt)
		case ev.Retry != nil && (!ev.Retry.Partial || ev.Retry.Attempt != 2 || ev.Step != 1):
			t.Fatalf("retry event %+v (step %d)", ev.Retry, ev.Step)
		case ev.Kind == EventResponse && ev.Response.Err != nil && ev.Response.Text != "":
			t.Fatalf("a failed attempt has no answer text: %+v", ev.Response)
		}
	}
	if !slices.Equal(attempts, []int{1, 2}) {
		t.Fatalf("attempts %v", attempts)
	}
	// The model never sees the cut answer: the retry resends the same
	// history, and only the complete answer is kept.
	if len(f.requests[0].Messages) != len(f.requests[1].Messages) || len(a.history) != 2 {
		t.Fatalf("history: %d then %d messages sent, %d kept", len(f.requests[0].Messages), len(f.requests[1].Messages), len(a.history))
	}
	if text := a.history[1].Parts[0].(provider.Text).Text; text != "The quick brown fox." {
		t.Fatalf("kept %q", text)
	}
}
