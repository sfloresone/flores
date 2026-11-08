package openai

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

// sse writes data lines the way Chat Completions does (no "event:" lines).
func sse(w http.ResponseWriter, chunks ...string) {
	w.Header().Set("Content-Type", "text/event-stream")
	for _, c := range chunks {
		fmt.Fprintf(w, "data: %s\n\n", c)
	}
}

const done = "[DONE]"

func delta(d string) string {
	return fmt.Sprintf(`{"id":"c1","object":"chat.completion.chunk","choices":[{"index":0,"delta":%s,"finish_reason":null}]}`, d)
}

func text(s string) string {
	b, _ := json.Marshal(s)
	return delta(fmt.Sprintf(`{"content":%s}`, b))
}

func finish(reason string) string {
	return fmt.Sprintf(`{"id":"c1","choices":[{"index":0,"delta":{},"finish_reason":%q}]}`, reason)
}

func usage(prompt, completion, cached int) string {
	return fmt.Sprintf(`{"id":"c1","choices":[],"usage":{"prompt_tokens":%d,"completion_tokens":%d,"total_tokens":%d,"prompt_tokens_details":{"cached_tokens":%d}}}`,
		prompt, completion, prompt+completion, cached)
}

func toolDelta(index int, id, name, args string) string {
	b, _ := json.Marshal(args)
	idx := fmt.Sprint(index)
	if index < 0 {
		idx = "null"
	}
	return delta(fmt.Sprintf(`{"tool_calls":[{"index":%s,"id":%q,"type":"function","function":{"name":%q,"arguments":%s}}]}`, idx, id, name, b))
}

func serve(t *testing.T, chunks ...string) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { sse(w, chunks...) }))
	t.Cleanup(srv.Close)
	return New(Config{BaseURL: srv.URL})
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
		Model:    "gpt-test",
		System:   "be brief",
		Messages: []provider.Message{{Role: provider.RoleUser, Parts: []provider.Part{provider.Text{Text: "hi"}}}},
	}
}

func toolCalls(events []provider.Event) []*provider.ToolCall {
	var calls []*provider.ToolCall
	for _, ev := range events {
		if ev.Kind == provider.EventToolCall {
			calls = append(calls, ev.ToolCall)
		}
	}
	return calls
}

func TestTextStreamHeadersAndBody(t *testing.T) {
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" || r.Header.Get("Authorization") != "Bearer k" ||
			r.Header.Get("User-Agent") != "flores/test" {
			t.Errorf("bad request: %s %v", r.URL.Path, r.Header)
		}
		gotBody, _ = io.ReadAll(r.Body)
		sse(w, delta(`{"role":"assistant","content":""}`), text("Hel"), text("lo"), finish("stop"), usage(12, 7, 5), done)
	}))
	defer srv.Close()

	events, err := collect(t, New(Config{BaseURL: srv.URL + "/v1/", APIKey: "k", UserAgent: "flores/test"}), hello())
	if err != nil {
		t.Fatal(err)
	}
	if events[0].Kind != provider.EventSent || events[0].Bytes != len(gotBody) {
		t.Fatalf("first event %+v, body %d bytes", events[0], len(gotBody))
	}
	var sb strings.Builder
	for _, ev := range events {
		if ev.Kind == provider.EventTextDelta {
			sb.WriteString(ev.Text)
		}
	}
	if sb.String() != "Hello" {
		t.Fatalf("text %q", sb.String())
	}
	last := events[len(events)-1]
	// prompt_tokens includes the cached input; provider.Usage keeps it apart.
	want := provider.Usage{InputTokens: 7, OutputTokens: 7, CacheReadTokens: 5}
	if last.Kind != provider.EventDone || last.Stop != provider.StopEndTurn || last.RawStop != "stop" || last.Usage != want {
		t.Fatalf("done %+v", last)
	}

	want2 := `{"model":"gpt-test","messages":[{"role":"system","content":"be brief"},{"role":"user","content":"hi"}],` +
		`"stream":true,"stream_options":{"include_usage":true}}`
	if string(gotBody) != want2 {
		t.Fatalf("body\n got %s\nwant %s", gotBody, want2)
	}
}

