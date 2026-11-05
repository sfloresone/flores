"""End-to-end: flores-stub + mock provider + trajectory schema.

These tests pin the headless contract the real flores binary must honour.
"""

import json
import subprocess
import urllib.request

import pytest

from flores_bench import trajectory
from flores_bench.adapters import ADAPTERS
from flores_bench.harness.scenarios import HARNESS_DIR, Scenario, render_mock
from flores_bench.mock import MockProvider
from flores_bench.paths import STUB_BIN


def stub_scenario(name: str, tmp_path) -> object:
    """Render a layer B scenario for flores-stub into a mock scenario file."""
    sc = Scenario.from_dict(json.loads((HARNESS_DIR / f"{name}.json").read_text()))
    out = tmp_path / f"{name}.json"
    out.write_text(json.dumps(render_mock(sc, ADAPTERS["flores-stub"])))
    return out


def run_stub(mock: MockProvider, *args: str) -> subprocess.CompletedProcess:
    return subprocess.run(
        [str(STUB_BIN), "run", "--output", "jsonl", "--base-url", f"{mock.url}/v1", *args],
        capture_output=True,
        text=True,
        timeout=30,
    )


def events_of(out: str) -> list[dict]:
    events, errors = trajectory.parse_lines(out.splitlines())
    assert not errors
    return events


def test_hello_trajectory_is_valid():
    with MockProvider() as mock:
        res = run_stub(mock, "--prompt", "hello world")
        assert res.returncode == 0, res.stderr
        events = events_of(res.stdout)
        assert trajectory.validate(events) == []
        summary = trajectory.summarize(events)
        assert summary["status"] == "success"
        assert summary["turns"] == 1
        assert summary["usage"]["output_tokens"] > 0
        # The mock saw exactly what the stub reported sending.
        (rec,) = mock.requests()
        req = next(e for e in events if e["type"] == "llm.request")
        assert rec["request_bytes"] == req["request_bytes"]
        assert rec["protocol"] == "openai-chat"


def test_tool_loop_scenario(tmp_path):
    with MockProvider(scenario=stub_scenario("tool-roundtrip", tmp_path)) as mock:
        res = run_stub(mock, "--prompt", "list files")
        assert res.returncode == 0, res.stderr
        events = events_of(res.stdout)
        assert trajectory.validate(events) == []
        s = trajectory.summarize(events)
        assert s["turns"] == 2
        assert s["tool_calls"] == 1 and s["tool_errors"] == 1
        assert [r["turn"] for r in mock.requests()] == [0, 1]


def test_provider_error_is_reported(tmp_path):
    with MockProvider(scenario=stub_scenario("rate-limit-recover", tmp_path)) as mock:
        res = run_stub(mock, "--prompt", "hi")
        assert res.returncode == 3
        events = events_of(res.stdout)
        assert trajectory.validate(events) == []
        err = next(e for e in events if e["type"] == "error")
        assert err["kind"] == "provider" and err["retryable"] is True
        assert events[-1]["status"] == "error"


@pytest.mark.parametrize(
    "mutate, expected",
    [
        (lambda evs: evs[1:], "first event must be session.start"),
        (lambda evs: evs[:-1], "last event must be session.end"),
        (lambda evs: [{**e, "seq": 7} if i == 1 else e for i, e in enumerate(evs)], "seq is 7"),
        (lambda evs: [{k: v for k, v in e.items() if k != "usage"} for e in evs], "'usage' is a required"),
        (
            lambda evs: [{**e, "duration_ms": 0.0, "ttft_ms": 1.0} if e["type"] == "llm.response" else e for e in evs],
            "duration_ms (0.0) < ttft_ms",
        ),
    ],
)
def test_validator_catches_broken_trajectories(mutate, expected):
    with MockProvider() as mock:
        events = events_of(run_stub(mock, "--prompt", "hi").stdout)
    errors = trajectory.validate(mutate(events))
    assert any(expected in e for e in errors), errors


@pytest.mark.parametrize(
    "path, body",
    [
        ("/v1/chat/completions", {"model": "m", "messages": [{"role": "user", "content": "hi"}]}),
        ("/v1/messages", {"model": "m", "max_tokens": 10, "messages": [{"role": "user", "content": "hi"}]}),
        ("/v1/responses", {"model": "m", "input": "hi"}),
    ],
)
def test_mock_speaks_all_protocols(path, body):
    with MockProvider() as mock:
        req = urllib.request.Request(
            mock.url + path, data=json.dumps(body).encode(), headers={"Content-Type": "application/json"}
        )
        with urllib.request.urlopen(req, timeout=10) as resp:
            text = resp.read().decode()
        assert "Hello! Nice to meet you." in text
