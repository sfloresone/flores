package tool

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"strings"
	"unicode/utf8"
)

const (
	readMaxLines   = 2000     // lines per call, the default and the maximum
	readMaxBytes   = 50 << 10 // bytes per call
	readMaxLineLen = 2000     // longer lines are cut
	binarySniff    = 8 << 10  // bytes checked for a NUL byte
)

type readInput struct {
	Path   string `json:"path"`
	Offset int    `json:"offset,omitempty"`
	Limit  int    `json:"limit,omitempty"`
}

// Read returns the read tool: a text file with line numbers.
func Read() Tool {
	return Tool{
		Name: "read",
		Description: "Read a text file. Each line is prefixed with its number and a tab. " +
			"Returns at most 2000 lines or 50 KB per call; use offset (1-based) and limit to read the rest. " +
			"Read a file before editing it.",
		Schema: json.RawMessage(`{"type":"object","properties":{` +
			`"path":{"type":"string","description":"File path, relative to the working directory or absolute"},` +
			`"offset":{"type":"integer","description":"First line to read, starting at 1"},` +
			`"limit":{"type":"integer","description":"Number of lines to read (max 2000)"}},` +
			`"required":["path"]}`),
		ReadOnly: true,
		Run:      runRead,
	}
}

func runRead(ctx context.Context, env Env, input json.RawMessage) Result {
	var in readInput
	if bad, ok := decode(input, &in); !ok {
		return bad
	}
	if bad, ok := required("path", in.Path); !ok {
		return bad
	}
	path := env.abs(in.Path)
	info, err := os.Stat(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return errorf("file not found: %s", in.Path)
	case err != nil:
		return errorf("cannot read %s: %v", in.Path, err)
	case info.IsDir():
		return errorf("%s is a directory; use glob to list files", in.Path)
	}
	f, err := os.Open(path)
	if err != nil {
		return errorf("cannot read %s: %v", in.Path, err)
	}
	defer f.Close()

	r := bufio.NewReaderSize(f, 64<<10)
	if head, _ := r.Peek(binarySniff); bytes.IndexByte(head, 0) >= 0 {
		return errorf("%s is a binary file (%d bytes); it cannot be shown as text", in.Path, info.Size())
	}
	first := max(in.Offset, 1)
	limit := readMaxLines
	if in.Limit > 0 {
		limit = min(in.Limit, readMaxLines)
	}

	var (
		out             strings.Builder
		total, shown    int
		lastShown       int
		stoppedForBytes bool
	)
	for {
		line, err := r.ReadString('\n')
		if line == "" && err != nil {
			if !errors.Is(err, io.EOF) {
				return errorf("cannot read %s: %v", in.Path, err)
			}
			break
		}
		total++
		if total < first || shown >= limit || stoppedForBytes {
			continue // keep counting lines for the summary
		}
		line = strings.TrimRight(line, "\r\n")
		if len(line) > readMaxLineLen {
			line = cutUTF8(line, readMaxLineLen) + "… (line truncated)"
		}
		entry := fmt.Sprintf("%d\t%s\n", total, line)
		if out.Len()+len(entry) > readMaxBytes && shown > 0 {
			stoppedForBytes = true
			continue
		}
		out.WriteString(entry)
		shown++
		lastShown = total
		if ctx.Err() != nil {
			return errorf("cancelled")
		}
	}
	switch {
	case total == 0:
		return Result{Content: "(empty file)"}
	case first > total:
		return errorf("offset %d is past the end of %s (%d lines)", first, in.Path, total)
	}
	if lastShown < total {
		fmt.Fprintf(&out, "[showing lines %d-%d of %d; continue with offset=%d]\n", first, lastShown, total, lastShown+1)
	}
	return Result{Content: out.String()}
}

// cutUTF8 cuts s to at most n bytes without splitting a character.
func cutUTF8(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}
