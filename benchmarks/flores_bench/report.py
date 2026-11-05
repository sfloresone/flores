"""Publish results and render the Markdown report.

Raw results (with every sample) stay in results/ and are git-ignored. Publishing
copies a compact version into published/, which *is* versioned:

    published/<suite>/<platform>/latest.json    stats of the last published run
    published/<suite>/<platform>/history.jsonl  one line per run, append-only
    published/README.md                         generated report (tables + charts)

JSON is the source of truth: the README is regenerated from it, and the future
website reads the same files to draw interactive charts.
"""

from __future__ import annotations

import json
from pathlib import Path

from flores_bench.paths import PUBLISHED_DIR

REFERENCE_PLATFORM = "linux-x86_64"

SUITE_TITLES = {
    "perf-startup": "Arranque (`<agent> --version`)",
    "perf-hello": 'Overhead del harness: un "hello world" contra el mock provider',
    "harness": "Comportamiento del harness (capa B): fallos guionizados",
    "tasks": "Tareas reales (capa C): flores-micro con modelos reales",
}

STATUS_ICON = {"pass": "✅", "fail": "❌", "n/a": "—", "error": "⚠️"}

# (metric, column title, unit transform)
COLUMNS = {
    "perf-startup": [
        ("wall_ms", "Tiempo p50 (ms)"),
        ("cpu_ms", "CPU p50 (ms)"),
        ("rss_mb", "RSS p50 (MiB)"),
    ],
    "perf-hello": [
        ("wall_ms", "Tiempo p50 (ms)"),
        ("harness_ms", "Harness p50 (ms)"),
        ("cpu_ms", "CPU p50 (ms)"),
        ("rss_mb", "RSS p50 (MiB)"),
        ("request_bytes", "Enviado al modelo (KiB)"),
        ("system_bytes", "System prompt (KiB)"),
        ("tools", "Tools"),
        ("llm_requests", "Peticiones"),
    ],
}

KIB_METRICS = {"request_bytes", "system_bytes"}


def compact(result: dict) -> dict:
    """Result without per-run samples (keeps published diffs small)."""
    out = dict(result)
    out["subjects"] = [{k: v for k, v in s.items() if k != "samples"} for s in result["subjects"]]
    return out


def history_line(result: dict) -> dict:
    subjects = {}
    if result["suite"] == "tasks":
        for s in result["subjects"]:
            if s.get("metrics"):
                m = s["metrics"]
                subjects[s["name"]] = {
                    "version": s.get("version"),
                    "pass_at_1": m["pass_at_1"],
                    "pass_hat_k": m["pass_hat_k"],
                    "cost_usd": m["cost_usd"],
                    "passed": {t: e["passed"] for t, e in s["tasks"].items()},
                }
        return {
            "created_at": result["created_at"],
            "sha": result["git"]["sha"],
            "dirty": result["git"]["dirty"],
            "tier": result["params"].get("tier"),
            "k": result["params"].get("k"),
            "upstream": result["params"].get("upstream"),
            "subjects": subjects,
        }
    if result["suite"] == "harness":
        for s in result["subjects"]:
            if s.get("scenarios"):
                subjects[s["name"]] = {
                    "version": s.get("version"),
                    **s["metrics"],
                    "status": {k: v["status"] for k, v in s["scenarios"].items()},
                }
        return {
            "created_at": result["created_at"],
            "sha": result["git"]["sha"],
            "dirty": result["git"]["dirty"],
            "subjects": subjects,
        }
    for s in result["subjects"]:
        if not s.get("metrics"):
            continue
        entry = {"version": s.get("version")}
        for metric, stats in s["metrics"].items():
            entry[f"{metric}.p50"] = round(stats["p50"], 4)
        subjects[s["name"]] = entry
    return {
        "created_at": result["created_at"],
        "sha": result["git"]["sha"],
        "dirty": result["git"]["dirty"],
        "cpu": result["machine"].get("cpu"),
        "subjects": subjects,
    }


def publish(result: dict, root: Path = PUBLISHED_DIR) -> Path:
    target = root / result["suite"] / result["machine"]["platform"]
    target.mkdir(parents=True, exist_ok=True)
    (target / "latest.json").write_text(json.dumps(compact(result), indent=2) + "\n")
    with open(target / "history.jsonl", "a") as f:
        f.write(json.dumps(history_line(result), separators=(",", ":")) + "\n")
    write_readme(root)
    return target


# ------------------------------------------------------------------ markdown


def _fmt(metric: str, stats: dict | None) -> str:
    if not stats:
        return "—"
    v = stats["p50"]
    if metric in KIB_METRICS:
        return f"{v / 1024:.1f}"
    if metric in ("tools", "llm_requests"):
        return f"{v:.0f}"
    return f"{v:.1f}" if v >= 10 else f"{v:.2f}"


def _sorted_subjects(result: dict) -> list[dict]:
    def key(s):
        m = s.get("metrics", {}).get("wall_ms")
        return (0, m["p50"]) if m else (1, 0)

    return sorted(result["subjects"], key=key)


