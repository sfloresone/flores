# 0003. The agent loop

> 🇪🇸 [Español](../0003-bucle-del-agente.md)

- **Status:** Accepted
- **Date:** 2026-11-05

## Context

Before writing the flores loop we studied how four real harnesses do it. We
read their code, not their docs (links pinned to a commit):

| | Where the loop lives | Default step limit | Other brakes | Parallel tools |
|---|---|---|---|---|
| **fx** (Zig) | [`orchestrator.zig`](https://github.com/vercel-labs/fx/blob/fa799f45866d7c4148f175e86e868cb535435e1e/src/core/agent/runtime/orchestrator.zig#L7131) | none ([`default_max_agent_steps = 0`](https://github.com/vercel-labs/fx/blob/fa799f45866d7c4148f175e86e868cb535435e1e/src/core/config/agent_steps.zig#L3)) | — | read-only ones, in groups |
| **opencode** (TS) | [`driveLoop`](https://github.com/anomalyco/opencode/blob/217cc811de9b0c1594924e89e48705871f1cacea/packages/ai/src/llm.ts#L218) and the [session runner](https://github.com/anomalyco/opencode/blob/217cc811de9b0c1594924e89e48705871f1cacea/packages/core/src/session/runner/llm.ts#L252) | none (`steps` is optional) | at the limit: one step without tools that asks for a summary | all, unbounded |
| **crush** (Go) | delegated to [fantasy](https://github.com/charmbracelet/fantasy/blob/ec258577c7d825ec38cfdf8cbeaaa2f7e197aacd/agent.go#L440) | none | [repetition](https://github.com/charmbracelet/crush/blob/c7854d54a8fc2eef37f5c31ee4e829318b1ea13c/internal/agent/loop_detection.go#L12) (more than 5 times in 10 steps) and a nearly full context | those marked `Parallel`, up to 5 |
| **Pi** (TS) | [`runLoop`](https://github.com/earendil-works/pi/blob/20d86c3657e90e857309d908b13adcf7fb8d9611/packages/agent/src/agent-loop.ts#L172) | none | — | yes, by default |

What we learned:

- **The core is the same in all four** and fits in about 50 lines: call the
  model, run the tools it asks for, append the results, and repeat until it
  answers without asking for tools. They differ at the edges: limits,
  parallelism, persistence and recovery.
- **None of them limits steps by default.** Layer B measured it: in
  `runaway-tool-loop` (the model asks for tools forever) Pi, Claude Code, Codex
  and fx sent between 3,000 and 7,000 requests in 45 s, until the runner stopped
  them. With a real model, the brake is the person watching, the subscription
  quota, or the spend limit in the provider's console.

## Decision

1. **Two loops.** The **REPL** (outer) only exists in interactive mode: it reads
   a message, runs a turn, prints and waits. The **agent loop** (inner) runs
   inside each turn. `flores -p` is a single pass of the outer loop. Both modes
   use the same inner loop, so the benchmark measures the code people use.
2. **Vocabulary.** A **turn** goes from a user message to the final answer. A
   **step** is one model call. A turn has one or more steps. The flag is
   `--max-steps` and the trajectory uses `step` (`trajectory/v1`).
3. **The loop is our own code**, in the style of fantasy: readable top to bottom
   and with no libraries. It lives in `internal/agent` and exposes
   `Run(ctx, cfg, prompt) iter.Seq2[Event, error]`, like the provider
   ([ADR 0002](0002-provider-interface.md)).
4. **Step limit: 0 by default, meaning no limit**, like the four references.
   `--max-steps N` turns it on for whoever wants it.
5. **Repetition detection, always on.** It is the default brake against endless
   loops:
   - Each tool call gets a signature: an FNV-64a hash of its name, arguments and
     result.
   - If a signature shows up **5 times in the last 10 steps**, the agent stops.
     The window does not need to be full, so an exact loop is caught at step 5.
   - This is stricter than crush, which needs more than 5 repeats and a full
     window of 10 steps.
6. **When a brake stops the agent, one more step to summarize.** The agent adds
   a message that says why it is stopping and asks for a summary of what was
   done, what is left and the next steps. If tools are declared, that step is
   sent with `tool_choice: none`; any tool calls that still arrive are ignored.
   This way both the user and the model know where the work stopped. The message
   is similar to opencode's `MAX_STEPS_PROMPT`.
7. **Exit codes** (the same as `flores-stub`):

   | Code | Meaning |
   |---|---|
   | 0 | success |
   | 2 | usage error |
   | 3 | provider error |
   | 4 | a brake stopped the run (`max_steps` or `repetition`), even with a summary |
   | 130 | cancelled (Ctrl-C) |

## Why

- **A default limit of 0** matches what people coming from other harnesses
  expect. That makes repetition detection the default brake, and it does not cut
  off long but legitimate tasks.
- **Repetition detection** covers the case measured in layer B (the same call
  with the same result) without an artificial ceiling on long tasks.
- **The summary step** costs one request, but avoids ending blind: the user
  knows what got done and the model can pick it up again.

## Rejected alternatives

- **A fixed default limit (25 or 40 steps).** It protects more, but cuts off
  real long tasks and departs from what the others do. It is still available
  through `--max-steps`.
- **Stopping without a summary.** Cheaper, but neither the user nor the model
  knows what state the work was left in.
- **Using fantasy or copying fx's orchestrator.** fantasy hides the loop behind
  more than 20 callbacks. fx's loop file is over 12,000 lines because it mixes
  permissions, subagents and recovery. Both are too much to learn from.

## How it is validated

- Layer B: `runaway-tool-loop` passes with no step limit (6 requests out of 50
  allowed), and `basic-text`, `unknown-tool` and `auth-error-fail-fast` keep
  passing.
- Tests of the `agent` package with a fake provider: text only, unknown tool,
  `--max-steps` with a summary, repetition with a summary, provider error and
  cancellation.
- The `trajectory/v1` trajectory validates against the schema in every case.
