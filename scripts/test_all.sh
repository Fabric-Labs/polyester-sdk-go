#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."

if [[ -f .env ]]; then
  set -a
  # shellcheck disable=SC1091
  source .env
  set +a
fi

echo "== unit =="
go test ./...

if [[ -z "${POLYESTER_API_KEY_ID:-}" || -z "${POLYESTER_API_PRIVATE_KEY:-}" ]]; then
  echo "Skipping integration tests (set POLYESTER_API_KEY_ID and POLYESTER_API_PRIVATE_KEY in .env)"
  exit 0
fi

echo "== integration =="
go test -tags=integration -v ./tests/integration/...
