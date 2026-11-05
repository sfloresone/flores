// Command meter is the metering proxy that sits between the agents under test
// and the real model providers (normally through Cloudflare AI Gateway).
//
// It exists for three reasons:
//
//  1. Real API keys never reach the agent. Agents get a per-run token; the
//     proxy checks it and swaps it for the real provider key.
//  2. Tokens and cost are measured the same way for every agent, from the
//     provider's own usage fields, instead of trusting each agent's report.
//  3. Spend is capped. Before forwarding a request the proxy checks the run
//     budget and the request cap. Cloudflare can rate-limit requests per
//     minute but cannot cap euros, and layer B showed agents can loop for
//     thousands of requests.
//
// Routes: /anthropic/<path> → <upstream-anthropic>/<path>
//
//	/openai/<path>    → <upstream-openai>/<path>
//
// Agents use http://<proxy>/anthropic as ANTHROPIC_BASE_URL and
// http://<proxy>/openai as OPENAI_BASE_URL.
//
// It must only listen on loopback or on the Docker bridge, never on a public
// interface: it holds real credentials.
package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"
)

const maxRequestBytes = 32 << 20

type config struct {
	upstream     map[string]string // route → upstream base URL
	keys         map[string]string // route → real provider key
	gatewayToken string            // Cloudflare AI Gateway token (optional)
	clientToken  string            // what agents must send as their API key
	pricing      Pricing
	budgetUSD    float64
	maxRequests  int
	log          io.Writer
}

type meter struct {
	cfg    config
	client *http.Client

	mu       sync.Mutex
	seq      int
	spent    float64
	requests int
	blocked  int
	byModel  map[string]*ModelTotals
	logMu    sync.Mutex
}

// ModelTotals aggregates usage and cost per model.
type ModelTotals struct {
	Requests  int     `json:"requests"`
	Usage     Usage   `json:"usage"`
	CostUSD   float64 `json:"cost_usd"`
	Estimated int     `json:"estimated_requests"`
}

// Record is one line of the JSONL log.
type Record struct {
	Seq          int     `json:"seq"`
	Time         string  `json:"time"`
	Route        string  `json:"route"`
	Path         string  `json:"path"`
	Model        string  `json:"model"`
	Stream       bool    `json:"stream"`
	Status       int     `json:"status"`
	Blocked      string  `json:"blocked,omitempty"`
	DurationMs   float64 `json:"duration_ms"`
	TTFBMs       float64 `json:"ttfb_ms"`
	RequestBytes int     `json:"request_bytes"`
	ResponseByte int     `json:"response_bytes"`
	Usage        Usage   `json:"usage"`
	UsageMissing bool    `json:"usage_missing,omitempty"`
	CostUSD      float64 `json:"cost_usd"`
	Estimated    bool    `json:"cost_estimated,omitempty"`
	SpentUSD     float64 `json:"spent_usd"`
}

func newMeter(cfg config) *meter {
	return &meter{
		cfg:     cfg,
		client:  &http.Client{Transport: &http.Transport{Proxy: http.ProxyFromEnvironment, ForceAttemptHTTP2: true}},
		byModel: map[string]*ModelTotals{},
	}
}

func (m *meter) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/healthz":
		w.WriteHeader(http.StatusOK)
		return
	case "/_meter/summary":
		m.writeSummary(w)
		return
	}

	route, rest, ok := splitRoute(r.URL.Path)
	if !ok || m.cfg.upstream[route] == "" {
		http.Error(w, "flores-meter: unknown route (use /anthropic/... or /openai/...)", http.StatusNotFound)
		return
	}
	if !m.authorized(r) {
		writeProviderError(w, route, http.StatusUnauthorized, "authentication_error", "flores-meter: invalid client token")
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, maxRequestBytes+1))
	if err != nil || len(body) > maxRequestBytes {
		writeProviderError(w, route, http.StatusRequestEntityTooLarge, "invalid_request_error", "flores-meter: request too large")
		return
	}
	var head struct {
		Model  string `json:"model"`
		Stream bool   `json:"stream"`
	}
	_ = json.Unmarshal(body, &head)

	rec := Record{Route: route, Path: rest, Model: head.Model, Stream: head.Stream, RequestBytes: len(body)}
	start := time.Now()

	// Gate: pricing known, budget and request cap. Fail closed.
	price, priced := m.cfg.pricing.Lookup(head.Model)
	if reason := m.admit(head.Model, priced); reason != "" {
		rec.Blocked = reason
		rec.Status = http.StatusPaymentRequired
		writeProviderError(w, route, rec.Status, "invalid_request_error", "flores-meter: "+reason)
		m.finish(&rec, start, Usage{}, price, false)
		return
	}

	up, err := http.NewRequestWithContext(r.Context(), r.Method, joinURL(m.cfg.upstream[route], rest, r.URL.RawQuery), bytes.NewReader(body))
	if err != nil {
		writeProviderError(w, route, http.StatusBadGateway, "api_error", "flores-meter: "+err.Error())
		return
	}
	copyRequestHeaders(up.Header, r.Header)
	m.setAuth(up.Header, route)

	resp, err := m.client.Do(up)
	if err != nil {
		rec.Status = http.StatusBadGateway
		writeProviderError(w, route, rec.Status, "api_error", "flores-meter: upstream: "+err.Error())
		m.finish(&rec, start, Usage{}, price, false)
		return
	}
	defer resp.Body.Close()
	rec.Status = resp.StatusCode
	rec.TTFBMs = msSince(start)

	// Content-Length is dropped on purpose: the response is then chunked and
	// the client only sees its end after finish() has booked the cost, so a
	// sequential client can never slip a request past an exhausted budget.
	for k, vs := range resp.Header {
		if k == "Content-Length" {
			continue
		}
		for _, v := range vs {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)

	// Stream through to the agent while collecting usage on the side.
	scan := newUsageScanner(route)
	flusher, _ := w.(http.Flusher)
	buf := make([]byte, 32<<10)
	for {
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			rec.ResponseByte += n
			scan.Write(buf[:n])
			if _, werr := w.Write(buf[:n]); werr != nil {
				break
			}
			if flusher != nil {
				flusher.Flush()
			}
		}
		if rerr != nil {
			break
		}
	}
	usage, found := scan.Result()
	m.finish(&rec, start, usage, price, found || resp.StatusCode >= 400)
}

