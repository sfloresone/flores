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
}

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


def render_result(result: dict) -> str:
    m = result["machine"]
    p = result["params"]
    ref = " (referencia)" if m["platform"] == REFERENCE_PLATFORM else ""
    lines = [
        f"### `{m['platform']}`{ref}",
        "",
        f"Commit `{result['git'].get('short') or 'sin commit'}`{' (dirty)' if result['git'].get('dirty') else ''} · "
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
