#!/bin/bash
set -euo pipefail
python3 - <<'EOF'
import csv, glob, json
by = {}
for f in glob.glob("/app/data/*.csv"):
    for r in csv.DictReader(open(f)):
        if r["status"] == "paid":
            by[r["region"]] = by.get(r["region"], 0) + float(r["amount"])
by = {k: round(v, 2) for k, v in by.items()}
json.dump({"total": round(sum(by.values()), 2), "by_region": by}, open("/app/report.json", "w"))
EOF
