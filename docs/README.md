# flores docs

Internal documentation for the monorepo, kept next to the code so it changes in
the same pull request as the behaviour it describes. It follows
[Diátaxis](https://diataxis.fr).

| Folder | Job | Read it when you want to… |
| --- | --- | --- |
| `tutorials/` | Learning | Build something end to end for the first time |
| `how-to/` | Tasks | Get a specific thing done (run the site, publish benches) |
| `reference/` | Facts | Look up a path, a script, an env var |
| `explanation/` | Understanding | Learn why the repo is shaped the way it is |
| `decisions/` | History | See which options were weighed ([MADR](https://adr.github.io/madr/)) |

## What lives where

- **Bench engineering reference** (methodology, metrics, adapters, taxonomy) stays
  in [`benchmarks/docs/`](../benchmarks/docs/). Do not copy it here.
- **Public educational guide** (what an agent is, how to read results) will live
  as MDX under [`apps/web`](../apps/web), not as a second copy of the bench docs.
- **This folder** is for monorepo how-tos, ADRs, and cross-cutting explanation.

Personal, unpublished notes go in `docs/local/`, which Git ignores.

Writing rules: [`style.md`](style.md).
