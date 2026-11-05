// Command flores is the flores coding agent.
//
// This file only wires things together: flags and environment → provider →
// agent loop → output. The interesting code lives in internal/ (see
// ARCHITECTURE.md).
package main

import (
	"fmt"
	"io"
	"os"
)

// version is overridden at release time with -ldflags "-X main.version=…".
var version = "0.0.0-dev"

const usageText = `flores - a small, provider-agnostic coding agent

Usage:
  flores --version      print the version
  flores --help         show this help
`

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usageText)
		return exitUsage
	}
	switch args[0] {
	case "--version", "-v", "version":
		fmt.Fprintf(stdout, "flores %s\n", version)
		return exitOK
	case "--help", "-h", "help":
		fmt.Fprint(stdout, usageText)
		return exitOK
	}
	fmt.Fprintf(stderr, "unknown argument %q\n\n%s", args[0], usageText)
	return exitUsage
}

// Exit codes (ADR 0003). The benchmark contract relies on them.
const (
	exitOK    = 0
	exitUsage = 2
)
