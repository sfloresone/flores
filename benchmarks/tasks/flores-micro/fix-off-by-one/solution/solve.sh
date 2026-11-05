#!/bin/bash
set -euo pipefail
sed -i 's/len(items) - 1/len(items)/' /app/pages.py
