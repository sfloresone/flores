"""Collect the metadata that makes a result reproducible and comparable."""

from __future__ import annotations

import os
import platform
import shutil
import subprocess
import sys
from datetime import UTC, datetime

from flores_bench.paths import REPO_ROOT


def _run(cmd: list[str]) -> str | None:
    try:
        out = subprocess.run(cmd, capture_output=True, text=True, timeout=20, cwd=REPO_ROOT)
    except (OSError, subprocess.TimeoutExpired):
        return None
    if out.returncode != 0:
        return None
    return (out.stdout or out.stderr).strip() or None


def now_iso() -> str:
    return datetime.now(UTC).strftime("%Y-%m-%dT%H:%M:%SZ")


def platform_key() -> str:
    """e.g. linux-x86_64, darwin-arm64. Published results are grouped by it."""
    machine = platform.machine().lower()
    machine = {"amd64": "x86_64", "aarch64": "arm64"}.get(machine, machine)
    return f"{platform.system().lower()}-{machine}"


def _cpu_model() -> str | None:
    if sys.platform == "darwin":
        return _run(["sysctl", "-n", "machdep.cpu.brand_string"])
    try:
        with open("/proc/cpuinfo") as f:
            for line in f:
                if line.startswith("model name"):
                    return line.split(":", 1)[1].strip()
    except OSError:
        pass
    return platform.processor() or None


def _mem_bytes() -> int | None:
    if sys.platform == "darwin":
        v = _run(["sysctl", "-n", "hw.memsize"])
        return int(v) if v and v.isdigit() else None
    try:
        return os.sysconf("SC_PAGE_SIZE") * os.sysconf("SC_PHYS_PAGES")
    except (ValueError, OSError):
        return None


def git_info() -> dict:
    sha = _run(["git", "rev-parse", "HEAD"])
    return {
        "sha": sha,
        "short": sha[:10] if sha else None,
        "branch": _run(["git", "rev-parse", "--abbrev-ref", "HEAD"]),
        "dirty": bool(_run(["git", "status", "--porcelain"])),
    }


def machine_info() -> dict:
    return {
        "platform": platform_key(),
        "os": platform.system(),
        "os_release": platform.release(),
        "cpu": _cpu_model(),
        "cores": os.cpu_count(),
        "mem_bytes": _mem_bytes(),
        "ci": bool(os.environ.get("CI")),
        # Only the CI runner name: local hostnames are personal and end up in a public repo.
        "runner": os.environ.get("RUNNER_NAME"),
    }


def tool_versions() -> dict:
    def first_line(cmd: list[str]) -> str | None:
        if not shutil.which(cmd[0]):
            return None
        v = _run(cmd)
        return v.splitlines()[0] if v else None

    return {
        "python": platform.python_version(),
        "go": first_line(["go", "version"]),
        "hyperfine": first_line(["hyperfine", "--version"]),
        "docker": first_line(["docker", "--version"]),
    }


def result_header(suite: str, params: dict) -> dict:
    return {
        "schema_version": "result/v0",
        "suite": suite,
        "created_at": now_iso(),
        "git": git_info(),
        "machine": machine_info(),
        "tools": tool_versions(),
        "params": params,
    }
