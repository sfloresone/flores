// Package tool holds the tools flores gives the model (ADR 0006): bash,
// read, edit, write, grep and glob.
//
// A tool is a plain struct value (tool.go): a name, a description, a JSON
// Schema written by hand (stable bytes for the prompt cache), whether it
// only reads, and a Run function. set.go keeps the tools the model may call
// and owns the errors every tool shares (unknown tool, bad arguments, a
// panic, an oversized result). Consecutive read-only calls of one step run
// at the same time; anything that writes runs alone, in order (Groups).
//
// These are flores' own function tools, not a vendor's built-in tool types:
// the harness executes either kind, and our own work with every provider.
//
// What this package must NOT do: talk to a model, decide when the turn ends,
// or ask for permission (M2 has none; see ADR 0006).
package tool
