"""Build the Go helper binaries (mock provider and flores-stub)."""

from __future__ import annotations

import subprocess

from flores_bench.paths import BENCH_ROOT, BIN_DIR, MOCK_BIN, STUB_BIN

TOOLS = {
    MOCK_BIN: "./tools/mockprovider",
    STUB_BIN: "./tools/flores-stub",
}


def build_tools() -> None:
    """Compile the Go tools into benchmarks/.bin.

    `go build` is incremental, so calling this before every benchmark is cheap
    and guarantees we never measure a stale binary.
    """
    BIN_DIR.mkdir(parents=True, exist_ok=True)
    for out, pkg in TOOLS.items():
        subprocess.run(["go", "build", "-trimpath", "-o", str(out), pkg], cwd=BENCH_ROOT, check=True)
