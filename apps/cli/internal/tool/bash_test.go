package tool

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBashSchema(t *testing.T) { schemaMatches(t, Bash(), bashInput{}) }

func TestBashRunsInTheWorkingDirectory(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "here.txt"), "", 0o644)
	r := runTool(t, Bash(), dir, bashInput{Command: "echo flores-$((40+2)); ls; echo $AI_AGENT"})
	wantOK(t, r, "flores-42\n", "here.txt\n", "flores\n")
	if strings.Contains(r.Content, "exit code") {
		t.Fatalf("a success should not report an exit code: %q", r.Content)
	}
}

func TestBashMergesStderrAndReportsExitCode(t *testing.T) {
	r := runTool(t, Bash(), t.TempDir(), bashInput{Command: "echo out; echo err >&2; exit 3"})
	wantErr(t, r, "[exit code 3]")
	if !strings.Contains(r.Content, "out\nerr\n") {
		t.Fatalf("got %q", r.Content)
	}
	wantOK(t, runTool(t, Bash(), t.TempDir(), bashInput{Command: "true"}), "(no output)")
	wantErr(t, runTool(t, Bash(), t.TempDir(), bashInput{Command: "kill -9 $$"}), "signal: killed")
}

func TestBashStdinIsEmpty(t *testing.T) {
	start := time.Now()
	r := runTool(t, Bash(), t.TempDir(), bashInput{Command: "cat; echo read-done"})
	wantOK(t, r, "read-done")
	if time.Since(start) > 5*time.Second {
		t.Fatal("the command waited for input")
	}
}

func TestBashTimeoutStopsTheWholeGroup(t *testing.T) {
	dir := t.TempDir()
	start := time.Now()
	// A background child keeps the pipe open: only stopping the group ends it.
	r := runTool(t, Bash(), dir, bashInput{Command: "sleep 30 & echo started; sleep 30", Timeout: 1})
	wantErr(t, r, "[timed out after 1s; the command was stopped]")
	wantErr(t, r, "started")
	if took := time.Since(start); took > 5*time.Second {
		t.Fatalf("took %s", took)
	}
}

func TestBashStopsBackgroundChildrenWhenDone(t *testing.T) {
	dir := t.TempDir()
	start := time.Now()
	r := runTool(t, Bash(), dir, bashInput{Command: "(sleep 1; touch late) & echo quick"})
	wantOK(t, r, "quick")
	if took := time.Since(start); took > 3*time.Second {
		t.Fatalf("waited for the background child: %s", took)
	}
	time.Sleep(1500 * time.Millisecond)
	if _, err := os.Stat(filepath.Join(dir, "late")); err == nil {
		t.Fatal("the background child kept running")
	}
}

func TestBashCancel(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	start := time.Now()
	r := New(Bash()).Run(ctx, Env{Dir: t.TempDir()}, call("bash", `{"command":"sleep 30"}`))
	wantErr(t, r, "[cancelled; the command was stopped]")
	if time.Since(start) > 5*time.Second {
		t.Fatal("cancel did not stop the command")
	}
}

func TestBashLongOutputKeepsHeadTailAndFile(t *testing.T) {
	tmp := t.TempDir()
	// 100000 numbered lines, about 590 KB.
	r := New(Bash()).Run(context.Background(), Env{Dir: t.TempDir(), TempDir: tmp},
		call("bash", `{"command":"seq 1 100000"}`))
	wantOK(t, r, "1\n2\n3\n", "99999\n100000\n", "bytes omitted; full output: ")
	if len(r.Content) > bashMaxOutput+300 {
		t.Fatalf("result is %d bytes", len(r.Content))
	}
	_, after, _ := strings.Cut(r.Content, "full output: ")
	path, _, _ := strings.Cut(after, " ...]")
	full := readFile(t, path)
	if !strings.HasPrefix(full, "1\n2\n") || !strings.HasSuffix(full, "\n100000\n") || strings.Count(full, "\n") != 100000 {
		t.Fatalf("full output file is wrong: %d bytes", len(full))
	}

	// Without a temp dir the output is still cut, and says so.
	r = New(Bash()).Run(context.Background(), Env{Dir: t.TempDir()}, call("bash", `{"command":"seq 1 100000"}`))
	wantOK(t, r, "full output not saved")
}
