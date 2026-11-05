"""Load suite configuration (agents.toml, budgets.toml)."""

from __future__ import annotations

import tomllib
from dataclasses import dataclass

from flores_bench.adapters import ADAPTERS, Adapter
from flores_bench.paths import SUITES_DIR


@dataclass
class AgentConfig:
    adapter: Adapter
    model: str
    pinned_version: str | None
    npm: str | None


def load_agents(selected: list[str] | None = None) -> list[AgentConfig]:
    data = tomllib.loads((SUITES_DIR / "perf" / "agents.toml").read_text())["agents"]
    names = selected or list(data)
    out = []
    for name in names:
        if name not in ADAPTERS:
            raise SystemExit(f"unknown agent {name!r}; known: {', '.join(ADAPTERS)}")
        cfg = data.get(name, {})
        out.append(AgentConfig(ADAPTERS[name], cfg.get("model", "mock-model"), cfg.get("version"), cfg.get("npm")))
    return out


def load_budgets() -> dict[str, dict[str, dict[str, float]]]:
    return tomllib.loads((SUITES_DIR / "perf" / "budgets.toml").read_text())
