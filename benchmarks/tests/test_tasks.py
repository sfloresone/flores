from datetime import UTC, datetime, timedelta

import pytest

from flores_bench import report
from flores_bench.tasks import catalog, harbor, runner

T0 = datetime(2026, 11, 5, 15, 0, 0, tzinfo=UTC)


def trial(task, name, reward, start_s, end_s, exc=None):
    return harbor.Trial(task, name, reward, T0 + timedelta(seconds=start_s), T0 + timedelta(seconds=end_s), exc)


def rec(at_s, cost, blocked=None, estimated=False):
    t = (T0 + timedelta(seconds=at_s)).strftime("%Y-%m-%dT%H:%M:%S.%f") + "123Z"  # Go writes nanoseconds
    return {
        "time": t,
        "cost_usd": cost,
        "usage": {"input": 100, "output": 10, "cache_read": 0, "cache_write": 0},
        "blocked": blocked,
        "cost_estimated": estimated,
    }


def test_harbor_never_gets_real_keys(monkeypatch):
    for k in ("ANTHROPIC_API_KEY", "OPENAI_API_KEY", "CF_AIG_TOKEN", "CF_ACCOUNT_ID", "CLAUDECODE"):
        monkeypatch.setenv(k, "real-secret")
    monkeypatch.setenv("DOCKER_HOST", "unix:///x.sock")
    env = harbor.harbor_env({"FLORES_METER_TOKEN": "tok"})
    assert "real-secret" not in env.values()
    assert env["FLORES_METER_TOKEN"] == "tok" and env["DOCKER_HOST"] == "unix:///x.sock"


def test_parse_time_handles_go_nanoseconds():
    assert harbor.parse_time("2026-11-05T15:08:00.123456789Z") == datetime(2026, 11, 5, 15, 8, 0, 123456, tzinfo=UTC)
    assert harbor.parse_time("2026-11-05T15:08:00Z") == datetime(2026, 11, 5, 15, 8, tzinfo=UTC)
    assert harbor.parse_time(None) is None


def test_attribution_by_time_window():
    trials = [trial("a", "a__1", 1.0, 0, 10), trial("a", "a__2", 0.0, 20, 30)]
    records = [rec(5, 0.1), rec(11, 0.2), rec(25, 0.3), rec(40, 0.4)]  # 11 s is within the 2 s slack
    by, loose = runner.attribute(trials, records)
    assert [r["cost_usd"] for r in by["a__1"]] == [0.1, 0.2]
    assert [r["cost_usd"] for r in by["a__2"]] == [0.3]
    assert [r["cost_usd"] for r in loose] == [0.4]


def test_summarize_pass_at_1_and_pass_hat_k():
    trials = [
        trial("easy", "e1", 1.0, 0, 10),
        trial("easy", "e2", 1.0, 20, 30),
        trial("hard", "h1", 1.0, 40, 50),
        trial("hard", "h2", None, 60, 70, exc="AgentTimeoutError: 600s"),
    ]
    records = [rec(5, 0.01), rec(25, 0.01), rec(45, 0.02), rec(65, 0.05), rec(66, 0, blocked="budget exhausted")]
    s = runner.summarize_subject({"name": "x", "label": "X"}, trials, records, k=2)
    m = s["metrics"]
    assert m["pass_at_1"] == 0.75  # 3 of 4 trials
    assert m["pass_hat_k"] == 0.5  # only "easy" passed both times
    assert m["errors"] == 1 and m["blocked_requests"] == 1 and m["requests"] == 4
    assert m["cost_usd"] == pytest.approx(0.09)
    assert s["tasks"]["hard"]["passed"] == 1 and s["tasks"]["hard"]["cost_usd"] == pytest.approx(0.07)


def test_subjects_pair_agents_with_tier_models():
    subs = runner.subjects_for(None, "cheap", runner.load_campaign())
    names = {s["name"] for s in subs}
    assert "claude-code@claude-haiku-5-5" in names and "codex@gpt-5.4-mini" in names
    assert {s["model"] for s in subs if s["agent"] == "pi"} == {"claude-haiku-5-5", "gpt-5.4-mini"}
    with pytest.raises(SystemExit):
        runner.subjects_for(None, "nope", runner.load_campaign())


def test_every_campaign_model_is_priced():
    import json

    priced = set(json.loads(runner.PRICING.read_text())["models"])
    for tier in runner.load_campaign()["tiers"].values():
        assert set(tier.values()) <= priced


def test_real_upstreams_require_a_budget(tmp_path):
    with pytest.raises(SystemExit, match="budget"):
        runner.run_campaign([], None, "cheap", 1, "direct", 0.0, tmp_path)


def test_catalog_loads_and_validates(tmp_path):
    tasks = catalog.load_tasks()
    assert len(tasks) >= 15
    assert len({t.name for t in tasks}) == len(tasks)
    bad = tmp_path / "broken"
    (bad / "environment").mkdir(parents=True)
    (bad / "task.toml").write_text('[task]\nname = "flores-micro/broken"\n')
    with pytest.raises(ValueError, match="missing"):
        catalog.load_task(bad)


def test_render_and_history_for_tasks():
    s = runner.summarize_subject(
        {"name": "pi@m", "label": "Pi · m", "version": "1.5.0"},
        [trial("t1", "a", 1.0, 0, 10), trial("t1", "b", 0.0, 20, 30)],
        [rec(5, 0.002)],
        k=2,
    )
    result = {
        "suite": "tasks",
        "created_at": "2026-11-05T15:00:00Z",
        "git": {"sha": "abc", "short": "abc", "dirty": False},
        "machine": {"platform": "darwin-arm64"},
        "params": {"tier": "cheap", "k": 2, "upstream": "mock", "budget_usd": 1.0},
        "tasks": [{"name": "t1", "category": "debug", "difficulty": "easy"}],
        "subjects": [s],
        "spent_usd": 0.002,
    }
    md = report.render_result(result)
    assert "| Pi · m | 1.5.0 | 50% | 0% |" in md and "| `t1` <sub>debug</sub> | 1/2 |" in md
    line = report.history_line(result)
    assert line["subjects"]["pi@m"]["passed"] == {"t1": 1} and line["k"] == 2
