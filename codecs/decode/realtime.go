package decode

import (
	"strconv"

	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	authv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/auth/v1"
	lifecyclev1 "github.com/Fabric-Labs/polyester-sdk-go/gen/chain/lifecycle/v1"
	zipperv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/chain/zipper/v1"
	ledgerrdv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/ledger/read/v1"
	marketdatav1 "github.com/Fabric-Labs/polyester-sdk-go/gen/marketdata/v1"
	marketoverviewv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/marketoverview/v1"
	orderbookv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/orderbook/v1"
	orderv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/orders/v1"
	triggersv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/triggers/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
	"google.golang.org/protobuf/proto"
)

func unmarshalProto[T proto.Message](payload []byte, msg T) error {
	return proto.Unmarshal(payload, msg)
}

// OrderbookDeltaFromBytes parses an orderbook delta protobuf payload.
func OrderbookDeltaFromBytes(payload []byte) (models.OrderBookDeltaUpdate, error) {
	var msg orderbookv1.OrderBookDelta
	if err := unmarshalProto(payload, &msg); err != nil {
		return models.OrderBookDeltaUpdate{}, err
	}
	bids := make([]models.PriceQtyPair, 0, len(msg.GetBids()))
	for _, level := range msg.GetBids() {
		bids = append(bids, models.PriceQtyPair{
			PriceTicks: level.GetPriceTicks(),
			QtyScaled:  level.GetQtyScaled(),
		})
	}
	asks := make([]models.PriceQtyPair, 0, len(msg.GetAsks()))
	for _, level := range msg.GetAsks() {
		asks = append(asks, models.PriceQtyPair{
			PriceTicks: level.GetPriceTicks(),
			QtyScaled:  level.GetQtyScaled(),
		})
	}
	return models.OrderBookDeltaUpdate{
		SymbolID:     msg.GetSymbolId(),
		BookSeqStart: strconv.FormatUint(msg.GetBookSeqStart(), 10),
		BookSeqEnd:   strconv.FormatUint(msg.GetBookSeqEnd(), 10),
		Reset:        msg.GetReset_(),
		Bids:         bids,
		Asks:         asks,
	}, nil
}

// MarketOverviewBatchFromBytes parses a market overview websocket batch.
func MarketOverviewBatchFromBytes(payload []byte) (models.MarketOverviewList, error) {
	var batch marketoverviewv1.MarketOverviewBatch
	if err := unmarshalProto(payload, &batch); err != nil {
		return models.MarketOverviewList{}, err
	}
	out := make([]models.MarketOverviewEntry, 0, len(batch.GetMarkets()))
	for _, row := range batch.GetMarkets() {
		out = append(out, MarketOverviewEntryFromProto(row))
	}
	return models.MarketOverviewList{Markets: out}, nil
}

// AssetBalanceFromBytes parses a balance publication.
func AssetBalanceFromBytes(payload []byte) (models.AssetBalance, error) {
	var msg ledgerrdv1.AssetBalance
	if err := unmarshalProto(payload, &msg); err != nil {
		return models.AssetBalance{}, err
	}
	return AssetBalanceFromProto(&msg), nil
}

// UserTradeFromBytes parses a private user trade publication.
func UserTradeFromBytes(payload []byte) (models.UserTrade, error) {
	var msg orderv1.UserTrade
	if err := unmarshalProto(payload, &msg); err != nil {
		return models.UserTrade{}, err
	}
	return UserTradeFromProto(&msg), nil
}

// LedgerTransferFromBytes parses a ledger transfer publication.
func LedgerTransferFromBytes(payload []byte) (models.LedgerTransfer, error) {
	var msg ledgerrdv1.TransferRow
	if err := unmarshalProto(payload, &msg); err != nil {
		return models.LedgerTransfer{}, err
	}
	return TransferRowFromProto(&msg), nil
}

// OrderFromBytes parses an order publication.
func OrderFromBytes(payload []byte) (models.Order, error) {
	var msg orderv1.Order
	if err := unmarshalProto(payload, &msg); err != nil {
		return models.Order{}, err
	}
	return OrderFromProto(&msg), nil
}

// TriggerFromBytes parses a trigger publication.
func TriggerFromBytes(payload []byte) (models.Trigger, error) {
	var msg triggersv1.Trigger
	if err := unmarshalProto(payload, &msg); err != nil {
		return models.Trigger{}, err
	}
	return TriggerFromProto(&msg), nil
}

// TriggerEventFromBytes parses a trigger event publication.
func TriggerEventFromBytes(payload []byte) (models.TriggerEvent, error) {
	var msg triggersv1.TriggerEvent
	if err := unmarshalProto(payload, &msg); err != nil {
		return models.TriggerEvent{}, err
	}
	return TriggerEventMessageFromProto(&msg), nil
}

// ApiKeyFromBytes parses an API key publication.
func ApiKeyFromBytes(payload []byte) (models.ApiKeySummary, error) {
	var msg authv1.ApiKey
	if err := unmarshalProto(payload, &msg); err != nil {
		return models.ApiKeySummary{}, err
	}
	if row := ApiKeyMessageFromProto(&msg); row != nil {
		return *row, nil
	}
	return models.ApiKeySummary{}, nil
}

