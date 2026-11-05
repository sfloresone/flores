"""perf-startup: how long does `<agent> --version` take, and how much memory?

This is the cheapest possible invocation of each harness. It isolates runtime
start-up cost (binary loading, JIT/VM boot, config parsing) from everything
the agent does afterwards. Time is measured with hyperfine, like fx's
startup.sh. Peak RSS comes from a separate short pass under tools/measure: on
Linux hyperfine's own RSS is inherited by every child at exec (see
docs/metrics.md), which would hide the small binaries we care about.
"""

from __future__ import annotations

import tempfile
from pathlib import Path

from flores_bench import hyperfine
from flores_bench.adapters import Unsupported
from flores_bench.config import AgentConfig
from flores_bench.envinfo import result_header
from flores_bench.perf.common import PROCESS_BASELINE, Subject, baseline_argv, round_orders, sanitized_env
from flores_bench.process import run_measured
from flores_bench.stats import summarize

SUITE = "perf-startup"
RSS_RUNS = 10  # peak RSS of `--version` barely varies; 10 runs per agent is plenty
RSS_TIMEOUT_S = 60


def run(agents: list[AgentConfig], runs: int, warmup: int, rounds: int, seed: int = 0) -> dict:
    subjects: dict[str, Subject] = {
        PROCESS_BASELINE: Subject(PROCESS_BASELINE, "process baseline (true)", True, version="-"),
    }
    commands: dict[str, list[str]] = {PROCESS_BASELINE: baseline_argv()}
    for a in agents:
        s = Subject.from_agent(a)
        subjects[s.name] = s
        if s.available:
            try:
                commands[s.name] = a.adapter.startup_argv()
            except Unsupported as e:
                s.available, s.skipped_reason = False, str(e)

    merged: dict[str, dict[str, list[float]]] = {}
    with tempfile.TemporaryDirectory(prefix="flores-bench-") as tmp:
        # Isolated, empty config dirs: reproducible and never touches the user's setup.
        env = sanitized_env(
            {
                "CLAUDE_CONFIG_DIR": str(Path(tmp, "claude")),
                "CODEX_HOME": str(Path(tmp, "codex")),
                "PI_CODING_AGENT_DIR": str(Path(tmp, "pi")),
                "PI_OFFLINE": "1",
            }
        )
        for d in ("claude", "codex", "pi"):
            Path(tmp, d).mkdir()
        for order in round_orders(list(commands), rounds, seed):
            res = hyperfine.run({n: commands[n] for n in order}, runs=runs, warmup=warmup, env=env)
            for name, cols in res.items():
                tgt = merged.setdefault(name, {k: [] for k in cols})
                for k, v in cols.items():
                    tgt[k].extend(v)
        rss: dict[str, tuple[list[float], list[float]]] = {}
        for name, argv in commands.items():
            procs = [run_measured(argv, env, Path(tmp), RSS_TIMEOUT_S) for _ in range(RSS_RUNS)]
            ok = [p for p in procs if p.exit_code == 0]
            rss[name] = ([p.rss_mb for p in ok], [p.rss_floor_mb for p in ok])

    for name, cols in merged.items():
        s = subjects[name]
        failures = sum(1 for c in cols["exit_codes"] if c != 0)
        s.metrics = {
            "wall_ms": summarize([v * 1000 for v in cols["wall_s"]]),
            "cpu_ms": summarize([(u + sy) * 1000 for u, sy in zip(cols["user_s"], cols["sys_s"], strict=True)]),
        }
        values, floors = rss.get(name, ([], []))
        if values:
            s.metrics["rss_mb"] = summarize(values)
            s.metrics["rss_floor_mb"] = summarize(floors)
        s.info = {"argv": [Path(commands[name][0]).name, *commands[name][1:]], "failures": failures}

    header = result_header(SUITE, {"runs_per_round": runs, "warmup": warmup, "rounds": rounds, "seed": seed})
    return {**header, "subjects": [s.to_json() for s in subjects.values()]}
