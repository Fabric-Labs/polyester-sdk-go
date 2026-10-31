package decode

import (
	"strconv"

	"github.com/Fabric-Labs/polyester-sdk-go/catalogs"
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
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
	if len(payload) == 0 {
		return &sdkerrors.RealtimeError{Msg: "proto decode: empty publication payload"}
	}
	if len(payload) > 8*1024*1024 {
		return &sdkerrors.RealtimeError{Msg: "proto decode: publication exceeds 8388608 bytes"}
	}
	if err := proto.Unmarshal(payload, msg); err != nil {
		return &sdkerrors.RealtimeError{Msg: "proto decode: " + err.Error()}
	}
	return nil
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
// Display Symbol is empty unless a catalog is supplied via MarketOverviewBatchDecoder.
func MarketOverviewBatchFromBytes(payload []byte) (models.MarketOverviewList, error) {
	return MarketOverviewBatchDecoder(nil)(payload)
}

// MarketOverviewBatchDecoder returns a batch decoder that fills display Symbol from catalogs.
func MarketOverviewBatchDecoder(cats *catalogs.Manager) func([]byte) (models.MarketOverviewList, error) {
	return func(payload []byte) (models.MarketOverviewList, error) {
		var batch marketoverviewv1.MarketOverviewBatch
		if err := unmarshalProto(payload, &batch); err != nil {
			return models.MarketOverviewList{}, err
		}
		out := make([]models.MarketOverviewEntry, 0, len(batch.GetMarkets()))
		for _, row := range batch.GetMarkets() {
			out = append(out, MarketOverviewEntryFromProto(row, cats))
		}
		return models.MarketOverviewList{Markets: out}, nil
	}
}

// AssetBalanceFromBytes parses a balance publication.
func AssetBalanceFromBytes(payload []byte) (models.AssetBalance, error) {
	var msg ledgerrdv1.AssetBalance
	if err := unmarshalProto(payload, &msg); err != nil {
		return models.AssetBalance{}, err
	}
	return AssetBalanceFromProto(&msg), nil
}

// UserTradeBatchFromBytes parses a private user trade publication: a batch
// of fills, oldest first per trading symbol. (match_id, order_id) identifies a
// fill; a repeated identity replaces the earlier copy.
func UserTradeBatchFromBytes(payload []byte) ([]models.UserTrade, error) {
	var msg orderv1.UserTradeBatch
	if err := unmarshalProto(payload, &msg); err != nil {
		return nil, err
	}
	out := make([]models.UserTrade, 0, len(msg.GetEvents()))
	for _, trade := range msg.GetEvents() {
		out = append(out, UserTradeFromProto(trade))
	}
	return out, nil
}

// LedgerTransferBatchFromBytes parses a ledger transfer publication: a batch
// of transfer legs, oldest first. (transfer_id, account_code, is_debit)
// identifies a leg; a repeated identity replaces the earlier copy.
func LedgerTransferBatchFromBytes(payload []byte) ([]models.LedgerTransfer, error) {
	var msg ledgerrdv1.TransferRowBatch
	if err := unmarshalProto(payload, &msg); err != nil {
		return nil, err
	}
	out := make([]models.LedgerTransfer, 0, len(msg.GetEvents()))
	for _, row := range msg.GetEvents() {
		out = append(out, TransferRowFromProto(row))
	}
	return out, nil
}

// TriggerFromBytes parses a trigger publication.
func TriggerFromBytes(payload []byte) (models.Trigger, error) {
	var msg triggersv1.Trigger
	if err := unmarshalProto(payload, &msg); err != nil {
		return models.Trigger{}, err
	}
	return TriggerFromProto(&msg), nil
}

// TriggerEventBatchFromBytes parses a trigger event publication: a batch of
// events, oldest first per trading symbol. (trigger_id, event_type, child_seq,
// ts_ns) identifies an event; a repeated identity replaces the earlier copy.
func TriggerEventBatchFromBytes(payload []byte) ([]models.TriggerEvent, error) {
	var msg triggersv1.TriggerEventBatch
	if err := unmarshalProto(payload, &msg); err != nil {
		return nil, err
	}
	out := make([]models.TriggerEvent, 0, len(msg.GetEvents()))
	for _, event := range msg.GetEvents() {
		out = append(out, TriggerEventMessageFromProto(event))
	}
	return out, nil
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
		return models.LifecycleFlowSummary{}, &sdkerrors.RealtimeError{
			Msg: "proto decode: flow detail is missing summary",
		}
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

// MarketTradeBatchFromBytes returns a decoder for public market trade
// publications: a batch of trades in execution order, oldest first.
func MarketTradeBatchFromBytes(quantityScale int) func([]byte) ([]models.MarketTrade, error) {
	return func(payload []byte) ([]models.MarketTrade, error) {
		var msg marketdatav1.MarketTradeBatch
		if err := unmarshalProto(payload, &msg); err != nil {
			return nil, err
		}
		out := make([]models.MarketTrade, 0, len(msg.GetEvents()))
		for _, trade := range msg.GetEvents() {
			out = append(out, marketTradeFromProto(trade, quantityScale))
		}
		return out, nil
	}
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
func ZippedAssetSupplyBatchFromBytes(payload []byte, scaleFn func(uint32) (int, bool)) (models.ZippedAssetSupplyBatch, error) {
	var msg zipperv1.ZippedAssetSupplyBatch
	if err := unmarshalProto(payload, &msg); err != nil {
		return models.ZippedAssetSupplyBatch{}, err
	}
	return ZippedAssetSupplyBatchFromProto(&msg, scaleFn)
}
