#!/bin/bash
set -euo pipefail
python3 - <<'EOF'
p = "/app/rates.py"
s = open(p).read()
s = s.replace("    return weight_kg * 5\n", "    return max(10, weight_kg * 4.5)\n")
open(p, "w").write(s)
EOF
