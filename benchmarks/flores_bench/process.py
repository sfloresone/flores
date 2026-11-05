"""Run one agent process and measure it (wall, CPU, peak RSS, exit, output).

The agent is launched by `tools/measure` (Go), not directly from Python. On
Linux a child's peak RSS includes the RSS of the process that exec'd it, so a
direct launch from this Python process put a ~24 MiB floor under every agent.
`measure` keeps that floor at ~3 MiB and reports it (`rss_floor_mb`), so a
value that cannot be told apart from the floor is shown as "≤ floor".
See docs/metrics.md (RSS).
"""

from __future__ import annotations

import json
import subprocess
import tempfile
from dataclasses import dataclass
from pathlib import Path

from flores_bench.paths import MEASURE_BIN


@dataclass
class ProcResult:
    exit_code: int
    timed_out: bool
    wall_ms: float
    cpu_ms: float
    rss_mb: float
    rss_floor_mb: float  # peak RSS the launcher passes on at exec (0 on macOS)
    stdout: str
    stderr: str


def run_measured(argv: list[str], env: dict[str, str], cwd: Path, timeout_s: float) -> ProcResult:
    """Run to completion (or kill at timeout) under tools/measure.

    Output goes to temp files, not pipes, so a chatty agent can never block on
    a full pipe buffer and distort the timing.
    """
    if not MEASURE_BIN.exists():
        raise FileNotFoundError(f"{MEASURE_BIN} not found; run `flores-bench build` first")
    with tempfile.TemporaryDirectory(prefix="flores-proc-") as tmp:
        out_path, err_path = Path(tmp, "stdout"), Path(tmp, "stderr")
        cmd = [str(MEASURE_BIN), "-timeout", f"{timeout_s}s", "-dir", str(cwd)]
        cmd += ["-stdout", str(out_path), "-stderr", str(err_path), "--", *argv]
        done = subprocess.run(cmd, env=env, capture_output=True, text=True, stdin=subprocess.DEVNULL, check=False)
        try:
            r = json.loads(done.stdout)
        except json.JSONDecodeError as e:
            raise RuntimeError(f"measure failed ({done.returncode}): {done.stderr.strip()}") from e
        if r.get("error"):
            raise RuntimeError(f"measure could not run {argv[0]}: {r['error']}")
        return ProcResult(
            exit_code=r["exit_code"],
            timed_out=r["timed_out"],
            wall_ms=r["wall_ms"],
            cpu_ms=r["cpu_ms"],
            rss_mb=r["rss_kib"] / 1024,
            rss_floor_mb=r["rss_floor_kib"] / 1024,
            stdout=out_path.read_text(errors="replace"),
            stderr=err_path.read_text(errors="replace"),
        )
