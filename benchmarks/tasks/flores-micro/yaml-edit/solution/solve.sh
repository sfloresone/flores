#!/bin/bash
set -euo pipefail
python3 - <<'EOF'
p = "/app/deploy.yaml"
s = open(p).read()
s = s.replace("    replicas: 2\n", "    replicas: 5\n")
s = s.replace("      QUEUE: jobs\n", "      QUEUE: jobs\n      LOG_LEVEL: debug\n")
open(p, "w").write(s)
EOF
