package auth

import (
	"crypto/ed25519"
	"strconv"
	"testing"
	"time"
)

// serverFutureAllowanceMS is how far ahead of the server clock the API accepts
// an API-key timestamp: one second. A timestamp further ahead is refused as
// TIMESTAMP_SKEW.
const serverFutureAllowanceMS int64 = 1_000

func TestAutomaticTimestampCeilingLeavesServerFutureAllowanceUnused(t *testing.T) {
	if MaxSigningFutureSkewMS <= 0 || MaxSigningFutureSkewMS >= serverFutureAllowanceMS {
		t.Fatalf("MaxSigningFutureSkewMS = %d, want between 1 and %d so clock skew still fits", MaxSigningFutureSkewMS, serverFutureAllowanceMS-1)
	}
}

func TestBurstOfAutomaticTimestampsStaysWithinServerFutureAllowance(t *testing.T) {
	_, private, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	creds := testCredentials(t, "key_future_allowance", private)
	const count = 2_000
	for i := range count {
		headers, err := SignRequest(creds, "GET", "https://api.example.test/v1/rt/token", nil, "")
		if err != nil {
			t.Fatalf("sign %d: %v", i, err)
		}
		now := time.Now().UnixMilli()
		ts, err := strconv.ParseInt(headers["X-API-TIMESTAMP"], 10, 64)
		if err != nil {
			t.Fatal(err)
		}
		if ahead := ts - now; ahead > serverFutureAllowanceMS {
			t.Fatalf("request %d signed %d ms ahead of the clock; the server refuses more than %d ms", i, ahead, serverFutureAllowanceMS)
		}
	}
}