func TestEncodeHistoryToolsAndToolChoice(t *testing.T) {
	req := provider.Request{
		Model: "m",
		Messages: []provider.Message{
			{Role: provider.RoleUser, Parts: []provider.Part{provider.Text{Text: "do it"}}},
			{Role: provider.RoleAssistant, Parts: []provider.Part{
				provider.Text{Text: ""},
				provider.ToolCall{ID: "c1", Name: "zeta", Input: json.RawMessage(`{bad`)},
				provider.ToolCall{ID: "c2", Name: "alpha", Input: json.RawMessage(`{"a":1}`)},
			}},
			{Role: provider.RoleUser, Parts: []provider.Part{
				provider.ToolResult{CallID: "c1", Content: "error: boom", IsError: true},
				provider.ToolResult{CallID: "c2", Content: ""},
			}},
			{Role: provider.RoleUser, Parts: []provider.Part{provider.Text{Text: "summarize"}}},
			{Role: provider.RoleAssistant, Parts: []provider.Part{provider.Text{Text: ""}}}, // nothing to send
		},
		Tools: []provider.ToolDef{
			{Name: "zeta", InputSchema: json.RawMessage(`{"type":"object"}`)},
			{Name: "alpha", Description: "first", InputSchema: json.RawMessage(`{"type":"object"}`)},
		},
		ToolChoice: provider.ToolChoiceNone,
		MaxTokens:  100,
	}
	b, err := encode(req)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"model":"m","messages":[` +
		`{"role":"user","content":"do it"},` +
		`{"role":"assistant","content":null,"tool_calls":[` +
		`{"id":"c1","type":"function","function":{"name":"zeta","arguments":"{}"}},` +
		`{"id":"c2","type":"function","function":{"name":"alpha","arguments":"{\"a\":1}"}}]},` +
		`{"role":"tool","content":"error: boom","tool_call_id":"c1"},` +
		`{"role":"tool","content":"(no output)","tool_call_id":"c2"},` +
		`{"role":"user","content":"summarize"}],` +
		`"tools":[{"type":"function","function":{"name":"alpha","description":"first","parameters":{"type":"object"}}},` +
		`{"type":"function","function":{"name":"zeta","parameters":{"type":"object"}}}],` +
		`"tool_choice":"none","max_completion_tokens":100,"stream":true,"stream_options":{"include_usage":true}}`
	if string(b) != want {
		t.Fatalf("body\n got %s\nwant %s", b, want)
	}
}

func TestToolChoiceNeedsTools(t *testing.T) {
	req := hello()
	req.ToolChoice = provider.ToolChoiceNone
	b, _ := encode(req)
	if strings.Contains(string(b), "tool_choice") {
		t.Fatalf("tool_choice without tools is rejected by the API: %s", b)
	}
}

func TestToolCallsAreAssembledFromFragments(t *testing.T) {
	c := serve(t,
		toolDelta(0, "call_a", "bash", ""),
		toolDelta(0, "", "", `{"comm`),
		toolDelta(1, "call_b", "noargs", ""),
		toolDelta(0, "", "", `and": "ls"}`),
		finish("tool_calls"), usage(10, 3, 0), done)
	events, err := collect(t, c, hello())
	if err != nil {
		t.Fatal(err)
	}
	calls := toolCalls(events)
	if len(calls) != 2 || calls[0].ID != "call_a" || calls[0].Name != "bash" || string(calls[0].Input) != `{"command": "ls"}` ||
		calls[1].ID != "call_b" || string(calls[1].Input) != "{}" {
		t.Fatalf("calls %+v", calls)
	}
	if last := events[len(events)-1]; last.Stop != provider.StopToolUse {
		t.Fatalf("done %+v", last)
	}
}

