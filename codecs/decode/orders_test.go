package decode_test

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	"github.com/Fabric-Labs/polyester-sdk-go/codecs/decode"
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	orderv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/orders/v1"
	ratelimitv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/polyester/ratelimit/v1"
	typev1 "github.com/Fabric-Labs/polyester-sdk-go/gen/polyester/type/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestOrderFromProtoMapsEnumsAndIDs(t *testing.T) {
	msg := &orderv1.Order{
		OrderId:         42,
		SymbolId:        3,
		ClientOrderId:   "coid-1",
		Side:            orderv1.Side_BUY,
		Status:          orderv1.OrderStatus_WORKING,
		OrderType:       orderv1.OrderType_LIMIT,
		TimeInForce:     orderv1.TimeInForce_GTC,
		OrigQtyScaled:   100,
		CumQtyScaled:    10,
		LeavesQtyScaled: 90,
		PriceTicks:      5000,
		AvgPriceTicks:   4990,
		CreatedTsNs:     1_700_000_000_000,
		PostOnly:        true,
	}
	order := decode.OrderFromProto(msg)
	if order.OrderID != codecs.FormatUint64ID(42) {
		t.Fatalf("order_id=%q", order.OrderID)
	}
	if order.Side != "buy" || order.Status != "working" || order.OrderType != "limit" || order.TIF != "gtc" {
		t.Fatalf("order=%+v", order)
	}
	if order.OrigQty.Scaled() != 100 {
		t.Fatalf("orig_qty=%+v", order.OrigQty)
	}
	if !order.PostOnly {
		t.Fatalf("expected post_only=true, got %+v", order)
	}
	msg.Version = 7
	order = decode.OrderFromProto(msg)
	if order.Version != 7 {
		t.Fatalf("version=%d", order.Version)
	}
}

func TestOrderOrigQtyIsCurrentAcceptedTotal(t *testing.T) {
	before := decode.OrderFromProto(&orderv1.Order{OrderId: 1, SymbolId: 3, OrigQtyScaled: 100, LeavesQtyScaled: 100})
	after := decode.OrderFromProto(&orderv1.Order{OrderId: 1, SymbolId: 3, OrigQtyScaled: 150, LeavesQtyScaled: 150})
	if before.OrigQty.Scaled() != 100 || after.OrigQty.Scaled() != 150 {
		t.Fatalf("orig_qty before=%+v after=%+v", before.OrigQty, after.OrigQty)
	}
}

