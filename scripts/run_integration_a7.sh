#!/usr/bin/env bash
# A7: run credentialed integration tests and emit executed/skipped/failed counts.
# Under POLYESTER_TEST_STRICT_LIVE=1, enforce POLYESTER_TEST_MIN_EXECUTED (default 5).
set -euo pipefail

cd "$(dirname "$0")/.."

if [[ -f .env ]]; then
  set -a
  # shellcheck disable=SC1091
  source .env
  set +a
fi

if [[ -z "${POLYESTER_API_KEY_ID:-}" || -z "${POLYESTER_API_PRIVATE_KEY:-}" ]]; then
  if [[ "${POLYESTER_TEST_STRICT_LIVE:-}" == "1" || "${POLYESTER_TEST_STRICT_LIVE:-}" == "true" ]]; then
    echo "STRICT_LIVE requires POLYESTER_API_KEY_ID and POLYESTER_API_PRIVATE_KEY" >&2
    exit 1
  fi
  echo "Skipping credentialed integration (set POLYESTER_API_KEY_ID / POLYESTER_API_PRIVATE_KEY)"
  exit 0
fi

OUT="$(mktemp)"
trap 'rm -f "$OUT"' EXIT

set +e
go test -tags=integration -count=1 -json ./tests/integration/... | tee "$OUT"
test_rc=${PIPESTATUS[0]}
set -e

python3 - "$OUT" "$test_rc" <<'PY'
import json, os, sys

path, test_rc = sys.argv[1], int(sys.argv[2])
passed = failed = skipped = 0
for line in open(path, encoding="utf-8"):
    line = line.strip()
    if not line:
        continue
    try:
        ev = json.loads(line)
    except json.JSONDecodeError:
        continue
    if not ev.get("Test"):
        continue
    action = ev.get("Action")
    if action == "pass":
        passed += 1
    elif action == "fail":
        failed += 1
    elif action == "skip":
        skipped += 1

executed = passed + failed
print(
    f"\nA7 live harness counts: executed={executed} skipped={skipped} failed={failed}",
    flush=True,
)
strict = os.environ.get("POLYESTER_TEST_STRICT_LIVE", "").lower() in ("1", "true", "yes")
min_floor = int(os.environ.get("POLYESTER_TEST_MIN_EXECUTED", "5"))
if strict and executed < min_floor:
    print(
        f"STRICT_LIVE requires at least {min_floor} executed live tests; got {executed}",
        file=sys.stderr,
    )
    sys.exit(1)
sys.exit(test_rc)
PY
