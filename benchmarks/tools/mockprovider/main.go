// Command mockprovider is a deterministic fake LLM provider for benchmarks.
//
// It speaks the OpenAI Chat Completions, OpenAI Responses and Anthropic
// Messages wire formats (streaming and non-streaming) and answers from a
// scripted scenario instead of a model. Benchmarks use it to measure harness
// overhead and behaviour without network noise or token cost.
//
// On startup it prints one JSON line with its address to stdout:
//
//	{"host":"127.0.0.1","port":54321,"url":"http://127.0.0.1:54321"}
//
// Control endpoints (not part of any provider API):
//
//	GET  /_mock/requests  records of requests received since the last reset
//	POST /_mock/reset     clear the recorded requests
//	GET  /healthz         liveness
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

const maxBodyBytes = 32 << 20

// Record is one received request, as exposed by /_mock/requests and the log.
type Record struct {
	Seq        int64   `json:"seq"`
	Time       string  `json:"time"`
	Method     string  `json:"method"`
	Path       string  `json:"path"`
	Status     int     `json:"status"`
	DurationMs float64 `json:"duration_ms"`
	UserAgent  string  `json:"user_agent,omitempty"`
	// Attempt is 1 for the first request of a step, 2 for its first retry...
	Attempt int `json:"attempt,omitempty"`
	// Truncated is true when the mock dropped the connection on purpose.
	Truncated bool `json:"truncated,omitempty"`
	RequestInfo
}

type server struct {
	scenario *Scenario
	seq      atomic.Int64

	mu       sync.Mutex
	records  []Record
	attempts map[int]int // requests seen per step since the last reset
	logFile  io.Writer
	// dumpDir, when set, receives every request body as <seq>.json so the
	// exact prompt and tool definitions each harness sends can be inspected.
	dumpDir string
}

func newServer(sc *Scenario, logFile io.Writer) *server {
	return &server{scenario: sc, logFile: logFile, attempts: map[int]int{}}
}

// attempt counts requests per conversation step. Retries re-send the same
// history, so they land on the same step; that is how fail_first and truncate
// know whether this is a first try or a retry. Benchmarks reset between runs.
func (s *server) attempt(step int) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.attempts[step]++
	return s.attempts[step]
}

func (s *server) record(rec Record) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.records = append(s.records, rec)
	if s.logFile != nil {
		line, _ := json.Marshal(rec)
		_, _ = s.logFile.Write(append(line, '\n'))
	}
}

// statusWriter remembers the status code for the request log.
type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) {
	if w.status == 0 {
		w.status = code
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func protocolFor(path string) (Protocol, bool) {
	switch {
	case strings.HasSuffix(path, "/chat/completions"):
		return ProtoChat, true
	case strings.HasSuffix(path, "/responses"):
		return ProtoResponses, true
	case strings.HasSuffix(path, "/messages"):
		return ProtoAnthropic, true
	}
	return "", false
}

func (s *server) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	start := time.Now()
	sw := &statusWriter{ResponseWriter: w}
	rec := Record{
		Seq:       s.seq.Add(1),
		Time:      start.UTC().Format(time.RFC3339Nano),
		Method:    req.Method,
		Path:      req.URL.Path,
		UserAgent: req.UserAgent(),
	}
	control := strings.HasPrefix(req.URL.Path, "/_mock/") || req.URL.Path == "/healthz"

	defer func() {
		// A deliberate connection drop: still record the request, then let
		// net/http abort the connection.
		if p := recover(); p != nil {
			if p == errTruncated {
				rec.Status = http.StatusOK
				rec.Truncated = true
				rec.DurationMs = float64(time.Since(start).Microseconds()) / 1000
				s.record(rec)
			}
			panic(p)
		}
	}()
	s.route(sw, req, &rec)

	if control {
		return
	}
	if sw.status == 0 {
		sw.status = http.StatusOK
	}
	rec.Status = sw.status
	rec.DurationMs = float64(time.Since(start).Microseconds()) / 1000
	s.record(rec)
}

func (s *server) route(w *statusWriter, req *http.Request, rec *Record) {
	path := req.URL.Path
	switch {
	case path == "/healthz":
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "scenario": s.scenario.Name})
		return
	case path == "/_mock/requests" && req.Method == http.MethodGet:
		s.mu.Lock()
		out := append([]Record{}, s.records...)
		s.mu.Unlock()
		writeJSON(w, http.StatusOK, out)
		return
	case path == "/_mock/reset" && req.Method == http.MethodPost:
		s.mu.Lock()
		s.records = nil
		s.attempts = map[int]int{}
		s.mu.Unlock()
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
		return
	case strings.HasSuffix(path, "/models") && req.Method == http.MethodGet:
		// Union of the OpenAI and Anthropic model-list shapes.
		writeJSON(w, http.StatusOK, map[string]any{
			"object": "list", "has_more": false,
			"data": []any{map[string]any{
				"id": s.scenario.Model, "object": "model", "type": "model",
				"display_name": s.scenario.Model, "created": 0, "created_at": "1970-01-01T00:00:00Z",
				"owned_by": "mock",
			}},
		})
		return
	}

	if req.Method != http.MethodPost {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": map[string]any{"message": "not found: " + path}})
		return
	}
	body, err := io.ReadAll(io.LimitReader(req.Body, maxBodyBytes))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": map[string]any{"message": err.Error()}})
		return
	}
	if s.dumpDir != "" {
		_ = os.WriteFile(filepath.Join(s.dumpDir, fmt.Sprintf("%06d.json", rec.Seq)), body, 0o644)
	}

	if strings.HasSuffix(path, "/messages/count_tokens") {
		rec.RequestBytes = len(body)
		writeJSON(w, http.StatusOK, map[string]any{"input_tokens": estimateTokens(len(body))})
		return
	}

	proto, ok := protocolFor(path)
	if !ok {
		rec.RequestBytes = len(body)
		writeJSON(w, http.StatusNotFound, map[string]any{"error": map[string]any{"message": "not found: " + path}})
		return
	}
	info, err := inspect(proto, body)
	rec.RequestInfo = info
	if err != nil {
		writeError(w, proto, &StepError{Status: 400, Type: "invalid_request_error", Message: "invalid JSON: " + err.Error()})
		return
	}

	step := s.scenario.pick(info.Step)
	rec.Attempt = s.attempt(info.Step)
	if step == nil {
		writeError(w, proto, &StepError{Status: 500, Type: "scenario_exhausted",
			Message: fmt.Sprintf("scenario %q has no step %d", s.scenario.Name, info.Step)})
		return
	}
	ttft := time.Duration(s.scenario.Latency.TTFTMs) * time.Millisecond
	if step.TTFTMs != nil {
		ttft = time.Duration(*step.TTFTMs) * time.Millisecond
	}
	if step.Error != nil {
		time.Sleep(ttft)
		writeError(w, proto, step.Error)
		return
	}
	if f := step.FailFirst; f != nil && rec.Attempt <= f.Times {
		time.Sleep(ttft)
		if f.RetryAfterS > 0 {
			w.Header().Set("Retry-After", strconv.Itoa(f.RetryAfterS))
		}
		msg := f.Message
		if msg == "" {
			msg = fmt.Sprintf("mock transient error (attempt %d of %d failing)", rec.Attempt, f.Times)
		}
		writeError(w, proto, &StepError{Status: f.Status, Type: f.Type, Message: msg})
		return
	}
	truncateAfter := -1
	if t := step.Truncate; t != nil && (t.Times == 0 || rec.Attempt <= t.Times) {
		truncateAfter = t.AfterChunks
	}

	model := info.Model
	if model == "" {
		model = s.scenario.Model
	}
	r := &reply{
		id:            strconv.FormatInt(rec.Seq, 10),
		model:         model,
		step:          step,
		usage:         usageFor(step, info),
		chunkRunes:    s.scenario.ChunkRunes,
		ttft:          ttft,
		chunkDelay:    time.Duration(s.scenario.Latency.ChunkDelayMs) * time.Millisecond,
		truncateAfter: truncateAfter,
	}
	switch proto {
	case ProtoChat:
		writeChat(w, r, info)
	case ProtoAnthropic:
		writeAnthropic(w, r, info)
	case ProtoResponses:
		writeResponses(w, r, info)
	}
}

