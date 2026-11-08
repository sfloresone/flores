package tool

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
)

// Tool is one thing the model can do.
type Tool struct {
	Name        string
	Description string
	// Schema is the JSON Schema of the input, written by hand so the bytes
	// sent on every step never change (prompt caching).
	Schema json.RawMessage
	// ReadOnly tools change nothing, so several can run at the same time.
	ReadOnly bool
	// Run does the work. Input is the raw JSON the model sent; Run decodes it
	// with decode. Anything the model should read, including failures, goes
	// in the Result: Run never returns a Go error.
	Run func(ctx context.Context, env Env, input json.RawMessage) Result
}

// Result is what goes back to the model.
type Result struct {
	Content string
	IsError bool
	// badInput marks arguments that did not decode; Set then explains what
	// was received and what the schema expects.
	badInput bool
}

// Env is where tools work.
type Env struct {
	// Dir is the working directory; relative paths are resolved against it.
	Dir string
	// TempDir receives full outputs that were too long to send (bash).
	TempDir string
}

// abs resolves a path from the model against the working directory.
func (e Env) abs(path string) string {
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	return filepath.Join(e.Dir, path)
}

// rel shows a path relative to the working directory when it is inside it.
func (e Env) rel(path string) string {
	if r, err := filepath.Rel(e.Dir, path); err == nil && !startsWithDotDot(r) {
		return filepath.ToSlash(r)
	}
	return path
}

func startsWithDotDot(p string) bool {
	return p == ".." || len(p) > 2 && p[:3] == ".."+string(filepath.Separator)
}

func errorf(format string, args ...any) Result {
	return Result{Content: fmt.Sprintf(format, args...), IsError: true}
}

// decode reads the model's arguments into v. Unknown fields are an error:
// a typo in a field name should not silently drop the value. On failure it
// returns the Result to send back, and ok is false.
func decode(input json.RawMessage, v any) (bad Result, ok bool) {
	if len(bytes.TrimSpace(input)) == 0 {
		input = json.RawMessage("{}")
	}
	dec := json.NewDecoder(bytes.NewReader(input))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return Result{Content: err.Error(), IsError: true, badInput: true}, false
	}
	return Result{}, true
}

// required reports a missing required string field the same way as a
// decode failure.
func required(name, value string) (Result, bool) {
	if value == "" {
		return Result{Content: fmt.Sprintf("missing required field %q", name), IsError: true, badInput: true}, false
	}
	return Result{}, true
}
