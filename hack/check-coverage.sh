#!/usr/bin/env bash
# Fails when the total coverage in a Go cover profile is below a threshold.
set -euo pipefail

cover_file="${1:-cover.out}"
threshold="${2:-80}"

if [ ! -f "$cover_file" ]; then
  echo "::error::Coverage file not found: $cover_file" >&2
  exit 1
fi

coverage=$(go tool cover -func="$cover_file" | awk '/^total:/ {gsub("%", "", $3); print $3}')
if [ -z "$coverage" ]; then
  echo "::error::Could not parse total coverage from $cover_file" >&2
  exit 1
fi

echo "Total coverage: ${coverage}%"
if [ "$(echo "$coverage < $threshold" | bc -l)" -eq 1 ]; then
  echo "::error::Coverage ${coverage}% is below ${threshold}% threshold"
  exit 1
fi
