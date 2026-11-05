"""Thin wrapper around the Harbor CLI (pinned in the `tasks` dependency group)."""

from __future__ import annotations

import json
import os
import re
import shutil
import subprocess
import sys
from dataclasses import dataclass
from datetime import datetime
from pathlib import Path

from flores_bench.adapters.base import NESTING_ENV, SECRET_ENV
from flores_bench.tasks.meter import KEY_ENV

HINT = "Harbor is not installed: run with `uv run --group tasks flores-bench ...`"


def harbor_bin() -> str:
    candidate = Path(sys.executable).parent / "harbor"
    if candidate.exists():
        return str(candidate)
    found = shutil.which("harbor")
    if not found:
        raise SystemExit(HINT)
    return found


def harbor_version() -> str | None:
    try:
        out = subprocess.run([harbor_bin(), "--version"], capture_output=True, text=True, timeout=60)
        return out.stdout.strip() or None
    except (SystemExit, OSError, subprocess.TimeoutExpired):
        return None


def harbor_env(extra: dict[str, str] | None = None) -> dict[str, str]:
    """Environment for the Harbor process: no real keys, ever.

    Harbor collects ambient provider keys for its agents; stripping them makes
    the meter token the only credential that can reach a container.
    """
    drop = set(SECRET_ENV) | set(NESTING_ENV) | set(KEY_ENV)
    env = {k: v for k, v in os.environ.items() if k not in drop and not k.startswith("CF_")}
    env.update(extra or {})
    return env


@dataclass
class Trial:
    task: str  # e.g. "fix-off-by-one"
    trial: str
    reward: float | None
    agent_start: datetime | None
    agent_end: datetime | None
    exception: str | None

    @property
    def passed(self) -> bool:
        return self.reward is not None and self.reward >= 1.0

    @property
    def agent_seconds(self) -> float | None:
        if self.agent_start and self.agent_end:
            return (self.agent_end - self.agent_start).total_seconds()
        return None


_FRACTION = re.compile(r"\.(\d+)")


def parse_time(value: str | None) -> datetime | None:
    """Parse RFC 3339 timestamps from Harbor (µs) and the meter (Go writes ns)."""
    if not value:
        return None
    value = _FRACTION.sub(lambda m: "." + m.group(1)[:6], value.replace("Z", "+00:00"), count=1)
    return datetime.fromisoformat(value)


def run_job(
    dataset: Path,
    agent: str,
    jobs_dir: Path,
    job_name: str,
    *,
    model: str | None = None,
    attempts: int = 1,
    concurrent: int = 1,
    include: list[str] | None = None,
    extra: list[str] | None = None,
    env: dict[str, str] | None = None,
) -> Path:
    cmd = [harbor_bin(), "run", "-p", str(dataset), "-a", agent, "-k", str(attempts), "-n", str(concurrent)]
    cmd += ["-o", str(jobs_dir), "--job-name", job_name]
    if model:
        cmd += ["-m", model]
    for name in include or []:
        cmd += ["-i", name]
    cmd += extra or []
    subprocess.run(cmd, env=harbor_env(env), check=False)
    job = jobs_dir / job_name
    if not (job / "result.json").exists():
        raise RuntimeError(f"harbor job {job_name} produced no result.json (see {job}/job.log)")
    return job


def parse_job(job: Path) -> list[Trial]:
    trials = []
    for f in sorted(job.glob("*/result.json")):
        r = json.loads(f.read_text())
        rewards = (r.get("verifier_result") or {}).get("rewards") or {}
        exc = r.get("exception_info")
        ae = r.get("agent_execution") or {}
        trials.append(
            Trial(
                task=(r.get("task_name") or "").split("/")[-1],
                trial=r.get("trial_name") or f.parent.name,
                reward=rewards.get("reward"),
                agent_start=parse_time(ae.get("started_at")),
                agent_end=parse_time(ae.get("finished_at")),
                exception=(
                    f"{exc.get('exception_type', '')}: {exc.get('exception_message', '')}"[:300]
                    if isinstance(exc, dict)
                    else (str(exc)[:300] if exc else None)
                ),
            )
        )
    return trials
