package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// fakeAnthropic answers every request with the given SSE payloads, or with
// the HTTP status when it is not 200.
func fakeAnthropic(t *testing.T, status int, events ...string) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if status != http.StatusOK {
			w.WriteHeader(status)
			fmt.Fprint(w, `{"type":"error","error":{"type":"authentication_error","message":"invalid x-api-key"}}`)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		for _, e := range events {
			fmt.Fprintf(w, "data: %s\n\n", e)
		}
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

var (
	textReply = []string{
		`{"type":"message_start","message":{"usage":{"input_tokens":5,"output_tokens":1}}}`,
		`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hello from the mock."}}`,
		`{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":4}}`,
		`{"type":"message_stop"}`,
	}
	toolReply = []string{
		`{"type":"message_start","message":{"usage":{"input_tokens":5,"output_tokens":1}}}`,
		`{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"toolu_1","name":"bash","input":{}}}`,
		`{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{}"}}`,
		`{"type":"content_block_stop","index":0}`,
		`{"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"output_tokens":4}}`,
		`{"type":"message_stop"}`,
	}
)

func flores(t *testing.T, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	var out, errOut bytes.Buffer
	code = run(args, strings.NewReader(""), &out, &errOut)
	return code, out.String(), errOut.String()
}

func TestVersionAndUsage(t *testing.T) {
	if code, out, _ := flores(t, "--version"); code != exitOK || !strings.HasPrefix(out, "flores ") {
		t.Fatalf("--version: %d %q", code, out)
	}
	if code, _, _ := flores(t); code != exitUsage {
		t.Fatalf("no args exit %d, want %d", code, exitUsage)
	}
	if code, _, errOut := flores(t, "-p", "hi", "--model", "m"); code != exitUsage || !strings.Contains(errOut, "API key") {
		t.Fatalf("missing key: %d %q", code, errOut)
	}
}

func TestTextOutput(t *testing.T) {
	url := fakeAnthropic(t, http.StatusOK, textReply...)
	code, out, errOut := flores(t, "-p", "Say hello.", "--base-url", url, "--model", "m", "--api-key", "k")
	if code != exitOK || out != "Hello from the mock.\n" {
		t.Fatalf("exit %d, stdout %q, stderr %q", code, out, errOut)
	}
}

func TestJSONLTrajectory(t *testing.T) {
	url := fakeAnthropic(t, http.StatusOK, textReply...)
	code, out, _ := flores(t, "-p", "Say hello.", "--output", "jsonl", "--base-url", url, "--model", "m", "--api-key", "k")
	if code != exitOK {
		t.Fatalf("exit %d", code)
	}
	var types []string
	var last map[string]any
	for i, line := range strings.Split(strings.TrimSpace(out), "\n") {
		var ev map[string]any
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			t.Fatalf("line %d is not JSON: %q", i, line)
		}
		if ev["seq"] != float64(i) {
			t.Fatalf("line %d has seq %v", i, ev["seq"])
		}
		types = append(types, ev["type"].(string))
		last = ev
	}
	want := "session.start step.start llm.request llm.response message session.end"
	if got := strings.Join(types, " "); got != want {
		t.Fatalf("events:\n got %s\nwant %s", got, want)
	}
	if last["status"] != "success" || last["steps"] != float64(1) || last["final_text"] != "Hello from the mock." {
		t.Fatalf("session.end %v", last)
	}
}

func TestAuthErrorExitsThree(t *testing.T) {
	url := fakeAnthropic(t, http.StatusUnauthorized)
	code, _, errOut := flores(t, "-p", "hi", "--base-url", url, "--model", "m", "--api-key", "bad")
	if code != exitProvider || !strings.Contains(errOut, "HTTP 401") || !strings.Contains(errOut, "hint:") {
		t.Fatalf("exit %d, stderr %q", code, errOut)
	}
}

func TestStepLimitExitsFour(t *testing.T) {
	url := fakeAnthropic(t, http.StatusOK, toolReply...)
	code, _, errOut := flores(t, "-p", "loop", "--max-steps", "2", "--base-url", url, "--model", "m", "--api-key", "k")
	if code != exitBrake || !strings.Contains(errOut, "--max-steps=2") {
		t.Fatalf("exit %d, stderr %q", code, errOut)
	}
}

// flakyAnthropic fails the first n requests with a 503 (asking to retry in
// 10ms) and then answers textReply. It counts the requests it gets.
func flakyAnthropic(t *testing.T, n int32) (string, *atomic.Int32) {
	t.Helper()
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) <= n {
			w.Header().Set("Retry-After-Ms", "10")
			w.WriteHeader(http.StatusServiceUnavailable)
			fmt.Fprint(w, `{"type":"error","error":{"type":"overloaded_error","message":"busy"}}`)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		for _, e := range textReply {
			fmt.Fprintf(w, "data: %s\n\n", e)
		}
	}))
	t.Cleanup(srv.Close)
	return srv.URL, &calls
}

func TestRetryRecoversAndSaysSo(t *testing.T) {
	url, calls := flakyAnthropic(t, 2)
	code, out, errOut := flores(t, "-p", "hi", "--base-url", url, "--model", "m", "--api-key", "k")
	if code != exitOK || out != "Hello from the mock.\n" || calls.Load() != 3 {
		t.Fatalf("exit %d, %d calls, stdout %q, stderr %q", code, calls.Load(), out, errOut)
	}
	if !strings.Contains(errOut, "HTTP 503") || !strings.Contains(errOut, "retrying in 10ms (attempt 3 of 7)") {
		t.Fatalf("stderr %q", errOut)
	}
}

func TestRetryTrajectory(t *testing.T) {
	url, _ := flakyAnthropic(t, 1)
	code, out, _ := flores(t, "-p", "hi", "--output", "jsonl", "--base-url", url, "--model", "m", "--api-key", "k")
	if code != exitOK {
		t.Fatalf("exit %d", code)
	}
	var types []string
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		var ev map[string]any
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			t.Fatal(err)
		}
		typ := ev["type"].(string)
		switch typ {
		case "llm.request":
			typ = fmt.Sprintf("request#%v", ev["attempt"])
		case "llm.response":
			typ = fmt.Sprintf("response:%v", ev["status"])
		case "error":
			if ev["retryable"] != true || ev["attempt"] != float64(1) || ev["retry_in_ms"] != float64(10) {
				t.Fatalf("error event %v", ev)
			}
		}
		types = append(types, typ)
	}
	want := "session.start step.start request#1 response:error error request#2 response:ok message session.end"
	if got := strings.Join(types, " "); got != want {
		t.Fatalf("events:\n got %s\nwant %s", got, want)
	}
}

func TestNoRetriesFailsOnTheFirstError(t *testing.T) {
	url, calls := flakyAnthropic(t, 1)
	code, _, errOut := flores(t, "-p", "hi", "--max-retries", "0", "--base-url", url, "--model", "m", "--api-key", "k")
	if code != exitProvider || calls.Load() != 1 || strings.Contains(errOut, "retrying") {
		t.Fatalf("exit %d, %d calls, stderr %q", code, calls.Load(), errOut)
	}
}

func TestGivingUpSaysHowManyAttempts(t *testing.T) {
	url, calls := flakyAnthropic(t, 100)
	code, _, errOut := flores(t, "-p", "hi", "--max-retries", "2", "--base-url", url, "--model", "m", "--api-key", "k")
	if code != exitProvider || calls.Load() != 3 || !strings.Contains(errOut, "gave up after 3 attempts") {
		t.Fatalf("exit %d, %d calls, stderr %q", code, calls.Load(), errOut)
	}
}
