package tool

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path"
	"regexp"
	"strings"
)

const (
	grepDefaultLimit = 100
	grepMaxLimit     = 1000
	grepMaxFileSize  = 1 << 20 // bigger files are skipped
	grepMaxLineLen   = 300     // longer matching lines are cut
)

type grepInput struct {
	Pattern    string `json:"pattern"`
	Path       string `json:"path,omitempty"`
	Include    string `json:"include,omitempty"`
	IgnoreCase bool   `json:"ignore_case,omitempty"`
	Limit      int    `json:"limit,omitempty"`
}

// Grep returns the grep tool: search file contents with a regular expression.
func Grep() Tool {
	return Tool{
		Name: "grep",
		Description: "Search file contents with a regular expression (RE2 syntax, as in Go) under path " +
			"(a directory or one file; default: the working directory). Prints path:line: text for each " +
			"matching line, sorted by path. include filters files by a glob on the name (\"*.go\") or the " +
			"path (\"src/**/*.ts\"). Skips .git, node_modules, binary files and files over 1 MB.",
		Schema: json.RawMessage(`{"type":"object","properties":{` +
			`"pattern":{"type":"string","description":"Regular expression to search for"},` +
			`"path":{"type":"string","description":"Directory or file to search (default: the working directory)"},` +
			`"include":{"type":"string","description":"Only search files matching this glob, e.g. *.go"},` +
			`"ignore_case":{"type":"boolean","description":"Match without regard to case"},` +
			`"limit":{"type":"integer","description":"Maximum number of matching lines (default 100, max 1000)"}},` +
			`"required":["pattern"]}`),
		ReadOnly: true,
		Run:      runGrep,
	}
}

func runGrep(ctx context.Context, env Env, input json.RawMessage) Result {
	var in grepInput
	if bad, ok := decode(input, &in); !ok {
		return bad
	}
	if bad, ok := required("pattern", in.Pattern); !ok {
		return bad
	}
	expr := in.Pattern
	if in.IgnoreCase {
		expr = "(?i)" + expr
	}
	re, err := regexp.Compile(expr)
	if err != nil {
		return errorf("bad regular expression %q: %v", in.Pattern, err)
	}
	if in.Include != "" {
		if err := validGlob(in.Include); err != nil {
			return errorf("bad include pattern %q: %v", in.Include, err)
		}
	}
	root, bad, ok := searchRoot(env, in.Path)
	if !ok {
		return bad
	}
	limit := clampLimit(in.Limit, grepDefaultLimit, grepMaxLimit)

	var out strings.Builder
	shown, more := 0, 0
	err = walkFiles(ctx, root, func(abs, rel string, d fs.DirEntry) bool {
		if rel == "." { // root is a file
			rel = d.Name()
		}
		if !included(in.Include, rel) {
			return true
		}
		grepFile(abs, re, func(n int, line []byte) {
			if shown >= limit {
				more++
				return
			}
			text := string(line)
			if len(text) > grepMaxLineLen {
				text = cutUTF8(text, grepMaxLineLen) + "…"
			}
			fmt.Fprintf(&out, "%s:%d: %s\n", env.rel(abs), n, text)
			shown++
		})
		return true
	})
	if err != nil {
		return errorf("grep failed: %v", err)
	}
	if shown == 0 {
		return Result{Content: fmt.Sprintf("no matches for %q", in.Pattern)}
	}
	if more > 0 {
		fmt.Fprintf(&out, "[%d more matches; narrow the pattern or raise limit]\n", more)
	}
	return Result{Content: out.String()}
}

// included applies grep's include glob: a pattern without a slash matches
// the file name at any depth, one with a slash matches the whole path.
func included(include, rel string) bool {
	switch {
	case include == "":
		return true
	case strings.Contains(include, "/"):
		return matchGlob(strings.TrimPrefix(include, "./"), rel)
	default:
		ok, _ := path.Match(include, path.Base(rel))
		return ok
	}
}

// grepFile calls match for each line of a text file that re matches.
// Large and binary files are skipped.
func grepFile(name string, re *regexp.Regexp, match func(n int, line []byte)) {
	info, err := os.Stat(name)
	if err != nil || info.Size() > grepMaxFileSize {
		return
	}
	data, err := os.ReadFile(name)
	if err != nil || bytes.IndexByte(data[:min(len(data), binarySniff)], 0) >= 0 {
		return
	}
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 0, 64<<10), grepMaxFileSize+1)
	for n := 1; sc.Scan(); n++ {
		line := bytes.TrimSuffix(sc.Bytes(), []byte("\r"))
		if re.Match(line) {
			match(n, line)
		}
	}
}