func TestOrderFromProtoMapsAttachedRisk(t *testing.T) {
	tpTriggerID, tpChildOrderID := uint64(101), uint64(201)
	slTriggerID, slChildOrderID := uint64(102), uint64(202)
	trailingTriggerID := uint64(103)
	msg := &orderv1.Order{
		OrderId:  1,
		SymbolId: 1,
		PostOnly: false,
		AttachedRisk: &orderv1.AttachedRisk{
			TakeProfit: &orderv1.AttachedRiskTakeProfit{
				Policy: &orderv1.TakeProfitPolicy{
					TriggerPriceTicks: 6000,
					Child: &orderv1.RiskExecution{
						Execution: &orderv1.RiskExecution_MarketIoc{MarketIoc: &orderv1.RiskMarketIoc{}},
					},
				},
				State: &orderv1.AttachedRiskLegState{
					Status:       orderv1.AttachedRiskLegState_COMPLETED,
					ArmedTsNs:    1_700_000_000_000_000_001,
					TerminalTsNs: 1_700_000_000_000_000_002,
					TriggerId:    &tpTriggerID,
					ChildOrderId: &tpChildOrderID,
				},
			},
			StopLoss: &orderv1.AttachedRiskStopLoss{
				Policy: &orderv1.StopLossPolicy{
					TriggerPriceTicks: 4500,
					Child: &orderv1.RiskExecution{
						Execution: &orderv1.RiskExecution_MarketIoc{MarketIoc: &orderv1.RiskMarketIoc{}},
					},
				},
				State: &orderv1.AttachedRiskLegState{
					Status:       orderv1.AttachedRiskLegState_FAILED,
					ArmedTsNs:    1_700_000_000_000_000_003,
					TerminalTsNs: 1_700_000_000_000_000_004,
					TriggerId:    &slTriggerID,
					ChildOrderId: &slChildOrderID,
				},
			},
			TrailingStop: &orderv1.AttachedRiskTrailingStop{
				Policy: &orderv1.TrailingStopPolicy{
					ActivationPriceTicks: 5500,
					TrailingDistance: &orderv1.TrailingStopPolicy_TrailingDistanceBps{
						TrailingDistanceBps: 25,
					},
					MaxSlippage: &orderv1.TrailingStopPolicy_MaxSlippageTicks{
						MaxSlippageTicks: 10,
					},
				},
				State: &orderv1.AttachedRiskLegState{
					Status:    orderv1.AttachedRiskLegState_ARMED,
					ArmedTsNs: 1_700_000_000_000_000_005,
					TriggerId: &trailingTriggerID,
				},
			},
			Oco: true,
		},
	}
	order := decode.OrderFromProto(msg)
	if order.AttachedRisk == nil {
		t.Fatal("expected attached_risk")
	}
	risk := order.AttachedRisk
	if !risk.Oco || risk.TakeProfit == nil || risk.StopLoss == nil || risk.TrailingStop == nil {
		t.Fatalf("risk=%+v", risk)
	}
	if risk.TakeProfit.TriggerPrice.Ticks() != 6000 || risk.TakeProfit.OrderType != "market" {
		t.Fatalf("take_profit=%+v", risk.TakeProfit)
	}
	if risk.StopLoss.TriggerPrice.Ticks() != 4500 || risk.StopLoss.OrderType != "market" {
		t.Fatalf("stop_loss=%+v", risk.StopLoss)
	}
	if risk.TakeProfit.State == nil || risk.TakeProfit.State.Status != "completed" ||
		risk.TakeProfit.State.ArmedTsNs != "1700000000000000001" ||
		risk.TakeProfit.State.TerminalTsNs != "1700000000000000002" ||
		risk.TakeProfit.State.TriggerID != codecs.FormatUint64ID(101) ||
		risk.TakeProfit.State.ChildOrderID != codecs.FormatUint64ID(201) {
		t.Fatalf("take_profit state=%+v", risk.TakeProfit.State)
	}
	if risk.StopLoss.State == nil || risk.StopLoss.State.Status != "failed" ||
		risk.StopLoss.State.TriggerID != codecs.FormatUint64ID(102) ||
		risk.StopLoss.State.ChildOrderID != codecs.FormatUint64ID(202) {
		t.Fatalf("stop_loss state=%+v", risk.StopLoss.State)
	}
	if risk.TrailingStop.DistanceBps != 25 || risk.TrailingStop.MaxSlippageTicks != 10 {
		t.Fatalf("trailing=%+v", risk.TrailingStop)
	}
	if risk.TrailingStop.ActivationPrice.Ticks() != 5500 {
		t.Fatalf("trailing=%+v", risk.TrailingStop)
	}
	if risk.TrailingStop.State == nil || risk.TrailingStop.State.Status != "armed" ||
		risk.TrailingStop.State.ArmedTsNs != "1700000000000000005" ||
		risk.TrailingStop.State.TriggerID != codecs.FormatUint64ID(103) {
		t.Fatalf("trailing state=%+v", risk.TrailingStop.State)
	}

	encoded, err := json.Marshal(risk)
	if err != nil {
		t.Fatal(err)
	}
	var projected map[string]any
	if err := json.Unmarshal(encoded, &projected); err != nil {
		t.Fatal(err)
	}
	assertState := func(leg, status, armed, terminal, triggerID, childOrderID string) {
		t.Helper()
		legValue, ok := projected[leg].(map[string]any)
		if !ok {
			t.Fatalf("%s missing from attached risk JSON: %s", leg, encoded)
		}
		state, ok := legValue["state"].(map[string]any)
		if !ok {
			t.Fatalf("%s runtime state missing from attached risk JSON: %s", leg, encoded)
		}
		for key, want := range map[string]string{
			"status": status, "armed_ts_ns": armed, "terminal_ts_ns": terminal,
			"trigger_id": triggerID, "child_order_id": childOrderID,
		} {
			if want == "" {
				if _, exists := state[key]; exists {
					t.Fatalf("%s.%s=%v want omitted", leg, key, state[key])
				}
				continue
			}
			if got, _ := state[key].(string); got != want {
				t.Fatalf("%s.%s=%v want %q; JSON=%s", leg, key, state[key], want, encoded)
			}
		}
	}
	assertState("take_profit", "completed", "1700000000000000001", "1700000000000000002", codecs.FormatUint64ID(101), codecs.FormatUint64ID(201))
	assertState("stop_loss", "failed", "1700000000000000003", "1700000000000000004", codecs.FormatUint64ID(102), codecs.FormatUint64ID(202))
	assertState("trailing_stop", "armed", "1700000000000000005", "", codecs.FormatUint64ID(103), "")
}

