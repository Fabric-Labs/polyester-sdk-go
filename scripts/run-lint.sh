#!/usr/bin/env bash
# Run golangci-lint from moon, PATH, or $(go env GOPATH)/bin (GitHub Desktop often omits the latter).
set -euo pipefail

cd "$(dirname "$0")/.."

if command -v go >/dev/null 2>&1; then
  export PATH="${PATH}:$(go env GOPATH)/bin"
fi
export PATH="${PATH}:/usr/local/bin:/opt/homebrew/bin"

if command -v moon >/dev/null 2>&1; then
  exec moon run :lint
fi

if command -v golangci-lint >/dev/null 2>&1; then
  exec golangci-lint run ./...
fi

echo "golangci-lint not found; install with:" >&2
echo "  go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.1.6" >&2
echo "and ensure \$(go env GOPATH)/bin is on your PATH." >&2
exit 127
