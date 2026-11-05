"""Run tools/meter (the metering proxy) as a context manager.

The meter is the only process that ever sees real provider keys. Agents in
Docker get a random per-run token instead.
"""

from __future__ import annotations

import json
import os
import platform
import subprocess
import urllib.request
from pathlib import Path
from types import TracebackType

from flores_bench.paths import METER_BIN

# Only these variables are passed to the meter, and only the meter gets them.
KEY_ENV = ("ANTHROPIC_API_KEY", "OPENAI_API_KEY", "CF_AIG_TOKEN")


def docker_host_address() -> tuple[str, str]:
    """(address the meter listens on, host name containers use to reach it).

    Never a public interface: the meter holds real credentials.
    - macOS (colima / Docker Desktop): loopback, reached as host.docker.internal.
    - Linux: the docker0 bridge gateway, reachable only from containers.
    """
    if platform.system() == "Darwin":
        return "127.0.0.1", "host.docker.internal"
    out = subprocess.run(
        ["docker", "network", "inspect", "bridge", "-f", "{{(index .IPAM.Config 0).Gateway}}"],
        capture_output=True,
        text=True,
        check=True,
    )
    ip = out.stdout.strip()
    return ip, ip


class Meter:
    def __init__(
        self,
        upstreams: dict[str, str],
        pricing: Path,
        budget_usd: float,
        max_requests: int,
        log: Path,
        real_keys: bool,
    ):
        self.upstreams = upstreams
        self.pricing = pricing
        self.budget_usd = budget_usd
        self.max_requests = max_requests
        self.log = log
        self.real_keys = real_keys
        self.proc: subprocess.Popen[str] | None = None
        self.url = ""  # as seen from the host
        self.container_url = ""  # as seen from inside Docker
        self.token = ""

    def __enter__(self) -> Meter:
        if not METER_BIN.exists():
            raise FileNotFoundError(f"{METER_BIN} not found; run `flores-bench build` first")
        listen, container_host = docker_host_address()
        cmd = [
            str(METER_BIN),
            "--listen",
            f"{listen}:0",
            "--pricing",
            str(self.pricing),
            "--budget-usd",
            str(self.budget_usd),
            "--max-requests",
            str(self.max_requests),
            "--log",
            str(self.log),
        ]
        for route, url in self.upstreams.items():
            cmd += [f"--upstream-{route}", url]
        env = {"PATH": os.environ.get("PATH", ""), "HOME": os.environ.get("HOME", "")}
        if self.real_keys:
            env.update({k: os.environ[k] for k in KEY_ENV if os.environ.get(k)})
        self.proc = subprocess.Popen(cmd, stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True, env=env)
        assert self.proc.stdout is not None
        line = self.proc.stdout.readline()
        if not line:
            err = self.proc.stderr.read() if self.proc.stderr else ""
            raise RuntimeError(f"meter failed to start: {err.strip()}")
        ready = json.loads(line)
        self.url, self.token = ready["url"], ready["token"]
        port = self.url.rsplit(":", 1)[1]
        self.container_url = f"http://{container_host}:{port}"
        return self

    def __exit__(
        self, exc_type: type[BaseException] | None, exc: BaseException | None, tb: TracebackType | None
    ) -> None:
        if self.proc:
            self.proc.terminate()
            try:
                self.proc.wait(timeout=10)
            except subprocess.TimeoutExpired:
                self.proc.kill()
            for stream in (self.proc.stdout, self.proc.stderr):
                if stream:
                    stream.close()

    def summary(self) -> dict:
        with urllib.request.urlopen(self.url + "/_meter/summary", timeout=10) as resp:
            return json.loads(resp.read())

    def records(self) -> list[dict]:
        if not self.log.exists():
            return []
        return [json.loads(line) for line in self.log.read_text().splitlines() if line.strip()]
