package provider

import (
	"context"
	"errors"
	"iter"
	"strings"
	"testing"
	"time"
)

func TestRetryDelay(t *testing.T) {
	p := RetryPolicy{MaxRetries: 6, Base: 500 * time.Millisecond, Max: 30 * time.Second, MaxServerWait: time.Minute,
		Rand: func() float64 { return 1 }} // no jitter: 0.8 + 0.2×1 = 100 %
	overloaded := &Error{Kind: ErrOverloaded, Status: 503}
	cases := []struct {
		name    string
		err     error
		n       int
		want    time.Duration
		giveUp  string // substring of the error; "" means retry
		sameErr bool   // the error is returned unchanged
	}{
		{"overload 1st", overloaded, 1, 500 * time.Millisecond, "", false},
		{"overload 2nd doubles", overloaded, 2, time.Second, "", false},
		{"overload 4th", overloaded, 4, 4 * time.Second, "", false},
		{"overload 6th", &Error{Kind: ErrOverloaded}, 6, 16 * time.Second, "", false},
		{"cut stream reconnects fast", &Error{Kind: ErrTruncated}, 1, 100 * time.Millisecond, "", false},
		{"cut stream then backs off", &Error{Kind: ErrTruncated}, 2, 500 * time.Millisecond, "", false},
		{"network 3rd", &Error{Kind: ErrNetwork}, 3, time.Second, "", false},
		{"retry-after exact", &Error{Kind: ErrRateLimit, RetryAfter: 1500 * time.Millisecond}, 1, 1500 * time.Millisecond, "", false},
		{"retry-after too long", &Error{Kind: ErrRateLimit, RetryAfter: time.Hour}, 1, 0, "asked to wait 1h0m0s", false},
		{"out of retries", overloaded, 7, 0, "gave up after 7 attempts", false},
		{"401 never", &Error{Kind: ErrAuth, Status: 401}, 1, 0, "", true},
		{"401 even if the server says yes", &Error{Kind: ErrAuth, Retry: RetryYes}, 1, 0, "", true},
		{"bad request", &Error{Kind: ErrBadRequest, Status: 400}, 1, 0, "", true},
		{"server says yes", &Error{Kind: ErrBadRequest, Status: 409, Retry: RetryYes}, 1, 500 * time.Millisecond, "", false},
		{"server says no", &Error{Kind: ErrOverloaded, Status: 503, Retry: RetryNo}, 1, 0, "", true},
		{"not a provider error", errors.New("boom"), 1, 0, "", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := p.Delay(c.err, c.n)
			switch {
			case c.sameErr:
				if err != c.err {
					t.Fatalf("err = %v, want the original error", err)
				}
			case c.giveUp != "":
				if err == nil || !strings.Contains(err.Error(), c.giveUp) || !errors.Is(err, c.err) {
					t.Fatalf("err = %v, want it to mention %q and wrap the original", err, c.giveUp)
				}
			default:
				if err != nil || got != c.want {
					t.Fatalf("Delay = %v, %v; want %v", got, err, c.want)
				}
			}
		})
	}
}

func TestRetryDelayIsCappedAtMax(t *testing.T) {
	p := RetryPolicy{MaxRetries: 20, Base: time.Second, Max: 30 * time.Second, MaxServerWait: time.Minute,
		Rand: func() float64 { return 1 }}
	if d, _ := p.Delay(&Error{Kind: ErrOverloaded}, 20); d != 30*time.Second {
		t.Fatalf("20th retry waits %v, want the 30s cap", d)
	}
}

func TestRetryJitterStaysWithin80To100Percent(t *testing.T) {
	p := DefaultRetryPolicy()
	p.Rand = func() float64 { return 0 }
	if d, _ := p.Delay(&Error{Kind: ErrOverloaded}, 1); d != 400*time.Millisecond {
		t.Fatalf("lowest jitter gives %v, want 400ms", d)
	}
}

// attempt is what one call to the scripted provider streams.
type attempt struct {
	text string
	err  error // after the text
}

type scripted struct {
	attempts []attempt
	calls    int
	requests []Request
}

func (s *scripted) Stream(ctx context.Context, req Request) iter.Seq2[Event, error] {
	s.requests = append(s.requests, req)
	a := s.attempts[min(s.calls, len(s.attempts)-1)]
	s.calls++
	return func(yield func(Event, error) bool) {
		if !yield(Event{Kind: EventSent, Bytes: 10}, nil) {
			return
		}
		if a.text != "" && !yield(Event{Kind: EventTextDelta, Text: a.text}, nil) {
			return
		}
		if a.err != nil {
			yield(Event{}, a.err)
			return
		}
		yield(Event{Kind: EventDone, RawStop: "end_turn"}, nil)
	}
}

