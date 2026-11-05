#!/bin/bash
set -euo pipefail
cd /app
sed -i 's/recieve/receive/g' src/*
git add -A src
git commit -q -m "fix: typo recieve -> receive"
