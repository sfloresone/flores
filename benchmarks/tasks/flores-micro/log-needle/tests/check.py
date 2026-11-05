import sys
try:
    got = open("/app/answer.txt").read().strip()
except FileNotFoundError:
    sys.exit(1)
sys.exit(0 if got == "req-7f3a9c0d1e2b" else 1)