func TestOrderFromProtoMapsStateOnlyAttachedRiskLegs(t *testing.T) {
	tpTriggerID, tpChildOrderID := uint64(301), uint64(401)
	slTriggerID, slChildOrderID := uint64(302), uint64(402)
	trailingTriggerID, trailingChildOrderID := uint64(303), uint64(403)
	order := decode.OrderFromProto(&orderv1.Order{
		OrderId:  2,
		SymbolId: 1,
		AttachedRisk: &orderv1.AttachedRisk{
			TakeProfit: &orderv1.AttachedRiskTakeProfit{
				State: &orderv1.AttachedRiskLegState{
					Status:       orderv1.AttachedRiskLegState_COMPLETED,
					ArmedTsNs:    1_700_000_000_000_000_011,
					TerminalTsNs: 1_700_000_000_000_000_012,
					TriggerId:    &tpTriggerID,
					ChildOrderId: &tpChildOrderID,
				},
			},
			StopLoss: &orderv1.AttachedRiskStopLoss{
				State: &orderv1.AttachedRiskLegState{
					Status:       orderv1.AttachedRiskLegState_FAILED,
					ArmedTsNs:    1_700_000_000_000_000_013,
					TerminalTsNs: 1_700_000_000_000_000_014,
					TriggerId:    &slTriggerID,
					ChildOrderId: &slChildOrderID,
				},
			},
			TrailingStop: &orderv1.AttachedRiskTrailingStop{
				State: &orderv1.AttachedRiskLegState{
					Status:       orderv1.AttachedRiskLegState_RUNNING,
					ArmedTsNs:    1_700_000_000_000_000_015,
					TerminalTsNs: 1_700_000_000_000_000_016,
					TriggerId:    &trailingTriggerID,
					ChildOrderId: &trailingChildOrderID,
				},
			},
			Oco: true,
		},
	})
	if order.AttachedRisk == nil {
		t.Fatal("state-only attached risk was omitted")
	}
	risk := order.AttachedRisk
	if !risk.Oco || risk.TakeProfit == nil || risk.StopLoss == nil || risk.TrailingStop == nil {
		t.Fatalf("state-only legs were not preserved together: %+v", risk)
	}
	assertState := func(name string, state *models.AttachedRiskLegState, status, armed, terminal string, triggerID, childOrderID uint64) {
		t.Helper()
		if state == nil ||
			state.Status != status ||
			state.ArmedTsNs != armed ||
			state.TerminalTsNs != terminal ||
			state.TriggerID != codecs.FormatUint64ID(triggerID) ||
			state.ChildOrderID != codecs.FormatUint64ID(childOrderID) {
			t.Fatalf("%s state=%+v", name, state)
		}
	}
	assertState("take_profit", risk.TakeProfit.State, "completed", "1700000000000000011", "1700000000000000012", 301, 401)
	assertState("stop_loss", risk.StopLoss.State, "failed", "1700000000000000013", "1700000000000000014", 302, 402)
	assertState("trailing_stop", risk.TrailingStop.State, "running", "1700000000000000015", "1700000000000000016", 303, 403)

	if risk.TakeProfit.TriggerPrice.Ticks() != 0 || risk.TakeProfit.OrderType != "" ||
		risk.TakeProfit.LimitPrice.Ticks() != 0 ||
		risk.StopLoss.TriggerPrice.Ticks() != 0 || risk.StopLoss.OrderType != "" ||
		risk.StopLoss.LimitPrice.Ticks() != 0 {
		t.Fatalf("state-only TP/SL fabricated policy fields: tp=%+v sl=%+v", risk.TakeProfit, risk.StopLoss)
	}
	if risk.TrailingStop.DistanceTicks != 0 || risk.TrailingStop.DistanceBps != 0 ||
		risk.TrailingStop.MaxSlippageTicks != 0 || risk.TrailingStop.MaxSlippageBps != 0 ||
		risk.TrailingStop.ActivationPrice.Ticks() != 0 ||
		risk.TrailingStop.TriggerPriceSource != "" || risk.TrailingStop.OrderType != "" {
		t.Fatalf("state-only trailing leg fabricated policy fields: %+v", risk.TrailingStop)
	}
}

