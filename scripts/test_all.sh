#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."

if [[ -f .env ]]; then
  set -a
  # shellcheck disable=SC1091
  source .env
  set +a
fi

echo "== public smoke (unit + hardening + public_smoke) =="
go test ./... ./tests/public_smoke/... -count=1

if [[ -z "${POLYESTER_API_KEY_ID:-}" || -z "${POLYESTER_API_PRIVATE_KEY:-}" ]]; then
  echo "Skipping credentialed integration tests (set POLYESTER_API_KEY_ID and POLYESTER_API_PRIVATE_KEY in .env)"
  exit 0
fi

echo "== credentialed integration (A7 counts) =="
./scripts/run_integration_a7.sh
