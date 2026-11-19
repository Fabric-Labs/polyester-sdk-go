package decode_test

import (
	"reflect"
	"testing"
	"time"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"github.com/Fabric-Labs/polyester-sdk-go/codecs/decode"
	lifecyclev1 "github.com/Fabric-Labs/polyester-sdk-go/gen/chain/lifecycle/v1"
	typev1 "github.com/Fabric-Labs/polyester-sdk-go/gen/polyester/type/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

// openWithdrawJSON is a flow summary in the shape the API's JSON answer has:
// an open withdrawal with its amount, fee, transactions, route, times,
// current progress and timeline.
const openWithdrawJSON = `{
 "smartAccountAddress": "0x00000000000000000000000000000000000000a1",
 "flowId": "flow_UZBpp6Kx7uX",
 "flowKind": "KIND_WITHDRAW",
 "currentStep": "FLOW_STEP_VALIDATION",
 "assetIds": {"zippedAssetId": 3, "unifiedAssetId": 1},
 "polyesterChainId": 2,
 "amountE18": {"hi": "1", "lo": "5000000000000000000"},
 "requestFee": {
  "assetIds": {"zippedAssetId": 3, "unifiedAssetId": 1},
  "amountE18": {"lo": "720000000000000000"},
  "recipientAddress": "0x00000000000000000000000000000000000000f1",
  "status": "REQUEST_FEE_STATUS_LOCKED"
 },
 "sourceTxHash": "0x6bb813ce477bf3aa09eb66315f10b2ae365a4d78285eba5e5cb6e09067f3a9b0",
 "txOccurrenceIndex": "8",
 "sourceAddress": "0x00000000000000000000000000000000000000a1",
 "destinationAddress": "0x00000000000000000000000000000000000000b2",
 "latestTxRef": "11455311618482410803791580314813518967",
 "sourceDomain": "DOMAIN_FUNDING",
 "destinationDomain": "DOMAIN_EXTERNAL_CHAIN",
 "latestLifecycleSource": "SOURCE_LEDGER",
 "startedAtUnixMs": "1795113306000",
 "updatedAtUnixMs": "1795113325000",
 "lastActivityAtUnixMs": "1795113325500",
 "isOpen": true,
 "currentStepSequence": 2,
 "currentProgress": {
  "currentStepStartedAtUnixMs": "1795113310000",
  "currentStepExpectedDurationMs": "11000",
  "currentConfirmations": 1,
  "requiredConfirmations": 12,
  "approveCount": 2,
  "rejectCount": 1,
  "validatorCount": 5,
  "requiredApprovals": 3,
  "requiredRejections": 3
 },
 "progressTimeline": [
  {"sequence": 1, "step": "FLOW_STEP_REQUEST", "status": "TIMELINE_STATUS_COMPLETED", "expectedDurationMs": "3000"},
  {"sequence": 2, "step": "FLOW_STEP_VALIDATION", "status": "TIMELINE_STATUS_CURRENT"},
  {"sequence": 3, "step": "FLOW_STEP_EXECUTION", "status": "TIMELINE_STATUS_PLANNED", "expectedDurationMs": "2000"}
 ],
 "estimatedCompletionUnixMs": "1795113347903"
}`

func openWithdrawView(t *testing.T) *lifecyclev1.FlowSummaryView {
	t.Helper()
	msg := &lifecyclev1.FlowSummaryView{}
	if err := protojson.Unmarshal([]byte(openWithdrawJSON), msg); err != nil {
		t.Fatal(err)
	}
	return msg
}

func ms(v int64) time.Time { return time.UnixMilli(v).UTC() }

func checkTime(t *testing.T, name string, got *time.Time, want int64) {
	t.Helper()
	if got == nil || !got.Equal(ms(want)) || got.Location() != time.UTC {
		t.Errorf("%s = %v, want %v", name, got, ms(want))
	}
}

func checkAmount(t *testing.T, name string, got *models.AssetAmountScaled, want string) {
	t.Helper()
	if got == nil {
		t.Errorf("%s is nil, want %s", name, want)
		return
	}
	if got.Scaled().String() != want || got.Domain() != models.QuantityDomainLedgerE18 || got.Scale() == nil || *got.Scale() != 18 {
		t.Errorf("%s = %s (domain %q, scale %v), want %s at the 18-decimal ledger scale", name, got.Scaled(), got.Domain(), got.Scale(), want)
	}
}

// checkOpenWithdraw checks every field openWithdrawJSON sets.
func checkOpenWithdraw(t *testing.T, flow models.LifecycleFlowSummary) {
	t.Helper()
	if flow.IntentID != "flow_UZBpp6Kx7uX" || flow.FlowKind != "withdraw" || flow.LatestStep != "validation" || !flow.IsOpen {
		t.Fatalf("flow header = %+v", flow)
	}
	if flow.ZippedAssetID != 3 || flow.UnifiedAssetID != 1 || flow.PolyesterChainID != 2 {
		t.Errorf("assets/chain = %d/%d/%d", flow.ZippedAssetID, flow.UnifiedAssetID, flow.PolyesterChainID)
	}
	// hi=1, lo=5e18: 2^64 + 5e18, past uint64, decoded exactly.
	checkAmount(t, "Amount", flow.Amount, "23446744073709551616")
	if flow.SourceTxHash != "0x6bb813ce477bf3aa09eb66315f10b2ae365a4d78285eba5e5cb6e09067f3a9b0" ||
		flow.TxOccurrenceIndex != 8 || flow.LatestTxRef != "11455311618482410803791580314813518967" {
		t.Errorf("transactions = %q/%d/%q", flow.SourceTxHash, flow.TxOccurrenceIndex, flow.LatestTxRef)
	}
	if flow.SourceDomain != "funding" || flow.DestinationDomain != "external_chain" {
		t.Errorf("domains = %q -> %q", flow.SourceDomain, flow.DestinationDomain)
	}
	checkTime(t, "StartedAt", flow.StartedAt, 1795113306000)
	checkTime(t, "UpdatedAt", flow.UpdatedAt, 1795113325000)
	checkTime(t, "LastActivityAt", flow.LastActivityAt, 1795113325500)
	checkTime(t, "EstimatedCompletionAt", flow.EstimatedCompletionAt, 1795113347903)
	if flow.TerminalAt != nil {
		t.Errorf("TerminalAt = %v on an open flow, want nil", flow.TerminalAt)
	}
	fee := flow.RequestFee
	if fee == nil {
		t.Fatal("RequestFee is nil")
	}
	if fee.ZippedAssetID != 3 || fee.UnifiedAssetID != 1 || fee.RecipientAddress != "0x00000000000000000000000000000000000000f1" || fee.Status != "locked" {
		t.Errorf("RequestFee = %+v", *fee)
	}
	checkAmount(t, "RequestFee.Amount", fee.Amount, "720000000000000000")
	if flow.CurrentStepSequence != 2 {
		t.Errorf("CurrentStepSequence = %d", flow.CurrentStepSequence)
	}
	p := flow.CurrentProgress
	if p == nil {
		t.Fatal("CurrentProgress is nil")
	}
	checkTime(t, "CurrentProgress.CurrentStepStartedAt", p.CurrentStepStartedAt, 1795113310000)
	if p.CurrentStepExpectedDurationMs != 11000 || p.CurrentConfirmations != 1 || p.RequiredConfirmations != 12 ||
		p.ApproveCount != 2 || p.RejectCount != 1 || p.ValidatorCount != 5 || p.RequiredApprovals != 3 || p.RequiredRejections != 3 {
		t.Errorf("CurrentProgress = %+v", *p)
	}
	want := []models.LifecycleFlowTimelineItem{
		{Sequence: 1, Step: "request", Status: "completed", ExpectedDurationMs: 3000},
		{Sequence: 2, Step: "validation", Status: "current"},
		{Sequence: 3, Step: "execution", Status: "planned", ExpectedDurationMs: 2000},
	}
	if flow.ProgressTimeline == nil || len(*flow.ProgressTimeline) != len(want) {
		t.Fatalf("ProgressTimeline = %+v", flow.ProgressTimeline)
	}
	for i, got := range *flow.ProgressTimeline {
		if got != want[i] {
			t.Errorf("ProgressTimeline[%d] = %+v, want %+v", i, got, want[i])
		}
	}
}

func TestListFlowsKeepsAmountTransactionsAndTimes(t *testing.T) {
	page := mustReply(decode.FlowsListFromProto(&lifecyclev1.ListFlowsResponse{Flows: []*lifecyclev1.FlowSummaryView{openWithdrawView(t)}}))
	if len(page.Flows) != 1 {
		t.Fatalf("flows = %+v", page.Flows)
	}
	checkOpenWithdraw(t, page.Flows[0])
}

func TestGetFlowKeepsAmountTransactionsAndTimes(t *testing.T) {
	flow := mustReply(decode.FlowFromGetResponse(&lifecyclev1.GetFlowResponse{Flow: &lifecyclev1.FlowDetailView{Summary: openWithdrawView(t)}}))
	checkOpenWithdraw(t, flow)
}

func TestAccountFlowsKeepAmountTransactionsAndTimes(t *testing.T) {
	page := mustReply(decode.AccountFlowsListFromProto(&lifecyclev1.ListAccountFlowsResponse{Flows: []*lifecyclev1.FlowSummaryView{openWithdrawView(t)}}))
	if len(page.Flows) != 1 {
		t.Fatalf("flows = %+v", page.Flows)
	}
	checkOpenWithdraw(t, page.Flows[0])
}

func TestGetAccountFlowKeepsAmountTransactionsAndTimes(t *testing.T) {
	flow := mustReply(decode.AccountFlowFromGetResponse(&lifecyclev1.GetAccountFlowResponse{Flow: &lifecyclev1.FlowDetailView{Summary: openWithdrawView(t)}}))
	checkOpenWithdraw(t, flow)
}

func TestRealtimeFlowUpdateKeepsAmountTransactionsAndTimes(t *testing.T) {
	payload, err := proto.Marshal(openWithdrawView(t))
	if err != nil {
		t.Fatal(err)
	}
	flow, err := decode.FlowSummaryFromBytes(payload)
	if err != nil {
		t.Fatal(err)
	}
	checkOpenWithdraw(t, flow)
}

func TestRealtimeFlowDetailKeepsAmountTransactionsAndTimes(t *testing.T) {
	payload, err := proto.Marshal(&lifecyclev1.FlowDetailView{Summary: openWithdrawView(t)})
	if err != nil {
		t.Fatal(err)
	}
	flow, err := decode.FlowDetailFromBytes(payload)
	if err != nil {
		t.Fatal(err)
	}
	checkOpenWithdraw(t, flow)
}

// A terminal flow has its terminal time, and a delayed one no estimate; a
// summary the server sends without them leaves every optional field unset
// rather than zero.
func TestFlowTimesAndOptionalFieldsWhenUnset(t *testing.T) {
	terminal := decode.FlowSummaryMessageFromProto(&lifecyclev1.FlowSummaryView{
		FlowId: "flow_t", IsTerminal: true, TerminalAtUnixMs: 1795113340254,
		RequestFee: &lifecyclev1.RequestFee{Status: lifecyclev1.RequestFeeStatus_REQUEST_FEE_STATUS_KEPT},
	})
	checkTime(t, "TerminalAt", terminal.TerminalAt, 1795113340254)
	if terminal.RequestFee == nil || terminal.RequestFee.Status != "kept" || terminal.RequestFee.Amount != nil {
		t.Errorf("RequestFee = %+v", terminal.RequestFee)
	}

	bare := decode.FlowSummaryMessageFromProto(&lifecyclev1.FlowSummaryView{FlowId: "flow_d", IsOpen: true, IsDelayed: true})
	if bare.Amount != nil || bare.RequestFee != nil || bare.CurrentProgress != nil || bare.ProgressTimeline != nil ||
		bare.StartedAt != nil || bare.UpdatedAt != nil || bare.TerminalAt != nil || bare.LastActivityAt != nil || bare.EstimatedCompletionAt != nil {
		t.Errorf("unset fields decoded as set: %+v", bare)
	}
	if bare.SourceDomain != "unspecified" || bare.DestinationDomain != "unspecified" || bare.ZippedAssetID != 0 || bare.PolyesterChainID != 0 {
		t.Errorf("bare = %+v", bare)
	}
}

func TestFlowEnumsNewerThanTheSDKKeepTheirNumber(t *testing.T) {
	flow := decode.FlowSummaryMessageFromProto(&lifecyclev1.FlowSummaryView{
		FlowId:           "flow_n",
		SourceDomain:     lifecyclev1.FlowDomain(42),
		RequestFee:       &lifecyclev1.RequestFee{Status: lifecyclev1.RequestFeeStatus(9)},
		ProgressTimeline: []*lifecyclev1.FlowTimelineItemView{{Sequence: 1, Status: lifecyclev1.FlowTimelineStatus(7)}},
	})
	if flow.SourceDomain != "unknown_domain_42" || flow.RequestFee.Status != "unknown_fee_status_9" || (*flow.ProgressTimeline)[0].Status != "unknown_timeline_status_7" {
		t.Fatalf("flow = %+v, fee %+v, timeline %+v", flow, flow.RequestFee, flow.ProgressTimeline)
	}
}

func TestFlowsByTxKeepAmountTransactionsAndActivity(t *testing.T) {
	page := mustReply(decode.FlowsByTxListFromProto(&lifecyclev1.ListFlowsByTxResponse{Matches: []*lifecyclev1.FlowTxMatchView{{
		FlowId:               "flow_m",
		FlowKind:             lifecyclev1.FlowKind_KIND_DEPOSIT,
		SourceTxHash:         "0xabc",
		LatestTxRef:          "0xdef",
		TxOccurrenceIndex:    4,
		SourceDomain:         lifecyclev1.FlowDomain_DOMAIN_EXTERNAL_CHAIN,
		DestinationDomain:    lifecyclev1.FlowDomain_DOMAIN_TRADING,
		AssetIds:             &lifecyclev1.AssetIds{ZippedAssetId: 11, UnifiedAssetId: 5},
		PolyesterChainId:     2,
		AmountE18:            &typev1.U128{Lo: 110000000000000},
		LastActivityAtUnixMs: 1795113314313,
	}}}))
	if len(page.Flows) != 1 {
		t.Fatalf("flows = %+v", page.Flows)
	}
	flow := page.Flows[0]
	if flow.SourceTxHash != "0xabc" || flow.LatestTxRef != "0xdef" || flow.TxOccurrenceIndex != 4 ||
		flow.SourceDomain != "external_chain" || flow.DestinationDomain != "trading" ||
		flow.ZippedAssetID != 11 || flow.UnifiedAssetID != 5 || flow.PolyesterChainID != 2 {
		t.Errorf("flow = %+v", flow)
	}
	checkAmount(t, "Amount", flow.Amount, "110000000000000")
	checkTime(t, "LastActivityAt", flow.LastActivityAt, 1795113314313)
	if flow.StartedAt != nil || flow.RequestFee != nil || flow.CurrentProgress != nil || flow.ProgressTimeline != nil {
		t.Errorf("a transaction match set fields it does not carry: %+v", flow)
	}
}

// A flow summary stays comparable, as it was before it carried its timeline:
// callers compare flows with == and key maps by them.
func TestFlowSummaryStaysComparable(t *testing.T) {
	flow := decode.FlowSummaryMessageFromProto(openWithdrawView(t))
	if !reflect.TypeFor[models.LifecycleFlowSummary]().Comparable() {
		t.Fatal("models.LifecycleFlowSummary is not comparable")
	}
	seen := map[models.LifecycleFlowSummary]bool{flow: true}
	if !seen[flow] || flow == (models.LifecycleFlowSummary{}) {
		t.Fatalf("comparison: seen=%v flow=%+v", seen[flow], flow)
	}
}
