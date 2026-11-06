# flores architecture

> 🇪🇸 [Español](ARCHITECTURE.md)

This document follows one request from start to finish. The decisions live in
the ADRs: [0001](../../docs/adr/0001-estructura-y-estilo-go.md) (structure),
[0002](../../docs/adr/0002-interfaz-provider.md) (provider) and
[0003](../../docs/adr/en/0003-agent-loop.md) (loop).

Status: **milestone M0**. There is a loop and an Anthropic provider, but no
tools and no retries yet.

## The packages

| Package | What it does | What it does not do |
|---|---|---|
| `cmd/flores` | Reads flags and environment variables, creates the provider and the agent, writes the output and picks the exit code. | Agent or protocol logic. |
| `internal/agent` | The loop: calls the model, answers tool calls, applies the brakes and asks for the final summary. | Speak HTTP, format output, retry. |
| `internal/provider` | Our own types (`Message`, `Request`, `Event`, `Error`) and the `Provider` interface. | Know about any specific provider. |
| `internal/provider/anthropic` | The Messages API protocol over `net/http`: body, headers, SSE and errors. | Retry, or decide what to do with tools. |
| `internal/trajectory` | Turns agent events into `trajectory/v1` (JSONL). | Decide anything about the run. |

## The path of `flores -p "Say hello."`

1. **`main.go`** validates the options and creates:
   - an `anthropic.Client` with a single `http.Client`, so the connection is
     reused between steps;
   - an `agent.Agent` with the model, the system prompt and `MaxSteps` (0 = no
     limit).
2. **`Agent.Run(ctx, prompt)`** appends the user message to the history and
   starts a **turn**. It returns an iterator of events; the last one is always
   `EventDone`, with the result.
3. **Each step** (`turn.step`):
   - builds a `provider.Request` with the whole history;
   - ranges over `provider.Stream(...)`: `EventSent` (bytes sent),
     `EventTextDelta` (text as it arrives), `EventToolCall` (complete calls) and
     `EventDone` (tokens and stop reason);
   - appends the assistant's reply to the history.
4. **The adapter** (`anthropic.Stream`):
   - serializes the request (tools are sorted by name, so the cache gets stable
     bytes);
   - sends `POST /v1/messages`;
   - reads the SSE stream (`stream.go`) and joins the JSON fragments of each
     `tool_use`.

   If something fails, it returns a `*provider.Error` that is already
   classified (`Auth`, `RateLimit`, `Overloaded`…).
5. **If the model asked for tools**, `runTools` answers them. In M0 all of them
   return the error `unknown tool`. The results are appended to the history and
   the loop goes back to step 3.
6. **The brakes** (`guard.go`) act after each step with tool calls:
   - `--max-steps N`, if set;
   - repetition detection: the same call with the same result 5 times in the
     last 10 steps.

   If one fires, `summarize` makes **one more step** without tools and asks the
   model for a summary of what was done and what is left.
7. **Output.** In `text` mode, the text is written as it arrives and the reason
   for stopping goes to stderr. In `jsonl` mode, `trajectory.Emitter` writes one
   event per line.
8. **Exit code:** 0 success, 2 usage, 3 provider or timeout, 4 brake,
   130 cancelled.

## Testing and measuring

```sh
cd apps/cli && go vet ./... && go test -race ./...     # unit tests, no network
cd benchmarks && uv run pytest -q                       # contract against the real mock
uv run flores-bench harness --agents flores             # layer B
uv run flores-bench perf hello --agents flores,fx       # layer A
```

The benchmark builds `flores` from here into `benchmarks/.bin/flores` on every
run, so it never measures a stale binary.
