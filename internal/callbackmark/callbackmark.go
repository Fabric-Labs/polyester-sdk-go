// Package callbackmark records which goroutines are running a callback of
// which stream, so a stream's Close called from one of its own callbacks can
// signal shutdown without waiting for the worker that is running it.
//
// A stream marks every callback it calls with Run, keyed by the stream
// itself, and so do the wrappers that call callbacks for it, such as the
// managed orderbook and market-overview subscriptions. Close asks Active.
package callbackmark

import (
	"bytes"
	"runtime"
	"strconv"
	"sync"
)

type entry struct {
	goroutine uint64
	owner     any
}

var (
	mu     sync.Mutex
	active = map[entry]int{}
)

// Run runs fn on the calling goroutine as a callback of owner, which must be
// comparable (a stream pointer). Calls nest.
func Run(owner any, fn func()) {
	id := goroutineID()
	if id == 0 {
		fn()
		return
	}
	key := entry{goroutine: id, owner: owner}
	mu.Lock()
	active[key]++
	mu.Unlock()
	defer func() {
		mu.Lock()
		if active[key]--; active[key] == 0 {
			delete(active, key)
		}
		mu.Unlock()
	}()
	fn()
}

// Active reports whether the calling goroutine is running a callback of owner.
func Active(owner any) bool {
	id := goroutineID()
	if id == 0 {
		return false
	}
	mu.Lock()
	defer mu.Unlock()
	return active[entry{goroutine: id, owner: owner}] > 0
}

// goroutineID returns the runtime's ID of the calling goroutine, read from the
// first line of its stack trace ("goroutine 18 [running]:"), or 0 when that
// line can't be read. IDs start at 1. Go has no API for it; it is read only to
// tell a Close made from a callback from every other Close.
func goroutineID() uint64 {
	var buf [64]byte
	line := bytes.TrimPrefix(buf[:runtime.Stack(buf[:], false)], []byte("goroutine "))
	end := bytes.IndexByte(line, ' ')
	if end <= 0 {
		return 0
	}
	id, err := strconv.ParseUint(string(line[:end]), 10, 64)
	if err != nil {
		return 0
	}
	return id
}
