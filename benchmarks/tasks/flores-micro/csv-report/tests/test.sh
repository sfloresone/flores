#!/bin/bash
# Reward 1 only if /tests/check.py passes.
mkdir -p /logs/verifier
if python3 /tests/check.py; then
  echo 1 > /logs/verifier/reward.txt
else
  echo 0 > /logs/verifier/reward.txt
fi
