// Package retrywait decides how long the SDK's retries of safe reads wait.
package retrywait

import (
	"time"

	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	"github.com/Fabric-Labs/polyester-sdk-go/wire"
)

// maxWait bounds a delay an answer asks for, so it never overflows.
const maxWait = 24 * time.Hour

// Delay returns how long a retry after err waits: fallback, or the delay the
// answer asked for with Retry-After when that is longer. It reads the delay
// from the one place the SDK keeps it, errors.RetryAfter of the mapped error.
// The caller gives up instead when the delay would end past its budget.
func Delay(err error, fallback time.Duration) time.Duration {
	seconds := sdkerrors.RetryAfter(wire.MapConnectError(err))
	if seconds == nil {
		return fallback
	}
	asked := maxWait
	if *seconds < maxWait.Seconds() {
		asked = time.Duration(*seconds * float64(time.Second))
	}
	return max(fallback, asked)
}
