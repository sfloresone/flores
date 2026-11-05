import json

from flores_bench import budgets, report


def fake_result(platform="linux-x86_64", wall=3.0, suite="perf-startup"):
    def m(v):
        return {"n": 3, "mean": v, "stddev": 0.1, "min": v, "p50": v, "p95": v, "max": v, "ci95_p50": [v, v]}

    return {
        "schema_version": "result/v0",
        "suite": suite,
        "created_at": "2026-11-05T12:00:00Z",
        "git": {"sha": "abc123def4567", "short": "abc123def4", "branch": "main", "dirty": False},
        "machine": {"platform": platform, "cpu": "Test CPU", "cores": 4, "ci": True},
        "tools": {},
        "params": {"rounds": 1, "runs_per_round": 3, "warmup": 1},
        "subjects": [
            {
                "name": "flores-stub",
                "label": "flores-stub",
                "available": True,
                "version": "0.0.0",
                "metrics": {"wall_ms": m(wall), "rss_mb": m(8.0)},
                "samples": [{"wall_ms": wall}],
            },
            {"name": "fx", "label": "fx", "available": False, "skipped_reason": "fx not installed", "metrics": {}},
        ],
    }


def test_publish_writes_latest_history_and_readme(tmp_path):
    report.publish(fake_result(), root=tmp_path)
    report.publish(fake_result(wall=4.0), root=tmp_path)
    d = tmp_path / "perf-startup" / "linux-x86_64"
    latest = json.loads((d / "latest.json").read_text())
    assert "samples" not in latest["subjects"][0], "published files must not carry raw samples"
    history = (d / "history.jsonl").read_text().splitlines()
    assert len(history) == 2
    assert json.loads(history[1])["subjects"]["flores-stub"]["wall_ms.p50"] == 4.0
    readme = (tmp_path / "README.md").read_text()
    assert "xychart-beta" in readme
    assert "(referencia)" in readme
    assert "fx not installed" in readme


def test_budgets_enforced_only_on_linux():
    assert budgets.check(fake_result(wall=1.0), log=lambda *_: None)
    assert not budgets.check(fake_result(wall=999.0), log=lambda *_: None)
    assert budgets.check(fake_result(platform="darwin-arm64", wall=999.0), log=lambda *_: None)
