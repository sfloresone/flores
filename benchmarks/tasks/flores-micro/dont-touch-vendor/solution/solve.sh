#!/bin/bash
set -euo pipefail
sed -i -E 's/bump\("(major|minor|patch)", current\)/bump(current, "\1")/' /app/release.py
