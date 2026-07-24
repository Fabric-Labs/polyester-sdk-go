package decode

import (
	"testing"

	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	authv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/auth/v1"
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

func TestApiPolicyFromBytes(t *testing.T) {
	msg := &authv1.ApiPolicyView{
		Id:          9,
		Name:        "bots",
		Description: "api key policy",
		Revision:    4,
	}
	payload, err := proto.Marshal(msg)
	if err != nil {
		t.Fatal(err)
	}
	policy, err := ApiPolicyFromBytes(payload)
	if err != nil {
		t.Fatal(err)
	}
	if policy.PolicyID != codecs.FormatUint64ID(9) || policy.Name != "bots" || policy.Revision != 4 {
		t.Fatalf("policy=%+v", policy)
	}
}

func TestSubaccountPolicyFromBytes(t *testing.T) {
	msg := &authv1.SubaccountPolicyView{
		Id:       7,
		Name:     "trader",
		Revision: 3,
	}
	payload, err := proto.Marshal(msg)
	if err != nil {
		t.Fatal(err)
	}
	policy, err := SubaccountPolicyFromBytes(payload)
	if err != nil {
		t.Fatal(err)
	}
	if policy.PolicyID != codecs.FormatUint64ID(7) || policy.Name != "trader" || policy.Revision != 3 {
		t.Fatalf("policy=%+v", policy)
	}
}
