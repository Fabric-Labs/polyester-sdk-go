package realtime

import (
	"errors"
	"testing"

	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
)

func TestEnqueueSucceedsWhenQueueHasCapacity(t *testing.T) {
	canceled := false
	sub := newSubscription[int](2, func() { canceled = true })

	if !sub.enqueue(1) {
		t.Fatal("expected enqueue to succeed")
	}
	if canceled {
		t.Fatal("did not expect cancel on successful enqueue")
	}
	if got := <-sub.Messages(); got != 1 {
		t.Fatalf("got %d, want 1", got)
	}
}

func TestEnqueueFailsSubscriptionOnOverflow(t *testing.T) {
	canceled := false
	sub := newSubscription[int](1, func() { canceled = true })

	if !sub.enqueue(1) {
		t.Fatal("expected first enqueue to succeed")
	}
	if sub.enqueue(2) {
		t.Fatal("expected second enqueue to fail on full queue")
	}
	if !canceled {
		t.Fatal("expected cancel on overflow")
	}
	err := sub.Err()
	if err == nil {
		t.Fatal("expected overflow error")
	}
	var overflow *sdkerrors.QueueOverflowError
	if !errors.As(err, &overflow) {
		t.Fatalf("expected QueueOverflowError, got %T: %v", err, err)
	}
	select {
	case <-sub.Done():
	default:
		t.Fatal("expected subscription Done after overflow")
	}
}

func TestEnqueueAfterOverflowIsRejected(t *testing.T) {
	sub := newSubscription[int](1, func() {})
	_ = sub.enqueue(1)
	_ = sub.enqueue(2)
	if sub.enqueue(3) {
		t.Fatal("expected enqueue after overflow to fail")
	}
}
