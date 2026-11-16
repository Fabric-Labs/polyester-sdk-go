package services

import (
	"context"
	"errors"
	"strings"
	"testing"

	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	ob "github.com/Fabric-Labs/polyester-sdk-go/gen/orderbook/v1"
)

// A subscription selected by SymbolID alone takes its symbol and quantity
// scale from the catalog entry for that ID.
func TestCreateSubscriptionSymbolIDAloneUsesCatalog(t *testing.T) {
	id := uint32(2)
	svc, sends, _, _ := bookTicketService(t, 1, true, id)
	sub, err := svc.CreateSubscription(context.Background(), CreateSubscriptionOptions{SymbolID: &id})
	if err != nil {
		t.Fatalf("SymbolID alone with a hydrated catalog: %v", err)
	}
	defer sub.Close()
	seed, ok := bookTicketNext(t, sub.Updates())
	if !ok || seed.Symbol != "ETH-USDT" {
		t.Fatalf("seed view %+v, want symbol ETH-USDT", seed)
	}
	sends <- bookTicketFrame(&ob.OrderBookDelta{SymbolId: id, BookSeqStart: 0, BookSeqEnd: 2, Reset_: true, Bids: []*ob.PriceLevel{{PriceTicks: 1000000000, QtyScaled: 1000000}}})
	reset, ok := bookTicketNext(t, sub.Updates())
	if !ok || reset.Symbol != "ETH-USDT" || len(reset.Bids) != 1 {
		t.Fatalf("reset view %+v", reset)
	}
	// ETH-USDT has quantity scale 6: 1_000_000 scaled is 1.
	if got, err := reset.Bids[0].Qty.Format(); err != nil || got != "1" {
		t.Fatalf("quantity %q err=%v, want 1 at the ETH-USDT scale", got, err)
	}
}

func TestCreateSubscriptionUnknownSymbolIDFailsBeforeIO(t *testing.T) {
	svc, _, _, calls := bookTicketService(t, 1, true)
	id := uint32(99)
	sub, err := svc.CreateSubscription(context.Background(), CreateSubscriptionOptions{SymbolID: &id})
	if sub != nil {
		sub.Close()
	}
	var validation *sdkerrors.ValidationError
	if !errors.As(err, &validation) || !strings.Contains(err.Error(), "symbol_id 99") {
		t.Fatalf("expected a validation error naming symbol_id 99, got %v", err)
	}
	if calls.Load() != 0 {
		t.Fatal("unknown symbol_id performed snapshot I/O")
	}
}

func TestCreateSubscriptionRefusesInvalidBucketBeforeIO(t *testing.T) {
	for _, bucket := range []string{"abc", "-1", "0"} {
		svc, _, _, calls := bookTicketService(t, 1, true)
		sub, err := svc.CreateSubscription(context.Background(), CreateSubscriptionOptions{Symbol: "BTC-USDT", Bucket: bucket})
		if sub != nil {
			sub.Close()
		}
		var validation *sdkerrors.ValidationError
		if !errors.As(err, &validation) || !strings.Contains(err.Error(), "bucket") {
			t.Fatalf("Bucket %q: err=%v, want a bucket *errors.ValidationError", bucket, err)
		}
		if calls.Load() != 0 {
			t.Fatalf("Bucket %q performed snapshot I/O", bucket)
		}
	}
}
