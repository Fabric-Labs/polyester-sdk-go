package decode

import (
	"fmt"

	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	lifecyclev1 "github.com/Fabric-Labs/polyester-sdk-go/gen/chain/lifecycle/v1"
	zipperv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/chain/zipper/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

// lifecycleReasonLabel maps LifecycleReason ints to snake labels matching the
// TypeScript SDK. Unknown codes become unknown_reason_<n> so open proto3 enums
// do not fail the whole flow payload.
func lifecycleReasonLabel(code lifecyclev1.LifecycleReason) string {
	switch code {
	case lifecyclev1.LifecycleReason_REASON_UNSPECIFIED:
		return "unspecified"
	case lifecyclev1.LifecycleReason_ZIPPER_VALIDATION_REJECTED:
		return "zipper_validation_rejected"
	case lifecyclev1.LifecycleReason_ZIPPER_EXECUTION_REJECTED:
		return "zipper_execution_rejected"
	case lifecyclev1.LifecycleReason_ZIPPER_WITHDRAW_EXECUTION_FAILED:
		return "zipper_withdraw_execution_failed"
	case lifecyclev1.LifecycleReason_ZIPPER_DEPOSIT_REFUND_FAILED:
		return "zipper_deposit_refund_failed"
	case lifecyclev1.LifecycleReason_LEDGER_MIRROR_REJECTED:
		return "ledger_mirror_rejected"
	case lifecyclev1.LifecycleReason_LEDGER_MIRROR_TRANSFER_EXCEEDS_CREDITS:
		return "ledger_mirror_transfer_exceeds_credits"
	case lifecyclev1.LifecycleReason_LEDGER_MIRROR_TRANSFER_EXISTS:
		return "ledger_mirror_transfer_exists"
	case lifecyclev1.LifecycleReason_LEDGER_MIRROR_PENDING_TRANSFER_NOT_FOUND:
		return "ledger_mirror_pending_transfer_not_found"
	case lifecyclev1.LifecycleReason_LEDGER_MIRROR_TRANSFER_ID_ALREADY_FAILED:
		return "ledger_mirror_transfer_id_already_failed"
	case lifecyclev1.LifecycleReason_TRADING_WITHDRAW_POLICY_DENIED:
		return "trading_withdraw_policy_denied"
	case lifecyclev1.LifecycleReason_TRADING_WITHDRAW_CONTRACT_REVERTED:
		return "trading_withdraw_contract_reverted"
	case lifecyclev1.LifecycleReason_TRADING_WITHDRAW_EXECUTION_FAILED:
		return "trading_withdraw_execution_failed"
	default:
		return fmt.Sprintf("unknown_reason_%d", int32(code))
	}
}

func zipperReasonFromProto(msg *zipperv1.ZipperReasonDetails) *models.ZipperReasonDetails {
	if msg == nil {
		return nil
	}
	return &models.ZipperReasonDetails{
		Code:     int32(msg.GetCode()),
		ReasonID: msg.GetReasonId(),
		Message:  msg.GetMessage(),
	}
}

func flowKindLabel(kind lifecyclev1.FlowKind) string {
	switch kind {
	case lifecyclev1.FlowKind_KIND_UNSPECIFIED:
		return "unspecified"
	case lifecyclev1.FlowKind_KIND_DEPOSIT:
		return "deposit"
	case lifecyclev1.FlowKind_KIND_WITHDRAW:
		return "withdraw"
	case lifecyclev1.FlowKind_KIND_TRANSFER:
		return "transfer"
	default:
		return ""
	}
}

func flowStepLabel(step lifecyclev1.FlowStep) string {
	switch step {
	case lifecyclev1.FlowStep_FLOW_STEP_UNSPECIFIED:
		return "unspecified"
	case lifecyclev1.FlowStep_FLOW_STEP_SOURCE:
		return "source"
	case lifecyclev1.FlowStep_FLOW_STEP_TRANSFER:
		return "transfer"
	case lifecyclev1.FlowStep_FLOW_STEP_REQUEST:
		return "request"
	case lifecyclev1.FlowStep_FLOW_STEP_VALIDATION:
		return "validation"
	case lifecyclev1.FlowStep_FLOW_STEP_EXECUTION:
		return "execution"
	case lifecyclev1.FlowStep_FLOW_STEP_BRIDGE_FULFILLMENT:
		return "bridge_fulfillment"
	case lifecyclev1.FlowStep_FLOW_STEP_DROPPED:
		return "dropped"
	case lifecyclev1.FlowStep_FLOW_STEP_FAILED:
		return "failed"
	case lifecyclev1.FlowStep_FLOW_STEP_REFUNDED:
		return "refunded"
	case lifecyclev1.FlowStep_FLOW_STEP_FULFILLING:
		return "fulfilling"
	case lifecyclev1.FlowStep_FLOW_STEP_SETTLEMENT:
		return "settlement"
	default:
		return ""
	}
}

func flowSummary(msg *lifecyclev1.FlowSummaryView) models.LifecycleFlowSummary {
	if msg == nil {
		return models.LifecycleFlowSummary{}
	}
	return models.LifecycleFlowSummary{
		IntentID:            msg.GetFlowId(),
		FlowKind:            flowKindLabel(msg.GetFlowKind()),
		LatestStep:          flowStepLabel(msg.GetCurrentStep()),
		IsOpen:              msg.GetIsOpen(),
		IsTerminal:          msg.GetIsTerminal(),
		OwnerAccountID:      codecs.FormatUint64ID(msg.GetOwnerAccountId()),
		SmartAccountAddress: msg.GetSmartAccountAddress(),
		LifecycleReason:     lifecycleReasonLabel(msg.GetLifecycleReason()),
		ZipperReason:        zipperReasonFromProto(msg.GetZipperReason()),
	}
}

// FlowSummaryMessageFromProto decodes one lifecycle flow summary message.
func FlowSummaryMessageFromProto(msg *lifecyclev1.FlowSummaryView) models.LifecycleFlowSummary {
	return flowSummary(msg)
}

func FlowsListFromProto(msg *lifecyclev1.ListFlowsResponse) models.LifecycleFlowsList {
	out := make([]models.LifecycleFlowSummary, 0, len(msg.GetFlows()))
	for _, f := range msg.GetFlows() {
		out = append(out, flowSummary(f))
	}
	return models.LifecycleFlowsList{Flows: out, NextPageToken: msg.GetNextPageToken()}
}

func FlowFromGetResponse(msg *lifecyclev1.GetFlowResponse) (models.LifecycleFlowSummary, error) {
	if msg.GetFlow() == nil {
		return models.LifecycleFlowSummary{}, &sdkerrors.TransportError{
			Msg: "invalid GetFlow response: missing flow",
		}
	}
	if msg.GetFlow().GetSummary() == nil {
		return models.LifecycleFlowSummary{}, &sdkerrors.TransportError{
			Msg: "invalid GetFlow response: missing flow summary",
		}
	}
	return flowSummary(msg.GetFlow().GetSummary()), nil
}

func flowTxMatch(msg *lifecyclev1.FlowTxMatchView) models.LifecycleFlowSummary {
	if msg == nil {
		return models.LifecycleFlowSummary{}
	}
	return models.LifecycleFlowSummary{
		IntentID:            msg.GetFlowId(),
		FlowKind:            flowKindLabel(msg.GetFlowKind()),
		LatestStep:          flowStepLabel(msg.GetCurrentStep()),
		IsOpen:              msg.GetIsOpen(),
		IsTerminal:          msg.GetIsTerminal(),
		OwnerAccountID:      codecs.FormatUint64ID(msg.GetOwnerAccountId()),
		SmartAccountAddress: msg.GetSmartAccountAddress(),
		LifecycleReason:     lifecycleReasonLabel(msg.GetLifecycleReason()),
		ZipperReason:        zipperReasonFromProto(msg.GetZipperReason()),
	}
}

// FlowFromGetByTxResponse decodes every match in a transaction lookup.
//
// The legacy helper name is retained for compatibility, but transaction
// lookups are one-to-many and must not silently discard bundled flows.
func FlowFromGetByTxResponse(msg *lifecyclev1.ListFlowsByTxResponse) (models.LifecycleFlowsList, error) {
	return FlowsByTxListFromProto(msg), nil
}

func FlowsByTxListFromProto(msg *lifecyclev1.ListFlowsByTxResponse) models.LifecycleFlowsList {
	out := make([]models.LifecycleFlowSummary, 0, len(msg.GetMatches()))
	for _, m := range msg.GetMatches() {
		out = append(out, flowTxMatch(m))
	}
	return models.LifecycleFlowsList{Flows: out, NextPageToken: msg.GetNextPageToken()}
}
