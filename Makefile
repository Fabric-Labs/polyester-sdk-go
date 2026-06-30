run:
	go test ./...

test:
	go test ./...

test-integration:
	go test -tags=integration -v ./tests/integration/...

test-all:
	./scripts/test_all.sh

lint:
	@./scripts/run-lint.sh

fix:
	@./scripts/run-fix.sh

build:
	go build ./...

.PHONY: run test test-integration test-all lint fix build
