import sys
import tomllib
from pathlib import Path

CONFIG = Path(__file__).parent / "config" / "settings.toml"


def check():
    if not CONFIG.exists():
        sys.exit(f"missing {CONFIG}; see README.md")
    cfg = tomllib.loads(CONFIG.read_text())
    assert cfg["environment"] in ("development", "production"), "bad environment"
    assert 1024 <= int(cfg["port"]) <= 65535, "bad port"
    assert cfg["database_url"].startswith("sqlite:///"), "bad database_url"
    print("config ok:", cfg["environment"])


if __name__ == "__main__" and "--check" in sys.argv:
    check()
