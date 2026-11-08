package tool

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"strconv"
	"strings"
)

type editInput struct {
	Path       string `json:"path"`
	OldString  string `json:"old_string"`
	NewString  string `json:"new_string"`
	ReplaceAll bool   `json:"replace_all,omitempty"`
}

// Edit returns the edit tool: replace an exact piece of text in a file.
func Edit() Tool {
	return Tool{
		Name: "edit",
		Description: "Replace old_string with new_string in a file. old_string must match the file exactly, " +
			"including indentation, and must appear once unless replace_all is true; include enough " +
			"surrounding lines to make it unique. Do not include the line numbers that read shows.",
		Schema: json.RawMessage(`{"type":"object","properties":{` +
			`"path":{"type":"string","description":"File path, relative to the working directory or absolute"},` +
			`"old_string":{"type":"string","description":"The exact text to replace"},` +
			`"new_string":{"type":"string","description":"The text to put in its place"},` +
			`"replace_all":{"type":"boolean","description":"Replace every occurrence (default false)"}},` +
			`"required":["path","old_string","new_string"]}`),
		Run: runEdit,
	}
}

const bom = "\xef\xbb\xbf" // UTF-8 byte order mark

func runEdit(ctx context.Context, env Env, input json.RawMessage) Result {
	var in editInput
	if bad, ok := decode(input, &in); !ok {
		return bad
	}
	if bad, ok := required("path", in.Path); !ok {
		return bad
	}
	switch {
	case in.OldString == "":
		return errorf("old_string is empty; to create a file or replace all of it, use write")
	case in.OldString == in.NewString:
		return errorf("old_string and new_string are the same; nothing to change")
	}
	path := env.abs(in.Path)
	raw, err := os.ReadFile(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return errorf("file not found: %s; use write to create it", in.Path)
	case err != nil:
		return errorf("cannot read %s: %v", in.Path, err)
	}

	// Work on the text without a BOM and, for a file that only uses CRLF,
	// with LF endings, so what the model copied from read matches; the
	// original form is restored when writing.
	text := string(raw)
	hasBOM := strings.HasPrefix(text, bom)
	text = strings.TrimPrefix(text, bom)
	crlf := strings.Count(text, "\r\n") > 0 && strings.Count(text, "\r\n") == strings.Count(text, "\n")
	old, repl := in.OldString, in.NewString
	if crlf {
		text = strings.ReplaceAll(text, "\r\n", "\n")
		old = strings.ReplaceAll(old, "\r\n", "\n")
		repl = strings.ReplaceAll(repl, "\r\n", "\n")
	}

	n := strings.Count(text, old)
	switch {
	case n == 0:
		hint := "re-read the file with read and copy the text exactly, including whitespace"
		if repl != "" && strings.Contains(text, repl) {
			hint = "new_string is already in the file, so the edit may have been made already"
		}
		return errorf("old_string was not found in %s; %s", in.Path, hint)
	case n > 1 && !in.ReplaceAll:
		return errorf("old_string appears %d times in %s; add surrounding lines to make it unique, or set replace_all to true", n, in.Path)
	}
	if in.ReplaceAll {
		text = strings.ReplaceAll(text, old, repl)
	} else {
		text = strings.Replace(text, old, repl, 1)
	}
	if crlf {
		text = strings.ReplaceAll(text, "\n", "\r\n")
	}
	if hasBOM {
		text = bom + text
	}
	if err := writeKeepingMode(path, []byte(text)); err != nil {
		return errorf("cannot write %s: %v", in.Path, err)
	}
	return Result{Content: "edited " + env.rel(path) + ": " + itoa(n) + " replacement(s)"}
}

func itoa(n int) string { return strconv.Itoa(n) }
