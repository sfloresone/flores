#!/bin/bash
set -euo pipefail
cat > /app/config/settings.toml <<'EOF'
environment = "production"
port = 8000
database_url = "sqlite:///data/inventory.db"
EOF
