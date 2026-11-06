// Command flores is the flores coding agent.
//
// This file only wires things together: flags and environment → provider →
// agent loop → output (text or a trajectory/v1 JSONL stream). The
// interesting code lives in internal/ (see ARCHITECTURE.md).
package main

import (
	"cmp"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"iter"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/sfloresone/flores/apps/cli/internal/agent"
	"github.com/sfloresone/flores/apps/cli/internal/provider"
	"github.com/sfloresone/flores/apps/cli/internal/provider/anthropic"
	"github.com/sfloresone/flores/apps/cli/internal/trajectory"
)

// version is overridden at release time with -ldflags "-X main.version=…".
var version = "0.0.0-dev"

// systemPrompt is deliberately tiny: every byte is sent on every step.
const systemPrompt = "You are flores, a coding agent. Be concise."

// Exit codes (ADR 0003). The benchmark contract relies on them.
const (
	exitOK        = 0
	exitUsage     = 2
	exitProvider  = 3
	exitBrake     = 4
	exitCancelled = 130
)

const usageText = `flores - a small, provider-agnostic coding agent

Usage:
  flores -p PROMPT [flags]    run one turn and exit (prompt from stdin if omitted)
  flores --version            print the version

Flags:
  -p, --prompt TEXT     the task
  --output MODE         text (default) or jsonl (trajectory/v1 events on stdout)
  --model ID            model id                (env FLORES_MODEL)
  --base-url URL        provider base URL        (env FLORES_BASE_URL)
  --api-key KEY         API key                  (env FLORES_API_KEY or ANTHROPIC_API_KEY)
  --max-steps N         stop after N model calls; 0 = no limit (default 0)
  --timeout DUR         overall timeout, e.g. 10m; 0 = none (default 0)

Provider: env FLORES_PROVIDER (only "anthropic" for now).
Exit codes: 0 success, 2 usage, 3 provider error or timeout,
            4 stopped by a brake (step limit or repeated tool call), 130 cancelled.
`

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

type options struct {
	prompt, output, model, baseURL, apiKey, providerName string
	maxSteps                                             int
	timeout                                              time.Duration
	version                                              bool
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		// Bare `flores` will start the interactive mode (REPL); not built yet.
		fmt.Fprint(stderr, usageText)
		return exitUsage
	}
	opts, code, ok := parse(args, stdin, stderr)
	if !ok {
		return code
	}
	if opts.version {
		fmt.Fprintf(stdout, "flores %s\n", version)
		return exitOK
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if opts.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, opts.timeout)
		defer cancel()
	}

	p := anthropic.New(anthropic.Config{
		BaseURL:    opts.baseURL,
		APIKey:     opts.apiKey,
		UserAgent:  "flores/" + version,
		HTTPClient: &http.Client{}, // one client: the connection is reused across steps
	})
	a := agent.New(p, agent.Config{Model: opts.model, System: systemPrompt, MaxSteps: opts.maxSteps})

	var res *agent.Result
	if opts.output == "jsonl" {
		cwd, _ := os.Getwd()
		em := trajectory.New(stdout, trajectory.Session{
			AgentName: "flores", AgentVersion: version, Model: opts.model,
			ProviderName: opts.providerName, Protocol: "anthropic-messages",
			BaseURL: cmp.Or(opts.baseURL, anthropic.DefaultBaseURL), Cwd: cwd,
		})
		for ev := range a.Run(ctx, opts.prompt) {
			em.Record(ev)
			if ev.Kind == agent.EventDone {
				res = ev.Result
			}
		}
	} else {
		res = printText(a.Run(ctx, opts.prompt), stdout)
	}
	return report(res, opts, stderr)
}

