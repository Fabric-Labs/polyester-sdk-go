package decode_test

import (
	"testing"
	"time"

	"github.com/Fabric-Labs/polyester-sdk-go/codecs/decode"
	authv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/auth/v1"
	chaindepositv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/chain/deposit/v1"
	chainwithdrawv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/chain/withdraw/v1"
	marketdatav1 "github.com/Fabric-Labs/polyester-sdk-go/gen/marketdata/v1"
	marketoverviewv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/marketoverview/v1"
	orderbookv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/orderbook/v1"
	typev1 "github.com/Fabric-Labs/polyester-sdk-go/gen/polyester/type/v1"
	transferv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/transfer/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestDepositAddressFromProto(t *testing.T) {
	msg := &chaindepositv1.ListDepositAddressesResponse{
		DepositAddresses: []*chaindepositv1.DepositAddress{
			{ChainId: 1, DepositAddress: "0xabc"},
		},
	}
	result := decode.DepositAddressesListFromProto(msg)
	if len(result.Addresses) != 1 || result.Addresses[0].ChainID != 1 || result.Addresses[0].DepositAddress != "0xabc" {
		t.Fatalf("addresses=%+v", result.Addresses)
	}
}

func TestWithdrawIntentFromProto(t *testing.T) {
	msg := &chainwithdrawv1.CreateTradingWithdrawResponse{IntentId: "intent-1"}
	result := decode.WithdrawIntentFromProto(msg)
	if result.IntentID != "intent-1" {
		t.Fatalf("intent=%+v", result)
	}
}

func TestInternalTransferFromProto(t *testing.T) {
	msg := &transferv1.CreateInternalTransferResponse{
		RequestId:  "req-1",
		TransferId: "xfer-1",
		AssetId:    3,
		AssetCode:  "USDT",
		AmountE18:  &typev1.U128{Lo: 500},
	}
	result := decode.InternalTransferFromProto(msg)
	qty, err := result.Quantity.Int64()
	if err != nil || result.RequestID != "req-1" || result.TransferID != "xfer-1" || qty != 500 {
		t.Fatalf("transfer=%+v err=%v", result, err)
	}
}

func TestAPIKeysFromProto(t *testing.T) {
	msg := &authv1.ListApiKeysResponse{
		ApiKeys: []*authv1.ApiKey{
			{
				KeyId: "key-1", Label: "bot", Status: authv1.ApiKeyStatus_ACTIVE,
				CreatedAt:  timestamppb.New(time.Unix(1, 0)),
				LastUsedAt: timestamppb.New(time.Unix(2, 0)),
				UpdatedAt:  timestamppb.New(time.Unix(3, 123_456_000)),
			},
		},
	}
	result := decode.ApiKeysListFromProto(msg)
	if len(result.Keys) != 1 || result.Keys[0].KeyID != "key-1" || result.Keys[0].Status != "ACTIVE" {
		t.Fatalf("keys=%+v", result.Keys)
	}
	key := result.Keys[0]
	if key.CreatedAt == nil || key.LastUsedAt == nil || key.UpdatedAt == nil ||
		key.CreatedAt.Unix() != 1 || key.LastUsedAt.Unix() != 2 ||
		key.UpdatedAt.Nanosecond() != 123_456_000 {
		t.Fatalf("timestamps=%+v", key)
	}
}

func TestMarketTradesFromProto(t *testing.T) {
	msg := &marketdatav1.GetTradesResponse{
		Trades: []*marketdatav1.MarketTrade{
			{SymbolId: 1, MatchId: 55, IsBuy: true, PriceTicks: 100, QtyScaled: 200, TsNs: 300},
		},
		NextPageToken: "56",
	}
	result := decode.MarketTradesFromProto(msg)
	if len(result.Trades) != 1 || result.Trades[0].MatchID != "55" {
		t.Fatalf("trades=%+v", result.Trades)
	}
	if result.NextPageToken != "56" {
		t.Fatalf("NextPageToken=%q want 56", result.NextPageToken)
	}
}

func TestMarketOverviewListFromProto(t *testing.T) {
	msg := &marketoverviewv1.ListMarketOverviewResponse{
		Markets: []*marketoverviewv1.MarketOverview{
			{SymbolId: 1, Symbol: "BTC-USD", LastPriceTicks: 50_000},
		},
	}
	result := decode.MarketOverviewListFromProto(msg)
	if len(result.Markets) != 1 || result.Markets[0].Symbol != "BTC-USD" {
		t.Fatalf("markets=%+v", result.Markets)
	}
	if result.Markets[0].LastPrice.Ticks != 50_000 {
		t.Fatalf("last_price=%+v want ticks 50000", result.Markets[0].LastPrice)
	}
}

func TestOrderbookFromProto(t *testing.T) {
	msg := &orderbookv1.GetOrderBookResponse{
		BookSeq: 42,
		Bids:    []*orderbookv1.PriceLevel{{PriceTicks: 100, QtyScaled: 50}},
		Asks:    []*orderbookv1.PriceLevel{{PriceTicks: 101, QtyScaled: 25}},
	}
	result := decode.OrderbookFromProto(msg, "BTC-USD", 50, 8)
	if result.BookSeq != "42" || len(result.Bids) != 1 || len(result.Asks) != 1 {
		t.Fatalf("book=%+v", result)
	}
	if result.Bids[0].Price.Ticks != 100 || result.Bids[0].Qty.Scaled != 50 {
		t.Fatalf("bid=%+v", result.Bids[0])
	}
}
