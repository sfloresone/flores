package tool

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/sfloresone/flores/apps/cli/internal/provider"
)

// maxResult is a safety cap on any result. Every tool bounds its own output
// well below it; this only stops a bug from flooding the context.
const maxResult = 64 << 10

// Set is the tools the model may call.
type Set struct {
	tools map[string]Tool
	names []string // sorted
}

// New returns a set with the given tools.
func New(tools ...Tool) *Set {
	s := &Set{tools: make(map[string]Tool, len(tools))}
	for _, t := range tools {
		s.tools[t.Name] = t
		s.names = append(s.names, t.Name)
	}
	slices.Sort(s.names)
	return s
}

// Defaults is flores' tool set (ADR 0006).
func Defaults() *Set {
	return New(Bash(), Read(), Edit(), Write(), Grep(), Glob())
}

// Defs describes the tools to the model, sorted by name so the request
// bytes never change between steps (prompt caching). A nil set has none.
func (s *Set) Defs() []provider.ToolDef {
	if s == nil {
		return nil
	}
	defs := make([]provider.ToolDef, 0, len(s.names))
	for _, n := range s.names {
		t := s.tools[n]
		defs = append(defs, provider.ToolDef{Name: t.Name, Description: t.Description, InputSchema: t.Schema})
	}
	return defs
}

// Run executes one call. Every failure comes back as an error result the
// model can read and act on; the turn goes on.
func (s *Set) Run(ctx context.Context, env Env, call provider.ToolCall) (res Result) {
	t, ok := s.lookup(call.Name)
	if !ok {
		return errorf("unknown tool %q; available tools: %s", call.Name, strings.Join(s.available(), ", "))
	}
	defer func() {
		if p := recover(); p != nil {
			res = errorf("tool %s crashed: %v", t.Name, p)
		}
	}()
	res = t.Run(ctx, env, call.Input)
	if res.badInput {
		res.Content = fmt.Sprintf("invalid arguments for %s: %s\nreceived: %s\nexpected schema: %s",
			t.Name, res.Content, cmp.Or(string(call.Input), "(nothing)"), t.Schema)
	}
	if len(res.Content) > maxResult {
		res.Content = res.Content[:maxResult] + fmt.Sprintf("\n[result truncated: %d bytes, limit %d]", len(res.Content), maxResult)
	}
	return res
}

func (s *Set) lookup(name string) (Tool, bool) {
	if s == nil {
		return Tool{}, false
	}
	t, ok := s.tools[name]
	return t, ok
}

func (s *Set) available() []string {
	if s == nil || len(s.names) == 0 {
		return []string{"(none)"}
	}
	return s.names
}

// Groups splits the calls of one step into the order they run in. A run of
// consecutive read-only calls forms one group and runs at the same time;
// every other call (bash, edit, write, or an unknown tool) is a group of
// its own, so nothing reads a file while another call writes it.
func (s *Set) Groups(calls []provider.ToolCall) [][]provider.ToolCall {
	var groups [][]provider.ToolCall
	readOnly := func(c provider.ToolCall) bool {
		t, ok := s.lookup(c.Name)
		return ok && t.ReadOnly
	}
	for i, c := range calls {
		n := len(groups)
		if readOnly(c) && n > 0 && readOnly(calls[i-1]) {
			groups[n-1] = append(groups[n-1], c)
			continue
		}
		groups = append(groups, []provider.ToolCall{c})
	}
	return groups
}

// Done is one finished call.
type Done struct {
	Result   provider.ToolResult
	Duration time.Duration
}

// RunGroup runs one group from Groups: a single call directly, several
// read-only calls at the same time. Results keep the order of the calls.
func (s *Set) RunGroup(ctx context.Context, env Env, group []provider.ToolCall) []Done {
	out := make([]Done, len(group))
	run := func(i int) {
		start := time.Now()
		r := s.Run(ctx, env, group[i])
		out[i] = Done{
			Result:   provider.ToolResult{CallID: group[i].ID, Content: r.Content, IsError: r.IsError},
			Duration: time.Since(start),
		}
	}
	if len(group) == 1 {
		run(0)
		return out
	}
	var wg sync.WaitGroup
	for i := range group {
		wg.Go(func() { run(i) })
	}
	wg.Wait()
	return out
}