func TestOrderFromProtoOmitsEmptyPolicyWrappers(t *testing.T) {
	order := decode.OrderFromProto(&orderv1.Order{
		OrderId:  3,
		SymbolId: 1,
		AttachedRisk: &orderv1.AttachedRisk{
			TakeProfit: &orderv1.AttachedRiskTakeProfit{Policy: &orderv1.TakeProfitPolicy{}},
			StopLoss:   &orderv1.AttachedRiskStopLoss{Policy: &orderv1.StopLossPolicy{}},
			TrailingStop: &orderv1.AttachedRiskTrailingStop{
				Policy: &orderv1.TrailingStopPolicy{},
			},
		},
	})
	if order.AttachedRisk != nil {
		t.Fatalf("unusable policy wrappers must be omitted: %+v", order.AttachedRisk)
	}
}

func TestOrderFromProtoOmitsEmptyStateWrappers(t *testing.T) {
	emptyState := &orderv1.AttachedRiskLegState{}
	order := decode.OrderFromProto(&orderv1.Order{
		OrderId:  3,
		SymbolId: 1,
		AttachedRisk: &orderv1.AttachedRisk{
			TakeProfit:   &orderv1.AttachedRiskTakeProfit{State: emptyState},
			StopLoss:     &orderv1.AttachedRiskStopLoss{State: emptyState},
			TrailingStop: &orderv1.AttachedRiskTrailingStop{State: emptyState},
		},
	})
	if order.AttachedRisk != nil {
		t.Fatalf("empty state wrappers must be omitted: %+v", order.AttachedRisk)
	}
}

func TestOrderFromProtoOmitsTrailingPolicyWithoutDistance(t *testing.T) {
	msg := &orderv1.Order{
		OrderId:  3,
		SymbolId: 1,
		AttachedRisk: &orderv1.AttachedRisk{
			TrailingStop: &orderv1.AttachedRiskTrailingStop{
				Policy: &orderv1.TrailingStopPolicy{
					ActivationPriceTicks: 5500,
					MaxSlippage: &orderv1.TrailingStopPolicy_MaxSlippageBps{
						MaxSlippageBps: 25,
					},
				},
			},
		},
	}
	order := decode.OrderFromProto(msg)
	if order.AttachedRisk != nil {
		t.Fatalf("trailing policy without positive distance must be omitted: %+v", order.AttachedRisk)
	}
}

func TestOrderFromProtoPreservesUsablePolicyOnlyLegs(t *testing.T) {
	order := decode.OrderFromProto(&orderv1.Order{
		OrderId:  4,
		SymbolId: 1,
		AttachedRisk: &orderv1.AttachedRisk{
			TakeProfit: &orderv1.AttachedRiskTakeProfit{Policy: &orderv1.TakeProfitPolicy{
				TriggerPriceTicks: 6000,
			}},
			StopLoss: &orderv1.AttachedRiskStopLoss{Policy: &orderv1.StopLossPolicy{
				TriggerPriceTicks: 4500,
			}},
			TrailingStop: &orderv1.AttachedRiskTrailingStop{Policy: &orderv1.TrailingStopPolicy{
				TrailingDistance: &orderv1.TrailingStopPolicy_TrailingDistanceBps{
					TrailingDistanceBps: 25,
				},
			}},
		},
	})
	if order.AttachedRisk == nil ||
		order.AttachedRisk.TakeProfit == nil ||
		order.AttachedRisk.StopLoss == nil ||
		order.AttachedRisk.TrailingStop == nil {
		t.Fatalf("usable policy-only legs were omitted: %+v", order.AttachedRisk)
	}
	if order.AttachedRisk.TakeProfit.TriggerPrice.Ticks() != 6000 ||
		order.AttachedRisk.StopLoss.TriggerPrice.Ticks() != 4500 ||
		order.AttachedRisk.TrailingStop.DistanceBps != 25 {
		t.Fatalf("usable policy-only fields were not preserved: %+v", order.AttachedRisk)
	}
}

func TestOrdersListFromProto(t *testing.T) {
	msg := &orderv1.GetOpenOrdersResponse{
		Orders:        []*orderv1.Order{{OrderId: 1, SymbolId: 1, Side: orderv1.Side_SELL}},
		NextPageToken: "tok",
	}
	result := decode.OrdersListFromOpen(msg)
	if len(result.Orders) != 1 || result.NextPageToken != "tok" {
		t.Fatalf("result=%+v", result)
	}
}

