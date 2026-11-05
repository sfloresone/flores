#!/bin/bash
set -euo pipefail
sed -i 's/n \* 360$/n * 60/' /app/dur.go
