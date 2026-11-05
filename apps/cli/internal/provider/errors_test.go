package provider

import (
	"errors"
	"io"
	"testing"
)

func TestRetryable(t *testing.T) {
	cases := map[ErrorKind]bool{
		ErrNetwork: true, ErrRateLimit: true, ErrOverloaded: true, ErrTruncated: true,
		ErrAuth: false, ErrBadRequest: false, ErrInvalidResponse: false,
	}
	for kind, want := range cases {
		if got := (&Error{Kind: kind}).Retryable(); got != want {
			t.Errorf("%s: Retryable() = %v, want %v", kind, got, want)
		}
	}
}

func TestErrorMessageAndUnwrap(t *testing.T) {
	err := &Error{Kind: ErrAuth, Status: 401, Message: "invalid x-api-key", Err: io.EOF}
	if got, want := err.Error(), "auth (HTTP 401): invalid x-api-key: EOF"; got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
	if !errors.Is(err, io.EOF) {
		t.Error("errors.Is should see the wrapped error")
	}
}
