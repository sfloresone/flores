// Command measure runs one process and reports its wall time, CPU time and
// peak resident memory (RSS) as a single JSON line on stdout.
//
// Why it exists: on Linux, a process's ru_maxrss includes the peak RSS of the
// address space it replaced at exec. A child spawned from Python (vfork or
// fork, then exec) therefore reports at least the Python parent's RSS: in CI
// every agent measured ~24 MiB, including a 2 MiB Go stub. Launching from this
// small program keeps that floor low, and the floor itself is reported
// (rss_floor_kib) so a result that cannot be told apart from it is shown as
// "≤ floor" instead of a fake number. macOS is not affected (floor 0).
//
// Usage:
//
//	measure [-timeout 60s] [-dir DIR] [-stdout FILE] [-stderr FILE] -- argv...
//
// The child inherits this process's environment. Its stdout/stderr go to
// files (never pipes), so a chatty child cannot block and distort the timing.
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"time"
)

// Result is the JSON line written to stdout.
type Result struct {
	ExitCode    int     `json:"exit_code"`
	TimedOut    bool    `json:"timed_out"`
	WallMs      float64 `json:"wall_ms"`
	CPUMs       float64 `json:"cpu_ms"`
	RSSKiB      int64   `json:"rss_kib"`
	RSSFloorKiB int64   `json:"rss_floor_kib"`
	Error       string  `json:"error,omitempty"`
}

func main() {
	timeout := flag.Duration("timeout", 60*time.Second, "kill the child after this long")
	dir := flag.String("dir", "", "working directory for the child")
	stdoutPath := flag.String("stdout", os.DevNull, "file for the child's stdout")
	stderrPath := flag.String("stderr", os.DevNull, "file for the child's stderr")
	flag.Parse()
	if flag.NArg() == 0 {
		fmt.Fprintln(os.Stderr, "usage: measure [flags] -- argv...")
		os.Exit(2)
	}
	res := run(flag.Args(), *dir, *stdoutPath, *stderrPath, *timeout)
	_ = json.NewEncoder(os.Stdout).Encode(res)
	if res.Error != "" {
		os.Exit(1)
	}
}

func run(argv []string, dir, stdoutPath, stderrPath string, timeout time.Duration) Result {
	stdout, err := os.Create(stdoutPath)
	if err != nil {
		return Result{ExitCode: -1, Error: err.Error()}
	}
	defer stdout.Close()
	stderr, err := os.Create(stderrPath)
	if err != nil {
		return Result{ExitCode: -1, Error: err.Error()}
	}
	defer stderr.Close()

	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Dir = dir
	cmd.Stdout, cmd.Stderr = stdout, stderr // *os.File: the child writes directly, no copy goroutines

	start := time.Now()
	if err := cmd.Start(); err != nil {
		return Result{ExitCode: -1, Error: err.Error()}
	}
	// Start returns after the child's exec, so our peak RSS now bounds what the
	// child inherited at exec.
	floor := ownPeakKiB()

	var timedOut atomic.Bool
	timer := time.AfterFunc(timeout, func() {
		timedOut.Store(true)
		_ = cmd.Process.Kill()
	})
	_ = cmd.Wait() // a non-zero exit is data, not an error
	wall := time.Since(start)
	timer.Stop()

	ru, _ := cmd.ProcessState.SysUsage().(*syscall.Rusage)
	res := Result{
		ExitCode:    cmd.ProcessState.ExitCode(),
		TimedOut:    timedOut.Load(),
		WallMs:      float64(wall.Microseconds()) / 1000,
		RSSFloorKiB: floor,
	}
	if ru != nil {
		res.CPUMs = float64(time.Duration(ru.Utime.Nano()+ru.Stime.Nano()).Microseconds()) / 1000
		res.RSSKiB = maxRSSKiB(ru.Maxrss)
	}
	if ws, ok := cmd.ProcessState.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
		res.ExitCode = -int(ws.Signal())
	}
	return res
}

// maxRSSKiB normalises ru_maxrss: Linux reports KiB, macOS bytes.
func maxRSSKiB(v int64) int64 {
	if runtime.GOOS == "darwin" {
		return v / 1024
	}
	return v
}

// ownPeakKiB is this process's peak RSS (VmHWM) on Linux, the floor any child
// reports there. Elsewhere the child starts from a fresh address space: 0.
func ownPeakKiB() int64 {
	if runtime.GOOS != "linux" {
		return 0
	}
	f, err := os.Open("/proc/self/status")
	if err != nil {
		return 0
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if rest, ok := strings.CutPrefix(sc.Text(), "VmHWM:"); ok {
			n, _ := strconv.ParseInt(strings.TrimSuffix(strings.TrimSpace(rest), " kB"), 10, 64)
			return n
		}
	}
	return 0
}
