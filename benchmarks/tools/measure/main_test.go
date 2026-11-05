package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"
	"time"
)

// The test binary doubles as the measured child: with MEASURE_CHILD set it
// touches MEASURE_CHILD_MIB MiB, sleeps MEASURE_CHILD_SLEEP, prints and exits
// with MEASURE_CHILD_EXIT.
func TestMain(m *testing.M) {
	if os.Getenv("MEASURE_CHILD") != "" {
		mib, _ := strconv.Atoi(os.Getenv("MEASURE_CHILD_MIB"))
		buf := make([]byte, mib<<20)
		for i := 0; i < len(buf); i += 4096 {
			buf[i] = 1
		}
		if d, err := time.ParseDuration(os.Getenv("MEASURE_CHILD_SLEEP")); err == nil {
			time.Sleep(d)
		}
		os.Stdout.WriteString("child out\n")
		runtime.KeepAlive(buf)
		code, _ := strconv.Atoi(os.Getenv("MEASURE_CHILD_EXIT"))
		os.Exit(code)
	}
	os.Exit(m.Run())
}

func child(t *testing.T, mib int, sleep string, exit int) {
	t.Setenv("MEASURE_CHILD", "1")
	t.Setenv("MEASURE_CHILD_MIB", strconv.Itoa(mib))
	t.Setenv("MEASURE_CHILD_SLEEP", sleep)
	t.Setenv("MEASURE_CHILD_EXIT", strconv.Itoa(exit))
}

func TestMeasuresChildMemoryAndOutput(t *testing.T) {
	child(t, 64, "", 3)
	out := filepath.Join(t.TempDir(), "out")
	r := run([]string{os.Args[0]}, "", out, os.DevNull, 30*time.Second)
	if r.Error != "" || r.ExitCode != 3 || r.TimedOut {
		t.Fatalf("unexpected result %+v", r)
	}
	if r.RSSKiB < 60<<10 {
		t.Fatalf("rss %d KiB, want ≥ 60 MiB (child touched 64 MiB)", r.RSSKiB)
	}
	if b, _ := os.ReadFile(out); string(b) != "child out\n" {
		t.Fatalf("stdout file = %q", b)
	}
	if r.WallMs <= 0 || r.CPUMs <= 0 {
		t.Fatalf("missing timings: %+v", r)
	}
}

// The point of this tool: a child must not report the launcher's memory.
// On Linux the floor is our own peak RSS, which a small child stays near.
func TestSmallChildIsNotInflatedByLauncher(t *testing.T) {
	ballast := make([]byte, 128<<20) // grow the launcher well past any child
	for i := 0; i < len(ballast); i += 4096 {
		ballast[i] = 1
	}
	child(t, 0, "", 0)
	r := run([]string{os.Args[0]}, "", os.DevNull, os.DevNull, 30*time.Second)
	runtime.KeepAlive(ballast)
	if runtime.GOOS == "linux" {
		// Linux: the child inherits our peak at exec. This test documents that
		// the floor is reported and that the result is never below it.
		if r.RSSFloorKiB < 100<<10 || r.RSSKiB < r.RSSFloorKiB-1024 {
			t.Fatalf("floor %d KiB, rss %d KiB: floor must be reported", r.RSSFloorKiB, r.RSSKiB)
		}
		return
	}
	if r.RSSFloorKiB != 0 || r.RSSKiB > 64<<10 {
		t.Fatalf("rss %d KiB / floor %d KiB: the launcher's 128 MiB leaked into the child", r.RSSKiB, r.RSSFloorKiB)
	}
}

func TestTimeoutKillsChild(t *testing.T) {
	child(t, 0, "30s", 0)
	start := time.Now()
	r := run([]string{os.Args[0]}, "", os.DevNull, os.DevNull, 300*time.Millisecond)
	if !r.TimedOut || r.ExitCode >= 0 || time.Since(start) > 10*time.Second {
		t.Fatalf("want a killed child, got %+v after %s", r, time.Since(start))
	}
}

func TestMissingBinaryIsAnError(t *testing.T) {
	r := run([]string{filepath.Join(t.TempDir(), "nope")}, "", os.DevNull, os.DevNull, time.Second)
	if r.Error == "" || r.ExitCode != -1 {
		t.Fatalf("want an error result, got %+v", r)
	}
}
