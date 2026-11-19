package decode

import (
	"fmt"
	"math"
	"time"

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
	case lifecyclev1.LifecycleReason_DEPOSIT_SOURCE_NOT_CONFIRMED:
		return "deposit_source_not_confirmed"
	case lifecyclev1.LifecycleReason_ZIPPER_COMPLIANCE_TRANSFERRED:
		return "zipper_compliance_transferred"
	case lifecyclev1.LifecycleReason_ZIPPER_DEPOSIT_DELIVERED_TO_WALLET:
		// The one reason a completed flow carries: the deposit ended at the
		// account's own wallet, not in Funding or Trading. Not a failure.
		return "zipper_deposit_delivered_to_wallet"
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

// flowParticipationLabel names a FlowParticipation the way the REST API
// spells it, lowercased. A value newer than this SDK reads
// unknown_participation_<n>, never as unspecified.
func flowParticipationLabel(participation lifecyclev1.FlowParticipation) string {
	switch participation {
	case lifecyclev1.FlowParticipation_PARTICIPATION_UNSPECIFIED:
		return "unspecified"
	case lifecyclev1.FlowParticipation_PARTICIPATION_OWNER:
		return "owner"
	case lifecyclev1.FlowParticipation_PARTICIPATION_RECIPIENT:
		return "recipient"
	default:
		return fmt.Sprintf("unknown_participation_%d", int32(participation))
	}
}

// flowDomainLabel names a FlowDomain lowercased without its DOMAIN_ prefix.
// A value newer than this SDK reads unknown_domain_<n>, never as unspecified.
func flowDomainLabel(domain lifecyclev1.FlowDomain) string {
	switch domain {
	case lifecyclev1.FlowDomain_DOMAIN_UNSPECIFIED:
		return "unspecified"
	case lifecyclev1.FlowDomain_DOMAIN_EXTERNAL_CHAIN:
		return "external_chain"
	case lifecyclev1.FlowDomain_DOMAIN_ZIPPER:
		return "zipper"
	case lifecyclev1.FlowDomain_DOMAIN_FUNDING:
		return "funding"
	case lifecyclev1.FlowDomain_DOMAIN_TRADING:
		return "trading"
	case lifecyclev1.FlowDomain_DOMAIN_LENDING:
		return "lending"
	default:
		return fmt.Sprintf("unknown_domain_%d", int32(domain))
	}
}

func requestFeeStatusLabel(status lifecyclev1.RequestFeeStatus) string {
	switch status {
	case lifecyclev1.RequestFeeStatus_REQUEST_FEE_STATUS_UNSPECIFIED:
		return "unspecified"
	case lifecyclev1.RequestFeeStatus_REQUEST_FEE_STATUS_LOCKED:
		return "locked"
	case lifecyclev1.RequestFeeStatus_REQUEST_FEE_STATUS_SETTLED:
		return "settled"
	case lifecyclev1.RequestFeeStatus_REQUEST_FEE_STATUS_KEPT:
		return "kept"
	default:
		return fmt.Sprintf("unknown_fee_status_%d", int32(status))
	}
}

func flowTimelineStatusLabel(status lifecyclev1.FlowTimelineStatus) string {
	switch status {
	case lifecyclev1.FlowTimelineStatus_TIMELINE_STATUS_UNSPECIFIED:
		return "unspecified"
	case lifecyclev1.FlowTimelineStatus_TIMELINE_STATUS_COMPLETED:
		return "completed"
	case lifecyclev1.FlowTimelineStatus_TIMELINE_STATUS_CURRENT:
		return "current"
	case lifecyclev1.FlowTimelineStatus_TIMELINE_STATUS_PLANNED:
		return "planned"
	default:
		return fmt.Sprintf("unknown_timeline_status_%d", int32(status))
	}
}

// unixMsTime reads a time the contract sends in milliseconds since the epoch
// (UTC). Zero is the contract's "no time" and reads nil, as does a value past
// what time.Time holds.
func unixMsTime(ms uint64) *time.Time {
	if ms == 0 || ms > math.MaxInt64 {
		return nil
	}
	t := time.UnixMilli(int64(ms)).UTC()
	return &t
}

func requestFee(msg *lifecyclev1.RequestFee) *models.LifecycleRequestFee {
	if msg == nil {
		return nil
	}
	return &models.LifecycleRequestFee{
		ZippedAssetID:    msg.GetAssetIds().GetZippedAssetId(),
		UnifiedAssetID:   msg.GetAssetIds().GetUnifiedAssetId(),
		Amount:           optionalLedgerAmount(msg.GetAmountE18()),
		RecipientAddress: msg.GetRecipientAddress(),
		Status:           requestFeeStatusLabel(msg.GetStatus()),
	}
}

func flowProgress(msg *lifecyclev1.FlowSummaryProgressView) *models.LifecycleFlowProgress {
	if msg == nil {
		return nil
	}
	return &models.LifecycleFlowProgress{
		CurrentStepStartedAt:          unixMsTime(msg.GetCurrentStepStartedAtUnixMs()),
		CurrentStepExpectedDurationMs: msg.GetCurrentStepExpectedDurationMs(),
		CurrentConfirmations:          msg.GetCurrentConfirmations(),
		RequiredConfirmations:         msg.GetRequiredConfirmations(),
		ApproveCount:                  msg.GetApproveCount(),
		RejectCount:                   msg.GetRejectCount(),
		ValidatorCount:                msg.GetValidatorCount(),
		RequiredApprovals:             msg.GetRequiredApprovals(),
		RequiredRejections:            msg.GetRequiredRejections(),
	}
}

func flowTimeline(items []*lifecyclev1.FlowTimelineItemView) []models.LifecycleFlowTimelineItem {
	if len(items) == 0 {
		return nil
	}
	out := make([]models.LifecycleFlowTimelineItem, 0, len(items))
	for _, item := range items {
		out = append(out, models.LifecycleFlowTimelineItem{
			Sequence:           item.GetSequence(),
			Step:               flowStepLabel(item.GetStep()),
			Status:             flowTimelineStatusLabel(item.GetStatus()),
			ExpectedDurationMs: item.GetExpectedDurationMs(),
		})
	}
	return out
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
		IsDelayed:           msg.GetIsDelayed(),
		SmartAccountAddress: msg.GetSmartAccountAddress(),
		LifecycleReason:     lifecycleReasonLabel(msg.GetLifecycleReason()),
		ZipperReason:        zipperReasonFromProto(msg.GetZipperReason()),
		Participation:       flowParticipationLabel(msg.GetParticipation()),
		SourceAddress:       msg.GetSourceAddress(),
		DestinationAddress:  msg.GetDestinationAddress(),

		ZippedAssetID:         msg.GetAssetIds().GetZippedAssetId(),
		UnifiedAssetID:        msg.GetAssetIds().GetUnifiedAssetId(),
		PolyesterChainID:      msg.GetPolyesterChainId(),
		Amount:                optionalLedgerAmount(msg.GetAmountE18()),
		RequestFee:            requestFee(msg.GetRequestFee()),
		SourceTxHash:          msg.GetSourceTxHash(),
		TxOccurrenceIndex:     msg.GetTxOccurrenceIndex(),
		LatestTxRef:           msg.GetLatestTxRef(),
		SourceDomain:          flowDomainLabel(msg.GetSourceDomain()),
		DestinationDomain:     flowDomainLabel(msg.GetDestinationDomain()),
		StartedAt:             unixMsTime(msg.GetStartedAtUnixMs()),
		UpdatedAt:             unixMsTime(msg.GetUpdatedAtUnixMs()),
		TerminalAt:            unixMsTime(msg.GetTerminalAtUnixMs()),
		LastActivityAt:        unixMsTime(msg.GetLastActivityAtUnixMs()),
		EstimatedCompletionAt: unixMsTime(msg.GetEstimatedCompletionUnixMs()),
		CurrentStepSequence:   msg.GetCurrentStepSequence(),
		CurrentProgress:       flowProgress(msg.GetCurrentProgress()),
		ProgressTimeline:      flowTimeline(msg.GetProgressTimeline()),
	}
}

