"""Layer C runner: validate tasks (oracle/nop) and run metered campaigns.

Validation (N1, free): every task must score 1 with Harbor's `oracle` agent
(runs solution/solve.sh) and 0 with `nop` (does nothing). That proves the task
is solvable and that its test does not pass on an untouched workspace.

Campaigns (N3, costs money): each agent × model runs every task k times,
sequentially, through the meter. Requests are attributed to a trial by time:
with one trial at a time, a request belongs to the trial whose agent phase
contains it.
"""

from __future__ import annotations

import os
import statistics
import tempfile
import tomllib
from collections.abc import Callable
from datetime import UTC, datetime, timedelta
from pathlib import Path

from flores_bench.config import load_agents
from flores_bench.envinfo import result_header
from flores_bench.mock import MockProvider
from flores_bench.paths import SUITES_DIR
from flores_bench.tasks import harbor
from flores_bench.tasks.catalog import TASKS_DIR, Task
from flores_bench.tasks.meter import Meter

CAMPAIGN = SUITES_DIR / "tasks" / "campaign.toml"
PRICING = SUITES_DIR / "tasks" / "pricing.json"

AGENT_LABELS = {"claude-code": "Claude Code", "codex": "Codex", "pi": "Pi", "fx": "fx"}
GATEWAY_URL = "https://gateway.ai.cloudflare.com/v1/{account}/{gateway}"
DIRECT_URLS = {"anthropic": "https://api.anthropic.com", "openai": "https://api.openai.com/v1"}
# A request is attributed to a trial if it ends within its agent phase (+ slack).
ATTRIBUTION_SLACK = timedelta(seconds=2)

Log = Callable[[str], None]


def load_campaign() -> dict:
    return tomllib.loads(CAMPAIGN.read_text())


# ------------------------------------------------------------------ validate


def validate(tasks: list[Task], jobs_dir: Path, concurrent: int = 4, log: Log = print) -> dict[str, dict]:
    """Return {task: {"oracle": reward, "nop": reward, "ok": bool}}."""
    out: dict[str, dict] = {t.name: {} for t in tasks}
    stamp = datetime.now(UTC).strftime("%Y%m%dT%H%M%S")
    for agent in ("oracle", "nop"):
        log(f"validate: {agent} over {len(tasks)} task(s)")
        job = harbor.run_job(
            TASKS_DIR,
            agent,
            jobs_dir,
            f"validate-{agent}-{stamp}",
            concurrent=concurrent,
            include=[t.name for t in tasks],
        )
        for trial in harbor.parse_job(job):
            if trial.task in out:
                out[trial.task][agent] = trial.reward
                if trial.exception:
                    out[trial.task][f"{agent}_error"] = trial.exception
    for r in out.values():
        r["ok"] = r.get("oracle") == 1.0 and r.get("nop") == 0.0
    return out


# ------------------------------------------------------------------ campaign


def upstreams_for(mode: str, mock_url: str = "") -> dict[str, str]:
    if mode == "mock":
        return {"anthropic": mock_url, "openai": mock_url + "/v1"}
    if mode == "direct":
        return dict(DIRECT_URLS)
    if mode == "gateway":
        account, gateway = os.environ.get("CF_ACCOUNT_ID"), os.environ.get("CF_GATEWAY_ID")
        if not account or not gateway:
            raise SystemExit("gateway mode needs CF_ACCOUNT_ID and CF_GATEWAY_ID in the environment")
        base = GATEWAY_URL.format(account=account, gateway=gateway)
        return {"anthropic": f"{base}/anthropic", "openai": f"{base}/openai"}
    raise SystemExit(f"unknown upstream mode {mode!r}")


def subjects_for(agent_names: list[str] | None, tier: str, campaign: dict) -> list[dict]:
    if tier not in campaign["tiers"]:
        raise SystemExit(f"unknown tier {tier!r}; known: {', '.join(campaign['tiers'])}")
    versions = {a.adapter.name: a.pinned_version for a in load_agents()}
    out = []
    for agent, spec in campaign["agents"].items():
        if agent_names and agent not in agent_names:
            continue
        for provider, api_format in spec["providers"].items():
            model = campaign["tiers"][tier][provider]
            out.append(
                {
                    "agent": agent,
                    "provider": provider,
                    "model": model,
                    "api_format": api_format,
                    "version": versions.get(agent),
                    "name": f"{agent}@{model}",
                    "label": f"{AGENT_LABELS.get(agent, agent)} · {model}",
                }
            )
    return out


def attribute(trials: list[harbor.Trial], records: list[dict]) -> tuple[dict[str, list[dict]], list[dict]]:
    """Map meter records to trials by time. Returns (by trial name, unattributed)."""
    by_trial: dict[str, list[dict]] = {t.trial: [] for t in trials}
    windows = [
        (t.trial, t.agent_start, t.agent_end + ATTRIBUTION_SLACK) for t in trials if t.agent_start and t.agent_end
    ]
    loose = []
    for rec in records:
        when = harbor.parse_time(rec.get("time"))
        owner = next((name for name, start, end in windows if when and start <= when <= end), None)
        (by_trial[owner] if owner else loose).append(rec)
    return by_trial, loose


def _usage_sum(records: list[dict]) -> dict[str, int]:
    keys = ("input", "output", "cache_read", "cache_write")
    return {k: sum((r.get("usage") or {}).get(k, 0) for r in records) for k in keys}


