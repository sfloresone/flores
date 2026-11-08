"""End-to-end: flores-stub and flores + mock provider + trajectory schema.

These tests pin the headless contract (trajectory/v1) both binaries honour.
"""

import json
import os
import subprocess
import urllib.request

import pytest

from flores_bench import trajectory
from flores_bench.adapters import ADAPTERS
from flores_bench.harness.scenarios import HARNESS_DIR, Scenario, render_mock
from flores_bench.mock import MockProvider
from flores_bench.paths import FLORES_BIN, STUB_BIN


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
        assert summary["steps"] == 1
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
        assert s["steps"] == 2
        assert s["tool_calls"] == 1 and s["tool_errors"] == 1
        assert [r["step"] for r in mock.requests()] == [0, 1]


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


# ------------------------------------------------------------------ flores


def flores_scenario(name: str, tmp_path) -> object:
    sc = Scenario.from_dict(json.loads((HARNESS_DIR / f"{name}.json").read_text()))
    out = tmp_path / f"{name}.json"
    out.write_text(json.dumps(render_mock(sc, ADAPTERS["flores"])))
    return out


def run_flores(mock: MockProvider, *args: str, cwd=None) -> subprocess.CompletedProcess:
    # A minimal environment: never hand real keys to the agent under test.
    # flores has real tools since M2: tests that run them pass a scratch cwd.
    env = {"PATH": os.environ.get("PATH", ""), "FLORES_API_KEY": "mock", "FLORES_MODEL": "claude-test"}
    return subprocess.run(
        [str(FLORES_BIN), "--output", "jsonl", "--base-url", mock.url, *args],
        capture_output=True,
        text=True,
        timeout=30,
        env=env,
        cwd=cwd,
    )


def test_flores_hello_trajectory_is_valid():
    with MockProvider() as mock:
        res = run_flores(mock, "-p", "hello world")
        assert res.returncode == 0, res.stderr
        events = events_of(res.stdout)
        assert trajectory.validate(events) == []
        s = trajectory.summarize(events)
        assert s["status"] == "success" and s["steps"] == 1
        assert s["usage"]["output_tokens"] > 0
        (rec,) = mock.requests()
        req = next(e for e in events if e["type"] == "llm.request")
        assert rec["protocol"] == "anthropic-messages"
        assert rec["request_bytes"] == req["request_bytes"]


def test_flores_runaway_loop_stops_by_repetition(tmp_path):
    # No --max-steps: the default has no limit, so repetition detection
    # (5 identical calls) plus one summary step must stop it.
    with MockProvider(scenario=flores_scenario("runaway-tool-loop", tmp_path)) as mock:
        res = run_flores(mock, "-p", "Do the task.")
        assert res.returncode == 4, res.stderr
        events = events_of(res.stdout)
        assert trajectory.validate(events) == []
        assert events[-1]["status"] == "repetition"
        assert len(mock.requests()) == 6


def test_flores_max_steps_adds_one_summary_step(tmp_path):
    with MockProvider(scenario=flores_scenario("runaway-tool-loop", tmp_path)) as mock:
        res = run_flores(mock, "-p", "Do the task.", "--max-steps", "2")
        assert res.returncode == 4, res.stderr
        events = events_of(res.stdout)
        assert trajectory.validate(events) == []
        assert events[-1]["status"] == "max_steps" and events[-1]["steps"] == 3
        assert len(mock.requests()) == 3


def test_flores_auth_error_fails_fast(tmp_path):
    with MockProvider(scenario=flores_scenario("auth-error-fail-fast", tmp_path)) as mock:
        res = run_flores(mock, "-p", "hi")
        assert res.returncode == 3
        events = events_of(res.stdout)
        assert trajectory.validate(events) == []
        err = next(e for e in events if e["type"] == "error")
        assert err["kind"] == "provider" and err["retryable"] is False
        assert len(mock.requests()) == 1


def _requests_by_attempt(events):
    return [e["attempt"] for e in events if e["type"] == "llm.request"]


def test_flores_retries_a_server_error(tmp_path):
    with MockProvider(scenario=flores_scenario("server-error-recover", tmp_path)) as mock:
        res = run_flores(mock, "-p", "Say something.")
        assert res.returncode == 0, res.stderr
        events = events_of(res.stdout)
        assert trajectory.validate(events) == []
        assert _requests_by_attempt(events) == [1, 2]
        (err,) = [e for e in events if e["type"] == "error"]
        assert err["retryable"] is True and err["attempt"] == 1 and err["retry_in_ms"] > 0
        assert events[-1]["final_text"] == "Recovered after server error."
        assert [r["status"] for r in mock.requests()] == [503, 200]


def test_flores_retries_a_cut_stream_without_keeping_the_partial_text(tmp_path):
    with MockProvider(scenario=flores_scenario("stream-truncated-recover", tmp_path)) as mock:
        res = run_flores(mock, "-p", "Say something.")
        assert res.returncode == 0, res.stderr
        events = events_of(res.stdout)
        assert trajectory.validate(events) == []
        assert events[-1]["steps"] == 1
        assert events[-1]["final_text"] == "The quick brown fox jumps over the lazy dog."
        # Same step, same history: the mock sees a retry, not a new step.
        assert [r["step"] for r in mock.requests()] == [0, 0]


def test_flores_gives_up_when_retry_after_is_too_long(tmp_path):
    with MockProvider(scenario=flores_scenario("retry-after-too-long", tmp_path)) as mock:
        res = run_flores(mock, "-p", "Say something.")
        assert res.returncode == 3
        assert "asked to wait 1h0m0s" in res.stderr
        events = events_of(res.stdout)
        assert trajectory.validate(events) == []
        assert len(mock.requests()) == 1


