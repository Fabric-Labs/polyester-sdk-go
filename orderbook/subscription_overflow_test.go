package orderbook

import (
	"errors"
	"testing"

	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

func TestOrderbookEnqueueFailsOnOverflow(t *testing.T) {
	sub := &Subscription{
		updates: make(chan models.OrderbookData, 1),
	}
	if !sub.Enqueue(models.OrderbookData{Symbol: "BTCUSDT"}) {
		t.Fatal("expected first enqueue to succeed")
	}
	if sub.Enqueue(models.OrderbookData{Symbol: "ETHUSDT"}) {
		t.Fatal("expected overflow to fail")
	}
	var overflow *sdkerrors.QueueOverflowError
	if !errors.As(sub.Err(), &overflow) {
		t.Fatalf("expected QueueOverflowError, got %v", sub.Err())
	}
}
