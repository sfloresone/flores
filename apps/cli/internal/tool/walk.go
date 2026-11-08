package tool

import (
	"context"
	"io/fs"
	"path"
	"path/filepath"
	"strings"
)

// skipDirs are never searched: version control data and installed
// dependencies are large and almost never what the model is looking for.
var skipDirs = map[string]bool{".git": true, ".hg": true, ".svn": true, "node_modules": true}

// walkFiles calls fn for every regular file under root, in lexical order,
// skipping skipDirs. rel is the path relative to root with forward slashes.
// fn returns false to stop the walk.
func walkFiles(ctx context.Context, root string, fn func(abs, rel string, d fs.DirEntry) bool) error {
	return filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if p == root {
				return err
			}
			return nil // an unreadable entry: skip it, keep searching
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if d.IsDir() {
			if p != root && skipDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		rel, _ := filepath.Rel(root, p)
		if !fn(p, filepath.ToSlash(rel), d) {
			return filepath.SkipAll
		}
		return nil
	})
}

// matchGlob reports whether a slash-separated path matches pattern, where
// "**" as a whole segment matches any number of segments (zero included)
// and every other segment follows path.Match (*, ?, [...]).
func matchGlob(pattern, name string) bool {
	return matchSegments(strings.Split(pattern, "/"), strings.Split(name, "/"))
}

func matchSegments(pat, name []string) bool {
	for len(pat) > 0 {
		if pat[0] == "**" {
			for len(pat) > 0 && pat[0] == "**" {
				pat = pat[1:]
			}
			if len(pat) == 0 {
				return true
			}
			for i := range name {
				if matchSegments(pat, name[i:]) {
					return true
				}
			}
			return false
		}
		if len(name) == 0 {
			return false
		}
		if ok, err := path.Match(pat[0], name[0]); err != nil || !ok {
			return false
		}
		pat, name = pat[1:], name[1:]
	}
	return len(name) == 0
}

// validGlob reports a malformed pattern before any walking.
func validGlob(pattern string) error {
	for _, seg := range strings.Split(pattern, "/") {
		if _, err := path.Match(seg, ""); err != nil {
			return err
		}
	}
	return nil
}
