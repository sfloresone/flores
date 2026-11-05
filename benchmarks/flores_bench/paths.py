"""Well-known locations inside the benchmarks directory."""

from __future__ import annotations

from pathlib import Path

BENCH_ROOT = Path(__file__).resolve().parent.parent
REPO_ROOT = BENCH_ROOT.parent
BIN_DIR = BENCH_ROOT / ".bin"
RESULTS_DIR = BENCH_ROOT / "results"
PUBLISHED_DIR = BENCH_ROOT / "published"
SUITES_DIR = BENCH_ROOT / "suites"
SCENARIOS_DIR = BENCH_ROOT / "scenarios"
SCHEMA_DIR = BENCH_ROOT / "schema"

MOCK_BIN = BIN_DIR / "mockprovider"
STUB_BIN = BIN_DIR / "flores-stub"
METER_BIN = BIN_DIR / "meter"
MEASURE_BIN = BIN_DIR / "measure"