func TestGetOrderFromProtoIncludesTrades(t *testing.T) {
	msg := &orderv1.GetOrderResponse{
		Order: &orderv1.Order{OrderId: 7, SymbolId: 2},
		Trades: []*orderv1.UserTrade{{
			SymbolId: 2, MatchId: 99, OrderId: 7, Side: orderv1.Side_BUY,
			FeeAmountE18:           &typev1.U128{Lo: 5},
			FeeAsset:               orderv1.FeeAsset_BASE,
			ReferralShareAmountE18: &typev1.U128{Lo: 2},
			FeeIsRebate:            true,
		}},
	}
	result := decode.GetOrderFromProto(msg)
	if result.Order == nil || result.Order.OrderID != codecs.FormatUint64ID(7) {
		t.Fatalf("order=%+v", result.Order)
	}
	if len(result.Trades) != 1 || result.Trades[0].MatchID != "99" {
		t.Fatalf("trades=%+v", result.Trades)
	}
	if result.Trades[0].FeeAmountE18 != "5" || result.Trades[0].FeeAsset != "base" ||
		result.Trades[0].ReferralShareAmountE18 != "2" || !result.Trades[0].FeeIsRebate {
		t.Fatalf("trade fee fields=%+v", result.Trades[0])
	}
}

func TestModifyOrderFromProtoActionTakenEnum(t *testing.T) {
	msg := &orderv1.ModifyOrderResponse{
		ActionTaken:  orderv1.ModifyActionTaken_AMENDED,
		OldOrderId:   10,
		FinalOrderId: 11,
		Code:         "ok",
	}
	result, err := decode.ModifyOrderFromProto(msg)
	if err != nil {
		t.Fatal(err)
	}
	if result.ActionTaken != "amended" {
		t.Fatalf("action_taken=%q", result.ActionTaken)
	}
	if result.OldOrderID != codecs.FormatUint64ID(10) || result.FinalOrderID != codecs.FormatUint64ID(11) {
		t.Fatalf("result=%+v", result)
	}
}

func TestOrderMutationFromProtoCreateIncludesClientOrderID(t *testing.T) {
	budget := int64(500)
	msg := &orderv1.CreateOrderResponse{
		OrderId:                      42,
		ClientOrderId:                "coid-1",
		ResolvedBaseQtyScaled:        100,
		SubmittedMaxQuoteDebitScaled: &budget,
	}
	result, err := decode.OrderMutationFromProto(msg)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "accepted" || result.OrderID != codecs.FormatUint64ID(42) || result.ClientOrderID != "coid-1" ||
		result.ResolvedBaseQtyScaled != "100" || result.ResolvedBaseQty == nil ||
		result.ResolvedBaseQty.Scaled() != 100 || result.SubmittedMaxQuoteDebitScaled != "500" {
		t.Fatalf("result=%+v", result)
	}
}

func TestPreviewOrderFromProto(t *testing.T) {
	sid := uint32(1)
	admissible := true
	resolved := int64(100)
	bound := int64(50_000_000_000)
	result, err := decode.PreviewOrderFromProto(&orderv1.PreviewOrderResponse{
		Admissible:               &admissible,
		ResolvedBaseQtyScaled:    &resolved,
		ProtectedPriceBoundTicks: &bound,
		EvaluatedAt:              timestamppb.New(time.Unix(1, 250_000_000)),
	}, 8, "BTC-USDT", &sid)
	if err != nil {
		t.Fatal(err)
	}
	if result.Admissible == nil || !*result.Admissible ||
		result.Rejection != nil ||
		result.ResolvedBaseQtyScaled != "100" || result.ResolvedBaseQty == nil ||
		result.ResolvedBaseQty.Scaled() != 100 ||
		result.ProtectedPriceBound == nil ||
		result.ProtectedPriceBound.Ticks() != 50_000_000_000 ||
		result.EvaluatedAtMs != 1_250 {
		t.Fatalf("result=%+v", result)
	}
}

