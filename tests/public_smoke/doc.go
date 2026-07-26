// Package public_smoke holds offline / no-credential smoke coverage for the Go SDK.
//
// Run with:
//
//	go test ./tests/public_smoke/...
//	make test-public-smoke
//
// Credentialed live suites live under tests/integration (build tag integration)
// and are gated by POLYESTER_API_KEY_* plus optional POLYESTER_TEST_STRICT_LIVE.
package public_smoke
