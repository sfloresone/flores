package tool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

const (
	bashDefaultTimeout = 120 * time.Second
	bashMaxTimeout     = 600 * time.Second
	// Output longer than bashMaxOutput keeps its first bashHead and last
	// bashTail bytes; the whole of it goes to a file in Env.TempDir.
	bashMaxOutput = 32 << 10
	bashHead      = 8 << 10
	bashTail      = bashMaxOutput - bashHead
	// killGrace is how long a process group has to stop after SIGTERM.
	killGrace = 2 * time.Second
	// pipeGrace is how long to wait for output after the shell exits, in
	// case a child it left behind still holds the pipe.
	pipeGrace = 500 * time.Millisecond
)

type bashInput struct {
	Command string `json:"command"`
	Timeout int    `json:"timeout,omitempty"`
}

// Bash returns the bash tool: run one shell command.
func Bash() Tool {
	return Tool{
		Name: "bash",
		Description: "Run a shell command with bash -c in the working directory and return its combined stdout " +
			"and stderr. Each call starts a new shell: cd and export do not carry over. stdin is empty, so " +
			"commands must not wait for input. The command is stopped after timeout seconds (default 120, " +
			"max 600), and processes it leaves in the background are stopped when it ends. Long output keeps " +
			"its start and end. Use read, grep and glob instead of cat, grep and find.",
		Schema: json.RawMessage(`{"type":"object","properties":{` +
			`"command":{"type":"string","description":"The command to run"},` +
			`"timeout":{"type":"integer","description":"Seconds before the command is stopped (default 120, max 600)"}},` +
			`"required":["command"]}`),
		Run: runBash,
	}
}

func runBash(ctx context.Context, env Env, input json.RawMessage) Result {
	var in bashInput
	if bad, ok := decode(input, &in); !ok {
		return bad
	}
	if bad, ok := required("command", in.Command); !ok {
		return bad
	}
	timeout := bashDefaultTimeout
	if in.Timeout > 0 {
		timeout = min(time.Duration(in.Timeout)*time.Second, bashMaxTimeout)
	}

	out := &capture{dir: env.TempDir}
	defer out.close()
	cmd := exec.Command(shell(), "-c", in.Command)
	cmd.Dir = env.Dir
	cmd.Env = append(os.Environ(),
		"FLORES=1", "AI_AGENT=flores", "PAGER=cat", "GIT_PAGER=cat", "GIT_TERMINAL_PROMPT=0")
	cmd.Stdout = out // the same writer for both: one pipe, output in arrival order
	cmd.Stderr = out
	cmd.WaitDelay = pipeGrace
	ownGroup(cmd)
	if err := cmd.Start(); err != nil {
		return errorf("cannot start the shell: %v", err)
	}

	waited := make(chan error, 1)
	go func() { waited <- cmd.Wait() }()
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	var stopped string
	var err error
	select {
	case err = <-waited:
	case <-timer.C:
		stopped = fmt.Sprintf("[timed out after %s; the command was stopped]", timeout)
		err = stop(cmd, waited)
	case <-ctx.Done():
		stopped = "[cancelled; the command was stopped]"
		err = stop(cmd, waited)
	}
	killGroup(cmd) // background children left by the command

	var res strings.Builder
	res.WriteString(out.text())
	if res.Len() == 0 {
		res.WriteString("(no output)")
	}
	code := 0
	if cmd.ProcessState != nil {
		code = cmd.ProcessState.ExitCode()
	}
	isErr := true
	switch {
	case stopped != "":
		res.WriteString("\n" + stopped)
	case err != nil && !errors.Is(err, exec.ErrWaitDelay) && code == 0:
		res.WriteString("\n[error: " + err.Error() + "]")
	case code < 0:
		res.WriteString("\n[" + cmd.ProcessState.String() + "]")
	case code != 0:
		fmt.Fprintf(&res, "\n[exit code %d]", code)
	default:
		isErr = false
	}
	return Result{Content: res.String(), IsError: isErr}
}

// shell is bash when it is installed, else sh.
var shell = sync.OnceValue(func() string {
	if p, err := exec.LookPath("bash"); err == nil {
		return p
	}
	return "sh"
})

// stop ends the command's process group: SIGTERM, then SIGKILL after
// killGrace. It returns what Wait returned.
func stop(cmd *exec.Cmd, waited <-chan error) error {
	terminateGroup(cmd)
	select {
	case err := <-waited:
		return err
	case <-time.After(killGrace):
		killGroup(cmd)
		return <-waited
	}
}

// capture keeps what the model will see of the output: all of it up to
// bashMaxOutput, then the first bashHead and last bashTail bytes. Once the
// output passes the limit, every byte also goes to a file so nothing is lost.
type capture struct {
	mu    sync.Mutex
	dir   string
	buf   []byte // all output while it fits; then the head
	tail  []byte // the last bashTail bytes, once over the limit
	total int
	file  *os.File
	ferr  error
}

func (c *capture) Write(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.total += len(p)
	if c.file == nil && c.ferr == nil && len(c.buf)+len(p) <= bashMaxOutput {
		c.buf = append(c.buf, p...)
		return len(p), nil
	}
	if c.file == nil && c.ferr == nil {
		c.spill()
	}
	if c.file != nil {
		c.file.Write(p) //nolint:errcheck // best effort: the model still gets head and tail
	}
	if c.tail == nil {
		c.tail = append(c.tail, c.buf[min(bashHead, len(c.buf)):]...)
		c.buf = c.buf[:min(bashHead, len(c.buf))]
	}
	c.tail = append(c.tail, p...)
	if over := len(c.tail) - bashTail; over > 0 {
		c.tail = append(c.tail[:0], c.tail[over:]...)
	}
	return len(p), nil
}

// spill opens the file for the full output and writes what came so far.
func (c *capture) spill() {
	if c.dir == "" {
		c.ferr = errors.New("no temp dir")
		return
	}
	c.file, c.ferr = os.CreateTemp(c.dir, "bash-*.log")
	if c.file != nil {
		c.file.Write(c.buf) //nolint:errcheck
	}
}

func (c *capture) text() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.tail == nil {
		return string(c.buf)
	}
	omitted := c.total - len(c.buf) - len(c.tail)
	// The cuts may split a character; drop the broken pieces.
	head := strings.ToValidUTF8(string(c.buf), "")
	tail := strings.ToValidUTF8(string(c.tail), "")
	where := "full output not saved"
	if c.file != nil {
		where = "full output: " + c.file.Name()
	}
	return fmt.Sprintf("%s\n[... %d bytes omitted; %s ...]\n%s", head, omitted, where, tail)
}

func (c *capture) close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.file != nil {
		c.file.Close()
	}
}