# ------------------------------------------------- flores --provider openai


def run_flores_openai(mock: MockProvider, *args: str, cwd=None) -> subprocess.CompletedProcess:
    env = {"PATH": os.environ.get("PATH", ""), "FLORES_API_KEY": "mock", "FLORES_MODEL": "gpt-test"}
    return subprocess.run(
        [str(FLORES_BIN), "--provider", "openai", "--output", "jsonl", "--base-url", f"{mock.url}/v1", *args],
        capture_output=True,
        text=True,
        timeout=30,
        env=env,
        cwd=cwd,
    )


def scenario_for(subject: str, name: str, tmp_path) -> object:
    sc = Scenario.from_dict(json.loads((HARNESS_DIR / f"{name}.json").read_text()))
    out = tmp_path / f"{subject}-{name}.json"
    out.write_text(json.dumps(render_mock(sc, ADAPTERS[subject])))
    return out


def test_flores_openai_hello_trajectory_is_valid():
    with MockProvider() as mock:
        res = run_flores_openai(mock, "-p", "hello world")
        assert res.returncode == 0, res.stderr
        events = events_of(res.stdout)
        assert trajectory.validate(events) == []
        assert events[0]["provider"] == {"name": "openai", "protocol": "openai-chat", "base_url": f"{mock.url}/v1"}
        s = trajectory.summarize(events)
        assert s["status"] == "success" and s["steps"] == 1
        assert s["usage"]["output_tokens"] > 0
        (rec,) = mock.requests()
        req = next(e for e in events if e["type"] == "llm.request")
        assert rec["protocol"] == "openai-chat"
        assert rec["request_bytes"] == req["request_bytes"]


def test_flores_openai_retries_a_cut_stream(tmp_path):
    with MockProvider(scenario=scenario_for("flores-openai", "stream-truncated-recover", tmp_path)) as mock:
        res = run_flores_openai(mock, "-p", "Say something.")
        assert res.returncode == 0, res.stderr
        events = events_of(res.stdout)
        assert trajectory.validate(events) == []
        assert _requests_by_attempt(events) == [1, 2]
        assert events[-1]["final_text"] == "The quick brown fox jumps over the lazy dog."
        assert [r["step"] for r in mock.requests()] == [0, 0]


@pytest.mark.parametrize("subject", ["flores", "flores-openai"])
def test_flores_out_of_credit_fails_at_once(subject, tmp_path):
    run = run_flores if subject == "flores" else run_flores_openai
    with MockProvider(scenario=scenario_for(subject, "out-of-credit-fail-fast", tmp_path)) as mock:
        res = run(mock, "-p", "Say something.")
        assert res.returncode == 3
        assert "no credit" in res.stderr and "retrying" not in res.stderr
        events = events_of(res.stdout)
        assert trajectory.validate(events) == []
        err = next(e for e in events if e["type"] == "error")
        assert err["retryable"] is False
        (rec,) = mock.requests()
        # Each vendor's real shape: OpenAI says 429, Anthropic says 400.
        assert rec["status"] == (400 if subject == "flores" else 429)


# ------------------------------------------------------- tools (M2, ADR 0006)


def run_tools_scenario(subject: str, name: str, tmp_path):
    """Run a layer B scenario in a scratch directory; return events and request bodies."""
    run = run_flores if subject == "flores" else run_flores_openai
    ws, dumps = tmp_path / "ws", tmp_path / "dumps"
    ws.mkdir()
    with MockProvider(scenario=scenario_for(subject, name, tmp_path), dump_dir=dumps) as mock:
        res = run(mock, "-p", "Run it.", cwd=ws)
        records = [r for r in mock.requests() if r.get("protocol")]
    events = events_of(res.stdout)
    assert trajectory.validate(events) == []
    bodies = [(dumps / f"{r['seq']:06d}.json").read_text() for r in records]
    return res, events, bodies


@pytest.mark.parametrize("subject", ["flores", "flores-openai"])
@pytest.mark.parametrize(
    ("scenario", "outputs"),
    [("tool-roundtrip", ["flores-42"]), ("parallel-tools", ["flores-11", "flores-22"])],
)
def test_flores_bash_output_reaches_the_model(subject, scenario, outputs, tmp_path):
    res, events, bodies = run_tools_scenario(subject, scenario, tmp_path)
    assert res.returncode == 0, res.stderr
    s = trajectory.summarize(events)
    assert s["steps"] == 2 and s["tool_calls"] == len(outputs) and s["tool_errors"] == 0
    # The model sees the real output of every call, and nothing before it ran.
    for out in outputs:
        assert out not in bodies[0] and out in bodies[1]
    # Six tools are sent on every step, the same bytes each time.
    tools = [json.loads(b)["tools"] for b in bodies]
    assert len(tools[0]) == 6 and tools[0] == tools[1]


@pytest.mark.parametrize("subject", ["flores", "flores-openai"])
def test_flores_explains_malformed_arguments(subject, tmp_path):
    res, events, bodies = run_tools_scenario(subject, "malformed-tool-args", tmp_path)
    assert res.returncode == 0, res.stderr
    (result,) = [e for e in events if e["type"] == "tool.result"]
    assert result["ok"] is False
    # What was received and what the schema expects, so the model can fix the call.
    assert "invalid arguments for bash" in result["error"]
    assert "expected schema" in result["error"]
    assert "invalid arguments for bash" in bodies[1]
