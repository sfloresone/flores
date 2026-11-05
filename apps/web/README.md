# `@flores/web`

Public site for flores: learning narrative and visualisation of benchmark
results. Astro + React islands + MDX + Tailwind v4.

## Requirements

- [Bun](https://bun.sh) 1.4 (pinned in the root `packageManager` field).
- [Node.js](https://nodejs.org) 22 (22.12+, see repo `.nvmrc`). Astro’s CLI and
  `astro check` run on Node even when you start them through Bun. Put Node 22
  first in `PATH` when building. The root `package.json` overrides pin
  `@emnapi/*` to `1.11.3` so check does not resolve a broken beta from Rolldown.

## Commands

From the repo root:

```sh
bun install
bun run dev:web
bun run build
```

Or from this directory: `bun run dev` / `bun run build`.

The provisional component catalog is available at `http://localhost:4321/dev`
while the development server is running. It is intentionally excluded from
production builds; its sections and designs are pending review.

## Data contract

- Source of truth for numbers:
  [`benchmarks/published/`](../../benchmarks/published/)
  (`perf-startup`, `perf-hello`, `harness` — each with `latest.json` /
  `history.jsonl` per platform).
- This app **reads** those files at build time (or a thin projection of them).
  It never writes them. `flores-bench publish` owns that tree.
- Do not hard-code benchmark figures in MDX. Prose explains; components render
  data; metadata (commit, platform, agent versions) stays visible next to charts.
- `harness/**/latest.json` can be large — project the fields a view needs; do
  not embed the whole file in a page module.

## Component folders

```text
src/components/
  ui/            reusable primitives (buttons, dialogs, …) — later
  docs/          components for technical content
  benchmarks/    charts, tables, run metadata
  landing/       presentation-only pieces
```

## Out of scope here

Base UI / shadcn / OpenSource UI identity, interactive charts, Starlight, and
the Go CLI (`apps/cli`) land in later passes.
