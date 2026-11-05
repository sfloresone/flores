"""Validate and summarize trajectory/v0 JSONL streams.

The JSON Schema checks each event in isolation. `validate` adds the checks a
schema cannot express: ordering, a single session.start/session.end pair, and
that every tool.result answers an earlier tool.call.
"""

from __future__ import annotations

import json
from collections.abc import Iterable
from functools import cache
from pathlib import Path

from jsonschema import Draft202012Validator

from flores_bench.paths import SCHEMA_DIR

SCHEMA_PATH = SCHEMA_DIR / "trajectory.v0.schema.json"


@cache
def _validator() -> Draft202012Validator:
    schema = json.loads(SCHEMA_PATH.read_text())
    Draft202012Validator.check_schema(schema)
    return Draft202012Validator(schema)


def parse_lines(lines: Iterable[str]) -> tuple[list[dict], list[str]]:
    events, errors = [], []
    for i, line in enumerate(lines, 1):
        line = line.strip()
        if not line:
            continue
        try:
            events.append(json.loads(line))
        except json.JSONDecodeError as e:
            errors.append(f"line {i}: invalid JSON: {e}")
    return events, errors


def validate(events: list[dict]) -> list[str]:
    """Return a list of human-readable problems (empty means valid)."""
    errors: list[str] = []
    v = _validator()
    for i, ev in enumerate(events):
        for err in v.iter_errors(ev):
            path = "/".join(str(p) for p in err.absolute_path) or "<event>"
            errors.append(f"event {i} ({ev.get('type')}): {path}: {err.message}")
    if not events:
        return ["empty trajectory"]

    if events[0].get("type") != "session.start":
        errors.append("first event must be session.start")
    if events[-1].get("type") != "session.end":
        errors.append("last event must be session.end")
    for name in ("session.start", "session.end"):
        count = sum(1 for e in events if e.get("type") == name)
        if count != 1:
            errors.append(f"expected exactly one {name}, got {count}")

    prev_t = -1.0
    calls: set[str] = set()
    for i, ev in enumerate(events):
        if ev.get("seq") != i:
            errors.append(f"event {i}: seq is {ev.get('seq')}, expected {i}")
        t = ev.get("t_ms")
        if isinstance(t, (int, float)):
            if t < prev_t:
                errors.append(f"event {i}: t_ms went backwards ({t} < {prev_t})")
            prev_t = t
        if ev.get("type") == "llm.response":
            ttft, dur = ev.get("ttft_ms"), ev.get("duration_ms")
            if isinstance(ttft, (int, float)) and isinstance(dur, (int, float)) and dur < ttft:
                errors.append(f"event {i}: llm.response duration_ms ({dur}) < ttft_ms ({ttft})")
        if ev.get("type") == "tool.call":
            calls.add(ev.get("id", ""))
        elif ev.get("type") == "tool.result" and ev.get("id") not in calls:
            errors.append(f"event {i}: tool.result for unknown call id {ev.get('id')!r}")
    return errors


def validate_file(path: Path) -> list[str]:
    events, errors = parse_lines(path.read_text().splitlines())
    return errors + validate(events)


def summarize(events: list[dict]) -> dict:
    """Derive the per-run metrics we track from a valid trajectory."""
    end = next((e for e in events if e.get("type") == "session.end"), {})
    responses = [e for e in events if e.get("type") == "llm.response"]
    llm_ms = sum(e.get("duration_ms", 0) for e in responses)
    ttfts = [e["ttft_ms"] for e in responses if e.get("ttft_ms") is not None]
    duration = end.get("duration_ms", 0.0)
    return {
        "status": end.get("status"),
        "turns": end.get("turns", 0),
        "duration_ms": duration,
        "llm_ms": llm_ms,
        # Time spent in the harness itself: everything that is not waiting on the model.
        "harness_ms": max(0.0, duration - llm_ms),
        "ttft_first_ms": ttfts[0] if ttfts else None,
        "llm_requests": sum(1 for e in events if e.get("type") == "llm.request"),
        "request_bytes": sum(e.get("request_bytes", 0) for e in events if e.get("type") == "llm.request"),
        "tool_calls": end.get("tool_calls", 0),
        "tool_errors": end.get("tool_errors", 0),
        "usage": end.get("usage", {}),
        "errors": [e for e in events if e.get("type") == "error"],
    }