func TestParallelCallsThatShareIndexZero(t *testing.T) {
	// Some compatible servers send every call with index 0; the id tells
	// them apart (Pi and fantasy merge them into one broken call).
	c := serve(t,
		toolDelta(0, "call_a", "read", `{"path":"a"}`),
		toolDelta(0, "call_b", "read", `{"path":"b"}`),
		finish("tool_calls"), done)
	events, err := collect(t, c, hello())
	if err != nil {
		t.Fatal(err)
	}
	calls := toolCalls(events)
	if len(calls) != 2 || string(calls[0].Input) != `{"path":"a"}` || string(calls[1].Input) != `{"path":"b"}` {
		t.Fatalf("calls %+v", calls)
	}
}

func TestDeltaWithoutIndexContinuesTheLastCall(t *testing.T) {
	c := serve(t, toolDelta(-1, "call_a", "bash", `{"x":`), toolDelta(-1, "", "", `1}`), finish("tool_calls"), done)
	events, err := collect(t, c, hello())
	if err != nil {
		t.Fatal(err)
	}
	if calls := toolCalls(events); len(calls) != 1 || string(calls[0].Input) != `{"x":1}` {
		t.Fatalf("calls %+v", calls)
	}
}

func TestStopWithToolCallsIsToolUse(t *testing.T) {
	// fantasy drops these calls when streaming; the calls are what matter.
	c := serve(t, toolDelta(0, "call_a", "bash", `{}`), finish("stop"), done)
	events, err := collect(t, c, hello())
	if err != nil {
		t.Fatal(err)
	}
	last := events[len(events)-1]
	if len(toolCalls(events)) != 1 || last.Stop != provider.StopToolUse || last.RawStop != "stop" {
		t.Fatalf("events %+v", events)
	}
}

func TestFinishReasons(t *testing.T) {
	for raw, want := range map[string]provider.StopReason{
		"stop": provider.StopEndTurn, "length": provider.StopMaxTokens,
		"content_filter": provider.StopOther, "tool_calls": provider.StopToolUse,
	} {
		events, err := collect(t, serve(t, text("x"), finish(raw), done), hello())
		if err != nil {
			t.Fatal(err)
		}
		if got := events[len(events)-1].Stop; got != want {
			t.Errorf("%s: got %v, want %v", raw, got, want)
		}
	}
}

func TestRefusalIsShownAsText(t *testing.T) {
	events, err := collect(t, serve(t, delta(`{"refusal":"I can't help with that."}`), finish("stop"), done), hello())
	if err != nil {
		t.Fatal(err)
	}
	if events[1].Kind != provider.EventTextDelta || events[1].Text != "I can't help with that." {
		t.Fatalf("events %+v", events)
	}
}

func TestLaterChunksWithoutUsageKeepIt(t *testing.T) {
	events, err := collect(t, serve(t, text("x"), finish("stop"), usage(3, 2, 0), `{"id":"c1","choices":[]}`, done), hello())
	if err != nil {
		t.Fatal(err)
	}
	if u := events[len(events)-1].Usage; u.InputTokens != 3 || u.OutputTokens != 2 {
		t.Fatalf("usage %+v", u)
	}
}

func TestEndWithoutDoneAfterFinishIsComplete(t *testing.T) {
	// fx discards this complete answer; only [DONE] is missing.
	events, err := collect(t, serve(t, text("all here"), finish("stop"), usage(3, 2, 0)), hello())
	if err != nil {
		t.Fatalf("got %v", err)
	}
	if last := events[len(events)-1]; last.Kind != provider.EventDone || last.Usage.OutputTokens != 2 {
		t.Fatalf("done %+v", last)
	}
}