var fast = RetryPolicy{MaxRetries: 3, Base: time.Millisecond, Max: 4 * time.Millisecond, MaxServerWait: time.Minute}

func collect(t *testing.T, s iter.Seq2[Event, error]) (kinds []string, last error) {
	t.Helper()
	names := map[EventKind]string{EventSent: "sent", EventTextDelta: "text", EventToolCall: "call", EventDone: "done", EventRetry: "retry"}
	for ev, err := range s {
		if err != nil {
			return kinds, err
		}
		kinds = append(kinds, names[ev.Kind])
	}
	return kinds, nil
}

func TestWithRetryResendsAfterACutStream(t *testing.T) {
	inner := &scripted{attempts: []attempt{
		{text: "The qu", err: &Error{Kind: ErrTruncated}},
		{text: "The quick brown fox."},
	}}
	var retry Event
	var kinds []string
	for ev, err := range WithRetry(inner, fast).Stream(context.Background(), Request{Model: "m"}) {
		if err != nil {
			t.Fatal(err)
		}
		kinds = append(kinds, map[EventKind]string{EventSent: "sent", EventTextDelta: "text", EventDone: "done", EventRetry: "retry"}[ev.Kind])
		if ev.Kind == EventRetry {
			retry = ev
		}
	}
	want := "sent text retry sent text done"
	if got := strings.Join(kinds, " "); got != want {
		t.Fatalf("events %q, want %q", got, want)
	}
	if retry.Attempt != 2 || retry.Delay <= 0 || retry.Delay > 100*time.Millisecond || retry.Err == nil {
		t.Fatalf("retry event %+v", retry)
	}
	if inner.calls != 2 || inner.requests[0].Model != inner.requests[1].Model {
		t.Fatalf("%d calls; the retry must resend the same request", inner.calls)
	}
}

func TestWithRetryGivesUp(t *testing.T) {
	inner := &scripted{attempts: []attempt{{err: &Error{Kind: ErrOverloaded, Status: 503}}}}
	_, err := collect(t, WithRetry(inner, fast).Stream(context.Background(), Request{}))
	var pe *Error
	if inner.calls != 4 || !errors.As(err, &pe) || !strings.Contains(err.Error(), "gave up after 4 attempts") {
		t.Fatalf("%d calls, err %v", inner.calls, err)
	}
}

func TestWithRetryNeverRetriesAuth(t *testing.T) {
	auth := &Error{Kind: ErrAuth, Status: 401}
	inner := &scripted{attempts: []attempt{{err: auth}}}
	if _, err := collect(t, WithRetry(inner, fast).Stream(context.Background(), Request{})); err != auth || inner.calls != 1 {
		t.Fatalf("%d calls, err %v", inner.calls, err)
	}
}

func TestWithRetryWaitIsCancellable(t *testing.T) {
	inner := &scripted{attempts: []attempt{{err: &Error{Kind: ErrRateLimit, RetryAfter: 30 * time.Second}}}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	start := time.Now()
	var err error
	for ev, e := range WithRetry(inner, fast).Stream(ctx, Request{}) {
		if ev.Kind == EventRetry {
			cancel() // Ctrl-C while waiting
		}
		err = e
	}
	if !errors.Is(err, context.Canceled) || time.Since(start) > 5*time.Second || inner.calls != 1 {
		t.Fatalf("err %v after %v and %d calls", err, time.Since(start), inner.calls)
	}
}

func TestWithRetryRespectsTheDeadline(t *testing.T) {
	inner := &scripted{attempts: []attempt{{err: &Error{Kind: ErrRateLimit, RetryAfter: 10 * time.Second}}}}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, err := collect(t, WithRetry(inner, fast).Stream(ctx, Request{}))
	if err == nil || !strings.Contains(err.Error(), "would not fit before the timeout") || inner.calls != 1 {
		t.Fatalf("%d calls, err %v", inner.calls, err)
	}
}

func TestWithRetryStopsWhenTheConsumerStops(t *testing.T) {
	inner := &scripted{attempts: []attempt{{err: &Error{Kind: ErrOverloaded}}}}
	for ev := range WithRetry(inner, fast).Stream(context.Background(), Request{}) {
		if ev.Kind == EventRetry {
			break
		}
	}
	if inner.calls != 1 {
		t.Fatalf("%d calls after the consumer stopped", inner.calls)
	}
}

func TestWithRetryZeroIsTheSameProvider(t *testing.T) {
	inner := &scripted{}
	if WithRetry(inner, RetryPolicy{}) != Provider(inner) {
		t.Fatal("MaxRetries 0 must not wrap the provider")
	}
}
