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
	ErrQuota                                // no credit or quota left (OpenAI 429 insufficient_quota, 402)
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
	case ErrQuota:
		return "quota"
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
	// Retry is the provider's own advice (x-should-retry header), if any.
	Retry RetryHint
	Err   error
}

// RetryHint is a provider's explicit advice on retrying.
type RetryHint int

const (
	RetryUnset RetryHint = iota // no advice: decide by Kind
	RetryYes
	RetryNo
)

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
// A bad key or an empty account never fixes itself, whatever the provider
// says (layer B: auth-error-fail-fast, out-of-credit-fail-fast; ADR 0004 and
// 0005). Otherwise the provider's hint wins.
func (e *Error) Retryable() bool {
	if e.Kind == ErrAuth || e.Kind == ErrQuota {
		return false
	}
	if e.Retry != RetryUnset {
		return e.Retry == RetryYes
	}
	switch e.Kind {
	case ErrNetwork, ErrRateLimit, ErrOverloaded, ErrTruncated:
		return true
	}
	return false
}
