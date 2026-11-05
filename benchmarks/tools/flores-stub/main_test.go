package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// fakeChat answers turn 1 with a tool call and turn 2 with text.
func fakeChat(t *testing.T) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Messages []map[string]any `json:"messages"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		w.Header().Set("Content-Type", "text/event-stream")
		send := func(s string) { fmt.Fprintf(w, "data: %s\n\n", s) }
		if len(req.Messages) == 2 {
			send(`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"c1","function":{"name":"bash","arguments":"{\"cmd\":"}}]}}]}`)
			send(`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"ls\"}"}}]}}]}`)
			send(`{"choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}`)
		} else {
			send(`{"choices":[{"index":0,"delta":{"content":"all "}}]}`)
			send(`{"choices":[{"index":0,"delta":{"content":"done"},"finish_reason":"stop"}]}`)
		}
		send(`{"choices":[],"usage":{"prompt_tokens":10,"completion_tokens":2}}`)
		send(`[DONE]`)
	}))
}

func TestRunJSONL(t *testing.T) {
	srv := fakeChat(t)
	defer srv.Close()
	var out, errOut bytes.Buffer
	code := run([]string{"run", "--output", "jsonl", "--base-url", srv.URL, "--prompt", "hi"}, nil, &out, &errOut)
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut.String())
	}
	var types []string
	var last map[string]any
	sc := bufio.NewScanner(&out)
	for i := 0; sc.Scan(); i++ {
		var ev map[string]any
		if err := json.Unmarshal(sc.Bytes(), &ev); err != nil {
			t.Fatal(err)
		}
		if int(ev["seq"].(float64)) != i {
			t.Errorf("seq %v at line %d", ev["seq"], i)
		}
		types = append(types, ev["type"].(string))
		last = ev
	}
	want := "session.start turn.start llm.request llm.response tool.call tool.result " +
		"turn.start llm.request llm.response message session.end"
	if got := strings.Join(types, " "); got != want {
		t.Errorf("events:\n got %s\nwant %s", got, want)
	}
	if last["status"] != "success" || last["final_text"] != "all done" || last["tool_errors"].(float64) != 1 {
		t.Errorf("session.end: %v", last)
	}
	if last["duration_ms"].(float64) <= 0 {
		t.Errorf("session duration must be > 0: %v", last["duration_ms"])
	}
	if u := last["usage"].(map[string]any); u["input_tokens"].(float64) != 20 {
		t.Errorf("usage not summed: %v", u)
	}
}

func TestMaxTurnsAndProviderError(t *testing.T) {
	srv := fakeChat(t)
	defer srv.Close()
	var out, errOut bytes.Buffer
	if code := run([]string{"run", "--base-url", srv.URL, "--max-turns", "1", "hi"}, nil, &out, &errOut); code != exitLimit {
		t.Errorf("max turns exit %d", code)
	}

	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":{"message":"nope"}}`, 429)
	}))
	defer bad.Close()
	out.Reset()
	if code := run([]string{"run", "--output", "jsonl", "--base-url", bad.URL, "hi"}, nil, &out, &errOut); code != exitProvider {
		t.Errorf("provider error exit %d", code)
	}
	if !strings.Contains(out.String(), `"retryable":true`) {
		t.Errorf("429 should be retryable: %s", out.String())
	}
}

func TestUsageErrors(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := run([]string{"run", "--prompt", "x"}, strings.NewReader(""), &out, &errOut); code != exitUsage {
		t.Errorf("missing base url exit %d", code)
	}
	if code := run([]string{"version"}, nil, &out, &errOut); code != exitOK || !strings.Contains(out.String(), version) {
		t.Errorf("version: %d %q", code, out.String())
	}
}
