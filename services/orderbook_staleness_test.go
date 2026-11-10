package services

import (
	"context"
	"testing"
	"time"

	ob "github.com/Fabric-Labs/polyester-sdk-go/gen/orderbook/v1"
)

// POLY-8563: a stalled publisher must be distinguishable from a quiet market.
// The server republishes every book about every 30 s, so the time since the
// last publication tells them apart; the subscription reports it.
func TestManagedBookReportsTimeSinceLastPublication(t *testing.T) {
	svc, sends, _, _ := bookTicketService(t, 1, true)
	created := time.Now()
	sub, err := svc.CreateSubscription(context.Background(), CreateSubscriptionOptions{Symbol: "BTC-USDT"})
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close()
	if _, ok := bookTicketNext(t, sub.Updates()); !ok {
		t.Fatal("no snapshot view")
	}

	// No publication yet: the age runs from the subscription's start.
	time.Sleep(250 * time.Millisecond)
	if age := sub.SinceLastPublication(); age < 250*time.Millisecond || age > time.Since(created) {
		t.Fatalf("age before any publication = %v, want between 250ms and %v", age, time.Since(created))
	}

	// A publication, here a reset republishing the same view, restarts it.
	sends <- bookTicketFrame(&ob.OrderBookDelta{SymbolId: 1, BookSeqStart: 0, BookSeqEnd: 2, Reset_: true})
	if _, ok := bookTicketNext(t, sub.Updates()); !ok {
		t.Fatal("no view for the publication")
	}
	if age := sub.SinceLastPublication(); age > 200*time.Millisecond {
		t.Fatalf("age just after a publication = %v", age)
	}

	// Silence makes it grow again; a bucket change is not a publication.
	time.Sleep(300 * time.Millisecond)
	sub.SetBucket("1")
	if _, ok := bookTicketNext(t, sub.Updates()); !ok {
		t.Fatal("no view for the bucket change")
	}
	if age := sub.SinceLastPublication(); age < 300*time.Millisecond {
		t.Fatalf("age after 300ms of silence = %v", age)
	}
}
