package auth

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"math"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
)

// MaxSigningFutureSkewMS bounds how far ahead of this host's clock an
// automatically allocated timestamp may run. The API refuses a timestamp more
// than one second ahead of its own clock as TIMESTAMP_SKEW; half a second
// leaves the other half for clock skew.
const MaxSigningFutureSkewMS int64 = 500

const maxSigningBackpressure = 10 * time.Second

type signingTimestampAllocator struct {
	last atomic.Int64
}

var signingTimestampAllocators sync.Map

func automaticSigningTimestamp(creds *Credentials) (string, error) {
	private := creds.privateKey()
	if len(private) != ed25519.PrivateKeySize {
		return "", &sdkerrors.AuthError{Msg: "Ed25519 API private key must be exactly 64 bytes"}
	}
	identity := sha256.Sum256(append(append([]byte{}, creds.KeyID...), private...))
	key := hex.EncodeToString(identity[:])
	value, _ := signingTimestampAllocators.LoadOrStore(key, &signingTimestampAllocator{})
	allocator := value.(*signingTimestampAllocator)
	started := time.Now()

	for {
		now := time.Now().UnixMilli()
		if now < 0 {
			return "", &sdkerrors.TransportError{Msg: "system clock is before UNIX_EPOCH"}
		}
		if now > math.MaxInt64-MaxSigningFutureSkewMS {
			return "", &sdkerrors.TransportError{Msg: "signing timestamp ceiling overflow"}
		}
		ceiling := now + MaxSigningFutureSkewMS
		observed := allocator.last.Load()
		candidate := now
		if observed >= now {
			if observed == math.MaxInt64 {
				return "", &sdkerrors.TransportError{Msg: "signing timestamp sequence exhausted"}
			}
			candidate = observed + 1
		}
		if candidate <= ceiling && allocator.last.CompareAndSwap(observed, candidate) {
			return strconv.FormatInt(candidate, 10), nil
		}
		if time.Since(started) >= maxSigningBackpressure {
			retryAfter := 0.001
			return "", &sdkerrors.RateLimitError{
				Msg:        "signing timestamp capacity exhausted; retry after clock advances",
				RetryAfter: &retryAfter,
			}
		}
		time.Sleep(time.Millisecond)
	}
}
