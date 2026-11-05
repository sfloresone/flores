"""perf-hello: one trivial prompt end-to-end against the mock provider.

Same idea as fx's competitive libfx benchmark: the model's answer is scripted
and instant, so what is left is pure harness overhead. Per run we record

* wall/cpu time and peak RSS of the agent process (os.wait4 rusage),
* what the harness sent to the model, as seen by the mock: number of
  requests, total request bytes, system-prompt bytes and declared tools,
* `harness_ms` = wall time minus the time the mock spent answering.

Runs cannot use hyperfine because every run needs its own fresh config
directory and the mock's request log must be read between runs.
"""

from __future__ import annotations

import os
import shutil
import subprocess
import tempfile
import time
from pathlib import Path

from flores_bench.adapters import HelloContext, Unsupported
from flores_bench.config import AgentConfig
from flores_bench.envinfo import result_header
from flores_bench.mock import MockProvider
from flores_bench.perf.common import Subject, round_orders, sanitized_env
from flores_bench.stats import summarize

SUITE = "perf-hello"
PROMPT = "hello world"
EXPECTED = "Hello! Nice to meet you."
TIMEOUT_S = 120


def _rss_bytes(ru_maxrss: int) -> int:
    # Linux reports KiB, macOS reports bytes.
    return ru_maxrss if os.uname().sysname == "Darwin" else ru_maxrss * 1024


def run_once(agent: AgentConfig, mock: MockProvider) -> dict:
    with tempfile.TemporaryDirectory(prefix="flores-hello-") as tmp:
        cfg_dir, ws = Path(tmp, "config"), Path(tmp, "ws")
        cfg_dir.mkdir()
        ws.mkdir()
        inv = agent.adapter.hello(HelloContext(mock.url, cfg_dir, ws, PROMPT, agent.model))
        env = sanitized_env(inv.env)
        mock.reset()
        out_path = Path(tmp, "stdout")
        err_path = Path(tmp, "stderr")
        with open(out_path, "w") as out, open(err_path, "w") as err:
            t0 = time.perf_counter()
            proc = subprocess.Popen(
                inv.argv,
                cwd=ws,
                env=env,
                stdout=out,
                stderr=err,
                stdin=subprocess.DEVNULL if inv.stdin_devnull else None,
            )
            deadline = t0 + TIMEOUT_S
            while True:
                pid, status, ru = os.wait4(proc.pid, os.WNOHANG)
                if pid:
                    break
                if time.perf_counter() > deadline:
                    proc.kill()
                    pid, status, ru = os.wait4(proc.pid, 0)
                    break
                time.sleep(0.0005)
            wall = time.perf_counter() - t0
        proc.returncode = os.waitstatus_to_exitcode(status)
        stdout = out_path.read_text(errors="replace")
        stderr = err_path.read_text(errors="replace")
    records = mock.requests()
    llm = [r for r in records if r.get("protocol")]
    mock_ms = sum(r.get("duration_ms", 0) for r in records)
    return {
        "exit_code": proc.returncode,
        "ok": proc.returncode == 0 and EXPECTED in stdout,
        "wall_ms": wall * 1000,
        "cpu_ms": (ru.ru_utime + ru.ru_stime) * 1000,
        "rss_mb": _rss_bytes(ru.ru_maxrss) / 2**20,
        "harness_ms": max(0.0, wall * 1000 - mock_ms),
        "requests": len(records),
        "llm_requests": len(llm),
        "request_bytes": sum(r.get("request_bytes", 0) for r in llm),
        "system_bytes": max((r.get("system_bytes", 0) for r in llm), default=0),
        "tools": max((r.get("tools", 0) for r in llm), default=0),
        "protocols": sorted({r["protocol"] for r in llm}),
        "unknown_paths": sorted({r["path"] for r in records if not r.get("protocol")}),
        "stderr_tail": stderr[-300:] if proc.returncode else "",
    }


NUMERIC = ("wall_ms", "cpu_ms", "rss_mb", "harness_ms", "request_bytes", "system_bytes", "llm_requests", "tools")


def run(agents: list[AgentConfig], runs: int, warmup: int, rounds: int, seed: int = 0, log=print) -> dict:
    subjects = {a.adapter.name: Subject.from_agent(a) for a in agents}
    by_name = {a.adapter.name: a for a in agents}
    runnable = []
    for name, s in subjects.items():
        if not s.available:
            continue
        try:  # probe: does the adapter support this workload at all?
            with tempfile.TemporaryDirectory() as tmp:
                by_name[name].adapter.hello(HelloContext("http://x", Path(tmp), Path(tmp), PROMPT, s.model or ""))
            runnable.append(name)
        except Unsupported as e:
            s.available, s.skipped_reason = False, str(e)

    with MockProvider() as mock:
        warmed: set[str] = set()
        for r_i, order in enumerate(round_orders(runnable, rounds, seed), 1):
            for name in order:
                agent = by_name[name]
                if name not in warmed:
                    for _ in range(warmup):
                        run_once(agent, mock)
                    warmed.add(name)
                for _ in range(runs):
                    subjects[name].samples.append(run_once(agent, mock))
                log(f"  round {r_i}/{rounds}  {name:<12} {len(subjects[name].samples)} samples")

    for s in subjects.values():
        if not s.samples:
            continue
        good = [x for x in s.samples if x["ok"]]
        s.metrics = {k: summarize([x[k] for x in good]) for k in NUMERIC} if good else {}
        s.info = {
            "failures": len(s.samples) - len(good),
            "protocols": sorted({p for x in s.samples for p in x["protocols"]}),
            "unknown_paths": sorted({p for x in s.samples for p in x["unknown_paths"]}),
            "binary": Path(shutil.which(by_name[s.name].adapter.binary) or by_name[s.name].adapter.binary).name,
        }
        if not good:
            s.skipped_reason = "all runs failed: " + (s.samples[-1]["stderr_tail"] or "unexpected output")

    header = result_header(
        SUITE,
        {
            "runs_per_round": runs,
            "warmup": warmup,
            "rounds": rounds,
            "seed": seed,
            "prompt": PROMPT,
            "scenario": "hello (built-in)",
        },
    )
    return {**header, "subjects": [s.to_json() for s in subjects.values()]}
