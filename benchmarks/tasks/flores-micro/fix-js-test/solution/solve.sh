#!/bin/bash
set -euo pipefail
sed -i 's/\${rest}`/${String(rest).padStart(2, "0")}`/' /app/src/money.js