def render_table(result: dict) -> str:
    cols = COLUMNS.get(result["suite"], [("wall_ms", "Tiempo p50 (ms)")])
    head = "| Agente | Versión | " + " | ".join(t for _, t in cols) + " |"
    sep = "|---|---|" + "---:|" * len(cols)
    rows = [head, sep]
    for s in _sorted_subjects(result):
        version = s.get("version") or "—"
        if s.get("version_mismatch"):
            version += f" ⚠ (ref {s['pinned_version']})"
        if not s.get("metrics"):
            reason = s.get("skipped_reason") or "sin datos"
            rows.append(f"| {s['label']} | {version} | " + " | ".join(["—"] * len(cols)) + f" |  <!-- {reason} -->")
            continue
        wall = s["metrics"].get("wall_ms")
        cells = []
        for metric, _ in cols:
            cell = _fmt(metric, s["metrics"].get(metric))
            if metric == "wall_ms" and wall and wall.get("ci95_p50"):
                lo, hi = wall["ci95_p50"]
                cell += f" <sub>[{lo:.1f}–{hi:.1f}]</sub>"
            cells.append(cell)
        rows.append(f"| {s['label']} | {version} | " + " | ".join(cells) + " |")
    skipped = [s for s in result["subjects"] if not s.get("metrics")]
    if skipped:
        rows.append("")
        rows.append("No medidos: " + "; ".join(f"**{s['label']}** ({s.get('skipped_reason')})" for s in skipped))
    return "\n".join(rows)


def render_chart(result: dict, metric: str = "wall_ms", title: str = "Tiempo p50 (ms)") -> str:
    """Mermaid bar chart: renders natively on GitHub, no JS needed."""
    subs = [s for s in _sorted_subjects(result) if s.get("metrics", {}).get(metric)]
    if not subs:
        return ""
    labels = ", ".join(json.dumps(s["label"]) for s in subs)
    values = ", ".join(f"{s['metrics'][metric]['p50']:.2f}" for s in subs)
    return (
        "```mermaid\nxychart-beta\n"
        f'    title "{title}"\n'
        f"    x-axis [{labels}]\n"
        f'    y-axis "{title}"\n'
        f"    bar [{values}]\n```"
    )


def _commit(result: dict) -> str:
    git = result["git"]
    return f"Commit `{git.get('short') or 'sin commit'}`" + (" (dirty)" if git.get("dirty") else "")


def render_harness(result: dict) -> str:
    """Scenario × agent matrix plus the reasons behind every failure."""
    subs = [s for s in result["subjects"] if s.get("scenarios")]
    skipped = [s for s in result["subjects"] if not s.get("scenarios")]
    head = "| Escenario | " + " | ".join(f"{s['label']}<br><sub>{s.get('version') or ''}</sub>" for s in subs) + " |"
    rows = [head, "|---|" + ":---:|" * len(subs)]
    for sc in result["scenarios"]:
        cells = [STATUS_ICON.get(s["scenarios"].get(sc["name"], {}).get("status"), "?") for s in subs]
        rows.append(
            f"| **{sc['title']}**<br><sub>`{sc['name']}` · {sc['category']}</sub> | " + " | ".join(cells) + " |"
        )
    totals = [f"**{s['metrics']['passed']}/{s['metrics']['applicable']}**" for s in subs]
    rows.append("| Superados / aplicables | " + " | ".join(totals) + " |")
    out = [
        "\n".join(rows),
        "",
        "✅ cumple la especificación · ❌ no la cumple · — no aplicable al agente · ⚠️ error del runner",
    ]
    if skipped:
        out += ["", "No medidos: " + "; ".join(f"**{s['label']}** ({s.get('skipped_reason')})" for s in skipped)]

    details = []
    for s in subs:
        for name, r in s["scenarios"].items():
            if r["status"] in ("fail", "error"):
                obs = (r.get("runs") or [{}])[-1].get("observed", {})
                failed = [c for c in (r.get("runs") or [{}])[-1].get("checks", []) if not c["ok"]]
                why = "; ".join(f"{c['name']}: esperado {c['expected']}, observado {c['observed']}" for c in failed)
                why = why or r.get("reason", "")
                details.append(
                    f"- **{s['label']}** · `{name}`: {why} "
                    f"<sub>(exit {obs.get('exit_code')}, {obs.get('llm_requests')} peticiones, "
                    f"{round(obs.get('wall_ms', 0))} ms)</sub>"
                )
    if details:
        out += ["", "<details><summary>Por qué falla cada caso</summary>", "", *details, "", "</details>"]
    return "\n".join(out)


def _usd(x: float) -> str:
    return f"${x:.4f}" if x < 1 else f"${x:.2f}"


