"""Layer B scenarios: load, validate and render them for one agent.

A scenario file (scenarios/harness/<name>.json) has three parts:

* `mock`   - the script the mock provider follows (mockprovider Scenario).
             Tool calls may use canonical forms that each adapter renders
             into its own tool name and argument schema:
               {"shell": "echo hi"}             -> e.g. Bash {"command": "echo hi"}
               {"shell_raw": "{\\"command\\": "} -> shell tool with these raw, broken arguments
               {"name": "x", "arguments": {...}} -> sent as is
* `expect` - what a correct harness does, judged only from the outside
             (exit code, stdout, requests seen by the mock), so the same
             scenario can judge flores and every competitor.
* metadata - title, why, category, prompt, timeout_s, requires.

The expectations are the specification for flores: TDD for harness behaviour.
"""

from __future__ import annotations

import copy
import json
from dataclasses import dataclass, field
from pathlib import Path

from flores_bench.adapters import Adapter, Unsupported
from flores_bench.paths import SCENARIOS_DIR

HARNESS_DIR = SCENARIOS_DIR / "harness"

EXPECT_KEYS = {
    "exit",  # "zero" | "nonzero" | "any"
    "stdout_contains",  # [str]
    "llm_requests",  # {"min": n, "max": n}
    "retries",  # {"min": n, "max": n}  requests beyond the first one of each turn
    "tool_results",  # {"min": n, "max": n} max tool outputs sent back in one request
    "request_contains",  # [str] some request body sent to the model contains it
    "max_wall_ms",  # number
    "completes",  # bool (default true): the agent exits before timeout_s
}


@dataclass
class Scenario:
    name: str
    title: str
    category: str
    why: str
    prompt: str
    timeout_s: float
    requires: list[str]
    mock: dict
    expect: dict
    path: Path | None = field(default=None, repr=False)

    @classmethod
    def from_dict(cls, d: dict, path: Path | None = None) -> Scenario:
        missing = {"name", "title", "category", "why", "mock", "expect"} - d.keys()
        if missing:
            raise ValueError(f"{path or d.get('name')}: missing {sorted(missing)}")
        unknown = set(d["expect"]) - EXPECT_KEYS
        if unknown:
            raise ValueError(f"{path or d['name']}: unknown expect keys {sorted(unknown)}")
        return cls(
            name=d["name"],
            title=d["title"],
            category=d["category"],
            why=d["why"],
            prompt=d.get("prompt", "Follow the plan."),
            timeout_s=float(d.get("timeout_s", 60)),
            requires=list(d.get("requires", [])),
            mock=d["mock"],
            expect=d["expect"],
            path=path,
        )


CATEGORY_ORDER = ["basics", "tools", "provider-errors", "streaming", "limits"]


def load_all(directory: Path = HARNESS_DIR) -> list[Scenario]:
    scenarios = [Scenario.from_dict(json.loads(p.read_text()), p) for p in sorted(directory.glob("*.json"))]
    for s in scenarios:
        if s.category not in CATEGORY_ORDER:
            raise ValueError(f"{s.path}: unknown category {s.category!r}; known: {CATEGORY_ORDER}")
    scenarios.sort(key=lambda s: (CATEGORY_ORDER.index(s.category), s.name))
    names = [s.name for s in scenarios]
    dupes = {n for n in names if names.count(n) > 1}
    if dupes:
        raise ValueError(f"duplicate scenario names: {sorted(dupes)}")
    return scenarios


def render_mock(scenario: Scenario, adapter: Adapter) -> dict:
    """Return the mockprovider scenario for this agent.

    Raises Unsupported when the scenario needs a capability (e.g. the shell
    tool mapping) the adapter does not provide.
    """
    if "shell" in scenario.requires:
        adapter.shell_call("true")  # raises Unsupported if unknown
    mock = copy.deepcopy(scenario.mock)
    mock.setdefault("name", scenario.name)
    for step in mock.get("steps", []):
        rendered = []
        for tc in step.get("tool_calls", []):
            if "shell" in tc:
                name, args = adapter.shell_call(tc["shell"])
                rendered.append({"name": name, "arguments": args})
            elif "shell_raw" in tc:
                name, _ = adapter.shell_call("true")
                rendered.append({"name": name, "raw_arguments": tc["shell_raw"]})
            else:
                rendered.append(tc)
        if rendered:
            step["tool_calls"] = rendered
    return mock


__all__ = ["HARNESS_DIR", "Scenario", "Unsupported", "load_all", "render_mock"]


CATEGORY_TITLES = {
    "basics": "Básicos",
    "tools": "Tools",
    "provider-errors": "Errores del proveedor",
    "streaming": "Streaming",
    "limits": "Límites",
}


def _expect_text(expect: dict) -> str:
    parts = []
    rule = expect.get("exit", "zero")
    parts.append({"zero": "sale con 0", "nonzero": "sale con error", "any": "cualquier código de salida"}[rule])
    if expect.get("completes", True):
        parts.append("termina antes del timeout")
    for k, label in (("llm_requests", "peticiones"), ("retries", "reintentos"), ("tool_results", "resultados de tool")):
        if k in expect:
            lo, hi = expect[k].get("min"), expect[k].get("max")
            if lo is not None and lo == hi:
                parts.append(f"{label} = {lo}")
            else:
                parts.append(
                    f"{label} "
                    + " y ".join(
                        x for x in (f"≥ {lo}" if lo is not None else "", f"≤ {hi}" if hi is not None else "") if x
                    )
                )
    for n in expect.get("stdout_contains", []):
        parts.append(f"stdout contiene «{n[:40]}»")
    for n in expect.get("request_contains", []):
        parts.append(f"el modelo recibe «{n}»")
    if "max_wall_ms" in expect:
        parts.append(f"≤ {expect['max_wall_ms'] / 1000:g} s")
    return "; ".join(parts)


def render_docs(scenarios: list[Scenario]) -> str:
    lines = [
        "# Escenarios de la capa B (comportamiento del harness)",
        "",
        "> Generado con `uv run flores-bench scenarios --write-docs` a partir de `scenarios/harness/*.json`.",
        "> No editar a mano: edita el JSON del escenario.",
        "",
        "Cada escenario es un guion para el mock provider más lo que un harness correcto",
        "debe hacer, comprobado **solo desde fuera** (código de salida, stdout y las",
        "peticiones que ve el mock). Por eso el mismo escenario juzga a flores y a cualquier",
        "competidor. Las expectativas son la especificación de flores: el TDD de comportamiento.",
        "",
        'Las tool calls se escriben de forma canónica (`{"shell": "…"}`) y cada adaptador',
        "las traduce a su tool (`Bash` en Claude Code, `bash` en Pi, `shell` en fx). Los",
        "comandos imprimen un marcador calculado (`echo flores-$((40+2))` → `flores-42`) que",
        "solo aparece si el comando se ejecutó de verdad y su salida volvió al modelo.",
        "",
    ]
    for cat in CATEGORY_ORDER:
        group = [s for s in scenarios if s.category == cat]
        if not group:
            continue
        lines += [f"## {CATEGORY_TITLES[cat]}", ""]
        for s in group:
            req = " · requiere tool de shell" if "shell" in s.requires else ""
            lines += [
                f"### `{s.name}`: {s.title}",
                "",
                f"**Por qué:** {s.why}",
                "",
                f"**Se espera:** {_expect_text(s.expect)}. <sub>timeout {s.timeout_s:g} s{req}</sub>",
                "",
            ]
    return "\n".join(lines).rstrip() + "\n"
