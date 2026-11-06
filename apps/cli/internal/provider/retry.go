package provider

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"math/rand/v2"
	"time"
)

// RetryPolicy decides whether and when a failed request is sent again
// (ADR 0004). It is plain data plus a pure function: no I/O, no clock.
type RetryPolicy struct {
	// MaxRetries is how many times a request may be resent after the first
	// attempt. 0 turns retries off.
	MaxRetries int
	// Base is the first wait after an overload or a 5xx; it doubles on
	// every retry up to Max.
	Base, Max time.Duration
	// MaxServerWait is the longest Retry-After flores accepts. When the
	// provider asks for more, flores gives up at once instead.
	MaxServerWait time.Duration
	// Rand returns a number in [0, 1) for the jitter; nil uses math/rand.
	Rand func() float64
}

// connectionDelay is the first wait after a dropped connection or a cut
// stream: a new connection is usually all it takes.
const connectionDelay = 100 * time.Millisecond

// DefaultRetryPolicy is what flores uses unless told otherwise.
func DefaultRetryPolicy() RetryPolicy {
	return RetryPolicy{MaxRetries: 6, Base: 500 * time.Millisecond, Max: 30 * time.Second, MaxServerWait: time.Minute}
}

// Delay returns how long to wait before retry n (1 is the first retry)
// after err. A non-nil error means "do not retry": it is the error to
// report, and it wraps err.
func (p RetryPolicy) Delay(err error, n int) (time.Duration, error) {
	var pe *Error
	if !errors.As(err, &pe) || !pe.Retryable() || p.MaxRetries <= 0 {
		return 0, err
	}
	if n > p.MaxRetries {
		return 0, fmt.Errorf("gave up after %d attempts: %w", n, err)
	}
	if pe.RetryAfter > 0 {
		if pe.RetryAfter > p.MaxServerWait {
			return 0, fmt.Errorf("%w (the provider asked to wait %s; flores waits at most %s)",
				err, pe.RetryAfter.Round(time.Second), p.MaxServerWait)
		}
		return pe.RetryAfter, nil // exactly what the server asked: it knows best
	}
	k := n
	if pe.Kind == ErrNetwork || pe.Kind == ErrTruncated {
		if n == 1 {
			return p.jitter(connectionDelay), nil
		}
		k = n - 1 // after the quick reconnect, back off like the rest
	}
	d := p.Base
	for i := 1; i < k && d < p.Max; i++ {
		d *= 2
	}
	return p.jitter(min(d, p.Max)), nil
}

// jitter keeps 80–100 % of d, so many clients failing together do not all
// come back at the same instant.
func (p RetryPolicy) jitter(d time.Duration) time.Duration {
	r := rand.Float64
	if p.Rand != nil {
		r = p.Rand
	}
	return time.Duration(float64(d) * (0.8 + 0.2*r()))
}

// WithRetry returns a Provider that resends failed requests following
// policy. Before each retry it emits EventRetry, so the consumer can drop
// what the failed attempt already streamed; then it waits (Ctrl-C cancels
// the wait) and sends exactly the same request again.
func WithRetry(p Provider, policy RetryPolicy) Provider {
	if policy.MaxRetries <= 0 {
		return p
	}
	return &retrying{inner: p, policy: policy}
}

type retrying struct {
	inner  Provider
	policy RetryPolicy
}

func (r *retrying) Stream(ctx context.Context, req Request) iter.Seq2[Event, error] {
	return func(yield func(Event, error) bool) {
		for n := 1; ; n++ {
			var failure error
			for ev, err := range r.inner.Stream(ctx, req) {
				if err != nil {
					failure = err
					break
				}
				if !yield(ev, nil) {
					return
				}
			}
			if failure == nil {
				return
			}
			if ctx.Err() != nil { // cancelled or timed out: never retry
				yield(Event{}, failure)
				return
			}
			delay, giveUp := r.policy.Delay(failure, n)
			if giveUp == nil {
				if deadline, ok := ctx.Deadline(); ok && time.Until(deadline) < delay {
					giveUp = fmt.Errorf("%w (a retry in %s would not fit before the timeout)", failure, delay.Round(time.Millisecond))
				}
			}
			if giveUp != nil {
				yield(Event{}, giveUp)
				return
			}
			if !yield(Event{Kind: EventRetry, Err: failure, Attempt: n + 1, Delay: delay}, nil) {
				return
			}
			if err := sleep(ctx, delay); err != nil {
				yield(Event{}, err)
				return
			}
		}
	}
}

// sleep waits for d or until ctx is done, whichever comes first.
func sleep(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
