package realtime

import (
	"bytes"
	"runtime"
	"strconv"
)

// currentGoroutineID returns the runtime's ID of the calling goroutine, read
// from the first line of its stack trace ("goroutine 18 [running]:"), or 0
// when that line can't be read. IDs start at 1.
//
// SnapshotThenStream uses it to tell a Close called from one of its callbacks,
// which must not wait, from every other Close, which must.
func currentGoroutineID() uint64 {
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
