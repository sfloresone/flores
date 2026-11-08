# flores — agent notes

Short rules for anyone (human or agent) working in this repo. Depth lives in
[`docs/`](docs/) and [`benchmarks/docs/`](benchmarks/docs/).

## Structure

| Path | What it is |
| --- | --- |
| `apps/web` | Public site. Astro + React islands + MDX + Tailwind. |
| `apps/cli` | The flores agent (Go): `flores -p` headless today, TUI later. Start at [`apps/cli/ARCHITECTURE.md`](apps/cli/ARCHITECTURE.md). |
| `benchmarks/` | Perf and harness-behaviour suite (Python + Go helpers). First-class, not under `apps/`. |
| `docs/` | Monorepo Diátaxis (how-to, decisions). Not a copy of `benchmarks/docs/`. |
| `packages/` | Shared libraries when needed. Flat only — no nested packages. |

## Boundaries

- **Bun only** for JavaScript. Install with `bun add`, run with `bun run` / `bunx --bun`. The only JS lockfile is `bun.lock`.
- **`benchmarks/` owns measurement.** The web reads `benchmarks/published/**` as data; it never rewrites those files or recalculates stats.
- **Do not paste benchmark numbers into MDX.** Charts and tables import JSON (or a thin projection of it) so a new publish updates the site without rewriting prose.
- **`benchmarks/docs/`** is engineering reference for the bench. The public educational guide lives as MDX under `apps/web` when written.
- **No build outputs or large files in git.** Compiled helpers go to `benchmarks/.bin/` (ignored). Stage paths explicitly, never a whole folder. CI rejects a PR if *any* of its commits adds a native binary or a blob over 1 MiB (`.github/scripts/check-new-blobs.sh`): a file added and removed later in the same PR still reaches `main`'s history.
- **No folders or packages “for later”.** `apps/cli` only holds what the current milestone uses (e.g. no `permission/` or `tui/` until their milestone).

## Web stack (starting point)

Astro, Bun, React islands, MDX, Tailwind v4. Base UI + selected shadcn components and OpenSource UI as visual reference come in a later pass. Starlight is under evaluation — do not add it unless a short trial (docs page + React island + sidebar + freeform landing) shows it helps more than it fights the design.

- Astro’s CLI / `astro check` need **Node.js 22** (see `.nvmrc`). Bun installs and runs scripts; Node runs the Astro tooling.
- Root `overrides` pin `@emnapi/runtime` and `@emnapi/core` to `1.11.3` so `astro check` does not pick up a broken `2.0.0-beta` from Rolldown’s wasm optional deps.
