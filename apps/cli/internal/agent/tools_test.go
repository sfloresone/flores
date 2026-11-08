package agent

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/sfloresone/flores/apps/cli/internal/provider"
	"github.com/sfloresone/flores/apps/cli/internal/tool"
)

func TestRealToolsAnswerInCallOrder(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("alpha\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	f := &fake{script: []reply{
		{calls: []provider.ToolCall{
			call("r", "read", `{"path":"a.txt"}`),
			call("g", "glob", `{"pattern":"*.txt"}`),
			call("b", "bash", `{"command":"echo flores-$((40+2))"}`),
		}},
		{text: "done"},
	}}
	cfg := Config{Tools: tool.Defaults(), Env: tool.Env{Dir: dir}}
	events, res := run(t, New(f, cfg), context.Background(), "look")
	if res.Status != StatusSuccess || res.ToolCalls != 3 || res.ToolErrors != 0 {
		t.Fatalf("result %+v", res)
	}
	var names []string
	for _, d := range f.requests[0].Tools {
		names = append(names, d.Name)
	}
	if !slices.Equal(names, []string{"bash", "edit", "glob", "grep", "read", "write"}) {
		t.Fatalf("tools sent: %v", names)
	}
	// read and glob run together, so both calls are announced first; bash
	// runs alone after them.
	want := []EventKind{EventStepStart, EventRequest, EventResponse,
		EventToolCall, EventToolCall, EventToolResult, EventToolResult, EventToolCall, EventToolResult,
		EventStepStart, EventRequest, EventResponse, EventDone}
	if got := kinds(events); !slices.Equal(got, want) {
		t.Fatalf("events %v, want %v", got, want)
	}
	msgs := f.requests[1].Messages
	parts := msgs[len(msgs)-1].Parts
	wantIDs := []string{"r", "g", "b"}
	wantContent := []string{"1\talpha\n", "a.txt\n", "flores-42\n"}
	for i, p := range parts {
		tr := p.(provider.ToolResult)
		if tr.CallID != wantIDs[i] || tr.Content != wantContent[i] {
			t.Fatalf("result %d: %+v", i, tr)
		}
	}
}

func TestCutReplyRunsNoTools(t *testing.T) {
	dir := t.TempDir()
	f := &fake{script: []reply{
		{calls: []provider.ToolCall{call("w", "write", `{"path":"x.txt","content":"half"}`)}, stop: provider.StopMaxTokens},
		{text: "ok"},
	}}
	_, res := run(t, New(f, Config{Tools: tool.Defaults(), Env: tool.Env{Dir: dir}}), context.Background(), "write")
	if res.Status != StatusSuccess || res.ToolErrors != 1 {
		t.Fatalf("result %+v", res)
	}
	if _, err := os.Stat(filepath.Join(dir, "x.txt")); err == nil {
		t.Fatal("a call from a cut reply was run")
	}
	msgs := f.requests[1].Messages
	tr := msgs[len(msgs)-1].Parts[0].(provider.ToolResult)
	if !tr.IsError || !strings.Contains(tr.Content, "issue the call again with complete arguments") {
		t.Fatalf("result %+v", tr)
	}
}