// FlowSummaryMessageFromProto decodes one lifecycle flow summary message.
func FlowSummaryMessageFromProto(msg *lifecyclev1.FlowSummaryView) models.LifecycleFlowSummary {
	return flowSummary(msg)
}

// FlowsListFromProto decodes one page of flows. A flow without its ID returns
// a *errors.ResponseContractError.
func FlowsListFromProto(msg *lifecyclev1.ListFlowsResponse) (models.LifecycleFlowsList, error) {
	out := make([]models.LifecycleFlowSummary, 0, len(msg.GetFlows()))
	for _, f := range msg.GetFlows() {
		if err := requireStringID("ListFlows", "flow_id", f.GetFlowId()); err != nil {
			return models.LifecycleFlowsList{}, err
		}
		out = append(out, flowSummary(f))
	}
	return models.LifecycleFlowsList{Flows: out, NextPageToken: msg.GetNextPageToken()}, nil
}

func FlowFromGetResponse(msg *lifecyclev1.GetFlowResponse) (models.LifecycleFlowSummary, error) {
	if msg.GetFlow() == nil {
		return models.LifecycleFlowSummary{}, &sdkerrors.ResponseContractError{Operation: "GetFlowById", Msg: "missing flow"}
	}
	if msg.GetFlow().GetSummary() == nil {
		return models.LifecycleFlowSummary{}, &sdkerrors.ResponseContractError{Operation: "GetFlowById", Msg: "missing flow summary"}
	}
	if err := requireStringID("GetFlowById", "flow_id", msg.GetFlow().GetSummary().GetFlowId()); err != nil {
		return models.LifecycleFlowSummary{}, err
	}
	return flowSummary(msg.GetFlow().GetSummary()), nil
}

