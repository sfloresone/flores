package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// runTool runs one tool through a Set, as the agent does.
func runTool(t *testing.T, tl Tool, dir string, input any) Result {
	t.Helper()
	raw, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	return New(tl).Run(context.Background(), Env{Dir: dir, TempDir: t.TempDir()}, call(tl.Name, string(raw)))
}

func writeFile(t *testing.T, path, content string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func wantOK(t *testing.T, r Result, contains ...string) {
	t.Helper()
	if r.IsError {
		t.Fatalf("unexpected error: %s", r.Content)
	}
	for _, c := range contains {
		if !strings.Contains(r.Content, c) {
			t.Fatalf("result %q does not contain %q", r.Content, c)
		}
	}
}

func wantErr(t *testing.T, r Result, contains string) {
	t.Helper()
	if !r.IsError || !strings.Contains(r.Content, contains) {
		t.Fatalf("got %+v, want an error containing %q", r, contains)
	}
}

func TestSchemasMatchInputs(t *testing.T) {
	schemaMatches(t, Read(), readInput{})
	schemaMatches(t, Write(), writeInput{})
	schemaMatches(t, Edit(), editInput{})
}

func TestReadNumbersLinesAndPages(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a.txt"), "one\ntwo\r\nthree\nfour", 0o644)

	r := runTool(t, Read(), dir, readInput{Path: "a.txt"})
	wantOK(t, r)
	if r.Content != "1\tone\n2\ttwo\n3\tthree\n4\tfour\n" {
		t.Fatalf("got %q", r.Content)
	}
	r = runTool(t, Read(), dir, readInput{Path: "a.txt", Offset: 2, Limit: 2})
	wantOK(t, r, "2\ttwo\n3\tthree\n", "[showing lines 2-3 of 4; continue with offset=4]")
	wantErr(t, runTool(t, Read(), dir, readInput{Path: "a.txt", Offset: 9}), "offset 9 is past the end")

	abs := filepath.Join(dir, "a.txt")
	wantOK(t, runTool(t, Read(), "/", readInput{Path: abs}), "1\tone")
}

func TestReadLimits(t *testing.T) {
	dir := t.TempDir()
	var lines []string
	for i := range 2500 {
		lines = append(lines, fmt.Sprintf("line %d", i+1))
	}
	writeFile(t, filepath.Join(dir, "long.txt"), strings.Join(lines, "\n"), 0o644)
	r := runTool(t, Read(), dir, readInput{Path: "long.txt"})
	wantOK(t, r, "2000\tline 2000\n", "[showing lines 1-2000 of 2500; continue with offset=2001]")

	writeFile(t, filepath.Join(dir, "wide.txt"), strings.Repeat("é", 3000), 0o644)
	r = runTool(t, Read(), dir, readInput{Path: "wide.txt"})
	wantOK(t, r, "… (line truncated)")
	if len(r.Content) > readMaxLineLen+100 {
		t.Fatalf("line not cut: %d bytes", len(r.Content))
	}

	// 200 lines of 1 KB: the 50 KB budget stops the read before the line limit.
	writeFile(t, filepath.Join(dir, "heavy.txt"), strings.Repeat(strings.Repeat("x", 1000)+"\n", 200), 0o644)
	r = runTool(t, Read(), dir, readInput{Path: "heavy.txt"})
	wantOK(t, r, "of 200; continue with offset=")
	if len(r.Content) > readMaxBytes+200 {
		t.Fatalf("byte budget not kept: %d", len(r.Content))
	}
}

func TestReadRefusals(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "bin"), "ELF\x00\x01\x02", 0o644)
	writeFile(t, filepath.Join(dir, "empty"), "", 0o644)
	wantErr(t, runTool(t, Read(), dir, readInput{Path: "bin"}), "binary file")
	wantErr(t, runTool(t, Read(), dir, readInput{Path: "."}), "is a directory; use glob")
	wantErr(t, runTool(t, Read(), dir, readInput{Path: "missing"}), "file not found: missing")
	wantErr(t, runTool(t, Read(), dir, map[string]any{}), `missing required field "path"`)
	wantOK(t, runTool(t, Read(), dir, readInput{Path: "empty"}), "(empty file)")
}

