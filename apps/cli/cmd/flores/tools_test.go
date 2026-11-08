package main

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// scripted answers request i with replies[i] (the last one after that) and
// keeps every request body.
func scripted(t *testing.T, replies ...[]string) (string, func() []string) {
	t.Helper()
	var mu sync.Mutex
	var bodies []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		bodies = append(bodies, string(b))
		reply := replies[min(len(bodies), len(replies))-1]
		mu.Unlock()
		w.Header().Set("Content-Type", "text/event-stream")
		for _, e := range reply {
			fmt.Fprintf(w, "data: %s\n\n", e)
		}
	}))
	t.Cleanup(srv.Close)
	return srv.URL, func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), bodies...)
	}
}

func bashReply(command string) []string {
	return []string{
		`{"type":"message_start","message":{"usage":{"input_tokens":5,"output_tokens":1}}}`,
		`{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"toolu_1","name":"bash","input":{}}}`,
		fmt.Sprintf(`{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":%q}}`,
			fmt.Sprintf(`{"command":%q}`, command)),
		`{"type":"content_block_stop","index":0}`,
		`{"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"output_tokens":4}}`,
		`{"type":"message_stop"}`,
	}
}

func TestBashOutputGoesBackToTheModel(t *testing.T) {
	t.Chdir(t.TempDir())
	url, bodies := scripted(t, bashReply("echo flores-$((40+2))"), textReply)
	code, out, errOut := flores(t, "-p", "run it", "--base-url", url, "--model", "m", "--api-key", "k")
	if code != exitOK || out != "Hello from the mock.\n" {
		t.Fatalf("exit %d, stdout %q, stderr %q", code, out, errOut)
	}
	b := bodies()
	if len(b) != 2 {
		t.Fatalf("%d requests", len(b))
	}
	for _, name := range []string{`"name":"bash"`, `"name":"edit"`, `"name":"glob"`, `"name":"grep"`, `"name":"read"`, `"name":"write"`, "Working directory: "} {
		if !strings.Contains(b[0], name) {
			t.Errorf("first request lacks %s", name)
		}
	}
	if !strings.Contains(b[1], `flores-42\n`) || strings.Contains(b[1], `"is_error":true`) {
		t.Fatalf("second request does not carry the bash output:\n%s", b[1])
	}
}

func TestNoToolsSendsNone(t *testing.T) {
	url, bodies := scripted(t, textReply)
	if code, _, errOut := flores(t, "-p", "hi", "--no-tools", "--base-url", url, "--model", "m", "--api-key", "k"); code != exitOK {
		t.Fatalf("exit %d, stderr %q", code, errOut)
	}
	if b := bodies()[0]; strings.Contains(b, `"tools"`) || strings.Contains(b, "Working directory") {
		t.Fatalf("request has tools:\n%s", b)
	}
}
