#!/bin/bash
set -euo pipefail
cd /app/src && grep -rn --include='*.py' --include='*.js' 'TODO' . | grep -v '^./vendor/' | cut -d: -f1,2 | sed 's#^\./##' | sort > /app/todos.txt