// estimateTokens uses the common ~4 bytes/token heuristic. The mock only needs
// numbers that are deterministic and roughly proportional to request size.
func estimateTokens(bytes int) int {
	if bytes <= 0 {
		return 0
	}
	return (bytes + 3) / 4
}

func usageFor(step *Step, info RequestInfo) Usage {
	u := Usage{InputTokens: estimateTokens(info.RequestBytes)}
	out := len(step.Text)
	for _, tc := range step.ToolCalls {
		out += len(tc.Name) + len(tc.args())
	}
	u.OutputTokens = max(1, estimateTokens(out))
	if step.Usage != nil {
		if step.Usage.InputTokens > 0 {
			u.InputTokens = step.Usage.InputTokens
		}
		if step.Usage.OutputTokens > 0 {
			u.OutputTokens = step.Usage.OutputTokens
		}
		u.CacheReadTokens = step.Usage.CacheReadTokens
	}
	return u
}

func main() {
	host := flag.String("host", "127.0.0.1", "address to listen on")
	port := flag.Int("port", 0, "port to listen on (0 picks a free port)")
	scenarioPath := flag.String("scenario", "", "scenario JSON file (default: built-in hello)")
	logPath := flag.String("log", "", "append one JSON line per request to this file")
	dumpDir := flag.String("dump-bodies", "", "write every request body to this directory")
	flag.Parse()

	sc, err := loadScenario(*scenarioPath)
	if err != nil {
		log.Fatal(err)
	}
	var logFile io.Writer
	if *logPath != "" {
		f, err := os.OpenFile(*logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			log.Fatal(err)
		}
		defer f.Close()
		logFile = f
	}

	ln, err := net.Listen("tcp", net.JoinHostPort(*host, strconv.Itoa(*port)))
	if err != nil {
		log.Fatal(err)
	}
	addr := ln.Addr().(*net.TCPAddr)
	ready, _ := json.Marshal(map[string]any{
		"host": *host, "port": addr.Port, "url": fmt.Sprintf("http://%s:%d", *host, addr.Port),
		"scenario": sc.Name,
	})
	fmt.Println(string(ready))

	handler := newServer(sc, logFile)
	if *dumpDir != "" {
		if err := os.MkdirAll(*dumpDir, 0o755); err != nil {
			log.Fatal(err)
		}
		handler.dumpDir = *dumpDir
	}
	srv := &http.Server{Handler: handler, ReadHeaderTimeout: 10 * time.Second}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdown)
	}()
	if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}
