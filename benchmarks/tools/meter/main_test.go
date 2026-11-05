package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// syncBuffer is a log sink safe to read while the proxy writes to it.
type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

var testPricing = func() Pricing {
	p := Pricing{Models: map[string]Price{
		"claude-test":     {Input: 1, Output: 5, CacheRead: 0.1, CacheWrite: 1.25},
		"gpt-test":        {Input: 2, Output: 8, CacheRead: 0.5},
		"gpt-test-mini":   {Input: 0.2, Output: 0.8},
		"claude-snapshot": {Input: 3, Output: 15},
	}}
	p.index()
	return p
}()

// upstream records what the proxy forwarded and answers with a canned body.
type upstream struct {
	mu      sync.Mutex
	headers []http.Header
	paths   []string
	body    string
	ctype   string
}

func (u *upstream) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	u.mu.Lock()
	u.headers = append(u.headers, r.Header.Clone())
	u.paths = append(u.paths, r.URL.Path)
	u.mu.Unlock()
	io.Copy(io.Discard, r.Body)
	w.Header().Set("Content-Type", u.ctype)
	io.WriteString(w, u.body)
}

func setup(t *testing.T, up *upstream, mod func(*config)) (*httptest.Server, *meter, *syncBuffer) {
	t.Helper()
	us := httptest.NewServer(up)
	t.Cleanup(us.Close)
	log := &syncBuffer{}
	cfg := config{
		upstream:     map[string]string{"anthropic": us.URL, "openai": us.URL + "/v1"},
		keys:         map[string]string{"anthropic": "sk-ant-REAL", "openai": "sk-REAL"},
		gatewayToken: "cf-REAL",
		clientToken:  "client-token",
		pricing:      testPricing,
		budgetUSD:    1,
		log:          log,
	}
	if mod != nil {
		mod(&cfg)
	}
	m := newMeter(cfg)
	ps := httptest.NewServer(m)
	t.Cleanup(ps.Close)
	return ps, m, log
}

func post(t *testing.T, url, auth, body string) (*http.Response, string) {
	t.Helper()
	req, _ := http.NewRequest("POST", url, strings.NewReader(body))
	if strings.HasPrefix(auth, "Bearer ") {
		req.Header.Set("Authorization", auth)
	} else if auth != "" {
		req.Header.Set("x-api-key", auth)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp, string(b)
}

// lastRecord waits for the record: with Content-Length the client can finish
// reading before the handler has logged.
func lastRecord(t *testing.T, log *syncBuffer) Record {
	t.Helper()
	for i := 0; i < 200 && strings.TrimSpace(log.String()) == ""; i++ {
		time.Sleep(5 * time.Millisecond)
	}
	lines := strings.Split(strings.TrimSpace(log.String()), "\n")
	var r Record
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &r); err != nil {
		t.Fatal(err)
	}
	return r
}

func TestSwapsCredentialsAndRoutes(t *testing.T) {
	up := &upstream{ctype: "application/json", body: `{"usage":{"input_tokens":10,"output_tokens":2}}`}
	ps, _, _ := setup(t, up, nil)

	post(t, ps.URL+"/anthropic/v1/messages", "client-token", `{"model":"claude-test"}`)
	post(t, ps.URL+"/openai/chat/completions", "Bearer client-token", `{"model":"gpt-test"}`)

	if up.paths[0] != "/v1/messages" || up.paths[1] != "/v1/chat/completions" {
		t.Fatalf("paths = %v", up.paths)
	}
	a, o := up.headers[0], up.headers[1]
	if a.Get("x-api-key") != "sk-ant-REAL" || a.Get("Authorization") != "" {
		t.Errorf("anthropic auth not swapped: %v", a)
	}
	if o.Get("Authorization") != "Bearer sk-REAL" || o.Get("x-api-key") != "" {
		t.Errorf("openai auth not swapped: %v", o)
	}
	for _, h := range up.headers {
		if h.Get("cf-aig-authorization") != "Bearer cf-REAL" || h.Get("cf-aig-skip-cache") != "true" {
			t.Errorf("gateway headers missing: %v", h)
		}
		for k, vs := range h {
			for _, v := range vs {
				if strings.Contains(v, "client-token") {
					t.Errorf("client token leaked upstream in %s", k)
				}
			}
		}
	}
}

