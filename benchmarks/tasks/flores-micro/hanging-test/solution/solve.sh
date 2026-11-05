#!/bin/bash
set -euo pipefail
sed -i 's/        polls += 1/        service.step()\n        polls += 1/' /app/service.py
