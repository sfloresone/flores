package anthropic

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/sfloresone/flores/apps/cli/internal/provider"
)

// sse writes server-sent events the way the Messages API does.
func sse(w http.ResponseWriter, events ...string) {
	w.Header().Set("Content-Type", "text/event-stream")
	for _, e := range events {
		var typ struct{ Type string }
		_ = json.Unmarshal([]byte(e), &typ)
		fmt.Fprintf(w, "event: %s\ndata: %s\n\n", typ.Type, e)
	}
}

const (
	msgStart = `{"type":"message_start","message":{"usage":{"input_tokens":12,"output_tokens":1,"cache_read_input_tokens":5,"cache_creation_input_tokens":2}}}`
	msgStop  = `{"type":"message_stop"}`
)

func msgDelta(stop string, out int) string {
	return fmt.Sprintf(`{"type":"message_delta","delta":{"stop_reason":%q},"usage":{"output_tokens":%d}}`, stop, out)
}

func textDelta(i int, s string) string {
	b, _ := json.Marshal(s)
	return fmt.Sprintf(`{"type":"content_block_delta","index":%d,"delta":{"type":"text_delta","text":%s}}`, i, b)
}

func collect(t *testing.T, c *Client, req provider.Request) ([]provider.Event, error) {
	t.Helper()
	var events []provider.Event
	for ev, err := range c.Stream(context.Background(), req) {
		if err != nil {
			return events, err
		}
		events = append(events, ev)
	}
	return events, nil
}

func hello() provider.Request {
	return provider.Request{
		Model:    "claude-test",
		System:   "be brief",
		Messages: []provider.Message{{Role: provider.RoleUser, Parts: []provider.Part{provider.Text{Text: "hi"}}}},
	}
}

func TestTextStreamHeadersAndBody(t *testing.T) {
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages" || r.Header.Get("X-Api-Key") != "k" ||
			r.Header.Get("Anthropic-Version") != apiVersion || r.Header.Get("User-Agent") != "flores/test" {
			t.Errorf("bad request: %s %v", r.URL.Path, r.Header)
		}
		gotBody, _ = io.ReadAll(r.Body)
		sse(w, msgStart,
			`{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
			textDelta(0, "Hel"), `{"type":"ping"}`, textDelta(0, "lo"),
			`{"type":"content_block_stop","index":0}`,
			msgDelta("end_turn", 7), msgStop)
	}))
	defer srv.Close()

	c := New(Config{BaseURL: srv.URL + "/", APIKey: "k", UserAgent: "flores/test"})
	events, err := collect(t, c, hello())
	if err != nil {
		t.Fatal(err)
	}
	if events[0].Kind != provider.EventSent || events[0].Bytes != len(gotBody) {
		t.Fatalf("first event %+v, body %d bytes", events[0], len(gotBody))
	}
	var text strings.Builder
	for _, ev := range events {
		if ev.Kind == provider.EventTextDelta {
			text.WriteString(ev.Text)
		}
	}
	if text.String() != "Hello" {
		t.Fatalf("text %q", text.String())
	}
	done := events[len(events)-1]
	want := provider.Usage{InputTokens: 12, OutputTokens: 7, CacheReadTokens: 5, CacheWriteTokens: 2}
	if done.Kind != provider.EventDone || done.Stop != provider.StopEndTurn || done.Usage != want {
		t.Fatalf("done %+v", done)
	}

	var body map[string]any
	_ = json.Unmarshal(gotBody, &body)
	if body["stream"] != true || body["system"] != "be brief" || body["max_tokens"] != float64(defaultMaxTokens) {
		t.Fatalf("body %s", gotBody)
	}
	if _, ok := body["tools"]; ok {
		t.Fatalf("tools must be omitted when there are none: %s", gotBody)
	}
}

func TestToolUseIsAssembledFromFragments(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sse(w, msgStart,
			`{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"toolu_1","name":"bash","input":{}}}`,
			`{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\"comm"}}`,
			`{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"and\": \"l"}}`,
			`{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"s\"}"}}`,
			`{"type":"content_block_stop","index":0}`,
			`{"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"toolu_2","name":"noargs","input":{}}}`,
			`{"type":"content_block_stop","index":1}`,
			msgDelta("tool_use", 3), msgStop)
	}))
	defer srv.Close()

	events, err := collect(t, New(Config{BaseURL: srv.URL}), hello())
	if err != nil {
		t.Fatal(err)
	}
	var calls []*provider.ToolCall
	for _, ev := range events {
		if ev.Kind == provider.EventToolCall {
			calls = append(calls, ev.ToolCall)
		}
	}
	if len(calls) != 2 || calls[0].ID != "toolu_1" || string(calls[0].Input) != `{"command": "ls"}` ||
		calls[1].Name != "noargs" || string(calls[1].Input) != "{}" {
		t.Fatalf("calls %+v %+v", calls[0], calls[1])
	}
	if done := events[len(events)-1]; done.Stop != provider.StopToolUse || done.RawStop != "tool_use" {
		t.Fatalf("done %+v", done)
	}
}