// SubaccountFromBytes parses a subaccount publication.
func SubaccountFromBytes(payload []byte) (models.SubAccount, error) {
	var msg authv1.Subaccount
	if err := unmarshalProto(payload, &msg); err != nil {
		return models.SubAccount{}, err
	}
	return SubaccountMessageFromProto(&msg), nil
}

// SubaccountPolicyFromBytes parses a subaccount policy publication.
func SubaccountPolicyFromBytes(payload []byte) (models.SubaccountPolicy, error) {
	var msg authv1.SubaccountPolicyView
	if err := unmarshalProto(payload, &msg); err != nil {
		return models.SubaccountPolicy{}, err
	}
	if row := SubaccountPolicyMessageFromProto(&msg); row != nil {
		return *row, nil
	}
	return models.SubaccountPolicy{}, nil
}

// ApiPolicyFromBytes parses an API key policy publication.
func ApiPolicyFromBytes(payload []byte) (models.ApiPolicy, error) {
	var msg authv1.ApiPolicyView
	if err := unmarshalProto(payload, &msg); err != nil {
		return models.ApiPolicy{}, err
	}
	if row := ApiPolicyMessageFromProto(&msg); row != nil {
		return *row, nil
	}
	return models.ApiPolicy{}, nil
}

// FlowSummaryFromBytes parses a lifecycle flow summary publication.
func FlowSummaryFromBytes(payload []byte) (models.LifecycleFlowSummary, error) {
	var msg lifecyclev1.FlowSummaryView
	if err := unmarshalProto(payload, &msg); err != nil {
		return models.LifecycleFlowSummary{}, err
	}
	return FlowSummaryMessageFromProto(&msg), nil
}

// FlowDetailFromBytes parses a lifecycle flow detail publication.
func FlowDetailFromBytes(payload []byte) (models.LifecycleFlowSummary, error) {
	var msg lifecyclev1.FlowDetailView
	if err := unmarshalProto(payload, &msg); err != nil {
		return models.LifecycleFlowSummary{}, err
	}
	if msg.GetSummary() == nil {
		return models.LifecycleFlowSummary{}, nil
	}
	return FlowSummaryMessageFromProto(msg.GetSummary()), nil
}

// AccountIdentityFromBytes parses a public identity update.
func AccountIdentityFromBytes(payload []byte) (models.AccountIdentity, error) {
	var msg authv1.AccountIdentity
	if err := unmarshalProto(payload, &msg); err != nil {
		return models.AccountIdentity{}, err
	}
	return AccountIdentityFromProto(&msg), nil
}

// AddressBookInvalidationFromBytes parses an address book invalidation event.
func AddressBookInvalidationFromBytes(payload []byte) (models.AddressBookViewInvalidation, error) {
	var msg authv1.AddressBookViewInvalidated
	if err := unmarshalProto(payload, &msg); err != nil {
		return models.AddressBookViewInvalidation{}, err
	}
	return AddressBookInvalidationFromProto(&msg), nil
}

// MarketTradeFromBytes parses a public market trade publication.
func MarketTradeFromBytes(payload []byte) (models.MarketTrade, error) {
	var msg marketdatav1.MarketTrade
	if err := unmarshalProto(payload, &msg); err != nil {
		return models.MarketTrade{}, err
	}
	side := "sell"
	if msg.GetIsBuy() {
		side = "buy"
	}
	return models.MarketTrade{
		SymbolID: msg.GetSymbolId(),
		MatchID:  strconv.FormatUint(msg.GetMatchId(), 10),
		Price:    codecs.DecodePriceTicks(msg.GetPriceTicks(), ""),
		Qty:      codecs.DecodeQtyScaled(msg.GetQtyScaled(), -1, "", nil),
		TsNs:     strconv.FormatUint(msg.GetTsNs(), 10),
		Side:     side,
	}, nil
}

// CandlePointFromBytes returns a decoder for candle point publications.
func CandlePointFromBytes(symbolID uint32, timeframe string, volumeScale int) func([]byte) (models.Candle, error) {
	return func(payload []byte) (models.Candle, error) {
		var point marketdatav1.CandlePoint
		if err := unmarshalProto(payload, &point); err != nil {
			return models.Candle{}, err
		}
		return CandlePointFromProto(&point, symbolID, timeframe, volumeScale), nil
	}
}

// HeatmapLiveBucketFromBytes parses a heatmap live bucket publication.
func HeatmapLiveBucketFromBytes(payload []byte) (models.ApiData, error) {
	var msg marketdatav1.HeatmapLiveBucket
	if err := unmarshalProto(payload, &msg); err != nil {
		return models.ApiData{}, err
	}
	return APIDataFromProto(&msg)
}

// ZippedAssetSupplyBatchFromBytes parses a zipper supply batch publication.
func ZippedAssetSupplyBatchFromBytes(payload []byte, scaleFn func(uint32) int) (models.ZippedAssetSupplyBatch, error) {
	var msg zipperv1.ZippedAssetSupplyBatch
	if err := unmarshalProto(payload, &msg); err != nil {
		return models.ZippedAssetSupplyBatch{}, err
	}
	return ZippedAssetSupplyBatchFromProto(&msg, scaleFn), nil
}
