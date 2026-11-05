#!/usr/bin/env bash
# Subset the display font to Latin (Spanish + English) for the public site.
#
#   U+0020-007E  ASCII
#   U+00A0-00FF  Latin-1 (á é í ó ú ü ñ ¿ ¡ « » …)
#   U+2010-2027  dashes, quotes, bullet, ellipsis
#   U+2030-203A  per mille, primes, single guillemets
#   U+20AC       euro
#
# Source: src/dev/fonts (full font, OFL). Output: src/assets/fonts (shipped).
# Needs uv (https://docs.astral.sh/uv/); fonttools and brotli run ephemerally.
set -euo pipefail
cd "$(dirname "$0")/.."

UNICODES="U+0020-007E,U+00A0-00FF,U+2010-2027,U+2030-203A,U+20AC"

subset() {
  uvx --from fonttools --with brotli pyftsubset "src/dev/fonts/$1.woff2" \
    --unicodes="$UNICODES" \
    --layout-features='kern,liga,clig,calt' \
    --flavor=woff2 \
    --no-hinting \
    --name-IDs='0,1,2,3,4,5,6' \
    --output-file="src/assets/fonts/$1.latin.woff2"
  printf '%s: %s -> %s bytes\n' "$1" \
    "$(wc -c < "src/dev/fonts/$1.woff2" | tr -d ' ')" \
    "$(wc -c < "src/assets/fonts/$1.latin.woff2" | tr -d ' ')"
}

subset Redaction35-Regular