func TestRejectsWrongToken(t *testing.T) {
	up := &upstream{ctype: "application/json", body: `{}`}
	ps, _, _ := setup(t, up, nil)
	for _, auth := range []string{"", "nope", "Bearer nope"} {
		resp, _ := post(t, ps.URL+"/anthropic/v1/messages", auth, `{"model":"claude-test"}`)
		if resp.StatusCode != 401 {
			t.Errorf("auth %q: status %d", auth, resp.StatusCode)
		}
	}
	if len(up.paths) != 0 {
		t.Fatal("unauthenticated request reached upstream")
	}
}

func TestRefusesUnpricedModel(t *testing.T) {
	up := &upstream{ctype: "application/json", body: `{}`}
	ps, m, _ := setup(t, up, nil)
	resp, body := post(t, ps.URL+"/openai/chat/completions", "Bearer client-token", `{"model":"gpt-unknown"}`)
	if resp.StatusCode != 402 || !strings.Contains(body, "no price") {
		t.Fatalf("status %d body %s", resp.StatusCode, body)
	}
	if len(up.paths) != 0 || m.summary().Blocked != 1 {
		t.Fatal("unpriced request was forwarded")
	}
}

func TestBudgetAndRequestCap(t *testing.T) {
	// 100k output tokens of claude-test = 0.5 USD per request.
	up := &upstream{ctype: "application/json", body: `{"usage":{"input_tokens":0,"output_tokens":100000}}`}
	ps, m, _ := setup(t, up, nil)
	codes := []int{}
	for range 4 {
		resp, _ := post(t, ps.URL+"/anthropic/v1/messages", "client-token", `{"model":"claude-test"}`)
		codes = append(codes, resp.StatusCode)
	}
	if want := []int{200, 200, 402, 402}; !equal(codes, want) {
		t.Fatalf("codes %v want %v", codes, want)
	}
	if s := m.summary(); s.SpentUSD != 1.0 || s.Blocked != 2 || len(up.paths) != 2 {
		t.Fatalf("summary %+v, upstream calls %d", s, len(up.paths))
	}

	ps2, _, _ := setup(t, &upstream{ctype: "application/json", body: `{}`}, func(c *config) { c.maxRequests = 1 })
	r1, _ := post(t, ps2.URL+"/anthropic/v1/messages", "client-token", `{"model":"claude-test"}`)
	r2, b2 := post(t, ps2.URL+"/anthropic/v1/messages", "client-token", `{"model":"claude-test"}`)
	if r1.StatusCode != 200 || r2.StatusCode != 402 || !strings.Contains(b2, "request cap") {
		t.Fatalf("cap: %d %d %s", r1.StatusCode, r2.StatusCode, b2)
	}
}

