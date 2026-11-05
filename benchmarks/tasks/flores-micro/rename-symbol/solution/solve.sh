#!/bin/bash
set -euo pipefail
cd /app && grep -rl calc_total . | xargs sed -i 's/calc_total/compute_total/g'
