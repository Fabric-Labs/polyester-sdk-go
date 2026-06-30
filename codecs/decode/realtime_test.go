package decode

import (
	"testing"

	orderbookv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/orderbook/v1"
	"google.golang.org/protobuf/proto"
)

func TestOrderbookDeltaFromBytes(t *testing.T) {
	msg := &orderbookv1.OrderBookDelta{
		SymbolId:     42,
		BookSeqStart: 1,
		BookSeqEnd:   2,
		Bids: []*orderbookv1.PriceLevel{
			{PriceTicks: 100, QtyScaled: 5},
		},
	}
	payload, err := proto.Marshal(msg)
	if err != nil {
		t.Fatal(err)
	}
	delta, err := OrderbookDeltaFromBytes(payload)
	if err != nil {
		t.Fatal(err)
	}
	if delta.SymbolID != 42 || delta.BookSeqEnd != "2" || len(delta.Bids) != 1 {
		t.Fatalf("delta=%+v", delta)
	}
}
