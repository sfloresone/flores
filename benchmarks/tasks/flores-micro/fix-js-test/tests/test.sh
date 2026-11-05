#!/bin/bash
mkdir -p /logs/verifier
cd /app
if echo "bf71d7e5524c88f13e4be536d63b11d6f3495da2b7e9d42627348659b057778e  test/money.test.js" | sha256sum -c --quiet && npm test; then
  echo 1 > /logs/verifier/reward.txt
else
  echo 0 > /logs/verifier/reward.txt
fi
