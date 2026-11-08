package provider

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Helpers every HTTP adapter needs to fill an *Error the same way.

// TransportError classifies a failure to get a response at all. A cancelled
// or expired context is returned as ctx.Err(), never as a provider error.
func TransportError(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return &Error{Kind: ErrNetwork, Err: err}
}

// RetryAfter reads the wait the provider asks for: retry-after-ms (more
// precise) first, then Retry-After in seconds or as an HTTP date.
func RetryAfter(h http.Header) time.Duration {
	if ms, err := strconv.ParseFloat(h.Get("Retry-After-Ms"), 64); err == nil && ms > 0 {
		return time.Duration(ms * float64(time.Millisecond))
	}
	v := h.Get("Retry-After")
	if v == "" {
		return 0
	}
	if s, err := strconv.ParseFloat(v, 64); err == nil && s >= 0 {
		return time.Duration(s * float64(time.Second))
	}
	if t, err := http.ParseTime(v); err == nil {
		return max(0, time.Until(t))
	}
	return 0
}

// ShouldRetry reads x-should-retry, the provider's explicit advice.
func ShouldRetry(h http.Header) RetryHint {
	switch strings.ToLower(h.Get("X-Should-Retry")) {
	case "true":
		return RetryYes
	case "false":
		return RetryNo
	}
	return RetryUnset
}

// KindForStatus is the classification shared by every adapter. Adapters
// refine it with what the body says (for example, a 429 that means "no
// credit left" rather than "too fast").
func KindForStatus(status int) ErrorKind {
	switch {
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		return ErrAuth
	case status == http.StatusPaymentRequired:
		return ErrQuota
	case status == http.StatusTooManyRequests:
		return ErrRateLimit
	case status == http.StatusRequestTimeout:
		return ErrNetwork
	case status == http.StatusConflict: // a lock or a race on their side; the official SDKs retry it
		return ErrOverloaded
	case status >= 500: // includes Anthropic's 529 "overloaded"
		return ErrOverloaded
	}
	return ErrBadRequest
}
