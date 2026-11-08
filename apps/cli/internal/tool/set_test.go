package tool

import (
	"context"
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/sfloresone/flores/apps/cli/internal/provider"
)

func call(name, input string) provider.ToolCall {
	return provider.ToolCall{ID: "c_" + name, Name: name, Input: json.RawMessage(input)}
}

// echoTool decodes {"text": "..."} and answers it back.
func echoTool() Tool {
	return Tool{
		Name: "echo", Description: "echo", ReadOnly: true,
		Schema: json.RawMessage(`{"type":"object","properties":{"text":{"type":"string"}},"required":["text"]}`),
		Run: func(ctx context.Context, env Env, input json.RawMessage) Result {
			var in struct {
				Text string `json:"text"`
			}
			if bad, ok := decode(input, &in); !ok {
				return bad
			}
			if bad, ok := required("text", in.Text); !ok {
				return bad
			}
			return Result{Content: in.Text}
		},
	}
}

func TestDefsAreSortedAndStable(t *testing.T) {
	s := New(Tool{Name: "zeta"}, Tool{Name: "alpha"}, Tool{Name: "mid"})
	var names []string
	for _, d := range s.Defs() {
		names = append(names, d.Name)
	}
	if !slices.Equal(names, []string{"alpha", "mid", "zeta"}) {
		t.Fatalf("got %v", names)
	}
	var none *Set
	if none.Defs() != nil {
		t.Fatal("a nil set has no tools")
	}
}

func TestCommonErrors(t *testing.T) {
	s := New(echoTool(), Tool{Name: "boom", Run: func(context.Context, Env, json.RawMessage) Result { panic("kaput") }})
	cases := []struct {
		call provider.ToolCall
		want []string
	}{
		{call("nope", `{}`), []string{`unknown tool "nope"`, "available tools: boom, echo"}},
		{call("echo", `{"text": "hi`), []string{"invalid arguments for echo", `received: {"text": "hi`, `expected schema: {"type":"object"`}},
		{call("echo", `{"txt": "hi"}`), []string{"invalid arguments for echo", `unknown field "txt"`}},
		{call("echo", `{"text": 3}`), []string{"invalid arguments for echo", "cannot unmarshal number"}},
		{call("echo", ``), []string{"invalid arguments for echo", `missing required field "text"`, "received: (nothing)"}},
		{call("boom", `{}`), []string{"tool boom crashed: kaput"}},
	}
	for _, tc := range cases {
		r := s.Run(context.Background(), Env{}, tc.call)
		for _, w := range tc.want {
			if !r.IsError || !strings.Contains(r.Content, w) {
				t.Errorf("%s %s: got %+v, want %q", tc.call.Name, tc.call.Input, r, w)
			}
		}
	}
	if r := s.Run(context.Background(), Env{}, call("echo", `{"text":"hi"}`)); r.IsError || r.Content != "hi" {
		t.Fatalf("got %+v", r)
	}
}

func TestResultsAreCapped(t *testing.T) {
	big := Tool{Name: "big", Run: func(context.Context, Env, json.RawMessage) Result {
		return Result{Content: strings.Repeat("x", maxResult+10)}
	}}
	r := New(big).Run(context.Background(), Env{}, call("big", `{}`))
	if len(r.Content) > maxResult+100 || !strings.Contains(r.Content, "[result truncated") {
		t.Fatalf("got %d bytes", len(r.Content))
	}
}

func TestGroupsRunReadsTogetherAndWritesAlone(t *testing.T) {
	s := New(Tool{Name: "r", ReadOnly: true}, Tool{Name: "w"})
	calls := []provider.ToolCall{call("r", ""), call("r", ""), call("w", ""), call("r", ""), call("x", ""), call("r", ""), call("r", "")}
	var sizes []int
	for _, g := range s.Groups(calls) {
		sizes = append(sizes, len(g))
	}
	// r r | w | r | x (unknown: alone) | r r
	if !slices.Equal(sizes, []int{2, 1, 1, 1, 2}) {
		t.Fatalf("groups %v", sizes)
	}
}

func TestRunGroupIsConcurrentAndKeepsOrder(t *testing.T) {
	// Each call waits for the other one: they only finish if they run at
	// the same time.
	a, b := make(chan struct{}), make(chan struct{})
	wait := func(mine, other chan struct{}, text string) Tool {
		return Tool{Name: text, ReadOnly: true, Run: func(ctx context.Context, env Env, input json.RawMessage) Result {
			close(mine)
			select {
			case <-other:
			case <-time.After(2 * time.Second):
				return errorf("not concurrent")
			}
			return Result{Content: text}
		}}
	}
	s := New(wait(a, b, "first"), wait(b, a, "second"))
	done := s.RunGroup(context.Background(), Env{}, []provider.ToolCall{call("first", ""), call("second", "")})
	if done[0].Result.Content != "first" || done[1].Result.Content != "second" || done[0].Result.CallID != "c_first" {
		t.Fatalf("got %+v", done)
	}
}

// schemaMatches checks that a hand-written schema names exactly the json
// fields of the input struct, so the two cannot drift apart.
func schemaMatches(t *testing.T, tl Tool, input any) {
	t.Helper()
	var schema struct {
		Type       string                     `json:"type"`
		Properties map[string]json.RawMessage `json:"properties"`
		Required   []string                   `json:"required"`
	}
	if err := json.Unmarshal(tl.Schema, &schema); err != nil || schema.Type != "object" {
		t.Fatalf("%s: schema is not a JSON object schema: %v", tl.Name, err)
	}
	var fields []string
	typ := reflect.TypeOf(input)
	for i := range typ.NumField() {
		name, _, _ := strings.Cut(typ.Field(i).Tag.Get("json"), ",")
		fields = append(fields, name)
	}
	var props []string
	for p := range schema.Properties {
		props = append(props, p)
	}
	slices.Sort(fields)
	slices.Sort(props)
	if !slices.Equal(fields, props) {
		t.Errorf("%s: schema properties %v, struct fields %v", tl.Name, props, fields)
	}
	for _, r := range schema.Required {
		if !slices.Contains(fields, r) {
			t.Errorf("%s: required %q is not a field", tl.Name, r)
		}
	}
	if len(tl.Description) == 0 || len(tl.Description) > 1024 {
		t.Errorf("%s: description is %d bytes", tl.Name, len(tl.Description))
	}
}
