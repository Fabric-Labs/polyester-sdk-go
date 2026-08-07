package decode_test

import (
	"errors"
	"testing"

	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	"github.com/Fabric-Labs/polyester-sdk-go/codecs/decode"
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	lifecyclev1 "github.com/Fabric-Labs/polyester-sdk-go/gen/chain/lifecycle/v1"
	zipperv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/chain/zipper/v1"
)

func TestFlowSummaryFromProto(t *testing.T) {
	msg := &lifecyclev1.FlowSummaryView{
		FlowId:              "flow-abc",
		FlowKind:            lifecyclev1.FlowKind_KIND_DEPOSIT,
		CurrentStep:         lifecyclev1.FlowStep_FLOW_STEP_SETTLEMENT,
		IsOpen:              false,
		IsTerminal:          true,
		OwnerAccountId:      99,
		SourceAddress:       "0xsource",
		SmartAccountAddress: "0xabc",
		LifecycleReason:     lifecyclev1.LifecycleReason_ZIPPER_VALIDATION_REJECTED,
		ZipperReason: &zipperv1.ZipperReasonDetails{
			Code:     zipperv1.ZipperReasonCode_DEPOSIT_AMOUNT_BELOW_MINIMUM,
			ReasonId: "deposit_amount_below_minimum",
			Message:  "Deposit amount is below the minimum.",
		},
	}
	flow := decode.FlowSummaryMessageFromProto(msg)
	if flow.IntentID != "flow-abc" || !flow.IsTerminal ||
		flow.FlowKind != "deposit" ||
		flow.LatestStep != "settlement" ||
		flow.SmartAccountAddress != "0xabc" ||
		flow.LifecycleReason != "zipper_validation_rejected" ||
		flow.ZipperReason == nil ||
		flow.ZipperReason.Code != int32(zipperv1.ZipperReasonCode_DEPOSIT_AMOUNT_BELOW_MINIMUM) ||
		flow.ZipperReason.ReasonID != "deposit_amount_below_minimum" ||
		flow.ZipperReason.Message != "Deposit amount is below the minimum." {
		t.Fatalf("flow=%+v", flow)
	}
}

func TestLifecycleReasonUnknownCodePreserved(t *testing.T) {
	flow := decode.FlowSummaryMessageFromProto(&lifecyclev1.FlowSummaryView{
		FlowId:          "flow-unknown",
		LifecycleReason: lifecyclev1.LifecycleReason(2001),
	})
	if flow.LifecycleReason != "unknown_reason_2001" {
		t.Fatalf("lifecycle_reason=%q", flow.LifecycleReason)
	}
}

func TestTradingWithdrawLifecycleReasons(t *testing.T) {
	cases := []struct {
		reason   lifecyclev1.LifecycleReason
		expected string
	}{
		{lifecyclev1.LifecycleReason_TRADING_WITHDRAW_POLICY_DENIED, "trading_withdraw_policy_denied"},
		{lifecyclev1.LifecycleReason_TRADING_WITHDRAW_CONTRACT_REVERTED, "trading_withdraw_contract_reverted"},
		{lifecyclev1.LifecycleReason_TRADING_WITHDRAW_EXECUTION_FAILED, "trading_withdraw_execution_failed"},
	}
	for _, tc := range cases {
		flow := decode.FlowSummaryMessageFromProto(&lifecyclev1.FlowSummaryView{
			FlowId:          "flow-trading-withdraw",
			FlowKind:        lifecyclev1.FlowKind_KIND_WITHDRAW,
			CurrentStep:     lifecyclev1.FlowStep_FLOW_STEP_FAILED,
			IsTerminal:      true,
			LifecycleReason: tc.reason,
		})
		if flow.LifecycleReason != tc.expected {
			t.Fatalf("reason=%v lifecycle_reason=%q want %q", tc.reason, flow.LifecycleReason, tc.expected)
		}
	}
}

func TestLifecycleReasonUnspecified(t *testing.T) {
	flow := decode.FlowSummaryMessageFromProto(&lifecyclev1.FlowSummaryView{
		FlowId: "flow-ok",
	})
	if flow.LifecycleReason != "unspecified" {
		t.Fatalf("lifecycle_reason=%q", flow.LifecycleReason)
	}
	if flow.FlowKind != "unspecified" || flow.LatestStep != "unspecified" {
		t.Fatalf("flow kind/step=%q/%q", flow.FlowKind, flow.LatestStep)
	}
}

func TestGetFlowResponseRejectsMissingRequiredEntity(t *testing.T) {
	_, err := decode.FlowFromGetResponse(&lifecyclev1.GetFlowResponse{})
	var transportErr *sdkerrors.TransportError
	if !errors.As(err, &transportErr) {
		t.Fatalf("expected TransportError for missing flow, got %T: %v", err, err)
	}
}

func TestFlowsListFromProto(t *testing.T) {
	msg := &lifecyclev1.ListFlowsResponse{
		Flows: []*lifecyclev1.FlowSummaryView{
			{FlowId: "a"},
			{FlowId: "b"},
		},
		NextPageToken: "next",
	}
	result := decode.FlowsListFromProto(msg)
	if len(result.Flows) != 2 || result.NextPageToken != "next" {
		t.Fatalf("result=%+v", result)
	}
}

func TestFlowByTxResponseReturnsAllMatches(t *testing.T) {
	result, err := decode.FlowFromGetByTxResponse(&lifecyclev1.ListFlowsByTxResponse{
		Matches: []*lifecyclev1.FlowTxMatchView{
			{
				FlowId:              "flow-a",
				SourceAddress:       "0xsource",
				OwnerAccountId:      99,
				SmartAccountAddress: "0xsmart",
				LifecycleReason:     lifecyclev1.LifecycleReason_LEDGER_MIRROR_REJECTED,
			},
			{FlowId: "flow-b"},
		},
		NextPageToken: "next",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Flows) != 2 ||
		result.Flows[0].OwnerAccountID != codecs.FormatUint64ID(99) ||
		result.Flows[0].SmartAccountAddress != "0xsmart" ||
		result.Flows[0].LifecycleReason != "ledger_mirror_rejected" ||
		result.Flows[1].IntentID != "flow-b" ||
		result.NextPageToken != "next" {
		t.Fatalf("result=%+v", result)
	}
}
