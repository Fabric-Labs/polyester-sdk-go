package auth

import (
	"encoding/hex"
	"fmt"
	"math/rand/v2"
	"os"
	"runtime"
	"strconv"
	"sync"
	"testing"
	"time"
)

// TestIndependentCredentialsForOneKeyNeverShareASignature drives seeded
// schedules in which several independently loaded Credentials for the same
// key sign identical requests from many goroutines. The server rejects a
// repeated (key ID, signature) pair, so every signature must be distinct and
// every timestamp must stay within the skew bound.
//
// POLYESTER_SIGNING_SEEDS sets the seed count (default 32); POLYESTER_SIGNING_SEED
// replays one seed.
func TestIndependentCredentialsForOneKeyNeverShareASignature(t *testing.T) {
	seeds := uint64(32)
	if raw := os.Getenv("POLYESTER_SIGNING_SEEDS"); raw != "" {
		parsed, err := strconv.ParseUint(raw, 10, 64)
		if err != nil {
			t.Fatal(err)
		}
		seeds = parsed
	}
	first := uint64(0)
	if raw := os.Getenv("POLYESTER_SIGNING_SEED"); raw != "" {
		parsed, err := strconv.ParseUint(raw, 10, 64)
		if err != nil {
			t.Fatal(err)
		}
		first, seeds = parsed, 1
	}
	for seed := first; seed < first+seeds; seed++ {
		t.Run(fmt.Sprint(seed), func(t *testing.T) { runSigningSchedule(t, seed) })
	}
}

type signingStep struct {
	credentials int
	yield       int // 0 none, 1 Gosched, 2 sleep
}

func runSigningSchedule(t *testing.T, seed uint64) {
	rng := rand.New(rand.NewPCG(seed, 0))
	seedHex := hex.EncodeToString(syntheticSeed())
	keyID := fmt.Sprintf("key_interleave_%d_%d", seed, time.Now().UnixNano())

	credentialCount := 2 + rng.IntN(4)
	credentials := make([]*Credentials, credentialCount)
	for i := range credentials {
		creds, err := LoadCredentials(keyID, seedHex, false)
		if err != nil {
			t.Fatal(err)
		}
		credentials[i] = creds
	}

	goroutines := 2 + rng.IntN(15)
	plans := make([][]signingStep, goroutines)
	for g := range plans {
		steps := 5 + rng.IntN(40)
		plans[g] = make([]signingStep, steps)
		for i := range plans[g] {
			plans[g][i] = signingStep{credentials: rng.IntN(credentialCount), yield: rng.IntN(3)}
		}
	}

	type signed struct {
		timestamp  int64
		signature  string
		observedAt int64
	}
	var mu sync.Mutex
	var results []signed
	before := time.Now().UnixMilli()
	start := make(chan struct{})
	var wg sync.WaitGroup
	for _, plan := range plans {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			for _, step := range plan {
				switch step.yield {
				case 1:
					runtime.Gosched()
				case 2:
					time.Sleep(50 * time.Microsecond)
				}
				headers, err := SignRequest(credentials[step.credentials], "POST", "https://api.example.test/orders", []byte("{}"), "")
				if err != nil {
					t.Errorf("sign: %v", err)
					return
				}
				ts, err := strconv.ParseInt(headers["X-API-TIMESTAMP"], 10, 64)
				if err != nil {
					t.Errorf("timestamp: %v", err)
					return
				}
				mu.Lock()
				results = append(results, signed{ts, headers["X-API-SIGNATURE"], time.Now().UnixMilli()})
				mu.Unlock()
			}
		}()
	}
	close(start)
	wg.Wait()

	timestamps := make(map[int64]struct{}, len(results))
	signatures := make(map[string]struct{}, len(results))
	for _, result := range results {
		if result.timestamp < before || result.timestamp > result.observedAt+MaxSigningFutureSkewMS {
			t.Fatalf("seed %d: timestamp %d outside [%d, %d]", seed, result.timestamp, before, result.observedAt+MaxSigningFutureSkewMS)
		}
		timestamps[result.timestamp] = struct{}{}
		signatures[result.signature] = struct{}{}
	}
	if len(signatures) != len(results) || len(timestamps) != len(results) {
		t.Fatalf(
			"seed %d: %d credentials, %d goroutines: %d signatures, %d distinct signatures, %d distinct timestamps",
			seed, credentialCount, goroutines, len(results), len(signatures), len(timestamps),
		)
	}
}
