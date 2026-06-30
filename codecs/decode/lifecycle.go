package decode

import (
	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	lifecyclev1 "github.com/Fabric-Labs/polyester-sdk-go/gen/chain/lifecycle/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

func flowSummary(msg *lifecyclev1.FlowSummaryView) models.LifecycleFlowSummary {
	if msg == nil {
		return models.LifecycleFlowSummary{}
	}
	return models.LifecycleFlowSummary{
		IntentID: msg.GetFlowId(), FlowKind: msg.GetFlowKind().String(), LatestStep: msg.GetCurrentStep().String(),
		IsOpen: msg.GetIsOpen(), IsTerminal: msg.GetIsTerminal(),
		OwnerAccountID: codecs.FormatUint64ID(msg.GetOwnerAccountId()), SmartAccountAddress: msg.GetSourceAddress(),
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

func FlowFromGetResponse(msg *lifecyclev1.GetFlowResponse) models.LifecycleFlowSummary {
	if msg.GetFlow() == nil {
		return models.LifecycleFlowSummary{}
	}
	return flowSummary(msg.GetFlow().GetSummary())
}

func flowTxMatch(msg *lifecyclev1.FlowTxMatchView) models.LifecycleFlowSummary {
	if msg == nil {
		return models.LifecycleFlowSummary{}
	}
	return models.LifecycleFlowSummary{
		IntentID: msg.GetFlowId(), FlowKind: msg.GetFlowKind().String(), LatestStep: msg.GetCurrentStep().String(),
		IsOpen: msg.GetIsOpen(), IsTerminal: msg.GetIsTerminal(), SmartAccountAddress: msg.GetSourceAddress(),
	}
}

func FlowFromGetByTxResponse(msg *lifecyclev1.ListFlowsByTxResponse) models.LifecycleFlowSummary {
	if len(msg.GetMatches()) == 0 {
		return models.LifecycleFlowSummary{}
	}
	return flowTxMatch(msg.GetMatches()[0])
}

func FlowsByTxListFromProto(msg *lifecyclev1.ListFlowsByTxResponse) models.LifecycleFlowsList {
	out := make([]models.LifecycleFlowSummary, 0, len(msg.GetMatches()))
	for _, m := range msg.GetMatches() {
		out = append(out, flowTxMatch(m))
	}
	return models.LifecycleFlowsList{Flows: out, NextPageToken: msg.GetNextPageToken()}
}
