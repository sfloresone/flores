"""Adapter interface: how to start, version and drive one coding-agent harness.

An adapter knows nothing about measuring. It only answers "what command, with
what environment and config files, makes this agent do X against provider Y".
Runners (perf/startup.py, perf/hello.py, later tasks/) do the measuring, so
every agent is timed by exactly the same code.
"""

from __future__ import annotations

import re
import shutil
import subprocess
from dataclasses import dataclass, field
from pathlib import Path

VERSION_RE = re.compile(r"\d+\.\d+\.\d+[0-9A-Za-z.+-]*")

# Never forward real credentials to an agent that is supposed to talk to the
# mock: a misconfigured adapter would otherwise silently spend real money.
SECRET_ENV = (
    "ANTHROPIC_API_KEY",
    "ANTHROPIC_AUTH_TOKEN",
    "ANTHROPIC_OAUTH_TOKEN",
    "CLAUDE_CODE_OAUTH_TOKEN",
    "OPENAI_API_KEY",
    "OPENAI_BASE_URL",
    "ANTHROPIC_BASE_URL",
    "CLOUDFLARE_API_KEY",
    "CLOUDFLARE_ACCOUNT_ID",
    "CLOUDFLARE_GATEWAY_ID",
    "AI_GATEWAY_API_KEY",
    "OPENROUTER_API_KEY",
    "GEMINI_API_KEY",
)

# Set when the benchmark itself runs inside an agent session; nested agents
# would otherwise change behaviour (e.g. Claude Code refuses nested sessions).
NESTING_ENV = ("CLAUDECODE", "CLAUDE_CODE_ENTRYPOINT", "CODEX_SANDBOX", "CODEX_THREAD_ID")


@dataclass
class HelloContext:
    """Inputs for a single scripted run against the mock provider."""

    mock_url: str  # e.g. http://127.0.0.1:53000 (no /v1)
    config_dir: Path  # fresh, empty, private config dir for this run
    workspace: Path  # fresh, empty working directory
    prompt: str
    model: str


@dataclass
class Invocation:
    argv: list[str]
    env: dict[str, str] = field(default_factory=dict)  # added on top of the sanitized environment
    stdin_devnull: bool = True


class Unsupported(Exception):
    """The adapter cannot run this workload (yet). Recorded, not fatal."""


class Adapter:
    name: str = ""
    label: str = ""
    binary: str = ""
    version_args: tuple[str, ...] = ("--version",)
    homepage: str = ""

    def path(self) -> str | None:
        return shutil.which(self.binary)

    def available(self) -> bool:
        return self.path() is not None

    def version(self) -> str | None:
        exe = self.path()
        if not exe:
            return None
        try:
            out = subprocess.run(
                [exe, *self.version_args],
                capture_output=True,
                text=True,
                timeout=30,
                stdin=subprocess.DEVNULL,
            )
        except (OSError, subprocess.TimeoutExpired):
            return None
        m = VERSION_RE.search(out.stdout + out.stderr)
        return m.group(0) if m else None

    def startup_argv(self) -> list[str]:
        """Cheapest meaningful invocation: measures process start-up cost."""
        exe = self.path()
        if not exe:
            raise Unsupported(f"{self.binary} not installed")
        return [exe, *self.version_args]

    def hello(self, ctx: HelloContext) -> Invocation:
        raise Unsupported(f"{self.name}: hello workload not implemented")
