"""Enforce budgets.toml against a result file (reference platform only)."""

from __future__ import annotations

from flores_bench.config import load_budgets


def metric_value(subject: dict, key: str) -> float | None:
    metric, _, stat = key.partition(".")
    return subject.get("metrics", {}).get(metric, {}).get(stat or "p50")


def check(result: dict, enforce: bool | None = None, log=print) -> bool:
    """Print PASS/FAIL/INFO lines; return False if any enforced budget fails."""
    if enforce is None:
        # Decided by where the result was measured, not where we check it.
        enforce = result["machine"]["platform"].startswith("linux-")
    budgets = load_budgets().get(result["suite"], {})
    ok = True
    subjects = {s["name"]: s for s in result["subjects"]}
    for name, limits in budgets.items():
        s = subjects.get(name)
        if not s or not s.get("available"):
            log(f"  SKIP  {name:<14} not measured")
            continue
        for key, limit in limits.items():
            v = metric_value(s, key)
            if v is None:
                log(f"  FAIL  {name:<14} {key:<20} missing metric")
                ok = False
                continue
            passed = v <= limit
            tag = ("PASS" if passed else "FAIL") if enforce else "INFO"
            log(f"  {tag}  {name:<14} {key:<20} {v:>10.2f}  (limit {limit})")
            if enforce and not passed:
                ok = False
    if not enforce:
        log("  (budgets are only enforced on the Linux reference platform)")
    return ok
