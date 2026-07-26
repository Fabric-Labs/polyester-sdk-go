package hardening

import (
	"testing"
	"time"
)

// WaitUntil polls pred until true or timeout; fatals on timeout.
func WaitUntil(t *testing.T, pred func() bool, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for !pred() {
		if time.Now().After(deadline) {
			t.Fatalf("condition not met within %s", timeout)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
