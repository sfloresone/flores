import hashlib, subprocess, sys
h = lambda p: hashlib.sha256(open(p, "rb").read()).hexdigest()
r = subprocess.run([sys.executable, "-m", "unittest", "-q"], cwd="/app", capture_output=True, timeout=120)
lines = open("/app/data/primes.txt").read().split()
ok = r.returncode == 0 and len(lines) == 1500 and lines[1006] == "7993"
ok = ok and h("/app/test_primes.py") == "1777cbff8ec1ed9c7302d7ace13ba44bf9ac8eba0847c1d07641831b95cc1959"
sys.exit(0 if ok else 1)