// AccountFlowsListFromProto decodes one page of the caller's own flows. A flow
// without its ID returns a *errors.ResponseContractError.
func AccountFlowsListFromProto(msg *lifecyclev1.ListAccountFlowsResponse) (models.LifecycleFlowsList, error) {
	out := make([]models.LifecycleFlowSummary, 0, len(msg.GetFlows()))
	for _, f := range msg.GetFlows() {
		if err := requireStringID("ListAccountFlows", "flow_id", f.GetFlowId()); err != nil {
			return models.LifecycleFlowsList{}, err
		}
		out = append(out, flowSummary(f))
	}
	return models.LifecycleFlowsList{Flows: out, NextPageToken: msg.GetNextPageToken()}, nil
}

// AccountFlowFromGetResponse decodes one of the caller's own flows. A reply
// without the flow, its summary or its ID returns a
// *errors.ResponseContractError.
func AccountFlowFromGetResponse(msg *lifecyclev1.GetAccountFlowResponse) (models.LifecycleFlowSummary, error) {
	if msg.GetFlow() == nil {
		return models.LifecycleFlowSummary{}, &sdkerrors.ResponseContractError{Operation: "GetAccountFlow", Msg: "missing flow"}
	}
	if msg.GetFlow().GetSummary() == nil {
		return models.LifecycleFlowSummary{}, &sdkerrors.ResponseContractError{Operation: "GetAccountFlow", Msg: "missing flow summary"}
	}
	if err := requireStringID("GetAccountFlow", "flow_id", msg.GetFlow().GetSummary().GetFlowId()); err != nil {
		return models.LifecycleFlowSummary{}, err
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
		SmartAccountAddress: msg.GetSmartAccountAddress(),
		LifecycleReason:     lifecycleReasonLabel(msg.GetLifecycleReason()),
		ZipperReason:        zipperReasonFromProto(msg.GetZipperReason()),
		SourceAddress:       msg.GetSourceAddress(),
		DestinationAddress:  msg.GetDestinationAddress(),
		ZippedAssetID:       msg.GetAssetIds().GetZippedAssetId(),
		UnifiedAssetID:      msg.GetAssetIds().GetUnifiedAssetId(),
		PolyesterChainID:    msg.GetPolyesterChainId(),
		Amount:              optionalLedgerAmount(msg.GetAmountE18()),
		SourceTxHash:        msg.GetSourceTxHash(),
		TxOccurrenceIndex:   msg.GetTxOccurrenceIndex(),
		LatestTxRef:         msg.GetLatestTxRef(),
		SourceDomain:        flowDomainLabel(msg.GetSourceDomain()),
		DestinationDomain:   flowDomainLabel(msg.GetDestinationDomain()),
		LastActivityAt:      unixMsTime(msg.GetLastActivityAtUnixMs()),
	}
}

// FlowFromGetByTxResponse decodes every match in a transaction lookup.
//
// The legacy helper name is retained for compatibility, but transaction
// lookups are one-to-many and must not silently discard bundled flows.
func FlowFromGetByTxResponse(msg *lifecyclev1.ListFlowsByTxResponse) (models.LifecycleFlowsList, error) {
	return FlowsByTxListFromProto(msg)
}

// FlowsByTxListFromProto decodes every flow a transaction matched. A match
// without its flow ID returns a *errors.ResponseContractError.
func FlowsByTxListFromProto(msg *lifecyclev1.ListFlowsByTxResponse) (models.LifecycleFlowsList, error) {
	out := make([]models.LifecycleFlowSummary, 0, len(msg.GetMatches()))
	for _, m := range msg.GetMatches() {
		if err := requireStringID("ListFlowsByTx", "flow_id", m.GetFlowId()); err != nil {
			return models.LifecycleFlowsList{}, err
		}
		out = append(out, flowTxMatch(m))
	}
	return models.LifecycleFlowsList{Flows: out, NextPageToken: msg.GetNextPageToken()}, nil
}
