"""Run the Go mock provider as a context manager."""

from __future__ import annotations

import json
import subprocess
import urllib.request
from pathlib import Path
from types import TracebackType

from flores_bench.paths import MOCK_BIN


class MockProvider:
    """Start `mockprovider` on a free port and talk to its control endpoints.

    >>> with MockProvider() as mock:
    ...     mock.reset()
    ...     # run an agent against mock.url
    ...     records = mock.requests()
    """

    def __init__(self, scenario: Path | None = None, dump_dir: Path | None = None, log: Path | None = None):
        self.scenario = scenario
        self.dump_dir = dump_dir
        self.log = log
        self.proc: subprocess.Popen[str] | None = None
        self.url = ""

    def __enter__(self) -> MockProvider:
        if not MOCK_BIN.exists():
            raise FileNotFoundError(f"{MOCK_BIN} not found; run `flores-bench build` first")
        cmd = [str(MOCK_BIN), "--port", "0"]
        if self.scenario:
            cmd += ["--scenario", str(self.scenario)]
        if self.dump_dir:
            cmd += ["--dump-bodies", str(self.dump_dir)]
        if self.log:
            cmd += ["--log", str(self.log)]
        self.proc = subprocess.Popen(cmd, stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)
        assert self.proc.stdout is not None
        line = self.proc.stdout.readline()
        if not line:
            err = self.proc.stderr.read() if self.proc.stderr else ""
            raise RuntimeError(f"mock provider failed to start: {err}")
        self.url = json.loads(line)["url"]
        return self

    def __exit__(
        self, exc_type: type[BaseException] | None, exc: BaseException | None, tb: TracebackType | None
    ) -> None:
        if self.proc:
            self.proc.terminate()
            try:
                self.proc.wait(timeout=5)
            except subprocess.TimeoutExpired:
                self.proc.kill()
            for stream in (self.proc.stdout, self.proc.stderr):
                if stream:
                    stream.close()

    def _call(self, method: str, path: str):
        req = urllib.request.Request(self.url + path, method=method)
        with urllib.request.urlopen(req, timeout=10) as resp:
            return json.loads(resp.read() or b"null")

    def reset(self) -> None:
        self._call("POST", "/_mock/reset")

    def requests(self) -> list[dict]:
        return self._call("GET", "/_mock/requests") or []
