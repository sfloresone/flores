package tool

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
)

type writeInput struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

// Write returns the write tool: create or overwrite a whole file.
func Write() Tool {
	return Tool{
		Name: "write",
		Description: "Create a file, or overwrite it with new content. Missing parent directories are created. " +
			"To change part of an existing file, use edit instead.",
		Schema: json.RawMessage(`{"type":"object","properties":{` +
			`"path":{"type":"string","description":"File path, relative to the working directory or absolute"},` +
			`"content":{"type":"string","description":"The complete new content of the file"}},` +
			`"required":["path","content"]}`),
		Run: runWrite,
	}
}

func runWrite(ctx context.Context, env Env, input json.RawMessage) Result {
	var in writeInput
	if bad, ok := decode(input, &in); !ok {
		return bad
	}
	if bad, ok := required("path", in.Path); !ok {
		return bad
	}
	path := env.abs(in.Path)
	if info, err := os.Stat(path); err == nil && info.IsDir() {
		return errorf("%s is a directory", in.Path)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return errorf("cannot create the directory for %s: %v", in.Path, err)
	}
	if err := writeKeepingMode(path, []byte(in.Content)); err != nil {
		return errorf("cannot write %s: %v", in.Path, err)
	}
	return Result{Content: "wrote " + env.rel(path) + " (" + itoa(len(in.Content)) + " bytes)"}
}

// writeKeepingMode writes data, keeping the permissions of an existing file
// (an executable script stays executable).
func writeKeepingMode(path string, data []byte) error {
	mode := os.FileMode(0o644)
	if info, err := os.Stat(path); err == nil {
		mode = info.Mode().Perm()
	}
	return os.WriteFile(path, data, mode)
}
