#!/usr/bin/env bash
# Install git pre-commit hook for golangci-lint (uses pre-commit framework when available).
set -euo pipefail

cd "$(dirname "$0")/.."

if command -v pre-commit >/dev/null 2>&1; then
  pre-commit install
  echo "Installed pre-commit hook (runs: moon run :lint or make lint on .go changes)"
  exit 0
fi

if ! command -v golangci-lint >/dev/null 2>&1; then
  echo "Install golangci-lint first: https://golangci-lint.run/welcome/install/"
  echo "Optional: pip install pre-commit  (or brew install pre-commit)"
  exit 1
fi

hook_path=".git/hooks/pre-commit"
mkdir -p "$(dirname "$hook_path")"
cat >"$hook_path" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
root="$(git rev-parse --show-toplevel)"
cd "$root"
if ! git diff --cached --name-only --diff-filter=ACM | grep -q '\.go$'; then
  exit 0
fi
if command -v moon >/dev/null 2>&1; then
  moon run :lint
else
  make lint
fi
EOF
chmod +x "$hook_path"
echo "Installed $hook_path (runs golangci-lint before commits with staged .go files)"
echo "Tip: pip install pre-commit && ./scripts/setup-hooks.sh for the shared .pre-commit-config.yaml"
