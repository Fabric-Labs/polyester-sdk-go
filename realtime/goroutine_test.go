package realtime

import "testing"

func TestCurrentGoroutineIDTellsGoroutinesApart(t *testing.T) {
	here := currentGoroutineID()
	if here == 0 || currentGoroutineID() != here {
		t.Fatalf("goroutine ID %d is not stable", here)
	}
	other := make(chan uint64)
	go func() { other <- currentGoroutineID() }()
	if id := <-other; id == 0 || id == here {
		t.Fatalf("goroutines %d and %d were not told apart", here, id)
	}
}
