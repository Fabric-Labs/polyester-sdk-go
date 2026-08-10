package decode_test

import (
	"errors"
	"testing"
	"time"

	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	"github.com/Fabric-Labs/polyester-sdk-go/codecs/decode"
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	orderv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/orders/v1"
	ratelimitv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/polyester/ratelimit/v1"
	typev1 "github.com/Fabric-Labs/polyester-sdk-go/gen/polyester/type/v1"
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

func TestOrderFromProtoMapsAttachedRisk(t *testing.T) {
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
			},
			Oco: true,
		},
	}
	order := decode.OrderFromProto(msg)
	if order.AttachedRisk == nil {
		t.Fatal("expected attached_risk")
	}
	risk := order.AttachedRisk
	if !risk.Oco || risk.TakeProfit == nil || risk.TrailingStop == nil {
		t.Fatalf("risk=%+v", risk)
	}
	if risk.StopLoss != nil {
		t.Fatalf("stop_loss should be suppressed when trailing present: %+v", risk.StopLoss)
	}
	if risk.TakeProfit.TriggerPrice.Ticks() != 6000 || risk.TakeProfit.OrderType != "market" {
		t.Fatalf("take_profit=%+v", risk.TakeProfit)
	}
	if risk.TrailingStop.DistanceBps != 25 || risk.TrailingStop.MaxSlippageTicks != 10 {
		t.Fatalf("trailing=%+v", risk.TrailingStop)
	}
	if risk.TrailingStop.ActivationPrice.Ticks() != 5500 {
		t.Fatalf("trailing=%+v", risk.TrailingStop)
	}
}

func TestOrderFromProtoOmitsTrailingStopWithoutDistance(t *testing.T) {
	msg := &orderv1.Order{
		OrderId:  3,
		SymbolId: 1,
		AttachedRisk: &orderv1.AttachedRisk{
			TrailingStop: &orderv1.AttachedRiskTrailingStop{
				Policy: &orderv1.TrailingStopPolicy{
					ActivationPriceTicks: 5500,
				},
			},
		},
	}
	order := decode.OrderFromProto(msg)
	if order.AttachedRisk != nil {
		t.Fatalf("expected attached_risk omitted when trailing distance missing, got %+v", order.AttachedRisk)
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
	msg := &orderv1.CancelOrderResponse{Status: "cancelled", OrderId: 42}
	result, err := decode.OrderMutationFromCancel(msg)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "cancelled" || result.OrderID != codecs.FormatUint64ID(42) || result.ClientOrderID != "" {
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
		Results:       []*orderv1.BatchCancelResultItem{{Status: "accepted"}},
		RejectedCount: 1,
	})
	var contractErr *sdkerrors.ResponseContractError
	if !errors.As(err, &contractErr) {
		t.Fatalf("expected ResponseContractError for count mismatch, got %T: %v", err, err)
	}

	_, err = decode.BatchCancelFromProto(&orderv1.BatchCancelOrdersResponse{
		Results:       []*orderv1.BatchCancelResultItem{{Status: "maybe"}},
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
		Status:           "submitted",
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
	if _, err := decode.CancelAllFromProto(&orderv1.CancelAllOrdersResponse{Status: "dry_run"}); err != nil {
		t.Fatal(err)
	}
	for _, status := range []string{"", "ok", "maybe", "accepted"} {
		_, err := decode.CancelAllFromProto(&orderv1.CancelAllOrdersResponse{Status: status, MatchedOrders: 1})
		var contractErr *sdkerrors.ResponseContractError
		if !errors.As(err, &contractErr) {
			t.Fatalf("status %q: expected ResponseContractError, got %T: %v", status, err, err)
		}
	}
}

func TestCancelAllAfterRequiresKnownStatus(t *testing.T) {
	result, err := decode.CancelAllAfterFromProto(&orderv1.CancelAllAfterResponse{
		Status:              "armed",
		EffectiveTimeoutSec: 30,
		ExpiresAtTsNs:       99,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "armed" || result.EffectiveTimeoutSec != 30 || result.ExpiresAtTsNs != "99" {
		t.Fatalf("result=%+v", result)
	}
	if _, err := decode.CancelAllAfterFromProto(&orderv1.CancelAllAfterResponse{Status: "disabled"}); err != nil {
		t.Fatal(err)
	}
	for _, status := range []string{"", "ok", "submitted", "maybe"} {
		_, err := decode.CancelAllAfterFromProto(&orderv1.CancelAllAfterResponse{Status: status, EffectiveTimeoutSec: 10})
		var contractErr *sdkerrors.ResponseContractError
		if !errors.As(err, &contractErr) {
			t.Fatalf("status %q: expected ResponseContractError, got %T: %v", status, err, err)
		}
	}
}
