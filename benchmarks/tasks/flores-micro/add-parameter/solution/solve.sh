#!/bin/bash
set -euo pipefail
cd /app
python3 - <<'EOF'
import re
def sub(p, a, b):
    s = open(p).read(); s = s.replace(a, b); open(p, "w").write(s)
sub("notify.py", "def send(user, message):", "def send(user, message, *, channel):")
sub("notify.py", '"message": message}', '"message": message, "channel": channel}')
for p, ch in (("services/billing.py", "email"), ("services/digest.py", "email"), ("services/alerts.py", "sms")):
    s = open(p).read()
    s = re.sub(r'((?:notify\.send|send|notify_user)\(user, [^\n]*)\)\n', lambda m: m.group(1) + f', channel="{ch}")\n', s)
    open(p, "w").write(s)
EOF