// admit reserves one request slot or returns why the request is refused.
func (m *meter) admit(model string, priced bool) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	switch {
	case !priced:
		m.blocked++
		return fmt.Sprintf("no price for model %q in the pricing table (refusing to spend unmetered)", model)
	case m.cfg.budgetUSD > 0 && m.spent >= m.cfg.budgetUSD:
		m.blocked++
		return fmt.Sprintf("budget exhausted (%.4f of %.4f USD)", m.spent, m.cfg.budgetUSD)
	case m.cfg.maxRequests > 0 && m.requests >= m.cfg.maxRequests:
		m.blocked++
		return fmt.Sprintf("request cap reached (%d)", m.cfg.maxRequests)
	}
	m.requests++
	return ""
}

// finish computes cost, updates totals and writes the log record.
// If the provider sent no usage, cost is estimated from bytes (≈4 bytes per
// token) so the budget still protects us.
func (m *meter) finish(rec *Record, start time.Time, usage Usage, price Price, usageFound bool) {
	rec.DurationMs = msSince(start)
	if rec.Blocked == "" && rec.Status < 400 && !usageFound {
		rec.UsageMissing = true
		rec.Estimated = true
		usage = Usage{Input: int64(rec.RequestBytes / 4), Output: int64(rec.ResponseByte / 4)}
	}
	rec.Usage = usage
	rec.CostUSD = price.Cost(usage)

	m.mu.Lock()
	m.seq++
	rec.Seq = m.seq
	m.spent += rec.CostUSD
	rec.SpentUSD = m.spent
	if rec.Blocked == "" {
		t := m.byModel[rec.Model]
		if t == nil {
			t = &ModelTotals{}
			m.byModel[rec.Model] = t
		}
		t.Requests++
		t.Usage.Add(usage)
		t.CostUSD += rec.CostUSD
		if rec.Estimated {
			t.Estimated++
		}
	}
	m.mu.Unlock()

	rec.Time = time.Now().UTC().Format(time.RFC3339Nano)
	if m.cfg.log != nil {
		line, _ := json.Marshal(rec)
		m.logMu.Lock()
		m.cfg.log.Write(append(line, '\n'))
		m.logMu.Unlock()
	}
}

// Summary is what /_meter/summary returns.
type Summary struct {
	Requests  int                     `json:"requests"`
	Blocked   int                     `json:"blocked"`
	SpentUSD  float64                 `json:"spent_usd"`
	BudgetUSD float64                 `json:"budget_usd"`
	ByModel   map[string]*ModelTotals `json:"by_model"`
}

func (m *meter) summary() Summary {
	m.mu.Lock()
	defer m.mu.Unlock()
	by := make(map[string]*ModelTotals, len(m.byModel))
	for k, v := range m.byModel {
		c := *v
		by[k] = &c
	}
	return Summary{Requests: m.requests, Blocked: m.blocked, SpentUSD: m.spent, BudgetUSD: m.cfg.budgetUSD, ByModel: by}
}

func (m *meter) writeSummary(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(m.summary())
}

func (m *meter) authorized(r *http.Request) bool {
	got := r.Header.Get("x-api-key")
	if got == "" {
		got = strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	}
	return got != "" && got == m.cfg.clientToken
}

// setAuth replaces the client token with the real credentials.
func (m *meter) setAuth(h http.Header, route string) {
	h.Del("x-api-key")
	h.Del("Authorization")
	if key := m.cfg.keys[route]; key != "" {
		if route == "anthropic" {
			h.Set("x-api-key", key)
		} else {
			h.Set("Authorization", "Bearer "+key)
		}
	}
	if m.cfg.gatewayToken != "" {
		h.Set("cf-aig-authorization", "Bearer "+m.cfg.gatewayToken)
	}
	// Repeated runs must hit the model, never a cached answer.
	h.Set("cf-aig-skip-cache", "true")
}

