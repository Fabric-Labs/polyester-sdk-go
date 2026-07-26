run:
	go test ./...

test:
	go test ./...

# Offline public smoke (no API credentials): unit + hardening L2 + public_smoke.
test-public-smoke:
	go test ./... ./tests/public_smoke/... -count=1

# Credentialed live suite (requires POLYESTER_API_KEY_*).
test-integration:
	go test -tags=integration -v ./tests/integration/...

# A7 release gate: credentialed suite with executed/skipped/failed counts.
# Set POLYESTER_TEST_STRICT_LIVE=1 and optionally POLYESTER_TEST_MIN_EXECUTED=5.
test-integration-strict:
	POLYESTER_TEST_STRICT_LIVE=1 ./scripts/run_integration_a7.sh

test-all:
	./scripts/test_all.sh

lint:
	@./scripts/run-lint.sh

fix:
	@./scripts/run-fix.sh

build:
	go build ./...

.PHONY: run test test-public-smoke test-integration test-integration-strict test-all lint fix build