def summarize_subject(subject: dict, trials: list[harbor.Trial], records: list[dict], k: int) -> dict:
    by_trial, loose = attribute(trials, records)
    tasks: dict[str, dict] = {}
    for t in trials:
        recs = by_trial.get(t.trial, [])
        entry = tasks.setdefault(
            t.task, {"rewards": [], "passed": 0, "cost_usd": 0.0, "requests": 0, "agent_s": [], "errors": []}
        )
        entry["rewards"].append(t.reward)
        entry["passed"] += int(t.passed)
        entry["cost_usd"] += sum(r.get("cost_usd", 0.0) for r in recs)
        entry["requests"] += sum(1 for r in recs if not r.get("blocked"))
        if t.agent_seconds is not None:
            entry["agent_s"].append(round(t.agent_seconds, 2))
        if t.exception:
            entry["errors"].append(t.exception)
    for entry in tasks.values():
        entry["cost_usd"] = round(entry["cost_usd"], 6)

    n_trials = len(trials)
    total_cost = sum(r.get("cost_usd", 0.0) for r in records)
    agent_s = [s for e in tasks.values() for s in e["agent_s"]]
    subject = dict(subject)
    subject["tasks"] = tasks
    subject["metrics"] = {
        "pass_at_1": round(sum(t.passed for t in trials) / n_trials, 4) if n_trials else 0.0,
        "pass_hat_k": round(sum(e["passed"] == k for e in tasks.values()) / len(tasks), 4) if tasks else 0.0,
        "k": k,
        "tasks": len(tasks),
        "trials": n_trials,
        "errors": sum(len(e["errors"]) for e in tasks.values()),
        "cost_usd": round(total_cost, 6),
        "cost_per_trial_usd": round(total_cost / n_trials, 6) if n_trials else 0.0,
        "requests": sum(1 for r in records if not r.get("blocked")),
        "blocked_requests": sum(1 for r in records if r.get("blocked")),
        "estimated_cost_requests": sum(1 for r in records if r.get("cost_estimated")),
        "unattributed_requests": len(loose),
        "tokens": _usage_sum(records),
        "agent_s_p50": round(statistics.median(agent_s), 2) if agent_s else None,
    }
    return subject


def run_campaign(
    tasks: list[Task],
    agent_names: list[str] | None,
    tier: str,
    k: int,
    upstream: str,
    budget_usd: float,
    jobs_dir: Path,
    max_requests_per_trial: int = 200,
    log: Log = print,
) -> dict:
    if upstream != "mock" and budget_usd <= 0:
        raise SystemExit("--budget-usd is required for real upstreams")
    if upstream == "mock":
        budget_usd = budget_usd or 1.0  # nominal: the mock costs nothing
    campaign = load_campaign()
    subjects = subjects_for(agent_names, tier, campaign)
    names = [t.name for t in tasks]
    spent = 0.0
    results = []
    stamp = datetime.now(UTC).strftime("%Y%m%dT%H%M%S")
    log(f"tasks: {len(tasks)} task(s) × k={k} for {len(subjects)} agent·model pair(s), upstream={upstream}")

    with tempfile.TemporaryDirectory(prefix="flores-tasks-") as tmp:
        tmp_path = Path(tmp)
        mock = MockProvider() if upstream == "mock" else None
        if mock:
            mock.__enter__()
        try:
            for s in subjects:
                remaining = budget_usd - spent
                if remaining <= 0:
                    results.append({**s, "skipped_reason": "campaign budget exhausted"})
                    continue
                meter_log = tmp_path / f"meter-{s['name'].replace('/', '_')}.jsonl"
                with Meter(
                    upstreams_for(upstream, mock.url if mock else ""),
                    PRICING,
                    budget_usd=remaining,
                    max_requests=max_requests_per_trial * len(tasks) * k,
                    log=meter_log,
                    real_keys=upstream != "mock",
                ) as meter:
                    route = s["provider"]
                    extra = [
                        "--api-format",
                        s["api_format"],
                        "--base-url",
                        f"{meter.container_url}/{route}",
                        "--api-key-name",
                        "FLORES_METER_TOKEN",
                    ]
                    if s["version"]:
                        extra += ["--ak", f"version={s['version']}"]
                    log(f"  {s['label']}: running (budget left {remaining:.4f} USD)")
                    job = harbor.run_job(
                        TASKS_DIR,
                        s["agent"],
                        jobs_dir,
                        f"{s['agent']}-{s['model']}-{stamp}",
                        model=f"{s['provider']}/{s['model']}",
                        attempts=k,
                        concurrent=1,
                        include=names,
                        extra=extra,
                        env={"FLORES_METER_TOKEN": meter.token},
                    )
                    records = meter.records()
                subject = summarize_subject(s, harbor.parse_job(job), records, k)
                subject["job_dir"] = str(job)
                spent += subject["metrics"]["cost_usd"]
                m = subject["metrics"]
                log(
                    f"  {s['label']}: pass@1 {m['pass_at_1']:.2f} · pass^{k} {m['pass_hat_k']:.2f} · "
                    f"{m['cost_usd']:.4f} USD · {m['requests']} requests"
                )
                results.append(subject)
        finally:
            if mock:
                mock.__exit__(None, None, None)

    result = result_header(
        "tasks",
        {
            "tier": tier,
            "k": k,
            "upstream": upstream,
            "budget_usd": budget_usd,
            "pricing": str(PRICING.relative_to(SUITES_DIR.parent)),
            "harbor": harbor.harbor_version(),
        },
    )
    result["tasks"] = [{"name": t.name, "category": t.category, "difficulty": t.difficulty} for t in tasks]
    result["subjects"] = results
    result["spent_usd"] = round(spent, 6)
    return result
