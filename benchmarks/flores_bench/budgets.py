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


def check_harness(result: dict, log=print) -> bool:
    """Behaviour is deterministic, so this gate applies on every platform.

    budgets.toml: [harness.<agent>] min_passed = N  and/or  must_pass = [...]
    It ratchets: raise min_passed / extend must_pass as flores fixes scenarios.
    """
    rules = load_budgets().get("harness", {})
    ok = True
    subjects = {s["name"]: s for s in result["subjects"]}
    for name, rule in rules.items():
        s = subjects.get(name)
        if not s or not s.get("scenarios"):
            log(f"  SKIP  {name:<14} not measured")
            continue
        if "min_passed" in rule:
            passed = s["metrics"].get("passed", 0)
            good = passed >= rule["min_passed"]
            ok &= good
            log(f"  {'PASS' if good else 'FAIL'}  {name:<14} passed {passed} (min {rule['min_passed']})")
        for scn in rule.get("must_pass", []):
            st = s["scenarios"].get(scn, {}).get("status", "not run")
            if st == "not run":
                log(f"  SKIP  {name:<14} {scn:<28} not run")
                continue
            good = st == "pass"
            ok &= good
            log(f"  {'PASS' if good else 'FAIL'}  {name:<14} {scn:<28} {st}")
    return ok
