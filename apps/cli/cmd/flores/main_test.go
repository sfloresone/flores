package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestVersionAndUsage(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := run([]string{"--version"}, &out, &errOut); code != exitOK {
		t.Fatalf("--version exit %d", code)
	}
	if !strings.HasPrefix(out.String(), "flores ") {
		t.Fatalf("--version printed %q", out.String())
	}
	if code := run(nil, &out, &errOut); code != exitUsage {
		t.Fatalf("no args exit %d, want %d", code, exitUsage)
	}
}
