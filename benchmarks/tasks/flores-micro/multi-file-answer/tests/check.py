import sys
try:
    got = open("/app/answer.txt").read().strip()
except FileNotFoundError:
    sys.exit(1)
sys.exit(0 if got == "8080" else 1)
