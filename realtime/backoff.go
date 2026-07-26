package realtime

import (
	cryptorand "crypto/rand"
	"encoding/binary"
	"time"
)

const (
	reconnectInitialCap = 500 * time.Millisecond
	reconnectMaxCap     = 30 * time.Second
)

// reconnectBackoff uses exponential caps and per-attempt jitter so a fleet
// does not reconnect in lockstep after an upstream interruption.
type reconnectBackoff struct {
	cap time.Duration
}

func (b *reconnectBackoff) reset() {
	b.cap = reconnectInitialCap
}

func (b *reconnectBackoff) next() time.Duration {
	if b.cap <= 0 {
		b.reset()
	}
	half := b.cap / 2
	var raw [8]byte
	var n uint64
	if _, err := cryptorand.Read(raw[:]); err == nil {
		n = binary.LittleEndian.Uint64(raw[:])
	} else {
		n = uint64(time.Now().UnixNano())
	}
	delay := half + time.Duration(n%uint64(half+1))
	if b.cap < reconnectMaxCap {
		b.cap *= 2
		if b.cap > reconnectMaxCap {
			b.cap = reconnectMaxCap
		}
	}
	return delay
}
