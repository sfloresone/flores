package provider

import (
	"context"
	"errors"
	"io"
	"net/http"
	"testing"
	"time"
)

func TestRetryAfter(t *testing.T) {
	cases := []struct {
		header map[string]string
		want   time.Duration
	}{
		{nil, 0},
		{map[string]string{"Retry-After": "2"}, 2 * time.Second},
		{map[string]string{"Retry-After": "0.5"}, 500 * time.Millisecond},
		{map[string]string{"Retry-After": "2", "Retry-After-Ms": "1500"}, 1500 * time.Millisecond},
		{map[string]string{"Retry-After": "soon"}, 0},
		{map[string]string{"Retry-After": time.Now().Add(-time.Hour).UTC().Format(http.TimeFormat)}, 0},
	}
	for _, tc := range cases {
		h := http.Header{}
		for k, v := range tc.header {
			h.Set(k, v)
		}
		if got := RetryAfter(h); got != tc.want {
			t.Errorf("%v: got %s, want %s", tc.header, got, tc.want)
		}
	}
	h := http.Header{}
	h.Set("Retry-After", time.Now().Add(30*time.Second).UTC().Format(http.TimeFormat))
	if got := RetryAfter(h); got < 28*time.Second || got > 30*time.Second {
		t.Errorf("HTTP date: got %s", got)
	}
}

func TestShouldRetry(t *testing.T) {
	for v, want := range map[string]RetryHint{"": RetryUnset, "true": RetryYes, "FALSE": RetryNo, "maybe": RetryUnset} {
		h := http.Header{}
		h.Set("x-should-retry", v)
		if got := ShouldRetry(h); got != want {
			t.Errorf("%q: got %v, want %v", v, got, want)
		}
	}
}

func TestKindForStatus(t *testing.T) {
	cases := map[int]ErrorKind{
		400: ErrBadRequest, 401: ErrAuth, 402: ErrQuota, 403: ErrAuth, 404: ErrBadRequest, 408: ErrNetwork,
		409: ErrOverloaded, 413: ErrBadRequest, 429: ErrRateLimit, 500: ErrOverloaded, 503: ErrOverloaded, 529: ErrOverloaded,
	}
	for status, want := range cases {
		if got := KindForStatus(status); got != want {
			t.Errorf("%d: got %s, want %s", status, got, want)
		}
	}
}

func TestTransportErrorKeepsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := TransportError(ctx, io.EOF); !errors.Is(err, context.Canceled) {
		t.Errorf("got %v", err)
	}
	var pe *Error
	if err := TransportError(context.Background(), io.EOF); !errors.As(err, &pe) || pe.Kind != ErrNetwork {
		t.Errorf("got %v", err)
	}
}