func TestWriteCreatesAndKeepsMode(t *testing.T) {
	dir := t.TempDir()
	r := runTool(t, Write(), dir, writeInput{Path: "deep/new/file.txt", Content: "hello"})
	wantOK(t, r, "wrote deep/new/file.txt (5 bytes)")
	if got := readFile(t, filepath.Join(dir, "deep/new/file.txt")); got != "hello" {
		t.Fatalf("got %q", got)
	}

	script := filepath.Join(dir, "run.sh")
	writeFile(t, script, "old", 0o755)
	wantOK(t, runTool(t, Write(), dir, writeInput{Path: "run.sh", Content: "#!/bin/sh\n"}))
	if info, _ := os.Stat(script); info.Mode().Perm() != 0o755 {
		t.Fatalf("mode %v", info.Mode().Perm())
	}
	wantErr(t, runTool(t, Write(), dir, writeInput{Path: "deep", Content: "x"}), "is a directory")
	wantOK(t, runTool(t, Write(), dir, writeInput{Path: "blank.txt"}), "(0 bytes)")
}

func TestEditReplacesExactText(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "main.go")
	writeFile(t, path, "a := 1\nb := 1\na := 1\n", 0o600)

	wantErr(t, runTool(t, Edit(), dir, editInput{Path: "main.go", OldString: "a := 1", NewString: "a := 2"}),
		"appears 2 times in main.go")
	wantOK(t, runTool(t, Edit(), dir, editInput{Path: "main.go", OldString: "b := 1", NewString: "b := 2"}),
		"edited main.go: 1 replacement(s)")
	wantOK(t, runTool(t, Edit(), dir, editInput{Path: "main.go", OldString: "a := 1", NewString: "a := 3", ReplaceAll: true}),
		"2 replacement(s)")
	if got := readFile(t, path); got != "a := 3\nb := 2\na := 3\n" {
		t.Fatalf("got %q", got)
	}
	if info, _ := os.Stat(path); info.Mode().Perm() != 0o600 {
		t.Fatalf("mode %v", info.Mode().Perm())
	}
}

func TestEditRefusals(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "f.txt"), "done\n", 0o644)
	cases := []struct {
		in   editInput
		want string
	}{
		{editInput{Path: "f.txt", OldString: "todo", NewString: "x"}, "re-read the file with read"},
		{editInput{Path: "f.txt", OldString: "todo", NewString: "done"}, "may have been made already"},
		{editInput{Path: "f.txt", OldString: "", NewString: "x"}, "use write"},
		{editInput{Path: "f.txt", OldString: "done", NewString: "done"}, "are the same"},
		{editInput{Path: "nope.txt", OldString: "a", NewString: "b"}, "file not found: nope.txt; use write"},
	}
	for _, tc := range cases {
		wantErr(t, runTool(t, Edit(), dir, tc.in), tc.want)
	}
	if got := readFile(t, filepath.Join(dir, "f.txt")); got != "done\n" {
		t.Fatalf("a refused edit changed the file: %q", got)
	}
}

func TestEditKeepsCRLFAndBOM(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "win.txt")
	writeFile(t, path, bom+"first\r\nsecond\r\nthird\r\n", 0o644)
	// The model copies what read showed: LF endings, no BOM.
	wantOK(t, runTool(t, Edit(), dir, editInput{Path: "win.txt", OldString: "first\nsecond", NewString: "1st\n2nd"}))
	if got := readFile(t, path); got != bom+"1st\r\n2nd\r\nthird\r\n" {
		t.Fatalf("got %q", got)
	}
}
