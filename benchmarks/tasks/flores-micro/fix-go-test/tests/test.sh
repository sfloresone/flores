#!/bin/bash
mkdir -p /logs/verifier
cd /app
if echo "432e1a5a58e5e22ddc81b4a283a8bb0c20a4eda4ea797c1b90f5dc70fede8ffd  dur_test.go" | sha256sum -c --quiet && go test ./...; then
  echo 1 > /logs/verifier/reward.txt
else
  echo 0 > /logs/verifier/reward.txt
fi
