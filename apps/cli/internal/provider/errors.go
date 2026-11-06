package provider

import (
	"fmt"
	"time"
)

// ErrorKind classifies a provider failure the same way for every vendor, so
// the agent can decide what to do without knowing HTTP codes or vendor
// error names.
type ErrorKind int

const (
	ErrNetwork         ErrorKind = iota + 1 // could not connect, or the connection dropped
	ErrAuth                                 // bad or missing credentials (401/403)
	ErrRateLimit                            // too many requests (429)
	ErrOverloaded                           // the provider is failing or busy (5xx, 529)
	ErrBadRequest                           // the request itself is wrong (400, 404, 413…)
	ErrTruncated                            // the stream ended before the provider finished
	ErrInvalidResponse                      // the provider sent something we cannot parse
)

func (k ErrorKind) String() string {
	switch k {
	case ErrNetwork:
		return "network"
	case ErrAuth:
		return "auth"
	case ErrRateLimit:
		return "rate_limit"
	case ErrOverloaded:
		return "overloaded"
	case ErrBadRequest:
		return "bad_request"
	case ErrTruncated:
		return "truncated"
	case ErrInvalidResponse:
		return "invalid_response"
	}
	return "unknown"
}

// Error is a classified provider failure.
type Error struct {
	Kind ErrorKind
	// Status is the HTTP status, or 0 when there was no HTTP response.
	Status int
	// RetryAfter is the provider's hint (Retry-After header), or 0.
	RetryAfter time.Duration
	// Message is the provider's explanation, when it gave one.
	Message string
	Err     error
}

func (e *Error) Error() string {
	s := e.Kind.String()
	if e.Status != 0 {
		s = fmt.Sprintf("%s (HTTP %d)", s, e.Status)
	}
	if e.Message != "" {
		s += ": " + e.Message
	}
	if e.Err != nil {
		s += ": " + e.Err.Error()
	}
	return s
}

func (e *Error) Unwrap() error { return e.Err }

// Retryable reports whether sending the same request again can succeed.
// Auth and bad requests never fix themselves (layer B: auth-error-fail-fast).
func (e *Error) Retryable() bool {
	switch e.Kind {
	case ErrNetwork, ErrRateLimit, ErrOverloaded, ErrTruncated:
		return true
	}
	return false
}
