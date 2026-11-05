"""Turn one run's observations into pass/fail checks against `expect`."""

from __future__ import annotations

from dataclasses import asdict, dataclass


@dataclass
class Observation:
    exit_code: int
    timed_out: bool
    wall_ms: float
    rss_mb: float
    text: str  # stdout, or final_text for trajectory agents
    llm_requests: int
    retries: int
    max_step: int
    tool_results: int
    truncated: int
    statuses: list[int]
    bodies: list[str]  # request bodies sent to the model (from mock dumps)
    trajectory_errors: list[str] | None = None  # None: agent does not emit trajectories

    def summary(self) -> dict:
        d = asdict(self)
        d.pop("bodies")
        d["text_tail"] = d.pop("text")[-200:]
        return d


@dataclass
class Check:
    name: str
    ok: bool
    expected: object
    observed: object


def _range(name: str, spec: dict, value: int) -> Check:
    lo, hi = spec.get("min"), spec.get("max")
    ok = (lo is None or value >= lo) and (hi is None or value <= hi)
    return Check(name, ok, spec, value)


def evaluate(expect: dict, obs: Observation) -> list[Check]:
    checks: list[Check] = []
    if expect.get("completes", True):
        checks.append(
            Check("completes", not obs.timed_out, "exits before timeout", "timeout" if obs.timed_out else "ok")
        )

    exit_rule = expect.get("exit", "zero")
    if exit_rule != "any" and not obs.timed_out:
        ok = obs.exit_code == 0 if exit_rule == "zero" else obs.exit_code != 0
        checks.append(Check("exit", ok, exit_rule, obs.exit_code))

    for needle in expect.get("stdout_contains", []):
        checks.append(Check("stdout_contains", needle in obs.text, needle, obs.text[-120:]))

    if "llm_requests" in expect:
        checks.append(_range("llm_requests", expect["llm_requests"], obs.llm_requests))
    if "retries" in expect:
        checks.append(_range("retries", expect["retries"], obs.retries))
    if "tool_results" in expect:
        checks.append(_range("tool_results", expect["tool_results"], obs.tool_results))

    for needle in expect.get("request_contains", []):
        # Only later requests can carry tool output back to the model.
        found = any(needle in b for b in obs.bodies[1:])
        checks.append(Check("request_contains", found, needle, "found" if found else "absent"))

    if "max_wall_ms" in expect:
        checks.append(
            Check("max_wall_ms", obs.wall_ms <= expect["max_wall_ms"], expect["max_wall_ms"], round(obs.wall_ms))
        )

    if obs.trajectory_errors is not None:
        checks.append(
            Check("trajectory_valid", not obs.trajectory_errors, "valid trajectory/v0", obs.trajectory_errors[:3])
        )
    return checks
