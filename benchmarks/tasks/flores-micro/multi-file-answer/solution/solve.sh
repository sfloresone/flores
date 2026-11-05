#!/bin/bash
set -euo pipefail
cd /app && APP_ENV=production python3 -c 'import settings; print(settings.load()["server"]["port"])' > answer.txt
