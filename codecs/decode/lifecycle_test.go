package decode_test

import (
	"testing"

	"github.com/Fabric-Labs/polyester-sdk-go/codecs/decode"
	lifecyclev1 "github.com/Fabric-Labs/polyester-sdk-go/gen/chain/lifecycle/v1"
)

func TestFlowSummaryFromProto(t *testing.T) {
	msg := &lifecyclev1.FlowSummaryView{
		FlowId:         "flow-abc",
		FlowKind:       lifecyclev1.FlowKind_KIND_DEPOSIT,
		CurrentStep:    lifecyclev1.FlowStep_FLOW_STEP_SETTLEMENT,
		IsOpen:         false,
		IsTerminal:     true,
		OwnerAccountId: 99,
		SourceAddress:  "0xabc",
	}
	flow := decode.FlowSummaryMessageFromProto(msg)
	if flow.IntentID != "flow-abc" || !flow.IsTerminal || flow.SmartAccountAddress != "0xabc" {
		t.Fatalf("flow=%+v", flow)
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
