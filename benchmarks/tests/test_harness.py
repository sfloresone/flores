import pytest

from flores_bench.adapters import ADAPTERS, Unsupported
from flores_bench.config import load_agents
from flores_bench.harness import runner
from flores_bench.harness.checks import Observation, evaluate
from flores_bench.harness.scenarios import Scenario, load_all, render_mock


def obs(**kw) -> Observation:
    base = dict(
        exit_code=0,
        timed_out=False,
        wall_ms=10.0,
        rss_mb=5.0,
        text="Done.",
        llm_requests=2,
        retries=0,
        max_step=1,
        tool_results=1,
        truncated=0,
        statuses=[200, 200],
        bodies=["{}", '{"x":"flores-42"}'],
    )
    base.update(kw)
    return Observation(**base)


def failed(checks):
    return sorted(c.name for c in checks if not c.ok)


def test_all_scenarios_load_and_render_for_every_adapter():
    scenarios = load_all()
    assert len(scenarios) >= 10
    for sc in scenarios:
        for adapter in ADAPTERS.values():
            try:
                mock = render_mock(sc, adapter)
            except Unsupported:
                assert "shell" in sc.requires, f"{sc.name} unsupported on {adapter.name} without requiring shell"
                continue
            for step in mock["steps"]:
                for tc in step.get("tool_calls", []):
                    assert "name" in tc and "shell" not in tc and "shell_raw" not in tc


def test_shell_calls_render_to_agent_tool_names():
    sc = next(s for s in load_all() if s.name == "tool-roundtrip")
    names = {a: render_mock(sc, ADAPTERS[a])["steps"][0]["tool_calls"][0]["name"] for a in ("claude-code", "pi", "fx")}
    assert names == {"claude-code": "Bash", "pi": "bash", "fx": "shell"}
    with pytest.raises(Unsupported):
        render_mock(sc, ADAPTERS["codex"])


def test_unknown_expect_key_is_rejected():
    with pytest.raises(ValueError, match="unknown expect keys"):
        Scenario.from_dict(
            {"name": "x", "title": "x", "category": "basics", "why": "x", "mock": {}, "expect": {"nope": 1}}
        )


def test_evaluate_checks():
    expect = {
        "exit": "zero",
        "stdout_contains": ["Done."],
        "llm_requests": {"min": 2, "max": 2},
        "tool_results": {"min": 1},
        "request_contains": ["flores-42"],
        "max_wall_ms": 100,
    }
    assert failed(evaluate(expect, obs())) == []
    # The marker only counts if it travels back in a *later* request.
    assert failed(evaluate(expect, obs(bodies=['{"x":"flores-42"}', "{}"]))) == ["request_contains"]
    assert failed(evaluate(expect, obs(exit_code=1, llm_requests=5))) == ["exit", "llm_requests"]
    assert failed(evaluate(expect, obs(timed_out=True, exit_code=-9))) == ["completes"]
    assert failed(evaluate({"exit": "any", "completes": True}, obs(exit_code=4))) == []


def test_stub_baseline_is_red_where_expected():
    """flores-stub has no tools and no retries: the TDD starting point."""
    names = {"basic-text", "unknown-tool", "auth-error-fail-fast", "rate-limit-recover", "tool-roundtrip"}
    scenarios = [s for s in load_all() if s.name in names]
    result = runner.run(load_agents(["flores-stub"]), scenarios, log=lambda *_: None)
    status = {k: v["status"] for k, v in result["subjects"][0]["scenarios"].items()}
    assert status == {
        "basic-text": "pass",
        "unknown-tool": "pass",
        "auth-error-fail-fast": "pass",
        "rate-limit-recover": "fail",
        "tool-roundtrip": "fail",
    }
    # Contract agents are also judged on their trajectory.
    run = result["subjects"][0]["scenarios"]["basic-text"]["runs"][0]
    assert any(c["name"] == "trajectory_valid" and c["ok"] for c in run["checks"])
