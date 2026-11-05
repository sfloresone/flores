"""Run layer B scenarios against one or more agents.

Every (scenario, agent, run) gets a fresh mock provider (with the scenario
rendered for that agent), a fresh config dir and an empty workspace. Agents
run with full access to their tools: the workspace is a throwaway temp dir
and the scripted commands only `echo`.
"""

from __future__ import annotations

import json
import tempfile
from dataclasses import asdict
from pathlib import Path

from flores_bench import trajectory
from flores_bench.adapters import RunContext, Unsupported
from flores_bench.config import AgentConfig
from flores_bench.envinfo import result_header
from flores_bench.harness.checks import Observation, evaluate
from flores_bench.harness.scenarios import Scenario, render_mock
from flores_bench.mock import MockProvider
from flores_bench.perf.common import Subject, sanitized_env
from flores_bench.process import run_measured

SUITE = "harness"


def _final_text(events: list[dict]) -> str:
    end = next((e for e in reversed(events) if e.get("type") == "session.end"), {})
    if end.get("final_text"):
        return end["final_text"]
    return "\n".join(e.get("text", "") for e in events if e.get("type") == "message")


def run_one(agent: AgentConfig, scenario: Scenario) -> Observation:
    adapter = agent.adapter
    with tempfile.TemporaryDirectory(prefix="flores-harness-") as tmp:
        tmp_path = Path(tmp)
        scenario_file = tmp_path / "scenario.json"
        scenario_file.write_text(json.dumps(render_mock(scenario, adapter)))
        cfg, ws, dumps = tmp_path / "config", tmp_path / "ws", tmp_path / "dumps"
        cfg.mkdir()
        ws.mkdir()
        with MockProvider(scenario=scenario_file, dump_dir=dumps) as mock:
            ctx = RunContext(
                mock.url,
                cfg,
                ws,
                scenario.prompt,
                agent.model,
                full_access=True,
                trajectory=adapter.supports_trajectory,
            )
            inv = adapter.invoke(ctx)
            proc = run_measured(inv.argv, sanitized_env(inv.env), ws, scenario.timeout_s)
            records = mock.requests()

        llm = [r for r in records if r.get("protocol")]
        bodies = []
        for r in llm:
            f = dumps / f"{r['seq']:06d}.json"
            bodies.append(f.read_text(errors="replace") if f.exists() else "")

        text, traj_errors = proc.stdout, None
        if adapter.supports_trajectory:
            events, parse_errors = trajectory.parse_lines(proc.stdout.splitlines())
            traj_errors = parse_errors + (trajectory.validate(events) if events else ["no events"])
            text = _final_text(events)

    steps = {r.get("step", 0) for r in llm}
    return Observation(
        exit_code=proc.exit_code,
        timed_out=proc.timed_out,
        wall_ms=proc.wall_ms,
        rss_mb=proc.rss_mb,
        text=text,
        llm_requests=len(llm),
        retries=len(llm) - len(steps),
        max_step=max(steps, default=0),
        tool_results=max((r.get("tool_results", 0) for r in llm), default=0),
        truncated=sum(1 for r in llm if r.get("truncated")),
        statuses=[r.get("status", 0) for r in llm],
        bodies=bodies,
        trajectory_errors=traj_errors,
    )


def run(
    agents: list[AgentConfig],
    scenarios: list[Scenario],
    runs: int = 1,
    log=print,
) -> dict:
    subjects: list[dict] = []
    for agent in agents:
        s = Subject.from_agent(agent)
        results: dict[str, dict] = {}
        if s.available:
            for sc in scenarios:
                results[sc.name] = _run_scenario(agent, sc, runs)
                r = results[sc.name]
                log(f"  {agent.adapter.name:<12} {sc.name:<28} {r['status']:<5} {r.get('reason', '')}")
        passed = sum(1 for r in results.values() if r["status"] == "pass")
        applicable = sum(1 for r in results.values() if r["status"] in ("pass", "fail"))
        d = s.to_json()
        d["metrics"] = {"passed": passed, "applicable": applicable, "total": len(scenarios)} if results else {}
        d["scenarios"] = results
        subjects.append(d)

    header = result_header(SUITE, {"runs_per_scenario": runs})
    return {
        **header,
        "scenarios": [
            {"name": sc.name, "title": sc.title, "category": sc.category, "why": sc.why, "expect": sc.expect}
            for sc in scenarios
        ],
        "subjects": subjects,
    }


def _run_scenario(agent: AgentConfig, sc: Scenario, runs: int) -> dict:
    try:
        render_mock(sc, agent.adapter)
    except Unsupported as e:
        return {"status": "n/a", "reason": str(e)}
    run_records = []
    try:
        for _ in range(runs):
            obs = run_one(agent, sc)
            checks = evaluate(sc.expect, obs)
            run_records.append({"checks": [asdict(c) for c in checks], "observed": obs.summary()})
    except Exception as e:  # a runner bug must not hide the other results
        return {"status": "error", "reason": f"{type(e).__name__}: {e}", "runs": run_records}
    failed = sorted({c["name"] for r in run_records for c in r["checks"] if not c["ok"]})
    status = "pass" if not failed else "fail"
    out = {"status": status, "runs": run_records}
    if failed:
        out["reason"] = "failed: " + ", ".join(failed)
    return out
