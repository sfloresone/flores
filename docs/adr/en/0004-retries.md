# 0004. Retrying model calls

> 🇪🇸 [Español](../0004-reintentos.md)

- **Status:** Accepted
- **Date:** 2026-11-06
- **Extends:** point 3 of [ADR 0002](../0002-interfaz-provider.md) (the `WithRetry` decorator)

## Context

Providers fail transiently: a 429 from a rate limit, a 503 or 529 from
overload, or an SSE stream that is cut off mid-way. In M0, a single failure
like this ended the turn with exit code 3. In layer B, flores failed
`rate-limit-recover`, `server-error-recover` and `stream-truncated-recover`,
which all four reference harnesses pass.

Before writing the retries we read how each of them does it (code pinned to a
commit):

| | Where | Attempts | Waits | `Retry-After` | 401 | Stream cut mid-response |
|---|---|---|---|---|---|---|
| **fx** | The agent loop ([`recovery.zig`](https://github.com/vercel-labs/fx/blob/fa799f45866d7c4148f175e86e868cb535435e1e/src/core/recovery/recovery.zig#L203)); the transport makes a single attempt | **No limit**, until Esc ([`request_recovery.zig`](https://github.com/vercel-labs/fx/blob/fa799f45866d7c4148f175e86e868cb535435e1e/src/core/agent/runtime/request_recovery.zig#L28-L30)) | 1, 2, 4, 8, 16 and 30 s, ×0.8–1 | Seconds only, and [cut to 30 s](https://github.com/vercel-labs/fx/blob/fa799f45866d7c4148f175e86e868cb535435e1e/src/core/recovery/recovery.zig#L397-L401): if the server asks for an hour, it retries after 30 s | One retry if it can refresh the credential | From scratch; the partial text stays on screen with "↻ Restarted the answer" |
| **opencode** | The session runner, per step | 1 + 10 | 2, 4, 8, then 10 s, with jitter ([`model-retry.ts`](https://github.com/anomalyco/opencode/blob/217cc811de9b0c1594924e89e48705871f1cacea/packages/core/src/model-retry.ts#L22-L36)) | `-ms`, seconds or date; used as the minimum wait, [capped at 15 min](https://github.com/anomalyco/opencode/blob/217cc811de9b0c1594924e89e48705871f1cacea/packages/core/src/model-retry.ts#L12) | No, unless `x-should-retry: true` or a plugin says so | If nothing was output yet, from scratch. If something was, it adds a message: ["Continue from where you left off"](https://github.com/anomalyco/opencode/blob/217cc811de9b0c1594924e89e48705871f1cacea/packages/core/src/session/runner/llm.ts#L37-L38) |
| **crush** (fantasy) | fantasy's loop, per step; the SDK runs with `MaxRetries(0)` | [1 + 3](https://github.com/charmbracelet/fantasy/blob/ec258577c7d825ec38cfdf8cbeaaa2f7e197aacd/retry.go#L113-L119) | 5, 10 and 20 s, no jitter, no cap | `-ms`, seconds or date, but [if it asks for 60 s or more it is ignored](https://github.com/charmbracelet/fantasy/blob/ec258577c7d825ec38cfdf8cbeaaa2f7e197aacd/retry.go#L47-L53) and the retry comes sooner. Also, [`toHeaderMap`](https://github.com/charmbracelet/fantasy/blob/ec258577c7d825ec38cfdf8cbeaaa2f7e197aacd/providers/anthropic/error.go#L130-L139) stores keys as `Retry-After` and then looks up `retry-after` | One `OnAuthRefresh` | From scratch; partial content is deleted |
| **Pi** | The session (`agent.continue()`); the provider layer is [off by default](https://github.com/earendil-works/pi/blob/20d86c3657e90e857309d908b13adcf7fb8d9611/packages/ai/src/utils/provider-retry.ts#L111) | [1 + 3](https://github.com/earendil-works/pi/blob/20d86c3657e90e857309d908b13adcf7fb8d9611/packages/coding-agent/src/core/settings-defaults.ts#L18-L26) | 2, 4 and 8 s, no jitter | Ignored by default: only the provider layer reads it | No | From scratch; the partial stays in the session, hidden from the model |

Total time to recover in layer B (Linux, published):

| | 503 | Stream cut | 429 with `Retry-After: 1`, twice |
|---|---|---|---|
| fx | 0.29 s | 0.29 s | 2.06 s |
| Codex | 0.55 s | 0.54 s | 2.37 s |
| Claude Code | 0.92 s | 0.92 s | 2.66 s |
| Pi | 2.3 s | 2.3 s | 6.3 s |

With 429 the floor is 2 s, because the server asks for two 1 s waits.

## Decision

1. **The policy is a pure function.** `provider.RetryPolicy.Delay(err, n)`
   returns how long to wait before retry `n`, or why not to retry. It does no
   I/O and reads no clock, like fx's `recovery.zig`, so it is tested with a
   table.
2. **What is retried:** network errors, 408, 409, 429, 5xx (529 included), an
   SSE `error` for overload or rate limiting, and a stream that ends without
   `message_stop`.
   - **A 401 or 403 is never retried**, even if the response carries
     `x-should-retry: true`. With an API key, retrying fixes nothing.
   - For everything else, `x-should-retry` (`true` or `false`) overrides the
     classification.
3. **How long to wait depends on the cause:**
   - **Dropped connection or cut stream:** the first retry goes out after
     ~100 ms, because a new connection is enough. After that, like the rest.
   - **Overload or 5xx:** 0.5 s, doubling up to a 30 s cap, with 80–100 %
     jitter so that many clients don't retry at the same moment.
   - **`Retry-After`** (or `retry-after-ms`) is honoured **exactly** up to
     60 s. If the server asks for more, flores **fails at once** with a clear
     message: it neither retries early (like fx and crush) nor leaves the user
     hanging.
   - If the wait doesn't fit in what is left of `--timeout`, it fails now.
4. **6 retries by default** (7 attempts), set with `--max-retries`. 0 turns
   them off.
   - There is a limit because in headless mode (CI) nobody is there to press
     Esc, which is the only thing that stops fx.
   - A retry is not a step: it doesn't count towards `--max-steps` or
     repetition detection ([ADR 0003](0003-agent-loop.md)).
5. **The decorator warns before retrying.** `provider.WithRetry(p, policy)`
   emits an `EventRetry` and then waits; Ctrl-C can cancel the wait. Then it
   resends **the same request**. On `EventRetry`, the loop discards the partial
   text and tool calls. This extends point 3 of ADR 0002: without the signal,
   the loop would receive the text twice.
6. **The model sees nothing of the failed attempts.** The assistant message is
   only added to the history when the step succeeds. The model is not asked to
   "continue", as opencode does.
7. **Every retry is visible:**
   - in the trajectory, `llm.request` carries `attempt`, and `error` carries
     `retryable`, `attempt` and `retry_in_ms`;
   - in text mode, a notice on stderr: `flores: overloaded (HTTP 503) —
     retrying in 0.5s (attempt 2 of 7)`.

   Partial text already written to stdout stays there, as in fx and Pi;
   whoever needs clean output uses `--output jsonl` (`final_text`).

## Why

- **A limit, unlike fx.** Without one, a headless run can hang forever. For a
  person in a TUI that is acceptable; for a script, it isn't.
- **Waits depend on the cause.** A network cut is fixed by reconnecting, so
  waiting seconds only wastes time. Overload needs room, and retrying straight
  away makes it worse.
- **Honour `Retry-After` or fail.** The server knows when it can serve us.
  Retrying earlier (fx after 30 s, crush with its exponential wait, Pi after
  2 s) spends requests that will fail and can extend the block. Waiting an hour
  without saying anything is worse.
- **The same request, no "continue".** Resending the same bytes is
  deterministic, reuses the prompt cache, and leaves no messages in the
  history that the user didn't write. Asking the model to continue saves output
  tokens on long answers, but the model may repeat or skip text. We will
  reconsider if the benchmark shows it matters.

## Alternatives rejected

- **Retrying inside `Stream` without warning** (the transparent decorator of
  the original ADR 0002). It can't be done right once deltas have arrived: the
  consumer would show them twice.
- **Retrying in the agent loop** (fx, crush). It works, but ties the policy to
  the loop. With the decorator, every model call inherits retries, as
  compaction or titles will in the future.
- **Fixed waits without jitter** (crush, Pi). If many clients fail at once,
  they retry at once.

## How it is validated

- **Table tests** of `Delay`: 401 never, exact `Retry-After`, over 60 s →
  fail, cap, jitter with a fixed `Rand`.
- **Decorator tests** with a fake provider: a cut mid-way then success,
  cancellation during the wait, deadline, a consumer that stops iterating.
- **Layer B:**
  - `rate-limit-recover`, `server-error-recover` and
    `stream-truncated-recover` go into `must_pass`;
  - the new scenario `retry-after-too-long` (429 with `Retry-After: 3600`)
    requires failing with a single request.
- **Time targets:** 503 ≈ 0.5 s, cut stream ≈ 0.1 s, 429 ≈ 2.0 s.
