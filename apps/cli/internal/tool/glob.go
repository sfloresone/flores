package tool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"
)

const (
	globDefaultLimit = 200
	globMaxLimit     = 1000
)

type globInput struct {
	Pattern string `json:"pattern"`
	Path    string `json:"path,omitempty"`
	Limit   int    `json:"limit,omitempty"`
}

// Glob returns the glob tool: find files by name.
func Glob() Tool {
	return Tool{
		Name: "glob",
		Description: "Find files whose path matches a glob pattern, relative to path (default: the working " +
			"directory). * and ? match within one path segment and ** matches any number of directories: " +
			"\"*.go\" only looks at the top level, \"**/*.go\" at every level. Skips .git and node_modules. " +
			"Returns paths sorted by name, at most limit (default 200).",
		Schema: json.RawMessage(`{"type":"object","properties":{` +
			`"pattern":{"type":"string","description":"Glob pattern, e.g. **/*_test.go"},` +
			`"path":{"type":"string","description":"Directory to search (default: the working directory)"},` +
			`"limit":{"type":"integer","description":"Maximum number of paths (default 200, max 1000)"}},` +
			`"required":["pattern"]}`),
		ReadOnly: true,
		Run:      runGlob,
	}
}

func runGlob(ctx context.Context, env Env, input json.RawMessage) Result {
	var in globInput
	if bad, ok := decode(input, &in); !ok {
		return bad
	}
	if bad, ok := required("pattern", in.Pattern); !ok {
		return bad
	}
	pattern := strings.TrimPrefix(in.Pattern, "./")
	if err := validGlob(pattern); err != nil {
		return errorf("bad glob pattern %q: %v", in.Pattern, err)
	}
	root, bad, ok := searchRoot(env, in.Path)
	if !ok {
		return bad
	}
	limit := clampLimit(in.Limit, globDefaultLimit, globMaxLimit)

	var found []string
	more := 0
	err := walkFiles(ctx, root, func(abs, rel string, _ fs.DirEntry) bool {
		if !matchGlob(pattern, rel) {
			return true
		}
		if len(found) < limit {
			found = append(found, env.rel(abs))
		} else {
			more++
		}
		return true
	})
	if err != nil {
		return errorf("glob failed: %v", err)
	}
	if len(found) == 0 {
		return Result{Content: fmt.Sprintf("no files match %q", in.Pattern)}
	}
	out := strings.Join(found, "\n") + "\n"
	if more > 0 {
		out += fmt.Sprintf("[%d more files; narrow the pattern or raise limit]\n", more)
	}
	return Result{Content: out}
}

// searchRoot resolves the directory (or, for grep, file) to search.
func searchRoot(env Env, p string) (root string, bad Result, ok bool) {
	root = env.Dir
	if p != "" {
		root = env.abs(p)
	}
	if _, err := os.Stat(root); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "", errorf("path not found: %s", p), false
		}
		return "", errorf("cannot search %s: %v", p, err), false
	}
	return root, Result{}, true
}

func clampLimit(n, def, most int) int {
	if n <= 0 {
		return def
	}
	return min(n, most)
}
