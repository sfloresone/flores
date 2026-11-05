"""Run one agent process and measure it (wall, CPU, peak RSS, exit, output)."""

from __future__ import annotations

import os
import subprocess
import tempfile
import time
from dataclasses import dataclass
from pathlib import Path


@dataclass
class ProcResult:
    exit_code: int
    timed_out: bool
    wall_ms: float
    cpu_ms: float
    rss_mb: float
    stdout: str
    stderr: str


def _rss_bytes(ru_maxrss: int) -> int:
    # Linux reports KiB, macOS reports bytes.
    return ru_maxrss if os.uname().sysname == "Darwin" else ru_maxrss * 1024


def run_measured(argv: list[str], env: dict[str, str], cwd: Path, timeout_s: float) -> ProcResult:
    """Run to completion (or kill at timeout) and collect rusage via os.wait4.

    Output goes to temp files, not pipes, so a chatty agent can never block on
    a full pipe buffer and distort the timing.
    """
    with tempfile.TemporaryDirectory(prefix="flores-proc-") as tmp:
        out_path, err_path = Path(tmp, "stdout"), Path(tmp, "stderr")
        timed_out = False
        with open(out_path, "w") as out, open(err_path, "w") as err:
            t0 = time.perf_counter()
            proc = subprocess.Popen(argv, cwd=cwd, env=env, stdout=out, stderr=err, stdin=subprocess.DEVNULL)
            deadline = t0 + timeout_s
            while True:
                pid, status, ru = os.wait4(proc.pid, os.WNOHANG)
                if pid:
                    break
                if time.perf_counter() > deadline:
                    timed_out = True
                    proc.kill()
                    pid, status, ru = os.wait4(proc.pid, 0)
                    break
                time.sleep(0.0005)
            wall = time.perf_counter() - t0
        return ProcResult(
            exit_code=os.waitstatus_to_exitcode(status),
            timed_out=timed_out,
            wall_ms=wall * 1000,
            cpu_ms=(ru.ru_utime + ru.ru_stime) * 1000,
            rss_mb=_rss_bytes(ru.ru_maxrss) / 2**20,
            stdout=out_path.read_text(errors="replace"),
            stderr=err_path.read_text(errors="replace"),
        )
