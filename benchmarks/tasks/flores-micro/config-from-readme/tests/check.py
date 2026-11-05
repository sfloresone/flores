import subprocess, sys, tomllib
r = subprocess.run([sys.executable, "main.py", "--check"], cwd="/app")
try:
    cfg = tomllib.load(open("/app/config/settings.toml", "rb"))
except Exception:
    sys.exit(1)
ok = r.returncode == 0 and cfg.get("environment") == "production"
ok = ok and cfg.get("database_url") == "sqlite:///data/inventory.db"
sys.exit(0 if ok else 1)
