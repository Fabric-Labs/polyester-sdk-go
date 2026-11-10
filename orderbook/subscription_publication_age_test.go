package orderbook

import (
	"sync"
	"testing"
	"time"
)

// POLY-8563 review: SinceLastPublication raced NotePublication. It sampled the
// clock before loading the last publication, so a publication noted between
// the two made the age negative; and a slow NotePublication could store an
// older time over a newer one. Writers and a reader run concurrently here; the
// age must never be negative or exceed the time since the subscription began.
func TestSinceLastPublicationUnderConcurrentPublications(t *testing.T) {
	start := time.Now()
	sub := NewSubscription(nil, nil)
	stop := make(chan struct{})
	var writers sync.WaitGroup
	for range 4 {
		writers.Add(1)
		go func() {
			defer writers.Done()
			for {
				select {
				case <-stop:
					return
				default:
					sub.NotePublication()
				}
			}
		}()
	}
	deadline := time.Now().Add(300 * time.Millisecond)
	var bad []time.Duration
	reads := 0
	for time.Now().Before(deadline) {
		age := sub.SinceLastPublication()
		reads++
		if age < 0 || age > time.Since(start) {
			bad = append(bad, age)
		}
	}
	close(stop)
	writers.Wait()
	if len(bad) > 0 {
		t.Fatalf("%d of %d reads out of range, first %v", len(bad), reads, bad[0])
	}

	// Once publications stop, the age grows from the last one.
	time.Sleep(50 * time.Millisecond)
	if age := sub.SinceLastPublication(); age < 50*time.Millisecond {
		t.Fatalf("age 50ms after the last publication = %v", age)
	}
}
