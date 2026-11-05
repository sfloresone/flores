"""Shared pieces of the perf runners."""

from __future__ import annotations

import os
import random
import shutil
from dataclasses import dataclass, field

from flores_bench.adapters.base import NESTING_ENV, SECRET_ENV
from flores_bench.config import AgentConfig

PROCESS_BASELINE = "process-baseline"


def sanitized_env(extra: dict[str, str] | None = None) -> dict[str, str]:
    env = {k: v for k, v in os.environ.items() if k not in SECRET_ENV and k not in NESTING_ENV}
    env["NO_COLOR"] = "1"
    if extra:
        env.update(extra)
    return env


def baseline_argv() -> list[str]:
    """A process that does nothing: the floor of what start-up can cost here."""
    return [shutil.which("true") or "/usr/bin/true"]


def round_orders(names: list[str], rounds: int, seed: int) -> list[list[str]]:
    """Shuffle the order of subjects every round so machine-load drift (thermal
    throttling, background jobs) is spread across all of them, as fx does."""
    rng = random.Random(seed)
    orders = []
    for _ in range(rounds):
        order = list(names)
        rng.shuffle(order)
        orders.append(order)
    return orders


@dataclass
class Subject:
    """One measured agent (or the process baseline) in a result file."""

    name: str
    label: str
    available: bool
    version: str | None = None
    pinned_version: str | None = None
    model: str | None = None
    skipped_reason: str | None = None
    metrics: dict = field(default_factory=dict)
    info: dict = field(default_factory=dict)
    samples: list[dict] = field(default_factory=list)

    @classmethod
    def from_agent(cls, a: AgentConfig) -> Subject:
        available = a.adapter.available()
        return cls(
            name=a.adapter.name,
            label=a.adapter.label,
            available=available,
            version=a.adapter.version() if available else None,
            pinned_version=a.pinned_version,
            model=a.model,
            skipped_reason=None if available else f"{a.adapter.binary} not installed",
        )

    @property
    def version_mismatch(self) -> bool:
        return bool(self.pinned_version and self.version and self.version != self.pinned_version)

    def to_json(self) -> dict:
        d = {
            "name": self.name,
            "label": self.label,
            "available": self.available,
            "version": self.version,
            "pinned_version": self.pinned_version,
            "version_mismatch": self.version_mismatch,
            "model": self.model,
            "skipped_reason": self.skipped_reason,
            "metrics": self.metrics,
            "info": self.info,
        }
        if self.samples:
            d["samples"] = self.samples
        return d
