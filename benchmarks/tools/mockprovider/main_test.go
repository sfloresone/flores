package main

import (
	"bufio"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func testScenario(t *testing.T) *Scenario {
	t.Helper()
	sc := &Scenario{
		Name: "test",
		Steps: []Step{
			{ToolCalls: []ToolCall{{Name: "bash", Arguments: json.RawMessage(`{"command":"ls"}`)}}},
			{Text: "Done: ñandú ✓"},
			{Error: &StepError{Status: 429, Type: "rate_limit_error", Message: "slow down"}},
		},
		AfterLast: "error",
	}
	if err := sc.validate(); err != nil {
		t.Fatal(err)
	}
	return sc
}

func post(t *testing.T, srv *httptest.Server, path, body string) *http.Response {
	t.Helper()
	resp, err := http.Post(srv.URL+path, "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	return resp
}

// sseEvents parses an SSE body into (event, data) pairs.
func sseEvents(t *testing.T, r io.Reader) [][2]string {
	t.Helper()
	var out [][2]string
	var event string
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		line := sc.Text()
		switch {
		case strings.HasPrefix(line, "event: "):
			event = strings.TrimPrefix(line, "event: ")
		case strings.HasPrefix(line, "data: "):
			out = append(out, [2]string{event, strings.TrimPrefix(line, "data: ")})
			event = ""
		}
	}
	return out
}

func TestCountRuns(t *testing.T) {
	cases := []struct {
		in   []bool
		want int
	}{
		{nil, 0},
		{[]bool{false}, 0},
		{[]bool{false, true, false}, 1},
		{[]bool{false, true, true, false, true}, 2},
	}
	for _, c := range cases {
		if got := countRuns(c.in); got != c.want {
			t.Errorf("countRuns(%v) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestInspectSystemBytesAndSteps(t *testing.T) {
	chat := `{"model":"m","messages":[{"role":"system","content":"abcd"},{"role":"user","content":"hi"},
	  {"role":"assistant","tool_calls":[]},{"role":"tool","content":"x"},{"role":"assistant","content":"ok"},
	  {"role":"user","content":[{"type":"text","text":"again"}]}],"tools":[{},{}]}`
	info, err := inspect(ProtoChat, []byte(chat))
	if err != nil {
		t.Fatal(err)
	}
	if info.SystemBytes != 4 || info.Step != 2 || info.Tools != 2 || info.Messages != 6 {
		t.Errorf("chat: %+v", info)
	}

	anth := `{"model":"m","system":[{"type":"text","text":"abc"},{"type":"text","text":"de"}],
	  "messages":[{"role":"user","content":"hi"},{"role":"assistant","content":"x"},{"role":"user","content":"y"}]}`
	info, _ = inspect(ProtoAnthropic, []byte(anth))
	if info.SystemBytes != 5 || info.Step != 1 {
		t.Errorf("anthropic: %+v", info)
	}

	resp := `{"model":"m","instructions":"sys","input":[{"type":"message","role":"developer","content":"dev"},
	  {"type":"message","role":"user","content":"hi"},{"type":"reasoning"},{"type":"function_call","call_id":"a"},
	  {"type":"function_call","call_id":"b"},{"type":"function_call_output","call_id":"a"}]}`
	info, _ = inspect(ProtoResponses, []byte(resp))
	if info.SystemBytes != 6 || info.Step != 1 {
		t.Errorf("responses: %+v", info)
	}
}

func TestChatStreamToolCallThenText(t *testing.T) {
	srv := httptest.NewServer(newServer(testScenario(t), nil))
	defer srv.Close()

	resp := post(t, srv, "/v1/chat/completions",
		`{"model":"x","stream":true,"stream_options":{"include_usage":true},"messages":[{"role":"user","content":"go"}]}`)
	events := sseEvents(t, resp.Body)
	if last := events[len(events)-1][1]; last != "[DONE]" {
		t.Fatalf("last event %q", last)
	}
	joined := ""
	for _, e := range events {
		joined += e[1]
	}
	for _, want := range []string{`"tool_calls"`, `"name":"bash"`, `"finish_reason":"tool_calls"`, `"prompt_tokens"`} {
		if !strings.Contains(joined, want) {
			t.Errorf("stream missing %s", want)
		}
	}

	// Second request carries one assistant message -> step 1 (text, multibyte safe).
	resp = post(t, srv, "/v1/chat/completions",
		`{"model":"x","messages":[{"role":"user","content":"go"},{"role":"assistant","tool_calls":[]},{"role":"tool","content":"out"}]}`)
	var body struct {
		Choices []struct {
			Message      struct{ Content string } `json:"message"`
			FinishReason string                   `json:"finish_reason"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Choices[0].Message.Content != "Done: ñandú ✓" || body.Choices[0].FinishReason != "stop" {
		t.Errorf("unexpected: %+v", body)
	}
}

func TestStreamedTextReassembles(t *testing.T) {
	srv := httptest.NewServer(newServer(testScenario(t), nil))
	defer srv.Close()
	history := `[{"role":"user","content":"go"},{"role":"assistant","content":"x"},{"role":"user","content":"y"}]`

	// Anthropic
	resp := post(t, srv, "/v1/messages", `{"model":"x","stream":true,"messages":`+history+`}`)
	text := ""
	var sawStop bool
	for _, e := range sseEvents(t, resp.Body) {
		var ev struct {
			Type  string `json:"type"`
			Delta struct {
				Text       string `json:"text"`
				StopReason string `json:"stop_reason"`
			} `json:"delta"`
		}
		_ = json.Unmarshal([]byte(e[1]), &ev)
		if e[0] != ev.Type {
			t.Errorf("event name %q != type %q", e[0], ev.Type)
		}
		text += ev.Delta.Text
		sawStop = sawStop || ev.Type == "message_stop"
	}
	if text != "Done: ñandú ✓" || !sawStop {
		t.Errorf("anthropic text %q stop=%v", text, sawStop)
	}

	// Responses
	resp = post(t, srv, "/v1/responses", `{"model":"x","stream":true,"input":`+history+`}`)
	text = ""
	var completed bool
	seq := 0
	for _, e := range sseEvents(t, resp.Body) {
		var ev struct {
			Type           string `json:"type"`
			Delta          string `json:"delta"`
			SequenceNumber int    `json:"sequence_number"`
		}
		_ = json.Unmarshal([]byte(e[1]), &ev)
		if ev.SequenceNumber != seq {
			t.Errorf("sequence_number %d, want %d", ev.SequenceNumber, seq)
		}
		seq++
		if ev.Type == "response.output_text.delta" {
			text += ev.Delta
		}
		completed = completed || ev.Type == "response.completed"
	}
	if text != "Done: ñandú ✓" || !completed {
		t.Errorf("responses text %q completed=%v", text, completed)
	}
}

func TestErrorStepAndExhaustion(t *testing.T) {
	srv := httptest.NewServer(newServer(testScenario(t), nil))
	defer srv.Close()
	two := `[{"role":"user","content":"a"},{"role":"assistant","content":"b"},{"role":"user","content":"c"},
	  {"role":"assistant","content":"d"},{"role":"user","content":"e"}]`
	resp := post(t, srv, "/v1/messages", `{"model":"x","messages":`+two+`}`)
	if resp.StatusCode != 429 {
		t.Fatalf("status %d", resp.StatusCode)
	}
	var e struct {
		Type  string                `json:"type"`
		Error struct{ Type string } `json:"error"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&e)
	if e.Type != "error" || e.Error.Type != "rate_limit_error" {
		t.Errorf("anthropic error shape: %+v", e)
	}

	three := `[{"role":"assistant","content":"1"},{"role":"user","content":"x"},{"role":"assistant","content":"2"},
	  {"role":"user","content":"x"},{"role":"assistant","content":"3"},{"role":"user","content":"x"}]`
	resp = post(t, srv, "/v1/chat/completions", `{"model":"x","messages":`+three+`}`)
	if resp.StatusCode != 500 {
		t.Errorf("exhausted scenario status %d, want 500", resp.StatusCode)
	}
}

func TestRequestsEndpointAndReset(t *testing.T) {
	srv := httptest.NewServer(newServer(defaultScenario(), nil))
	defer srv.Close()
	post(t, srv, "/v1/chat/completions", `{"model":"x","messages":[{"role":"system","content":"You are X"},{"role":"user","content":"hello"}]}`)
	post(t, srv, "/v1/unknown", `{}`)

	get := func() []Record {
		resp, err := http.Get(srv.URL + "/_mock/requests")
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		var recs []Record
		_ = json.NewDecoder(resp.Body).Decode(&recs)
		return recs
	}
	recs := get()
	if len(recs) != 2 {
		t.Fatalf("got %d records", len(recs))
	}
	if recs[0].Protocol != ProtoChat || recs[0].SystemBytes != 9 || recs[0].Status != 200 {
		t.Errorf("record 0: %+v", recs[0])
	}
	if recs[1].Status != 404 {
		t.Errorf("record 1 status %d", recs[1].Status)
	}
	post(t, srv, "/_mock/reset", ``)
	if n := len(get()); n != 0 {
		t.Errorf("after reset: %d records", n)
	}
}

func TestSplitRunes(t *testing.T) {
	got := splitRunes("añb✓c", 2)
	want := []string{"añ", "b✓", "c"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("got %q", got)
	}
}

func faultScenario(t *testing.T) *Scenario {
	t.Helper()
	sc := &Scenario{
		Name: "faults",
		Steps: []Step{
			{Text: "recovered", FailFirst: &FailFirst{Times: 2, Status: 503, RetryAfterS: 1}},
			{Text: "abcdefghijklmnopqrstuvwxyz", Truncate: &Truncate{AfterChunks: 1, Times: 1}},
			{ToolCalls: []ToolCall{{Name: "bash", RawArguments: `{"command": "ec`}}},
			{Text: "ab", Repeat: 3},
		},
	}
	sc.ChunkRunes = 4
	if err := sc.validate(); err != nil {
		t.Fatal(err)
	}
	return sc
}

func history(assistantSteps int) string {
	msgs := []string{`{"role":"user","content":"go"}`}
	for i := 0; i < assistantSteps; i++ {
		msgs = append(msgs, `{"role":"assistant","content":"x"}`, `{"role":"user","content":"y"}`)
	}
	return "[" + strings.Join(msgs, ",") + "]"
}

func TestFailFirstThenSucceedWithRetryAfter(t *testing.T) {
	srv := httptest.NewServer(newServer(faultScenario(t), nil))
	defer srv.Close()
	body := `{"model":"x","messages":` + history(0) + `}`
	for i, want := range []int{503, 503, 200, 200} {
		resp := post(t, srv, "/v1/chat/completions", body)
		if resp.StatusCode != want {
			t.Fatalf("attempt %d: status %d, want %d", i+1, resp.StatusCode, want)
		}
		if want == 503 && resp.Header.Get("Retry-After") != "1" {
			t.Errorf("attempt %d: missing Retry-After", i+1)
		}
	}
	// Reset clears the attempt counters: the first request fails again.
	post(t, srv, "/_mock/reset", "")
	if resp := post(t, srv, "/v1/chat/completions", body); resp.StatusCode != 503 {
		t.Errorf("after reset: status %d", resp.StatusCode)
	}
}

func TestTruncateDropsStreamOnceAndIsRecorded(t *testing.T) {
	srv := httptest.NewServer(newServer(faultScenario(t), nil))
	defer srv.Close()
	body := `{"model":"x","stream":true,"messages":` + history(1) + `}`

	resp := post(t, srv, "/v1/chat/completions", body)
	raw, err := io.ReadAll(resp.Body)
	if err == nil && strings.Contains(string(raw), "[DONE]") {
		t.Fatalf("first attempt should be cut, got full stream")
	}
	if strings.Contains(string(raw), "efgh") {
		t.Errorf("truncated stream leaked later chunks: %q", raw)
	}

	resp = post(t, srv, "/v1/chat/completions", body)
	raw, _ = io.ReadAll(resp.Body)
	if !strings.Contains(string(raw), "[DONE]") {
		t.Errorf("retry should complete: %q", raw)
	}

	r, err := http.Get(srv.URL + "/_mock/requests")
	if err != nil {
		t.Fatal(err)
	}
	defer r.Body.Close()
	var recs []Record
	_ = json.NewDecoder(r.Body).Decode(&recs)
	if len(recs) != 2 || !recs[0].Truncated || recs[1].Truncated || recs[1].Attempt != 2 {
		t.Errorf("records: %+v", recs)
	}
}

func TestRawArgumentsAndRepeat(t *testing.T) {
	srv := httptest.NewServer(newServer(faultScenario(t), nil))
	defer srv.Close()
	resp := post(t, srv, "/v1/chat/completions", `{"model":"x","messages":`+history(2)+`}`)
	raw, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(raw), `"arguments":"{\"command\": \"ec"`) {
		t.Errorf("raw arguments not sent verbatim: %s", raw)
	}
	// Anthropic non-streaming must stay valid JSON even with malformed args.
	resp = post(t, srv, "/v1/messages", `{"model":"x","messages":`+history(2)+`}`)
	if !json.Valid(mustRead(t, resp)) {
		t.Error("anthropic body is not valid JSON")
	}
	resp = post(t, srv, "/v1/chat/completions", `{"model":"x","messages":`+history(3)+`}`)
	if !strings.Contains(string(mustRead(t, resp)), `"content":"ababab"`) {
		t.Error("repeat not applied")
	}
}

func TestAdditionalToolsAndToolResultsCounted(t *testing.T) {
	body := `{"model":"m","input":[
	  {"type":"additional_tools","role":"developer","tools":[{"type":"function","name":"read"},
	    {"type":"namespace","name":"functions","tools":[{"type":"function","name":"exec"},{"type":"function","name":"wait"}]}]},
	  {"type":"message","role":"user","content":"hi"},
	  {"type":"function_call","call_id":"a"},{"type":"function_call_output","call_id":"a","output":"x"}]}`
	info, err := inspect(ProtoResponses, []byte(body))
	if err != nil {
		t.Fatal(err)
	}
	if info.Tools != 3 || info.ToolResults != 1 || info.Step != 1 {
		t.Errorf("%+v", info)
	}
	anth := `{"model":"m","messages":[{"role":"user","content":"hi"},{"role":"assistant","content":[{"type":"tool_use"}]},
	  {"role":"user","content":[{"type":"tool_result","tool_use_id":"t"},{"type":"tool_result","tool_use_id":"u"}]}]}`
	info, _ = inspect(ProtoAnthropic, []byte(anth))
	if info.ToolResults != 2 {
		t.Errorf("anthropic tool results: %+v", info)
	}
}

func TestErrorCodeAndPerVendorShape(t *testing.T) {
	sc := &Scenario{Name: "credit", Steps: []Step{{Error: &StepError{
		Status: 429, Type: "insufficient_quota", Code: "insufficient_quota", Message: "no quota",
		Anthropic: &StepError{Status: 400, Type: "invalid_request_error", Message: "Your credit balance is too low"},
	}}}}
	if err := sc.validate(); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(newServer(sc, nil))
	defer srv.Close()

	resp := post(t, srv, "/v1/chat/completions", `{"model":"x","messages":[{"role":"user","content":"hi"}]}`)
	if resp.StatusCode != 429 || !strings.Contains(string(mustRead(t, resp)), `"code":"insufficient_quota"`) {
		t.Errorf("openai shape: %d", resp.StatusCode)
	}
	resp = post(t, srv, "/v1/messages", `{"model":"x","messages":[{"role":"user","content":"hi"}]}`)
	if body := string(mustRead(t, resp)); resp.StatusCode != 400 || !strings.Contains(body, "credit balance") {
		t.Errorf("anthropic shape: %d %s", resp.StatusCode, body)
	}
}

func TestOpenAIUsageIncludesCachedInput(t *testing.T) {
	sc := &Scenario{Name: "cache", Steps: []Step{{Text: "hi", Usage: &Usage{InputTokens: 10, OutputTokens: 2, CacheReadTokens: 30}}}}
	if err := sc.validate(); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(newServer(sc, nil))
	defer srv.Close()
	resp := post(t, srv, "/v1/chat/completions", `{"model":"x","messages":[{"role":"user","content":"hi"}]}`)
	var body struct {
		Usage struct {
			PromptTokens int `json:"prompt_tokens"`
			Details      struct {
				Cached int `json:"cached_tokens"`
			} `json:"prompt_tokens_details"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(mustRead(t, resp), &body); err != nil {
		t.Fatal(err)
	}
	if body.Usage.PromptTokens != 40 || body.Usage.Details.Cached != 30 {
		t.Errorf("usage %+v", body.Usage)
	}
}

func mustRead(t *testing.T, resp *http.Response) []byte {
	t.Helper()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