def render_tasks(result: dict) -> str:
    """Summary per agent·model, then the task × subject matrix (passes out of k)."""
    k = result["params"].get("k")
    subs = [s for s in result["subjects"] if s.get("metrics")]
    skipped = [s for s in result["subjects"] if not s.get("metrics")]
    rows = [
        f"| Agente · modelo | Versión | pass@1 | pass^{k} | Coste total | Coste / ejecución "
        "| Tokens entrada / salida | Peticiones / ejecución | Tiempo del agente p50 (s) |",
        "|---|---|---:|---:|---:|---:|---:|---:|---:|",
    ]
    for s in sorted(subs, key=lambda s: -s["metrics"]["pass_at_1"]):
        m = s["metrics"]
        tok = m["tokens"]
        tin = tok["input"] + tok["cache_read"] + tok["cache_write"]
        per = m["requests"] / m["trials"] if m["trials"] else 0
        warn = " ⚠" if m["estimated_cost_requests"] or m["unattributed_requests"] or m["errors"] else ""
        rows.append(
            f"| {s['label']}{warn} | {s.get('version') or ''} | {m['pass_at_1']:.0%} | {m['pass_hat_k']:.0%} | "
            f"{_usd(m['cost_usd'])} | {_usd(m['cost_per_trial_usd'])} | {tin:,} / {tok['output']:,} | "
            f"{per:.1f} | {m['agent_s_p50'] if m['agent_s_p50'] is not None else '-'} |"
        )
    out = ["\n".join(rows)]
    if subs:
        head = "| Tarea | " + " | ".join(s["label"] for s in subs) + " |"
        matrix = [head, "|---|" + ":---:|" * len(subs)]
        for t in result.get("tasks", []):
            cells = []
            for s in subs:
                e = s["tasks"].get(t["name"])
                cells.append(f"{e['passed']}/{len(e['rewards'])}" if e else "-")
            matrix.append(f"| `{t['name']}` <sub>{t['category']}</sub> | " + " | ".join(cells) + " |")
        out += ["", "\n".join(matrix)]
    out += [
        "",
        f"pass@1 = media de aciertos sobre las {k} ejecuciones de cada tarea · "
        f"pass^{k} = tareas resueltas en las {k} ejecuciones (fiabilidad) · "
        "coste medido por el proxy `meter` con `suites/tasks/pricing.json` · "
        "un ⚠ junto al agente indica errores, costes estimados o peticiones sin atribuir (detalle en el JSON).",
    ]
    if skipped:
        out += ["", "No medidos: " + "; ".join(f"**{s['label']}** ({s.get('skipped_reason')})" for s in skipped)]
    return "\n".join(out)


def render_result(result: dict) -> str:
    m = result["machine"]
    if result["suite"] == "tasks":
        p = result["params"]
        return "\n".join(
            [
                f"### `{m['platform']}` · modelos {p.get('tier')} · upstream {p.get('upstream')}",
                "",
                f"{_commit(result)} · {result['created_at']} · k={p.get('k')} · Harbor {p.get('harbor') or '?'} · "
                f"gasto {_usd(result.get('spent_usd', 0))} de {_usd(p.get('budget_usd', 0))}",
                "",
                render_tasks(result),
                "",
            ]
        )
    if result["suite"] == "harness":
        return "\n".join(
            [
                f"### `{m['platform']}`",
                "",
                f"{_commit(result)} · {result['created_at']} · "
                f"{result['params'].get('runs_per_scenario')} ejecución(es) por escenario",
                "",
                "Escenarios y su porqué: [`../docs/harness-scenarios.md`](../docs/harness-scenarios.md).",
                "",
                render_harness(result),
                "",
            ]
        )
    p = result["params"]
    ref = " (referencia)" if m["platform"] == REFERENCE_PLATFORM else ""
    lines = [
        f"### `{m['platform']}`{ref}",
        "",
        f"{_commit(result)} · "
        f"{result['created_at']} · {m.get('cpu')} · {m.get('cores')} cores · "
        f"{'CI' if m.get('ci') else 'local'} · "
        f"{p.get('rounds')}×{p.get('runs_per_round')} runs (+{p.get('warmup')} warmup)",
        "",
        render_table(result),
        "",
        render_chart(result),
        "",
    ]
    if result["suite"] == "perf-hello":
        lines += [render_chart(result, "request_bytes", "Bytes enviados al modelo"), ""]
    return "\n".join(lines)


def write_readme(root: Path = PUBLISHED_DIR) -> Path:
    parts = [
        "# Resultados publicados",
        "",
        "> Fichero generado por `flores-bench publish` / `flores-bench report`. No editar a mano.",
        "> Fuente de verdad: los `latest.json` / `history.jsonl` de cada carpeta.",
        "> Metodología: [`../docs/methodology.md`](../docs/methodology.md).",
        "",
        "p50 = mediana; entre corchetes, intervalo de confianza del 95 % de la mediana (bootstrap).",
        "",
    ]
    for suite_dir in sorted(p for p in root.iterdir() if p.is_dir()):
        latest = sorted(
            suite_dir.glob("*/latest.json"),
            key=lambda f: (f.parent.name != REFERENCE_PLATFORM, f.parent.name),
        )
        if not latest:
            continue
        parts += [f"## {SUITE_TITLES.get(suite_dir.name, suite_dir.name)}", ""]
        for f in latest:
            parts.append(render_result(json.loads(f.read_text())))
    out = root / "README.md"
    out.write_text("\n".join(parts).rstrip() + "\n")
    return out
