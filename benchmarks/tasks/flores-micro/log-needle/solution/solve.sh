#!/bin/bash
set -euo pipefail
grep 'user=u-4242 ' /app/logs/access.log | grep 'status=503' | sed -E 's/.*request_id=([^ ]+).*/\1/' > /app/answer.txt
