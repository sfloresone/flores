"""Concrete adapters for flores-stub and the competitor harnesses.

How each one is pointed at the mock provider is documented in
benchmarks/docs/adapters.md; keep both in sync.
"""

from __future__ import annotations

import json

from flores_bench.adapters.base import Adapter, Invocation, RunContext, Unsupported
from flores_bench.paths import STUB_BIN


class FloresStub(Adapter):
    name = "flores-stub"
    label = "flores-stub"
    binary = str(STUB_BIN)
    version_args = ("version",)
    supports_trajectory = True

    def path(self) -> str | None:
        return str(STUB_BIN) if STUB_BIN.exists() else None

    def invoke(self, ctx: RunContext) -> Invocation:
        argv = [str(STUB_BIN), "run", "--prompt", ctx.prompt]
        if ctx.trajectory:
            argv += ["--output", "jsonl"]
        return Invocation(
            argv=argv,
            env={"FLORES_BASE_URL": f"{ctx.mock_url}/v1", "FLORES_MODEL": ctx.model, "FLORES_API_KEY": "mock"},
        )

    def shell_call(self, command: str) -> tuple[str, dict]:
        # The stub has no tools; it answers every call with an error. This is
        # the name the real flores shell tool is expected to use.
        return "bash", {"command": command}


class Pi(Adapter):
    name = "pi"
    label = "Pi"
    binary = "pi"
    homepage = "https://pi.dev"

    def invoke(self, ctx: RunContext) -> Invocation:
        # Override only the base URL of the built-in providers so Pi keeps the
        # model metadata, prompts and wire protocol it would use in real life.
        (ctx.config_dir / "models.json").write_text(
            json.dumps(
                {
                    "providers": {
                        "openai": {"baseUrl": f"{ctx.mock_url}/v1"},
                        "anthropic": {"baseUrl": ctx.mock_url},
                    }
                }
            )
        )
        return Invocation(
            argv=[self.path() or self.binary, "-p", "--model", ctx.model, "--no-session", ctx.prompt],
            env={
                "PI_CODING_AGENT_DIR": str(ctx.config_dir),
                "PI_OFFLINE": "1",
                "PI_TELEMETRY": "0",
                "OPENAI_API_KEY": "mock",
                "ANTHROPIC_API_KEY": "mock",
            },
        )

    def shell_call(self, command: str) -> tuple[str, dict]:
        return "bash", {"command": command}


class ClaudeCode(Adapter):
    name = "claude-code"
    label = "Claude Code"
    binary = "claude"
    homepage = "https://code.claude.com"

    def invoke(self, ctx: RunContext) -> Invocation:
        argv = [self.path() or self.binary, "-p", "--model", ctx.model]
        if ctx.full_access:
            argv.append("--dangerously-skip-permissions")
        return Invocation(
            argv=[*argv, ctx.prompt],
            env={
                "CLAUDE_CONFIG_DIR": str(ctx.config_dir),
                "ANTHROPIC_BASE_URL": ctx.mock_url,
                "ANTHROPIC_API_KEY": "mock",
                # Skip telemetry/update checks: they would hit the network and
                # are not part of the work we want to measure.
                "CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC": "1",
                "DISABLE_AUTOUPDATER": "1",
            },
        )

    def shell_call(self, command: str) -> tuple[str, dict]:
        return "Bash", {"command": command, "description": "Run a command"}


class Codex(Adapter):
    name = "codex"
    label = "Codex"
    binary = "codex"
    homepage = "https://github.com/openai/codex"

    def invoke(self, ctx: RunContext) -> Invocation:
        (ctx.config_dir / "config.toml").write_text(
            f'model = "{ctx.model}"\n'
            'model_provider = "mock"\n\n'
            "[model_providers.mock]\n"
            'name = "Mock"\n'
            f'base_url = "{ctx.mock_url}/v1"\n'
            'env_key = "MOCK_API_KEY"\n'
            'wire_api = "responses"\n'
        )
        argv = [self.path() or self.binary, "exec", "--skip-git-repo-check"]
        if ctx.full_access:
            argv.append("--dangerously-bypass-approvals-and-sandbox")
        return Invocation(
            argv=[*argv, ctx.prompt],
            env={"CODEX_HOME": str(ctx.config_dir), "MOCK_API_KEY": "mock"},
        )

    def shell_call(self, command: str) -> tuple[str, dict]:
        # With gpt-6.x Codex groups its tools in namespaces and `functions.exec`
        # is a freeform tool without a JSON schema; mapping pending.
        raise Unsupported("codex: shell tool is a namespaced freeform tool (mapping pending)")


class Fx(Adapter):
    name = "fx"
    label = "fx"
    binary = "fx"
    homepage = "https://fx.sh"

    def invoke(self, ctx: RunContext) -> Invocation:
        # fx 0.0.16 reads ~/.fx and ignores FX_HOME (documented for later
        # releases), so isolate it through HOME. Without isolation it silently
        # uses the user's own login (e.g. a Codex subscription) instead of the mock.
        fx_dir = ctx.config_dir / ".fx"
        fx_dir.mkdir(mode=0o700, exist_ok=True)
        settings = fx_dir / "settings.json"
        settings.write_text(
            json.dumps(
                {
                    "provider": "mock",
                    "providers": {
                        "mock": {
                            "protocol": "openai-chat-completions",
                            "base_url": f"{ctx.mock_url}/v1",
                            "auth": {"type": "bearer", "env": "MOCK_API_KEY"},
                        }
                    },
                    "models": {"mock": ctx.model.removeprefix("openai/")},
                }
            )
        )
        settings.chmod(0o600)
        ctx.config_dir.chmod(0o700)
        argv = [self.path() or self.binary, "ask", "--no-save"]
        if ctx.full_access:
            argv.append("--full-access")
        return Invocation(
            argv=[*argv, ctx.prompt],
            env={
                "HOME": str(ctx.config_dir),
                "FX_HOME": str(fx_dir),
                "FX_PROVIDER": "mock",
                "MOCK_API_KEY": "mock",
            },
        )

    def shell_call(self, command: str) -> tuple[str, dict]:
        return "shell", {"action": "run", "command": command}


ADAPTERS: dict[str, Adapter] = {a.name: a for a in (FloresStub(), Pi(), ClaudeCode(), Codex(), Fx())}