func TestUsageFormats(t *testing.T) {
	cases := []struct {
		name, route, model, ctype, body string
		want                            Usage
		cost                            float64
	}{
		{
			name: "anthropic stream", route: "anthropic", model: "claude-test", ctype: "text/event-stream",
			body: "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"usage\":{\"input_tokens\":100,\"cache_read_input_tokens\":1000,\"cache_creation_input_tokens\":200,\"output_tokens\":1}}}\n\n" +
				"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"delta\":{\"text\":\"hi\"}}\n\n" +
				"event: message_delta\ndata: {\"type\":\"message_delta\",\"usage\":{\"output_tokens\":50}}\n\n",
			want: Usage{Input: 100, Output: 50, CacheRead: 1000, CacheWrite: 200},
			cost: (100*1 + 50*5 + 1000*0.1 + 200*1.25) / 1e6,
		},
		{
			name: "openai chat stream with include_usage", route: "openai", model: "gpt-test", ctype: "text/event-stream",
			body: "data: {\"choices\":[{\"delta\":{\"content\":\"hi\"}}],\"usage\":null}\n\n" +
				"data: {\"choices\":[],\"usage\":{\"prompt_tokens\":1200,\"completion_tokens\":30,\"prompt_tokens_details\":{\"cached_tokens\":1000}}}\n\ndata: [DONE]\n\n",
			want: Usage{Input: 200, Output: 30, CacheRead: 1000},
			cost: (200*2 + 30*8 + 1000*0.5) / 1e6,
		},
		{
			name: "openai responses stream", route: "openai", model: "openai/gpt-test-mini", ctype: "text/event-stream",
			body: "event: response.output_text.delta\ndata: {\"type\":\"response.output_text.delta\",\"delta\":\"hi\"}\n\n" +
				"event: response.completed\ndata: {\"type\":\"response.completed\",\"response\":{\"usage\":{\"input_tokens\":500,\"output_tokens\":40,\"input_tokens_details\":{\"cached_tokens\":0}}}}\n\n",
			want: Usage{Input: 500, Output: 40},
			cost: (500*0.2 + 40*0.8) / 1e6,
		},
		{
			name: "dated snapshot uses prefix price", route: "anthropic", model: "claude-snapshot-20261001", ctype: "application/json",
			body: `{"usage":{"input_tokens":1000,"output_tokens":100}}`,
			want: Usage{Input: 1000, Output: 100},
			cost: (1000*3 + 100*15) / 1e6,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			up := &upstream{ctype: c.ctype, body: c.body}
			ps, _, log := setup(t, up, nil)
			auth := "client-token"
			path := "/anthropic/v1/messages"
			if c.route == "openai" {
				auth, path = "Bearer client-token", "/openai/chat/completions"
			}
			_, got := post(t, ps.URL+path, auth, `{"model":"`+c.model+`","stream":true}`)
			if got != c.body {
				t.Fatal("body not passed through unchanged")
			}
			r := lastRecord(t, log)
			if r.Usage != c.want || r.UsageMissing {
				t.Fatalf("usage %+v want %+v (missing=%v)", r.Usage, c.want, r.UsageMissing)
			}
			if diff := r.CostUSD - c.cost; diff > 1e-12 || diff < -1e-12 {
				t.Fatalf("cost %v want %v", r.CostUSD, c.cost)
			}
		})
	}
}

func TestMissingUsageIsEstimated(t *testing.T) {
	up := &upstream{ctype: "text/event-stream", body: "data: {\"choices\":[{\"delta\":{\"content\":\"hello\"}}]}\n\ndata: [DONE]\n\n"}
	ps, _, log := setup(t, up, nil)
	post(t, ps.URL+"/openai/chat/completions", "Bearer client-token", `{"model":"gpt-test","stream":true}`)
	r := lastRecord(t, log)
	if !r.UsageMissing || !r.Estimated || r.CostUSD <= 0 {
		t.Fatalf("expected an estimated, non-zero cost: %+v", r)
	}
}

func TestLongPromptTier(t *testing.T) {
	p := Price{Input: 0.1, Output: 0.5, CacheRead: 0.01, LongAbove: 100_000,
		Long: &Price{Input: 0.5, Output: 2.5, CacheRead: 0.05}}
	short := p.Cost(Usage{Input: 50_000, CacheRead: 50_000, Output: 1000})
	long := p.Cost(Usage{Input: 50_000, CacheRead: 50_001, Output: 1000})
	if want := (50_000*0.1 + 50_000*0.01 + 1000*0.5) / 1e6; short != want {
		t.Errorf("short %v want %v", short, want)
	}
	if want := (50_000*0.5 + 50_001*0.05 + 1000*2.5) / 1e6; long != want {
		t.Errorf("long %v want %v (cache reads count toward the threshold)", long, want)
	}
}

func equal(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
