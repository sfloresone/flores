"""Effective settings: default.toml, then <APP_ENV>.toml, then .env overrides."""
import os
import tomllib
from pathlib import Path

ROOT = Path(__file__).parent


def load():
    env = os.environ.get("APP_ENV", "development")
    cfg = tomllib.loads((ROOT / "config" / "default.toml").read_text())
    layer = ROOT / "config" / f"{env}.toml"
    if layer.exists():
        cfg["server"].update(tomllib.loads(layer.read_text()).get("server", {}))
    # .env is a developer convenience and is ignored in production
    if env != "production" and (ROOT / ".env").exists():
        for line in (ROOT / ".env").read_text().splitlines():
            key, _, value = line.partition("=")
            if key == "APP_PORT":
                cfg["server"]["port"] = int(value)
    return cfg
