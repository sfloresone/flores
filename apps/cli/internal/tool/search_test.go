package tool

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

// tree builds a small project:
//
//	main.go  README.md  cmd/app/app.go  cmd/app/app_test.go
//	.git/config  node_modules/x/index.js  logo.png (binary)  big.txt (> 1 MB)
func tree(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{
		"main.go":                 "package main\n\nfunc main() { run() } // TODO start\n",
		"README.md":               "# demo\nTODO: write docs\r\n",
		"cmd/app/app.go":          "package app\n\nfunc Run() {}\n",
		"cmd/app/app_test.go":     "package app\n\n// todo: test Run\n",
		".git/config":             "TODO in git\n",
		"node_modules/x/index.js": "// TODO in deps\n",
		"logo.png":                "\x89PNG\x00TODO",
		"big.txt":                 strings.Repeat("TODO filler line\n", 70000),
	}
	for name, content := range files {
		writeFile(t, filepath.Join(dir, name), content, 0o644)
	}
	return dir
}

func TestSearchSchemas(t *testing.T) {
	schemaMatches(t, Grep(), grepInput{})
	schemaMatches(t, Glob(), globInput{})
}

func TestMatchGlob(t *testing.T) {
	cases := []struct {
		pattern, name string
		want          bool
	}{
		{"*.go", "main.go", true},
		{"*.go", "cmd/app/app.go", false},
		{"**/*.go", "main.go", true},
		{"**/*.go", "cmd/app/app.go", true},
		{"cmd/**", "cmd/app/app.go", true},
		{"cmd/**/*_test.go", "cmd/app/app_test.go", true},
		{"cmd/**/*_test.go", "cmd/app/app.go", false},
		{"**/app/*.go", "cmd/app/app.go", true},
		{"cmd/*.go", "cmd/app/app.go", false},
		{"**", "a/b/c", true},
		{"[mr]*.go", "main.go", true},
	}
	for _, c := range cases {
		if got := matchGlob(c.pattern, c.name); got != c.want {
			t.Errorf("matchGlob(%q, %q) = %v", c.pattern, c.name, got)
		}
	}
}

func TestGlob(t *testing.T) {
	dir := tree(t)
	r := runTool(t, Glob(), dir, globInput{Pattern: "**/*.go"})
	wantOK(t, r)
	if r.Content != "cmd/app/app.go\ncmd/app/app_test.go\nmain.go\n" {
		t.Fatalf("got %q", r.Content)
	}
	wantOK(t, runTool(t, Glob(), dir, globInput{Pattern: "*.go"}), "main.go\n")
	r = runTool(t, Glob(), dir, globInput{Pattern: "*.go", Path: "cmd/app"})
	if r.Content != "cmd/app/app.go\ncmd/app/app_test.go\n" {
		t.Fatalf("paths stay relative to the working directory: %q", r.Content)
	}
	wantOK(t, runTool(t, Glob(), dir, globInput{Pattern: "**/*.js"}), `no files match "**/*.js"`)
	wantOK(t, runTool(t, Glob(), dir, globInput{Pattern: "**", Limit: 2}), "more files; narrow the pattern or raise limit")
	wantErr(t, runTool(t, Glob(), dir, globInput{Pattern: "[x"}), "bad glob pattern")
	wantErr(t, runTool(t, Glob(), dir, globInput{Pattern: "*", Path: "nope"}), "path not found: nope")
}

func TestGrep(t *testing.T) {
	dir := tree(t)
	r := runTool(t, Grep(), dir, grepInput{Pattern: "TODO"})
	wantOK(t, r)
	// Sorted by path; .git, node_modules, the binary and the big file are skipped;
	// the CR of a CRLF line is not shown.
	if r.Content != "README.md:2: TODO: write docs\nmain.go:3: func main() { run() } // TODO start\n" {
		t.Fatalf("got %q", r.Content)
	}
	r = runTool(t, Grep(), dir, grepInput{Pattern: "todo", IgnoreCase: true, Include: "*_test.go"})
	if r.Content != "cmd/app/app_test.go:3: // todo: test Run\n" {
		t.Fatalf("got %q", r.Content)
	}
	wantOK(t, runTool(t, Grep(), dir, grepInput{Pattern: `func \w+\(`, Include: "cmd/**/*.go"}), "cmd/app/app.go:3: func Run() {}")
	wantOK(t, runTool(t, Grep(), dir, grepInput{Pattern: "package", Path: "main.go"}), "main.go:1: package main")
	wantOK(t, runTool(t, Grep(), dir, grepInput{Pattern: "nothing-here"}), `no matches for "nothing-here"`)
	wantErr(t, runTool(t, Grep(), dir, grepInput{Pattern: "("}), "bad regular expression")
}

func TestGrepLimitsAndLongLines(t *testing.T) {
	dir := t.TempDir()
	var b strings.Builder
	for i := range 150 {
		fmt.Fprintf(&b, "match %d\n", i)
	}
	b.WriteString("match " + strings.Repeat("y", 1000) + "\n")
	writeFile(t, filepath.Join(dir, "many.txt"), b.String(), 0o644)

	r := runTool(t, Grep(), dir, grepInput{Pattern: "match"})
	wantOK(t, r, "many.txt:100: match 99\n", "[51 more matches; narrow the pattern or raise limit]")
	r = runTool(t, Grep(), dir, grepInput{Pattern: "match y"})
	wantOK(t, r, "…")
	if len(r.Content) > grepMaxLineLen+50 {
		t.Fatalf("long line not cut: %d bytes", len(r.Content))
	}
}