func parse(args []string, stdin io.Reader, stderr io.Writer) (options, int, bool) {
	var o options
	fs := flag.NewFlagSet("flores", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&o.prompt, "p", "", "")
	fs.StringVar(&o.prompt, "prompt", "", "")
	fs.StringVar(&o.output, "output", "text", "")
	fs.StringVar(&o.model, "model", os.Getenv("FLORES_MODEL"), "")
	fs.StringVar(&o.baseURL, "base-url", os.Getenv("FLORES_BASE_URL"), "")
	fs.StringVar(&o.apiKey, "api-key", cmp.Or(os.Getenv("FLORES_API_KEY"), os.Getenv("ANTHROPIC_API_KEY")), "")
	fs.IntVar(&o.maxSteps, "max-steps", 0, "")
	fs.DurationVar(&o.timeout, "timeout", 0, "")
	fs.BoolVar(&o.version, "version", false, "")
	fs.BoolVar(&o.version, "v", false, "")
	help := fs.Bool("help", false, "")
	fs.BoolVar(help, "h", false, "")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(stderr, "error: %v\n\n%s", err, usageText)
		return o, exitUsage, false
	}
	if *help {
		fmt.Fprint(stderr, usageText)
		return o, exitOK, false
	}
	if o.version {
		return o, 0, true
	}
	o.providerName = cmp.Or(os.Getenv("FLORES_PROVIDER"), "anthropic")
	if o.prompt == "" && fs.NArg() > 0 {
		o.prompt = strings.Join(fs.Args(), " ")
	}
	if o.prompt == "" && stdin != nil {
		b, _ := io.ReadAll(stdin)
		o.prompt = strings.TrimSpace(string(b))
	}
	var problem string
	switch {
	case o.prompt == "":
		problem = "empty prompt: pass -p TEXT or pipe it on stdin"
	case o.output != "text" && o.output != "jsonl":
		problem = "--output must be text or jsonl"
	case o.providerName != "anthropic":
		problem = fmt.Sprintf("unknown provider %q (FLORES_PROVIDER): only \"anthropic\" exists so far", o.providerName)
	case o.model == "":
		problem = "no model: pass --model or set FLORES_MODEL"
	case o.apiKey == "":
		problem = "no API key: pass --api-key or set FLORES_API_KEY / ANTHROPIC_API_KEY"
	case o.maxSteps < 0:
		problem = "--max-steps must be >= 0 (0 = no limit)"
	}
	if problem != "" {
		fmt.Fprintln(stderr, "error:", problem)
		return o, exitUsage, false
	}
	return o, 0, true
}

// printText streams the assistant text to stdout as it arrives.
func printText(events iter.Seq[agent.Event], stdout io.Writer) *agent.Result {
	var res *agent.Result
	var last byte = '\n' // last byte written, to separate steps cleanly
	for ev := range events {
		switch ev.Kind {
		case agent.EventStepStart:
			if last != '\n' {
				io.WriteString(stdout, "\n")
				last = '\n'
			}
		case agent.EventTextDelta:
			if ev.Text != "" {
				io.WriteString(stdout, ev.Text)
				last = ev.Text[len(ev.Text)-1]
			}
		case agent.EventDone:
			res = ev.Result
		}
	}
	if last != '\n' {
		io.WriteString(stdout, "\n")
	}
	return res
}

// report explains how the turn ended on stderr and picks the exit code.
func report(res *agent.Result, opts options, stderr io.Writer) int {
	switch res.Status {
	case agent.StatusSuccess:
		return exitOK
	case agent.StatusMaxSteps:
		fmt.Fprintf(stderr, "flores: stopped after reaching --max-steps=%d\n", opts.maxSteps)
		return exitBrake
	case agent.StatusRepetition:
		fmt.Fprintln(stderr, "flores: stopped because the same tool call kept returning the same result")
		return exitBrake
	case agent.StatusCancelled:
		fmt.Fprintln(stderr, "flores: cancelled")
		return exitCancelled
	}
	switch {
	case errors.Is(res.Err, context.DeadlineExceeded):
		fmt.Fprintf(stderr, "error: timed out after %s\n", opts.timeout)
	case res.Err != nil:
		fmt.Fprintln(stderr, "error:", res.Err)
	}
	var pe *provider.Error
	if errors.As(res.Err, &pe) && pe.Kind == provider.ErrAuth {
		fmt.Fprintln(stderr, "hint: check the API key (FLORES_API_KEY or ANTHROPIC_API_KEY)")
	}
	return exitProvider
}
