import hashlib, subprocess, sys
t = hashlib.sha256(open("/app/test_service.py", "rb").read()).hexdigest()
try:
    r = subprocess.run([sys.executable, "-m", "unittest"], cwd="/app", timeout=30)
except subprocess.TimeoutExpired:
    sys.exit(1)
sys.exit(0 if r.returncode == 0 and t == "754b487ddedfb019590dcb1a7145912d5c1bbe94249e9bc08e7866d7ef22df1e" else 1)
