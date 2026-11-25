package decode_test

import (
	stderrors "errors"
	"testing"

	"github.com/Fabric-Labs/polyester-sdk-go/codecs/decode"
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	marketdatav1 "github.com/Fabric-Labs/polyester-sdk-go/gen/marketdata/v1"
	marketoverviewv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/marketoverview/v1"
	orderv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/orders/v1"
	privatewsv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/privatews/v1"
	triggersv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/triggers/v1"
)

// requireNegativeQty fails unless err is the *ResponseContractError a
// negative quantity in field returns.
func requireNegativeQty(t *testing.T, err error, field string, scaled string) {
	t.Helper()
	var contract *sdkerrors.ResponseContractError
	if !stderrors.As(err, &contract) {
		t.Fatalf("err = %v, want *ResponseContractError", err)
	}
	if contract.Operation != field || contract.Msg != "negative quantity "+scaled {
		t.Fatalf("err = %+v, want %s negative quantity %s", contract, field, scaled)
	}
}

func TestOrderDecodersRefuseANegativeQuantity(t *testing.T) {
	_, err := decode.OrderFromProto(&orderv1.Order{OrderId: 1, SymbolId: 3, HiddenQtyScaled: -4})
	requireNegativeQty(t, err, "Order.hidden_qty_scaled", "-4")

	_, err = decode.OrderFromProto(&orderv1.Order{OrderId: 1, SymbolId: 3, FeeTotal: &orderv1.OrderFeeTotal{QtyScaled: -2}})
	requireNegativeQty(t, err, "OrderFeeTotal.qty_scaled", "-2")

	_, err = decode.OrdersListFromHistory(&orderv1.GetOrderHistoryResponse{Orders: []*orderv1.Order{{OrderId: 1, OrigQtyScaled: -1}}})
	requireNegativeQty(t, err, "Order.orig_qty_scaled", "-1")

	_, err = decode.UserTradeFromProto(&orderv1.UserTrade{OrderId: 1, SymbolId: 3, QtyScaled: -5})
	requireNegativeQty(t, err, "UserTrade.qty_scaled", "-5")

	_, err = decode.OrderMutationFromProto(&orderv1.CreateOrderResponse{OrderId: 1, ResolvedBaseQtyScaled: -6})
	requireNegativeQty(t, err, "CreateOrderResponse.resolved_base_qty_scaled", "-6")
}

func TestOrderStreamDecodersRefuseANegativeQuantity(t *testing.T) {
	_, err := decode.OrdersFromProto([]*orderv1.Order{{OrderId: 1, LeavesQtyScaled: -1}})
	requireNegativeQty(t, err, "Order.leaves_qty_scaled", "-1")

	_, err = decode.OrderUnitEventsFromProto([]*privatewsv1.UnitEvent{{Kind: &privatewsv1.UnitEvent_Order{Order: &privatewsv1.OrderEvent{
		OrderId: 1, Kind: privatewsv1.OrderEvent_FILLED,
		Fill: &privatewsv1.Fill{MatchId: 9, QtyScaled: -7, PriceTicks: 1_000_000_000},
	}}}}, nil)
	requireNegativeQty(t, err, "OrderEvent.fill.qty_scaled", "-7")
}

func TestTriggerFromProtoRefusesANegativeQuantity(t *testing.T) {
	_, err := decode.TriggerFromProto(&triggersv1.Trigger{TriggerId: 1, SymbolId: 3, QtyScaled: -1})
	requireNegativeQty(t, err, "Trigger.qty_scaled", "-1")

	_, err = decode.TriggerFromProto(&triggersv1.Trigger{
		TriggerId: 1, SymbolId: 3, QtyScaled: 100,
		RuntimeDetails: &triggersv1.Trigger_TwapState{TwapState: &triggersv1.TwapDetails{ExecutedQtyScaled: -2}},
	})
	requireNegativeQty(t, err, "Trigger.twap_state.executed_qty_scaled", "-2")

	_, err = decode.TriggersListFromProto(&triggersv1.ListTriggersResponse{Triggers: []*triggersv1.Trigger{{TriggerId: 1, QtyScaled: -3}}})
	requireNegativeQty(t, err, "Trigger.qty_scaled", "-3")
}

func TestMarketDecodersRefuseANegativeQuantity(t *testing.T) {
	_, err := decode.MarketTradesFromProto(&marketdatav1.GetTradesResponse{Trades: []*marketdatav1.MarketTrade{{SymbolId: 1, MatchId: 1, PriceTicks: 5, QtyScaled: -1}}}, 8)
	requireNegativeQty(t, err, "MarketTrade.qty_scaled", "-1")

	_, err = decode.MarketOverviewEntryFromProto(&marketoverviewv1.MarketOverview{SymbolId: 1, BestAskTicks: 75, BestAskQtyScaled: -3}, nil)
	requireNegativeQty(t, err, "MarketOverview.best_ask_qty_scaled", "-3")

	_, err = decode.MarketOverviewListFromProto(&marketoverviewv1.ListMarketOverviewResponse{Markets: []*marketoverviewv1.MarketOverview{{SymbolId: 1, BestBidTicks: 74, BestBidQtyScaled: -2}}}, nil)
	requireNegativeQty(t, err, "MarketOverview.best_bid_qty_scaled", "-2")
}