func TestPreviewOrderFromProtoRejectionPreservesUnknownCode(t *testing.T) {
	admissible := false
	result, err := decode.PreviewOrderFromProto(&orderv1.PreviewOrderResponse{
		Admissible: &admissible,
		Rejection: &orderv1.ErrorDetail{
			Code: orderv1.ErrorCode(99_999),
			Violations: []*orderv1.FieldViolation{{
				FieldPath: "order.base_qty_scaled",
				RuleId:    "positive",
				Message:   "Quantity must be positive.",
			}},
		},
		EvaluatedAt: timestamppb.New(time.Unix(1, 0)),
	}, 8, "BTC-USDT", nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Admissible == nil || *result.Admissible ||
		result.Rejection == nil ||
		result.Rejection.Code != "UNKNOWN_ERROR_CODE(99999)" ||
		len(result.Rejection.Violations) != 1 ||
		result.Rejection.Violations[0].FieldPath != "order.base_qty_scaled" ||
		result.Rejection.Violations[0].RuleID != "positive" ||
		result.Rejection.Violations[0].Message != "Quantity must be positive." ||
		result.EvaluatedAtMs != 1_000 {
		t.Fatalf("result=%+v", result)
	}
}

func TestPreviewOrderFromProtoSurfacesRateLimitDetail(t *testing.T) {
	admissible := false
	limit := uint64(50)
	retryAfterMs := uint64(500)
	result, err := decode.PreviewOrderFromProto(&orderv1.PreviewOrderResponse{
		Admissible: &admissible,
		Rejection: &orderv1.ErrorDetail{
			Code: orderv1.ErrorCode_ERROR_CODE_RATE_LIMIT_EXCEEDED,
			RateLimit: &ratelimitv1.RateLimitDetail{
				Reason:       ratelimitv1.FailureReason_QUOTA_EXCEEDED,
				Limit:        &limit,
				RetryAfterMs: &retryAfterMs,
				OperationId:  "orders.preview",
				PolicyClass:  ratelimitv1.PolicyClass_TRADING_PLACE,
				Scope:        ratelimitv1.LimiterScope_ACCOUNT,
			},
		},
		EvaluatedAt: timestamppb.New(time.Unix(1, 0)),
	}, 8, "BTC-USDT", nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Rejection == nil ||
		result.Rejection.Code != "RATE_LIMIT_EXCEEDED" ||
		result.Rejection.RateLimit == nil ||
		result.Rejection.RateLimit.Reason != "QUOTA_EXCEEDED" ||
		result.Rejection.RateLimit.RetryAfterMs == nil ||
		*result.Rejection.RateLimit.RetryAfterMs != 500 ||
		result.Rejection.RateLimit.Limit == nil ||
		*result.Rejection.RateLimit.Limit != 50 ||
		result.Rejection.RateLimit.Remaining != nil {
		t.Fatalf("result=%+v", result)
	}
}

func TestPreviewOrderFromProtoOmitsUnsetOptionalFields(t *testing.T) {
	admissible := false
	result, err := decode.PreviewOrderFromProto(&orderv1.PreviewOrderResponse{
		Admissible:  &admissible,
		EvaluatedAt: timestamppb.New(time.Unix(2, 0)),
		Rejection: &orderv1.ErrorDetail{
			Code: orderv1.ErrorCode_ERROR_CODE_BAD_QTY,
		},
	}, 8, "BTC-USDT", nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.ResolvedBaseQtyScaled != "" || result.ResolvedBaseQty != nil ||
		result.ProtectedPriceBound != nil ||
		result.Rejection == nil || result.Rejection.Code != "BAD_QTY" ||
		result.EvaluatedAtMs != 2_000 {
		t.Fatalf("result=%+v", result)
	}
}

func TestPreviewOrderFromProtoRejectsMissingEvaluatedAt(t *testing.T) {
	_, err := decode.PreviewOrderFromProto(
		&orderv1.PreviewOrderResponse{},
		8,
		"BTC-USDT",
		nil,
	)
	var contractErr *sdkerrors.ResponseContractError
	if !errors.As(err, &contractErr) {
		t.Fatalf("expected ResponseContractError, got %T: %v", err, err)
	}
}

func TestOrderMutationFromProtoCancelOmitsClientOrderID(t *testing.T) {
	msg := &orderv1.CancelOrderResponse{Status: orderv1.CancelOrderResponse_ACCEPTED, OrderId: 42}
	result, err := decode.OrderMutationFromCancel(msg)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "accepted" || result.OrderID != codecs.FormatUint64ID(42) || result.ClientOrderID != "" {
		t.Fatalf("result=%+v", result)
	}
}

func TestBatchCreateRejectsMissingOutcome(t *testing.T) {
	_, err := decode.BatchCreateFromProto(&orderv1.BatchCreateOrdersResponse{
		Results: []*orderv1.BatchCreateResultItem{{ClientOrderId: "missing"}},
	})
	var contractErr *sdkerrors.ResponseContractError
	if !errors.As(err, &contractErr) {
		t.Fatalf("expected ResponseContractError, got %T: %v", err, err)
	}
}

func TestBatchCreatePreservesUnknownRejectionCode(t *testing.T) {
	result, err := decode.BatchCreateFromProto(&orderv1.BatchCreateOrdersResponse{
		Results: []*orderv1.BatchCreateResultItem{{
			ClientOrderId: "rejected",
			Outcome: &orderv1.BatchCreateResultItem_Rejected{
				Rejected: &orderv1.BatchCreateRejected{
					Error: &orderv1.ErrorDetail{Code: orderv1.ErrorCode(99_999)},
				},
			},
		}},
		RejectedCount: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Results[0].Code != "UNKNOWN_ERROR_CODE(99999)" {
		t.Fatalf("code=%q", result.Results[0].Code)
	}
}

func TestBatchCreateRejectsCountMismatch(t *testing.T) {
	_, err := decode.BatchCreateFromProto(&orderv1.BatchCreateOrdersResponse{
		Results: []*orderv1.BatchCreateResultItem{{
			Outcome: &orderv1.BatchCreateResultItem_Accepted{
				Accepted: &orderv1.BatchCreateAccepted{OrderId: 1},
			},
		}},
		RejectedCount: 1,
	})
	var contractErr *sdkerrors.ResponseContractError
	if !errors.As(err, &contractErr) {
		t.Fatalf("expected ResponseContractError, got %T: %v", err, err)
	}
}

func TestBatchCancelRejectsCountMismatchAndUnknownStatus(t *testing.T) {
	_, err := decode.BatchCancelFromProto(&orderv1.BatchCancelOrdersResponse{
		Results:       []*orderv1.BatchCancelResultItem{{Status: orderv1.BatchCancelResultItem_ACCEPTED}},
		RejectedCount: 1,
	})
	var contractErr *sdkerrors.ResponseContractError
	if !errors.As(err, &contractErr) {
		t.Fatalf("expected ResponseContractError for count mismatch, got %T: %v", err, err)
	}

	_, err = decode.BatchCancelFromProto(&orderv1.BatchCancelOrdersResponse{
		Results:       []*orderv1.BatchCancelResultItem{{Status: orderv1.BatchCancelResultItem_Status(99)}},
		AcceptedCount: 1,
	})
	if !errors.As(err, &contractErr) {
		t.Fatalf("expected ResponseContractError for unknown status, got %T: %v", err, err)
	}
}

func TestBatchReplaceReconcilesAdmissionCounts(t *testing.T) {
	valid := &orderv1.BatchReplaceOrdersResponse{
		BatchRequestId: 42,
		Status:         orderv1.BatchReplaceAdmissionStatus_BATCH_REPLACE_ADMISSION_STATUS_PARTIALLY_ADMITTED,
		Results: []*orderv1.BatchReplaceAdmissionItem{
			{ItemIndex: 0, Status: orderv1.BatchReplaceItemAdmissionStatus_BATCH_REPLACE_ITEM_ADMISSION_STATUS_ADMITTED, OldOrderId: 1, ReplacementOrderId: 2},
			{ItemIndex: 1, Status: orderv1.BatchReplaceItemAdmissionStatus_BATCH_REPLACE_ITEM_ADMISSION_STATUS_REJECTED, OldOrderId: 3, Code: "REJECTED"},
		},
		AcceptedCount: 1,
		RejectedCount: 1,
	}
	result, err := decode.BatchReplaceFromProto(valid)
	if err != nil {
		t.Fatal(err)
	}
	if result.BatchRequestID == "" || result.BatchRequestID == "0" || result.Status != "partially_admitted" ||
		result.AcceptedCount != 1 || result.RejectedCount != 1 {
		t.Fatalf("result=%+v", result)
	}

	valid.AcceptedCount = 2
	_, err = decode.BatchReplaceFromProto(valid)
	var contractErr *sdkerrors.ResponseContractError
	if !errors.As(err, &contractErr) {
		t.Fatalf("expected ResponseContractError for count mismatch, got %T: %v", err, err)
	}
}

func TestBatchReplaceStatusDecodesPhases(t *testing.T) {
	result, err := decode.BatchReplaceStatusFromProto(&orderv1.GetBatchReplaceStatusResponse{
		BatchRequestId:  7,
		AdmissionStatus: orderv1.BatchReplaceAdmissionStatus_BATCH_REPLACE_ADMISSION_STATUS_ADMITTED,
		Items: []*orderv1.BatchReplaceStatusItem{
			{ItemIndex: 0, Phase: orderv1.BatchReplacePhase_BATCH_REPLACE_PHASE_WORKING, OldOrderId: 1, ReplacementOrderId: 2, OrderStatus: orderv1.OrderStatus_WORKING},
			{ItemIndex: 1, Phase: orderv1.BatchReplacePhase_BATCH_REPLACE_PHASE_TERMINAL, OldOrderId: 3, ReplacementOrderId: 4, OrderStatus: orderv1.OrderStatus_FILLED},
		},
		AcceptedCount: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.BatchRequestID == "" || result.BatchRequestID == "0" || result.AdmissionStatus != "admitted" || len(result.Items) != 2 {
		t.Fatalf("result=%+v", result)
	}
	if result.Items[0].Phase != "working" || result.Items[1].Phase != "terminal" {
		t.Fatalf("phases=%+v", result.Items)
	}
}

func TestBatchReplaceStatusRejectsCountMismatch(t *testing.T) {
	_, err := decode.BatchReplaceStatusFromProto(&orderv1.GetBatchReplaceStatusResponse{
		BatchRequestId:  7,
		AdmissionStatus: orderv1.BatchReplaceAdmissionStatus_BATCH_REPLACE_ADMISSION_STATUS_ADMITTED,
		Items: []*orderv1.BatchReplaceStatusItem{
			{ItemIndex: 0, Phase: orderv1.BatchReplacePhase_BATCH_REPLACE_PHASE_REJECTED},
		},
		AcceptedCount: 1,
	})
	var contractErr *sdkerrors.ResponseContractError
	if !errors.As(err, &contractErr) {
		t.Fatalf("expected ResponseContractError for count mismatch, got %T: %v", err, err)
	}
}

func TestCancelAllRequiresKnownStatus(t *testing.T) {
	result, err := decode.CancelAllFromProto(&orderv1.CancelAllOrdersResponse{
		Status:           orderv1.CancelAllOrdersResponse_SUBMITTED,
		MatchedOrders:    3,
		SubmittedCancels: 2,
		FailedCancels:    1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "submitted" || result.FailedCancels != 1 {
		t.Fatalf("result=%+v", result)
	}
	if _, err := decode.CancelAllFromProto(&orderv1.CancelAllOrdersResponse{Status: orderv1.CancelAllOrdersResponse_DRY_RUN}); err != nil {
		t.Fatal(err)
	}
	for _, status := range []orderv1.CancelAllOrdersResponse_Status{
		orderv1.CancelAllOrdersResponse_STATUS_UNSPECIFIED,
		orderv1.CancelAllOrdersResponse_Status(99),
	} {
		_, err := decode.CancelAllFromProto(&orderv1.CancelAllOrdersResponse{Status: status, MatchedOrders: 1})
		var contractErr *sdkerrors.ResponseContractError
		if !errors.As(err, &contractErr) {
			t.Fatalf("status %v: expected ResponseContractError, got %T: %v", status, err, err)
		}
	}
}

func TestCancelAllAfterRequiresKnownStatus(t *testing.T) {
	result, err := decode.CancelAllAfterFromProto(&orderv1.CancelAllAfterResponse{
		Status:              orderv1.CancelAllAfterResponse_ARMED,
		EffectiveTimeoutSec: 30,
		ExpiresAtTsNs:       99,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "armed" || result.EffectiveTimeoutSec != 30 || result.ExpiresAtTsNs != "99" {
		t.Fatalf("result=%+v", result)
	}
	if _, err := decode.CancelAllAfterFromProto(&orderv1.CancelAllAfterResponse{Status: orderv1.CancelAllAfterResponse_DISABLED}); err != nil {
		t.Fatal(err)
	}
	for _, status := range []orderv1.CancelAllAfterResponse_Status{
		orderv1.CancelAllAfterResponse_STATUS_UNSPECIFIED,
		orderv1.CancelAllAfterResponse_Status(99),
	} {
		_, err := decode.CancelAllAfterFromProto(&orderv1.CancelAllAfterResponse{Status: status, EffectiveTimeoutSec: 10})
		var contractErr *sdkerrors.ResponseContractError
		if !errors.As(err, &contractErr) {
			t.Fatalf("status %v: expected ResponseContractError, got %T: %v", status, err, err)
		}
	}
}
