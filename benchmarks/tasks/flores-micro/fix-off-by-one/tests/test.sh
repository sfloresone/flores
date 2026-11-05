#!/bin/bash
# Reward 1 only if the original tests pass AND the tests were not modified.
mkdir -p /logs/verifier
cd /app
if sha256sum -c /tests/test_pages.sha256 --quiet && python -m unittest -q 2>&1; then
  echo 1 > /logs/verifier/reward.txt
else
  echo 0 > /logs/verifier/reward.txt
fi
