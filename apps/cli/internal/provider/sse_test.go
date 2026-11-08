package provider

import (
	"errors"
	"strings"
	"testing"
)

func readAll(t *testing.T, body string, stopAt int) ([]string, error) {
	t.Helper()
	var got []string
	err := ReadSSE(strings.NewReader(body), func(data []byte) (bool, error) {
		got = append(got, string(data))
		return len(got) != stopAt, nil
	})
	return got, err
}

func TestReadSSEFraming(t *testing.T) {
	body := ": keep-alive\r\n" +
		"event: x\r\ndata: one\r\n\r\n" + // CRLF
		"data:two\n\n" + // no space after the colon
		"data: a\ndata: b\n\n" + // multi-line data joins with \n
		"id: 7\n\n" + // an event without data is not dispatched
		"data: last\n\n"
	got, err := readAll(t, body, 4)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"one", "two", "a\nb", "last"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestReadSSEEndOfBodyIsStreamEnded(t *testing.T) {
	got, err := readAll(t, "data: one\n\ndata: half", -1)
	if !errors.Is(err, ErrStreamEnded) || len(got) != 1 {
		t.Fatalf("got %q, %v", got, err)
	}
}

func TestReadSSEHandlerStopsWithoutError(t *testing.T) {
	if _, err := readAll(t, "data: one\n\ndata: two\n\n", 1); err != nil {
		t.Fatalf("a handler that stops is not an error: %v", err)
	}
}
