package callbackmark

import "testing"

func TestGoroutineIDTellsGoroutinesApart(t *testing.T) {
	here := goroutineID()
	if here == 0 || goroutineID() != here {
		t.Fatalf("goroutine ID %d is not stable", here)
	}
	other := make(chan uint64)
	go func() { other <- goroutineID() }()
	if id := <-other; id == 0 || id == here {
		t.Fatalf("goroutines %d and %d were not told apart", here, id)
	}
}

func TestActiveOnlyInsideRunForTheSameOwnerAndGoroutine(t *testing.T) {
	a, b := new(int), new(int)
	if Active(a) {
		t.Fatal("active outside Run")
	}
	Run(a, func() {
		if !Active(a) || Active(b) {
			t.Fatal("marks the wrong owner")
		}
		Run(a, func() {})
		if !Active(a) {
			t.Fatal("a nested Run cleared the outer mark")
		}
		elsewhere := make(chan bool)
		go func() { elsewhere <- Active(a) }()
		if <-elsewhere {
			t.Fatal("marks another goroutine")
		}
	})
	if Active(a) {
		t.Fatal("still active after Run returned")
	}
	mu.Lock()
	defer mu.Unlock()
	if len(active) != 0 {
		t.Fatalf("leaked marks: %v", active)
	}
}
