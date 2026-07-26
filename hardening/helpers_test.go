package hardening_test

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Fabric-Labs/polyester-sdk-go/auth"
	"github.com/Fabric-Labs/polyester-sdk-go/realtime"
)

const testKeyHex = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func testCreds(t *testing.T) *auth.Credentials {
	t.Helper()
	creds, err := auth.LoadCredentials("ak_test", testKeyHex, false)
	if err != nil || creds == nil {
		t.Fatalf("creds: %v", err)
	}
	return creds
}

func newRT(wsURL, apiURL string, creds *auth.Credentials, timeout time.Duration) *realtime.Client {
	return realtime.NewClient(wsURL, apiURL, creds, &http.Client{Timeout: timeout}, 1000)
}

func errText(err error) string {
	if err == nil {
		return ""
	}
	return strings.ToLower(err.Error())
}

func isTimeoutErr(err error) bool {
	s := errText(err)
	return strings.Contains(s, "timeout") ||
		strings.Contains(s, "timed out") ||
		strings.Contains(s, "deadline exceeded")
}

func subAlive[T any](sub *realtime.Subscription[T]) bool {
	if sub == nil {
		return false
	}
	select {
	case <-sub.Done():
		return false
	default:
		return true
	}
}

func identityDecode(b []byte) ([]byte, error) { return b, nil }