var hopHeaders = map[string]bool{
	"Connection": true, "Keep-Alive": true, "Proxy-Authenticate": true, "Proxy-Authorization": true,
	"Te": true, "Trailer": true, "Transfer-Encoding": true, "Upgrade": true, "Host": true,
	"Content-Length": true, "Accept-Encoding": true,
}

func copyRequestHeaders(dst, src http.Header) {
	for k, vs := range src {
		if hopHeaders[http.CanonicalHeaderKey(k)] || strings.HasPrefix(strings.ToLower(k), "cf-aig-") {
			continue
		}
		for _, v := range vs {
			dst.Add(k, v)
		}
	}
}

func splitRoute(path string) (route, rest string, ok bool) {
	p := strings.TrimPrefix(path, "/")
	route, rest, _ = strings.Cut(p, "/")
	return route, rest, route == "anthropic" || route == "openai"
}

func joinURL(base, rest, query string) string {
	u := strings.TrimRight(base, "/") + "/" + strings.TrimLeft(rest, "/")
	if query != "" {
		u += "?" + query
	}
	return u
}

func writeProviderError(w http.ResponseWriter, route string, status int, typ, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if route == "anthropic" {
		json.NewEncoder(w).Encode(map[string]any{"type": "error", "error": map[string]string{"type": typ, "message": msg}})
		return
	}
	json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"type": typ, "message": msg, "code": typ}})
}

func msSince(t time.Time) float64 { return float64(time.Since(t).Microseconds()) / 1000 }

func randomToken() string {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return "flores-meter-" + hex.EncodeToString(b)
}

func main() {
	listen := flag.String("listen", "127.0.0.1:0", "address to listen on: loopback or the Docker bridge, never public")
	anthropicURL := flag.String("upstream-anthropic", os.Getenv("METER_ANTHROPIC_URL"), "upstream equivalent of https://api.anthropic.com")
	openaiURL := flag.String("upstream-openai", os.Getenv("METER_OPENAI_URL"), "upstream equivalent of https://api.openai.com/v1")
	pricingPath := flag.String("pricing", "", "pricing table (JSON)")
	budget := flag.Float64("budget-usd", 0, "refuse requests once this much has been spent (0 = no budget; not allowed with real upstreams)")
	maxReq := flag.Int("max-requests", 0, "refuse requests after this many (0 = unlimited)")
	logPath := flag.String("log", "", "JSONL log of every request")
	token := flag.String("token", "", "client token agents must send (random if empty)")
	flag.Parse()

	fail := func(format string, a ...any) {
		fmt.Fprintf(os.Stderr, "meter: "+format+"\n", a...)
		os.Exit(2)
	}
	if *anthropicURL == "" && *openaiURL == "" {
		fail("at least one of --upstream-anthropic / --upstream-openai is required")
	}
	if *pricingPath == "" {
		fail("--pricing is required: the proxy refuses to forward unpriced requests")
	}
	pricing, err := LoadPricing(*pricingPath)
	if err != nil {
		fail("%v", err)
	}
	if *budget <= 0 && (os.Getenv("ANTHROPIC_API_KEY") != "" || os.Getenv("OPENAI_API_KEY") != "") {
		fail("--budget-usd is required when real provider keys are present")
	}
	host, _, err := net.SplitHostPort(*listen)
	if err != nil {
		fail("bad --listen: %v", err)
	}
	if ip := net.ParseIP(host); ip == nil || ip.IsUnspecified() {
		fail("refusing to listen on %q: use 127.0.0.1 or the Docker bridge IP", host)
	}

	cfg := config{
		upstream:     map[string]string{"anthropic": *anthropicURL, "openai": *openaiURL},
		keys:         map[string]string{"anthropic": os.Getenv("ANTHROPIC_API_KEY"), "openai": os.Getenv("OPENAI_API_KEY")},
		gatewayToken: os.Getenv("CF_AIG_TOKEN"),
		clientToken:  *token,
		pricing:      pricing,
		budgetUSD:    *budget,
		maxRequests:  *maxReq,
	}
	if cfg.clientToken == "" {
		cfg.clientToken = randomToken()
	}
	if *logPath != "" {
		f, err := os.OpenFile(*logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
		if err != nil {
			fail("%v", err)
		}
		defer f.Close()
		cfg.log = f
	}

	ln, err := net.Listen("tcp", *listen)
	if err != nil {
		fail("%v", err)
	}
	m := newMeter(cfg)
	srv := &http.Server{Handler: m, ReadHeaderTimeout: 10 * time.Second}
	ready, _ := json.Marshal(map[string]any{"meter": "ready", "url": "http://" + ln.Addr().String(), "token": cfg.clientToken})
	fmt.Println(string(ready))

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		srv.Shutdown(shutdown)
	}()
	if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
		fail("%v", err)
	}
	s, _ := json.Marshal(m.summary())
	fmt.Fprintln(os.Stderr, string(s))
}