func TestHTTPErrorsAreClassified(t *testing.T) {
	cases := []struct {
		status    int
		header    map[string]string
		kind      provider.ErrorKind
		wait      time.Duration
		retryable bool
	}{
		{401, nil, provider.ErrAuth, 0, false},
		{401, map[string]string{"X-Should-Retry": "true"}, provider.ErrAuth, 0, false}, // never, whatever it says
		{429, map[string]string{"Retry-After": "2"}, provider.ErrRateLimit, 2 * time.Second, true},
		{429, map[string]string{"Retry-After": "2", "Retry-After-Ms": "1500"}, provider.ErrRateLimit, 1500 * time.Millisecond, true},
		{409, nil, provider.ErrOverloaded, 0, true},
		{529, nil, provider.ErrOverloaded, 0, true},
		{503, map[string]string{"X-Should-Retry": "false"}, provider.ErrOverloaded, 0, false},
		{400, nil, provider.ErrBadRequest, 0, false},
		{400, map[string]string{"x-should-retry": "true"}, provider.ErrBadRequest, 0, true},
	}
	for _, tc := range cases {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			for k, v := range tc.header {
				w.Header().Set(k, v)
			}
			w.WriteHeader(tc.status)
			fmt.Fprint(w, `{"type":"error","error":{"type":"x","message":"nope"}}`)
		}))
		_, err := collect(t, New(Config{BaseURL: srv.URL}), hello())
		srv.Close()
		var pe *provider.Error
		if !errors.As(err, &pe) || pe.Kind != tc.kind || pe.Status != tc.status || pe.RetryAfter != tc.wait ||
			pe.Message != "nope" || pe.Retryable() != tc.retryable {
			t.Errorf("status %d %v: got %#v (retryable %v)", tc.status, tc.header, err, pe != nil && pe.Retryable())
		}
	}
}

func TestStreamWithoutMessageStopIsTruncated(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sse(w, msgStart, textDelta(0, "The quick"))
	}))
	defer srv.Close()
	events, err := collect(t, New(Config{BaseURL: srv.URL}), hello())
	var pe *provider.Error
	if !errors.As(err, &pe) || pe.Kind != provider.ErrTruncated || !pe.Retryable() {
		t.Fatalf("got %v", err)
	}
	if last := events[len(events)-1]; last.Text != "The quick" {
		t.Fatalf("text before the cut should still arrive, got %+v", last)
	}
}

func TestErrorEventMidStream(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sse(w, msgStart, `{"type":"error","error":{"type":"overloaded_error","message":"busy"}}`)
	}))
	defer srv.Close()
	_, err := collect(t, New(Config{BaseURL: srv.URL}), hello())
	var pe *provider.Error
	if !errors.As(err, &pe) || pe.Kind != provider.ErrOverloaded || pe.Message != "busy" {
		t.Fatalf("got %v", err)
	}
}

func TestOneMegabyteLine(t *testing.T) {
	big := strings.Repeat("x", 1<<20)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sse(w, msgStart, textDelta(0, big), msgDelta("end_turn", 1), msgStop)
	}))
	defer srv.Close()
	events, err := collect(t, New(Config{BaseURL: srv.URL}), hello())
	if err != nil {
		t.Fatal(err)
	}
	if events[1].Text != big {
		t.Fatalf("got %d bytes, want %d", len(events[1].Text), len(big))
	}
}

func TestEncodeMergesRolesSortsToolsAndToolChoice(t *testing.T) {
	req := provider.Request{
		Model: "m",
		Messages: []provider.Message{
			{Role: provider.RoleUser, Parts: []provider.Part{provider.Text{Text: "do it"}}},
			{Role: provider.RoleAssistant, Parts: []provider.Part{
				provider.Text{Text: ""}, // dropped: the API rejects empty text
				provider.ToolCall{ID: "t1", Name: "zeta", Input: json.RawMessage(`{bad`)},
			}},
			{Role: provider.RoleUser, Parts: []provider.Part{provider.ToolResult{CallID: "t1", Content: "boom", IsError: true}}},
			{Role: provider.RoleUser, Parts: []provider.Part{provider.Text{Text: "summarize"}}},
		},
		Tools: []provider.ToolDef{
			{Name: "zeta", InputSchema: json.RawMessage(`{"type":"object"}`)},
			{Name: "alpha", InputSchema: json.RawMessage(`{"type":"object"}`)},
		},
		ToolChoice: provider.ToolChoiceNone,
	}
	b, err := encode(req)
	if err != nil {
		t.Fatal(err)
	}
	var w wireRequest
	if err := json.Unmarshal(b, &w); err != nil {
		t.Fatal(err)
	}
	if len(w.Messages) != 3 || len(w.Messages[2].Content) != 2 {
		t.Fatalf("consecutive user messages must merge: %s", b)
	}
	if blocks := w.Messages[1].Content; len(blocks) != 1 || string(blocks[0].Input) != "{}" {
		t.Fatalf("invalid tool input must be replayed as {}: %s", b)
	}
	if w.Tools[0].Name != "alpha" || w.ToolChoice == nil || w.ToolChoice.Type != "none" {
		t.Fatalf("tools must be sorted and tool_choice none: %s", b)
	}
}

func TestCancelledContextIsNotAProviderError(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-release // never answers while the client waits
	}))
	defer srv.Close()
	defer close(release) // runs first, so Close does not wait on the handler
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	var err error
	for _, e := range New(Config{BaseURL: srv.URL}).Stream(ctx, hello()) {
		err = e
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("got %v", err)
	}
}

func TestNoCreditIsAQuotaError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, `{"type":"error","error":{"type":"invalid_request_error","message":"Your credit balance is too low to access the Anthropic API."}}`)
	}))
	defer srv.Close()
	_, err := collect(t, New(Config{BaseURL: srv.URL}), hello())
	var pe *provider.Error
	if !errors.As(err, &pe) || pe.Kind != provider.ErrQuota || pe.Retryable() {
		t.Fatalf("got %#v", err)
	}
}
