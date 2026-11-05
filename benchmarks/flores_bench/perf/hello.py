"""perf-hello: one trivial prompt end-to-end against the mock provider.

Same idea as fx's competitive libfx benchmark: the model's answer is scripted
and instant, so what is left is pure harness overhead. Per run we record

* wall/cpu time and peak RSS of the agent process (rusage, via tools/measure),
* what the harness sent to the model, as seen by the mock: number of
  requests, total request bytes, system-prompt bytes and declared tools,
* `harness_ms` = wall time minus the time the mock spent answering.

Runs cannot use hyperfine because every run needs its own fresh config
directory and the mock's request log must be read between runs.
"""

from __future__ import annotations

import shutil
import tempfile
from pathlib import Path

from flores_bench.adapters import RunContext, Unsupported
from flores_bench.config import AgentConfig
from flores_bench.envinfo import result_header
from flores_bench.mock import MockProvider
from flores_bench.perf.common import Subject, round_orders, sanitized_env
from flores_bench.process import run_measured
from flores_bench.stats import summarize

SUITE = "perf-hello"
PROMPT = "hello world"
EXPECTED = "Hello! Nice to meet you."
TIMEOUT_S = 120


def run_once(agent: AgentConfig, mock: MockProvider) -> dict:
    with tempfile.TemporaryDirectory(prefix="flores-hello-") as tmp:
        cfg_dir, ws = Path(tmp, "config"), Path(tmp, "ws")
        cfg_dir.mkdir()
        ws.mkdir()
        inv = agent.adapter.invoke(RunContext(mock.url, cfg_dir, ws, PROMPT, agent.model))
        mock.reset()
        proc = run_measured(inv.argv, sanitized_env(inv.env), ws, TIMEOUT_S)
    records = mock.requests()
    llm = [r for r in records if r.get("protocol")]
    mock_ms = sum(r.get("duration_ms", 0) for r in records)
    return {
        "exit_code": proc.exit_code,
        "ok": proc.exit_code == 0 and EXPECTED in proc.stdout,
        "wall_ms": proc.wall_ms,
        "cpu_ms": proc.cpu_ms,
        "rss_mb": proc.rss_mb,
        "rss_floor_mb": proc.rss_floor_mb,
        "harness_ms": max(0.0, proc.wall_ms - mock_ms),
        "requests": len(records),
        "llm_requests": len(llm),
        "request_bytes": sum(r.get("request_bytes", 0) for r in llm),
        "system_bytes": max((r.get("system_bytes", 0) for r in llm), default=0),
        "tools": max((r.get("tools", 0) for r in llm), default=0),
        "protocols": sorted({r["protocol"] for r in llm}),
        "unknown_paths": sorted({r["path"] for r in records if not r.get("protocol")}),
        "stderr_tail": proc.stderr[-300:] if proc.exit_code else "",
    }


NUMERIC = (
    "wall_ms",
    "cpu_ms",
    "rss_mb",
    "rss_floor_mb",
    "harness_ms",
    "request_bytes",
    "system_bytes",
    "llm_requests",
    "tools",
)


def run(agents: list[AgentConfig], runs: int, warmup: int, rounds: int, seed: int = 0, log=print) -> dict:
    subjects = {a.adapter.name: Subject.from_agent(a) for a in agents}
    by_name = {a.adapter.name: a for a in agents}
    runnable = []
    for name, s in subjects.items():
        if not s.available:
            continue
        try:  # probe: does the adapter support this workload at all?
            with tempfile.TemporaryDirectory() as tmp:
                by_name[name].adapter.invoke(RunContext("http://x", Path(tmp), Path(tmp), PROMPT, s.model or ""))
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
