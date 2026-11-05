"""Run hyperfine and normalise its JSON export (1.x and 2.x formats)."""

from __future__ import annotations

import json
import shutil
import subprocess
import tempfile
from pathlib import Path


def require() -> str:
    exe = shutil.which("hyperfine")
    if not exe:
        raise SystemExit(
            "hyperfine is not installed:\n  brew install hyperfine  (macOS)\n  apt install hyperfine   (Debian/Ubuntu)"
        )
    return exe


def parse_export(data: dict) -> dict[str, dict[str, list[float]]]:
    """Return {command: {"wall_s": [...], "user_s": [...], "sys_s": [...], "rss_bytes": [...],
    "exit_codes": [...]}} for either export format.

    hyperfine 1.x: results[].times / user / system / memory_usage_byte / exit_codes
    hyperfine 2.x: results[].measurements[] with time_wall_clock, time_user, ... objects
    """
    out: dict[str, dict[str, list[float]]] = {}
    for r in data["results"]:
        cols: dict[str, list[float]] = {"wall_s": [], "user_s": [], "sys_s": [], "rss_bytes": [], "exit_codes": []}
        if "measurements" in r:
            for m in r["measurements"]:
                cols["wall_s"].append(m["time_wall_clock"]["value"])
                cols["user_s"].append(m.get("time_user", {}).get("value", 0.0))
                cols["sys_s"].append(m.get("time_system", {}).get("value", 0.0))
                if "memory_peak_resident" in m:
                    cols["rss_bytes"].append(m["memory_peak_resident"]["value"])
                cols["exit_codes"].append(m.get("exit_code", 0))
        else:
            cols["wall_s"] = list(r.get("times", []))
            n = len(cols["wall_s"])
            # 1.x stores only aggregate user/system time.
            cols["user_s"] = [r.get("user", 0.0)] * n
            cols["sys_s"] = [r.get("system", 0.0)] * n
            cols["rss_bytes"] = list(r.get("memory_usage_byte", []))
            cols["exit_codes"] = list(r.get("exit_codes", [0] * n))
        # 2.x keeps --command-name in "name"; 1.x replaces "command" with it.
        out[r.get("name") or r["command"]] = cols
    return out


def run(
    commands: dict[str, list[str]], runs: int, warmup: int, env: dict[str, str] | None = None
) -> dict[str, dict[str, list[float]]]:
    """Benchmark named argv lists in one hyperfine invocation (no shell, -N)."""
    exe = require()
    with tempfile.TemporaryDirectory() as tmp:
        export = Path(tmp) / "hf.json"
        cmd = [exe, "-N", "--style", "none", "--runs", str(runs), "--warmup", str(warmup), "--export-json", str(export)]
        for name, argv in commands.items():
            cmd += ["--command-name", name, _join(argv)]
        subprocess.run(cmd, check=True, env=env, stdout=subprocess.DEVNULL)
        return parse_export(json.loads(export.read_text()))


def _join(argv: list[str]) -> str:
    # hyperfine -N splits on whitespace with shell-like quoting.
    def q(a: str) -> str:
        return a if a and all(c.isalnum() or c in "/._-=:@+," for c in a) else "'" + a.replace("'", "'\\''") + "'"

    return " ".join(q(a) for a in argv)
