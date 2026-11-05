package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
)

// Usage is normalised across providers. Input excludes cached tokens: OpenAI
// reports prompt/input tokens *including* cached ones, Anthropic excludes them.
type Usage struct {
	Input      int64 `json:"input"`
	Output     int64 `json:"output"`
	CacheRead  int64 `json:"cache_read"`
	CacheWrite int64 `json:"cache_write"`
}

func (u *Usage) Add(o Usage) {
	u.Input += o.Input
	u.Output += o.Output
	u.CacheRead += o.CacheRead
	u.CacheWrite += o.CacheWrite
}

// Price is USD per million tokens. Some models charge more for long prompts
// (Claude Haiku 5.5 above 100K, gpt-6.1-sol above 272K): when the prompt —
// uncached input + cache reads + cache writes — exceeds LongAbove, the whole
// request is billed at Long.
type Price struct {
	Input      float64 `json:"input"`
	Output     float64 `json:"output"`
	CacheRead  float64 `json:"cache_read"`
	CacheWrite float64 `json:"cache_write"`
	LongAbove  int64   `json:"long_above_prompt_tokens,omitempty"`
	Long       *Price  `json:"long,omitempty"`
}

func (p Price) Cost(u Usage) float64 {
	if p.Long != nil && u.Input+u.CacheRead+u.CacheWrite > p.LongAbove {
		p = *p.Long
	}
	return (float64(u.Input)*p.Input + float64(u.Output)*p.Output +
		float64(u.CacheRead)*p.CacheRead + float64(u.CacheWrite)*p.CacheWrite) / 1e6
}

// Pricing maps a model ID (or ID prefix) to its price.
type Pricing struct {
	Models map[string]Price `json:"models"`
	keys   []string         // longest first, for prefix matching
}

func LoadPricing(path string) (Pricing, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Pricing{}, err
	}
	var p Pricing
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var file struct {
		Source string           `json:"source"`
		Date   string           `json:"date"`
		Note   string           `json:"note"`
		Models map[string]Price `json:"models"`
	}
	if err := dec.Decode(&file); err != nil {
		return Pricing{}, fmt.Errorf("pricing %s: %w", path, err)
	}
	p.Models = file.Models
	p.index()
	return p, nil
}

func (p *Pricing) index() {
	p.keys = p.keys[:0]
	for k := range p.Models {
		p.keys = append(p.keys, k)
	}
	sort.Slice(p.keys, func(i, j int) bool { return len(p.keys[i]) > len(p.keys[j]) })
}

// Lookup matches the exact model ID, or the longest pricing key that is a
// prefix of it (dated snapshots such as "claude-x-20261001"). The provider
// prefix some agents add ("openai/…") is ignored.
func (p Pricing) Lookup(model string) (Price, bool) {
	if model == "" {
		return Price{}, false
	}
	if i := strings.LastIndex(model, "/"); i >= 0 {
		model = model[i+1:]
	}
	if pr, ok := p.Models[model]; ok {
		return pr, true
	}
	for _, k := range p.keys {
		if strings.HasPrefix(model, k) {
			return p.Models[k], true
		}
	}
	return Price{}, false
}

// usageScanner watches a response body (plain JSON or SSE) and keeps the
// largest value seen for each usage field. Streams report usage in several
// events (Anthropic: message_start + message_delta), and the values are
// cumulative, so max is the right merge.
type usageScanner struct {
	openai bool
	buf    bytes.Buffer
	usage  Usage
	found  bool
}

func newUsageScanner(route string) *usageScanner { return &usageScanner{openai: route == "openai"} }

func (s *usageScanner) Write(p []byte) { s.buf.Write(p) }

func (s *usageScanner) Result() (Usage, bool) {
	data := s.buf.Bytes()
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) > 0 && trimmed[0] == '{' {
		s.scanJSON(trimmed)
	} else {
		sc := bufio.NewScanner(bytes.NewReader(data))
		sc.Buffer(make([]byte, 1<<20), 64<<20)
		for sc.Scan() {
			line := sc.Bytes()
			if v, ok := bytes.CutPrefix(line, []byte("data:")); ok {
				s.scanJSON(bytes.TrimSpace(v))
			}
		}
	}
	return s.usage, s.found
}

func (s *usageScanner) scanJSON(b []byte) {
	var obj map[string]any
	if json.Unmarshal(b, &obj) != nil {
		return
	}
	for _, u := range []any{obj["usage"], dig(obj, "message", "usage"), dig(obj, "response", "usage")} {
		if m, ok := u.(map[string]any); ok {
			s.merge(m)
		}
	}
}

func (s *usageScanner) merge(m map[string]any) {
	in := first(m, "input_tokens", "prompt_tokens")
	out := first(m, "output_tokens", "completion_tokens")
	cacheRead := first(m, "cache_read_input_tokens")
	if v := num(dig(m, "prompt_tokens_details", "cached_tokens")); v > cacheRead {
		cacheRead = v
	}
	if v := num(dig(m, "input_tokens_details", "cached_tokens")); v > cacheRead {
		cacheRead = v
	}
	cacheWrite := first(m, "cache_creation_input_tokens")
	if s.openai {
		in -= cacheRead // OpenAI input includes the cached part
		if in < 0 {
			in = 0
		}
	}
	if in == 0 && out == 0 && cacheRead == 0 && cacheWrite == 0 {
		return
	}
	s.found = true
	s.usage.Input = max(s.usage.Input, in)
	s.usage.Output = max(s.usage.Output, out)
	s.usage.CacheRead = max(s.usage.CacheRead, cacheRead)
	s.usage.CacheWrite = max(s.usage.CacheWrite, cacheWrite)
}

func dig(m map[string]any, keys ...string) any {
	var cur any = m
	for _, k := range keys {
		mm, ok := cur.(map[string]any)
		if !ok {
			return nil
		}
		cur = mm[k]
	}
	return cur
}

func first(m map[string]any, keys ...string) int64 {
	for _, k := range keys {
		if v, ok := m[k]; ok && v != nil {
			return num(v)
		}
	}
	return 0
}

func num(v any) int64 {
	if f, ok := v.(float64); ok {
		return int64(f)
	}
	return 0
}
