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
// SnapshotThenStream.Close uses it to tell its own worker, which can't wait
// for itself to exit, from every other caller, which must.
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