func TestEndWithoutFinishIsTruncated(t *testing.T) {
	events, err := collect(t, serve(t, text("The quick")), hello())
	var pe *provider.Error
	if !errors.As(err, &pe) || pe.Kind != provider.ErrTruncated || !pe.Retryable() {
		t.Fatalf("got %v", err)
	}
	if last := events[len(events)-1]; last.Text != "The quick" {
		t.Fatalf("text before the cut should still arrive, got %+v", last)
	}
}

func TestErrorChunkMidStream(t *testing.T) {
	cases := []struct {
		chunk string
		kind  provider.ErrorKind
	}{
		{`{"error":{"message":"busy","type":"server_error","code":null}}`, provider.ErrOverloaded},
		{`{"error":{"message":"slow down","type":"requests","code":"rate_limit_exceeded"}}`, provider.ErrRateLimit},
		{`{"error":{"message":"no credit","type":"insufficient_quota","code":"insufficient_quota"}}`, provider.ErrQuota},
		{`{"error":{"message":"too long","type":"invalid_request_error","code":"context_length_exceeded"}}`, provider.ErrBadRequest},
		{`{"error":"model crashed"}`, provider.ErrOverloaded},
	}
	for _, tc := range cases {
		_, err := collect(t, serve(t, text("x"), tc.chunk), hello())
		var pe *provider.Error
		if !errors.As(err, &pe) || pe.Kind != tc.kind || pe.Message == "" {
			t.Errorf("%s: got %#v", tc.chunk, err)
		}
	}
}

func TestHTTPErrorsAreClassified(t *testing.T) {
	cases := []struct {
		status    int
		header    map[string]string
		body      string
		kind      provider.ErrorKind
		wait      time.Duration
		retryable bool
	}{
		{401, nil, `{"error":{"message":"nope","type":"invalid_request_error","code":"invalid_api_key"}}`, provider.ErrAuth, 0, false},
		{429, map[string]string{"Retry-After-Ms": "1500"}, `{"error":{"message":"nope","type":"requests","code":"rate_limit_exceeded"}}`, provider.ErrRateLimit, 1500 * time.Millisecond, true},
		// Same status, but no credit: retrying never helps (fantasy and Pi retry it).
		{429, map[string]string{"X-Should-Retry": "true"}, `{"error":{"message":"nope","type":"insufficient_quota","code":"insufficient_quota"}}`, provider.ErrQuota, 0, false},
		{402, nil, `{"error":{"message":"nope","code":402}}`, provider.ErrQuota, 0, false},
		{503, nil, `{"error":{"message":"nope","type":"server_error"}}`, provider.ErrOverloaded, 0, true},
		{404, nil, `{"error":"nope"}`, provider.ErrBadRequest, 0, false}, // Ollama's shape
	}
	for _, tc := range cases {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			for k, v := range tc.header {
				w.Header().Set(k, v)
			}
			w.WriteHeader(tc.status)
			fmt.Fprint(w, tc.body)
		}))
		_, err := collect(t, New(Config{BaseURL: srv.URL}), hello())
		srv.Close()
		var pe *provider.Error
		if !errors.As(err, &pe) || pe.Kind != tc.kind || pe.Status != tc.status || pe.RetryAfter != tc.wait ||
			pe.Message != "nope" || pe.Retryable() != tc.retryable {
			t.Errorf("status %d %s: got %#v (retryable %v)", tc.status, tc.body, err, pe != nil && pe.Retryable())
		}
	}
}

func TestConsumerCanStopDuringToolCalls(t *testing.T) {
	c := serve(t, toolDelta(0, "a", "x", "{}"), toolDelta(1, "b", "y", "{}"), finish("tool_calls"), done)
	n := 0
	for ev, err := range c.Stream(context.Background(), hello()) {
		if err != nil {
			t.Fatal(err)
		}
		if ev.Kind == provider.EventToolCall {
			n++
			break
		}
	}
	if n != 1 {
		t.Fatalf("got %d calls", n)
	}
}

func TestCancelledContextIsNotAProviderError(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-release
	}))
	defer srv.Close()
	defer close(release)
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
