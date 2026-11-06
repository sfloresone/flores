// Package agent is the agent loop (ADR 0003): call the model, run the tools
// it asks for, append the results and repeat until it answers without tools.
//
// An Agent is one conversation. Each call to Run is one turn (a user message
// up to the final answer) made of one or more steps (model calls). The REPL
// of the future interactive mode calls Run once per user message; `flores -p`
// calls it once.
//
// Brakes: an optional step limit (MaxSteps, 0 = none) and an always-on
// repetition detector. When one fires, the agent makes one extra step
// without tools that asks the model to summarize where the work stands.
//
// What this package must NOT do: speak a wire protocol (that is provider/),
// format output (that is cmd/flores and trajectory/), or retry failed calls
// (that will be a provider decorator, ADR 0002).
package agent
