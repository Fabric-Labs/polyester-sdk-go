package polyester

import (
	"context"
	stderrors "errors"
	"math"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/Fabric-Labs/polyester-sdk-go/auth"
	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	lifecyclev1 "github.com/Fabric-Labs/polyester-sdk-go/gen/chain/lifecycle/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/gen/chain/lifecycle/v1/chainlifecyclev1connect"
	ledgerrdv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/ledger/read/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/gen/ledger/read/v1/ledgerrdv1connect"
	typev1 "github.com/Fabric-Labs/polyester-sdk-go/gen/polyester/type/v1"
	transferv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/transfer/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/gen/transfer/v1/transferv1connect"
	"github.com/Fabric-Labs/polyester-sdk-go/internal/hardening"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
	"github.com/Fabric-Labs/polyester-sdk-go/services"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// contractAPI answers the RPCs public contract 1945423c added, recording
// every request and whether it was signed.
type contractAPI struct {
	transferv1connect.UnimplementedInternalTransferServiceHandler
	transferv1connect.UnimplementedDustConversionServiceHandler
	ledgerrdv1connect.UnimplementedStatementExportServiceHandler
	ledgerrdv1connect.UnimplementedLedgerReadServiceHandler
	chainlifecyclev1connect.UnimplementedLifecycleReadServiceHandler

	mu       sync.Mutex
	requests []proto.Message
	unsigned int

	batch      func(*transferv1.BatchCreateInternalTransfersRequest) *transferv1.BatchCreateInternalTransfersResponse
	batchRead  *transferv1.GetInternalTransferBatchResponse
	conversion *transferv1.DustConversion
	export     *ledgerrdv1.StatementExport
	flow       *lifecyclev1.FlowDetailView
}

func (a *contractAPI) record(h http.Header, msg proto.Message) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.requests = append(a.requests, proto.Clone(msg))
	if h.Get("X-API-KEY-ID") == "" || h.Get("X-API-SIGNATURE") == "" {
		a.unsigned++
	}
}

func (a *contractAPI) sent(t *testing.T) []proto.Message {
	t.Helper()
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.unsigned != 0 {
		t.Fatalf("%d unsigned requests", a.unsigned)
	}
	return append([]proto.Message(nil), a.requests...)
}

func (a *contractAPI) BatchCreateInternalTransfers(_ context.Context, req *connect.Request[transferv1.BatchCreateInternalTransfersRequest]) (*connect.Response[transferv1.BatchCreateInternalTransfersResponse], error) {
	a.record(req.Header(), req.Msg)
	return connect.NewResponse(a.batch(req.Msg)), nil
}

func (a *contractAPI) CreateDustConversion(_ context.Context, req *connect.Request[transferv1.CreateDustConversionRequest]) (*connect.Response[transferv1.CreateDustConversionResponse], error) {
	a.record(req.Header(), req.Msg)
	return connect.NewResponse(&transferv1.CreateDustConversionResponse{Conversion: a.conversion}), nil
}

func (a *contractAPI) ListDustBalances(_ context.Context, req *connect.Request[transferv1.ListDustBalancesRequest]) (*connect.Response[transferv1.ListDustBalancesResponse], error) {
	a.record(req.Header(), req.Msg)
	return connect.NewResponse(&transferv1.ListDustBalancesResponse{TargetAssetId: 3, Targets: []uint32{3}, FeeBps: 200}), nil
}

func (a *contractAPI) ListDustConversions(_ context.Context, req *connect.Request[transferv1.ListDustConversionsRequest]) (*connect.Response[transferv1.ListDustConversionsResponse], error) {
	a.record(req.Header(), req.Msg)
	return connect.NewResponse(&transferv1.ListDustConversionsResponse{}), nil
}

func (a *contractAPI) CreateStatementExport(_ context.Context, req *connect.Request[ledgerrdv1.CreateStatementExportRequest]) (*connect.Response[ledgerrdv1.CreateStatementExportResponse], error) {
	a.record(req.Header(), req.Msg)
	return connect.NewResponse(&ledgerrdv1.CreateStatementExportResponse{Export: a.export}), nil
}

func (a *contractAPI) GetStatementExport(_ context.Context, req *connect.Request[ledgerrdv1.GetStatementExportRequest]) (*connect.Response[ledgerrdv1.GetStatementExportResponse], error) {
	a.record(req.Header(), req.Msg)
	return connect.NewResponse(&ledgerrdv1.GetStatementExportResponse{Export: a.export}), nil
}

func (a *contractAPI) ListStatementExports(_ context.Context, req *connect.Request[ledgerrdv1.ListStatementExportsRequest]) (*connect.Response[ledgerrdv1.ListStatementExportsResponse], error) {
	a.record(req.Header(), req.Msg)
	if req.Msg.GetPageToken() == "" {
		return connect.NewResponse(&ledgerrdv1.ListStatementExportsResponse{Exports: []*ledgerrdv1.StatementExport{a.export}, NextPageToken: "p2"}), nil
	}
	return connect.NewResponse(&ledgerrdv1.ListStatementExportsResponse{Exports: []*ledgerrdv1.StatementExport{a.export}}), nil
}

func (a *contractAPI) GetInternalTransferBatch(_ context.Context, req *connect.Request[transferv1.GetInternalTransferBatchRequest]) (*connect.Response[transferv1.GetInternalTransferBatchResponse], error) {
	a.record(req.Header(), req.Msg)
	return connect.NewResponse(a.batchRead), nil
}

func (a *contractAPI) ListAccountFlows(_ context.Context, req *connect.Request[lifecyclev1.ListAccountFlowsRequest]) (*connect.Response[lifecyclev1.ListAccountFlowsResponse], error) {
	a.record(req.Header(), req.Msg)
	resp := &lifecyclev1.ListAccountFlowsResponse{Flows: []*lifecyclev1.FlowSummaryView{{
		FlowId: "flow_" + req.Msg.GetPageToken() + "A", FlowKind: lifecyclev1.FlowKind_KIND_TRANSFER, Participation: lifecyclev1.FlowParticipation_PARTICIPATION_RECIPIENT,
	}}}
	if req.Msg.GetPageToken() == "" {
		resp.NextPageToken = "B"
	}
	return connect.NewResponse(resp), nil
}

func (a *contractAPI) GetAccountFlow(_ context.Context, req *connect.Request[lifecyclev1.GetAccountFlowRequest]) (*connect.Response[lifecyclev1.GetAccountFlowResponse], error) {
	a.record(req.Header(), req.Msg)
	return connect.NewResponse(&lifecyclev1.GetAccountFlowResponse{Flow: a.flow}), nil
}

func newContractClient(t *testing.T, api *contractAPI, defaultSub *string) *Client {
	t.Helper()
	mux := http.NewServeMux()
	for _, register := range []func() (string, http.Handler){
		func() (string, http.Handler) { return transferv1connect.NewInternalTransferServiceHandler(api) },
		func() (string, http.Handler) { return transferv1connect.NewDustConversionServiceHandler(api) },
		func() (string, http.Handler) { return ledgerrdv1connect.NewStatementExportServiceHandler(api) },
		func() (string, http.Handler) { return ledgerrdv1connect.NewLedgerReadServiceHandler(api) },
		func() (string, http.Handler) { return chainlifecyclev1connect.NewLifecycleReadServiceHandler(api) },
	} {
		path, handler := register()
		mux.Handle(path, handler)
	}
	srv := hardening.NewTestServer(mux)
	t.Cleanup(srv.Close)
	keypair := auth.GenerateEd25519Keypair()
	client, err := New(Config{APIURL: srv.URL, APIKeyID: "ak_contract", APIPrivateKey: keypair.SecretKeyHex, DefaultSubAccountID: defaultSub, SkipCatalogHydration: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	return client
}

func acceptAll(req *transferv1.BatchCreateInternalTransfersRequest) *transferv1.BatchCreateInternalTransfersResponse {
	resp := &transferv1.BatchCreateInternalTransfersResponse{RequestId: "92640625967207661803151833376978068587", Status: transferv1.InternalTransferBatchStatus_BATCH_ACCEPTED}
	for i, leg := range req.GetLegs() {
		resp.Legs = append(resp.Legs, &transferv1.InternalTransferBatchLegResult{
			LegIndex: uint32(i), Status: transferv1.InternalTransferBatchLegStatus_LEG_CREDITED,
			TransferId: "tr-" + string(rune('a'+i)), FlowId: "flow_X",
			CreditedAtTsNs: 1_700_000_000_000_000_001, AssetId: leg.GetAssetId(), AssetCode: "USDT", AmountE18: leg.GetAmountE18(),
		})
	}
	return resp
}

// CreateBatch converts each leg exactly as Create does, sends every leg's own
// source and destination, and decodes the per-leg outcome.
func TestInternalTransfersCreateBatchSendsExactLegs(t *testing.T) {
	api := &contractAPI{batch: acceptAll}
	defaultSub := codecs.FormatID(9000000001)
	client := newContractClient(t, api, &defaultSub)
	scale := 6
	dest, otherSub, destAccount := "0x2222222222222222222222222222222222222222", codecs.FormatID(9000000002), codecs.FormatID(42)
	result, err := client.InternalTransfers.CreateBatch(context.Background(), []models.InternalTransferBatchLeg{
		{AssetID: 1, Quantity: models.AssetAmountFromDecimal("1844674407370.955161"), QuantityScale: &scale, DestinationSmartAccountAddress: &dest},
		{AssetID: 3, Quantity: models.AssetAmountFromDecimal("0.000001"), QuantityScale: &scale, SubAccountID: &otherSub, DestinationAccountID: &destAccount},
	}, "rebalance-2026-11-13-1")
	if err != nil {
		t.Fatal(err)
	}
	sent := api.sent(t)
	if len(sent) != 1 {
		t.Fatalf("requests = %d", len(sent))
	}
	req := sent[0].(*transferv1.BatchCreateInternalTransfersRequest)
	if req.GetIdempotencyKey() != "rebalance-2026-11-13-1" || len(req.GetLegs()) != 2 {
		t.Fatalf("request = %v", req)
	}
	first, second := req.GetLegs()[0], req.GetLegs()[1]
	if got := codecs.U128ToStr(first.GetAmountE18().GetHi(), first.GetAmountE18().GetLo()); got != "1844674407370955161000000000000" {
		t.Fatalf("leg 0 amount_e18 = %s", got)
	}
	if first.GetSubaccountId() != 9000000001 || first.GetDestinationSmartAccountAddress() != dest || first.GetAssetId() != 1 {
		t.Fatalf("leg 0 = %v; want the default sub-account as source", first)
	}
	if got := codecs.U128ToStr(second.GetAmountE18().GetHi(), second.GetAmountE18().GetLo()); got != "1000000000000" {
		t.Fatalf("leg 1 amount_e18 = %s", got)
	}
	if second.GetSubaccountId() != 9000000002 || second.GetDestinationAccountId() != 42 {
		t.Fatalf("leg 1 = %v; want its own source and destination", second)
	}
	if result.Status != "accepted" || result.RequestID != "92640625967207661803151833376978068587" || len(result.Legs) != 2 {
		t.Fatalf("result = %+v", result)
	}
	leg := result.Legs[0]
	if leg.Status != "credited" || leg.TransferID != "tr-a" || leg.FlowID != "flow_X" || leg.CreditedTsNs != 1_700_000_000_000_000_001 || leg.ErrorCode != "" {
		t.Fatalf("leg 0 = %+v", leg)
	}
	if text, err := leg.Quantity.Format(); err != nil || text != "1844674407370.955161" {
		t.Fatalf("leg 0 quantity = %q, %v", text, err)
	}
}

func TestInternalTransfersCreateBatchRefusesOutOfContractRequestsBeforeSending(t *testing.T) {
	api := &contractAPI{batch: acceptAll}
	client := newContractClient(t, api, nil)
	scale := 18
	dest := "0x2222222222222222222222222222222222222222"
	leg := models.InternalTransferBatchLeg{AssetID: 1, Quantity: models.AssetAmountFromDecimal("1"), QuantityScale: &scale, DestinationSmartAccountAddress: &dest}
	noDest, twoDest, noAsset, float := leg, leg, leg, leg
	noDest.DestinationSmartAccountAddress = nil
	other := codecs.FormatID(5)
	twoDest.DestinationAccountID = &other
	noAsset.AssetID = 0
	float.Quantity = models.AssetAmountFromDecimal("1e-3")
	cases := map[string]struct {
		legs []models.InternalTransferBatchLeg
		key  string
	}{
		"no legs":            {nil, "k"},
		"empty key":          {[]models.InternalTransferBatchLeg{leg}, ""},
		"key over 128 chars": {[]models.InternalTransferBatchLeg{leg}, strings.Repeat("k", 129)},
		"no destination":     {[]models.InternalTransferBatchLeg{leg, noDest}, "k"},
		"two destinations":   {[]models.InternalTransferBatchLeg{twoDest}, "k"},
		"no asset":           {[]models.InternalTransferBatchLeg{noAsset}, "k"},
		"inexact amount":     {[]models.InternalTransferBatchLeg{float}, "k"},
	}
	for name, tc := range cases {
		_, err := client.InternalTransfers.CreateBatch(context.Background(), tc.legs, tc.key)
		var validation *sdkerrors.ValidationError
		if !stderrors.As(err, &validation) {
			t.Fatalf("%s: err = %T %v, want ValidationError", name, err, err)
		}
	}
	if n := len(api.sent(t)); n != 0 {
		t.Fatalf("%d requests sent", n)
	}
	// The contract states no maximum: the API refuses more than 500 legs
	// itself, so 501 legs are sent.
	if codecs.BatchInternalTransferLegsMin != 1 || codecs.BatchInternalTransferLegsMax != math.MaxInt {
		t.Fatalf("contract legs = %d..%d", codecs.BatchInternalTransferLegsMin, codecs.BatchInternalTransferLegsMax)
	}
	many := make([]models.InternalTransferBatchLeg, 501)
	for i := range many {
		many[i] = leg
	}
	if _, err := client.InternalTransfers.CreateBatch(context.Background(), many, "k"); err != nil {
		t.Fatal(err)
	}
	if n := len(api.sent(t)); n != 1 {
		t.Fatalf("%d requests sent, want 1", n)
	}
}

func TestInternalTransfersCreateBatchFailsClosedOnAMalformedReply(t *testing.T) {
	scale := 18
	dest := "0x2222222222222222222222222222222222222222"
	legs := []models.InternalTransferBatchLeg{
		{AssetID: 1, Quantity: models.AssetAmountFromDecimal("1"), QuantityScale: &scale, DestinationSmartAccountAddress: &dest},
		{AssetID: 1, Quantity: models.AssetAmountFromDecimal("2"), QuantityScale: &scale, DestinationSmartAccountAddress: &dest},
	}
	breakages := map[string]func(*transferv1.BatchCreateInternalTransfersResponse){
		"unspecified status":       func(r *transferv1.BatchCreateInternalTransfersResponse) { r.Status = 0 },
		"unknown status":           func(r *transferv1.BatchCreateInternalTransfersResponse) { r.Status = 9 },
		"removed submitted status": func(r *transferv1.BatchCreateInternalTransfersResponse) { r.Status = 1 },
		"accepted without request": func(r *transferv1.BatchCreateInternalTransfersResponse) { r.RequestId = "" },
		"processing without request": func(r *transferv1.BatchCreateInternalTransfersResponse) {
			r.Status, r.RequestId = transferv1.InternalTransferBatchStatus_BATCH_PROCESSING, ""
			for _, leg := range r.Legs {
				leg.Status = transferv1.InternalTransferBatchLegStatus_LEG_PROCESSING
			}
		},
		"missing leg": func(r *transferv1.BatchCreateInternalTransfersResponse) { r.Legs = r.Legs[:1] },
		"legs out of order": func(r *transferv1.BatchCreateInternalTransfersResponse) {
			r.Legs[0].LegIndex, r.Legs[1].LegIndex = 1, 0
		},
		"credited without transfer": func(r *transferv1.BatchCreateInternalTransfersResponse) { r.Legs[1].TransferId = "" },
		"credited without flow":     func(r *transferv1.BatchCreateInternalTransfersResponse) { r.Legs[0].FlowId = "" },
		"processing leg in an accepted batch": func(r *transferv1.BatchCreateInternalTransfersResponse) {
			r.Legs[1].Status = transferv1.InternalTransferBatchLegStatus_LEG_PROCESSING
		},
		"processing without transfer": func(r *transferv1.BatchCreateInternalTransfersResponse) {
			r.Status = transferv1.InternalTransferBatchStatus_BATCH_PROCESSING
			for _, leg := range r.Legs {
				leg.Status = transferv1.InternalTransferBatchLegStatus_LEG_PROCESSING
			}
			r.Legs[0].TransferId = ""
		},
		"credited leg in a refused batch": func(r *transferv1.BatchCreateInternalTransfersResponse) {
			r.Status = transferv1.InternalTransferBatchStatus_BATCH_REFUSED
			r.Legs[0].Status = transferv1.InternalTransferBatchLegStatus_LEG_NOT_MOVED
		},
		"removed leg status":     func(r *transferv1.BatchCreateInternalTransfersResponse) { r.Legs[1].Status = 1 },
		"unspecified leg status": func(r *transferv1.BatchCreateInternalTransfersResponse) { r.Legs[1].Status = 0 },
	}
	for name, breakReply := range breakages {
		api := &contractAPI{batch: func(req *transferv1.BatchCreateInternalTransfersRequest) *transferv1.BatchCreateInternalTransfersResponse {
			resp := acceptAll(req)
			breakReply(resp)
			return resp
		}}
		client := newContractClient(t, api, nil)
		_, err := client.InternalTransfers.CreateBatch(context.Background(), legs, "k")
		var contractErr *sdkerrors.ResponseContractError
		if !stderrors.As(err, &contractErr) {
			t.Fatalf("%s: err = %T %v, want ResponseContractError", name, err, err)
		}
	}
	// A refused batch may have no request ID; nothing moved, and the leg
	// that caused it says why.
	api := &contractAPI{batch: func(req *transferv1.BatchCreateInternalTransfersRequest) *transferv1.BatchCreateInternalTransfersResponse {
		return &transferv1.BatchCreateInternalTransfersResponse{Status: transferv1.InternalTransferBatchStatus_BATCH_REFUSED, Legs: []*transferv1.InternalTransferBatchLegResult{
			{LegIndex: 0, Status: transferv1.InternalTransferBatchLegStatus_LEG_NOT_MOVED},
			{LegIndex: 1, Status: transferv1.InternalTransferBatchLegStatus_LEG_NOT_MOVED, ErrorCode: transferv1.ErrorCode_ERROR_CODE_INSUFFICIENT_FUNDS, ErrorMessage: "insufficient funds"},
		}}
	}}
	result, err := newContractClient(t, api, nil).InternalTransfers.CreateBatch(context.Background(), legs, "k")
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "refused" || result.Legs[0].Status != "not_moved" || result.Legs[0].ErrorCode != "" || result.Legs[1].Status != "not_moved" || result.Legs[1].ErrorCode != "insufficient_funds" {
		t.Fatalf("result = %+v", result)
	}
	// A processing batch carries its request ID and every leg's transfer ID.
	api = &contractAPI{batch: func(req *transferv1.BatchCreateInternalTransfersRequest) *transferv1.BatchCreateInternalTransfersResponse {
		resp := acceptAll(req)
		resp.Status = transferv1.InternalTransferBatchStatus_BATCH_PROCESSING
		for _, leg := range resp.Legs {
			leg.Status, leg.FlowId, leg.CreditedAtTsNs = transferv1.InternalTransferBatchLegStatus_LEG_PROCESSING, "", 0
		}
		return resp
	}}
	result, err = newContractClient(t, api, nil).InternalTransfers.CreateBatch(context.Background(), legs, "k")
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "processing" || result.Legs[1].Status != "processing" || result.Legs[1].TransferID != "tr-b" {
		t.Fatalf("result = %+v", result)
	}
}

// GetBatch reads a batch by its request ID and checks the reply as CreateBatch
// does, refusing a request ID the contract does not take before sending.
func TestInternalTransfersGetBatch(t *testing.T) {
	const requestID = "92640625967207661803151833376978068587"
	api := &contractAPI{batchRead: &transferv1.GetInternalTransferBatchResponse{
		RequestId: requestID, Status: transferv1.InternalTransferBatchStatus_BATCH_ACCEPTED,
		Legs: []*transferv1.InternalTransferBatchLegResult{{
			LegIndex: 0, Status: transferv1.InternalTransferBatchLegStatus_LEG_CREDITED, TransferId: "tr-a", FlowId: "flow_A",
			CreditedAtTsNs: 7, AssetId: 7, AssetCode: "BTC",
		}},
	}}
	client := newContractClient(t, api, nil)
	result, err := client.InternalTransfers.GetBatch(context.Background(), requestID)
	if err != nil {
		t.Fatal(err)
	}
	if result.RequestID != requestID || result.Status != "accepted" || len(result.Legs) != 1 || result.Legs[0].FlowID != "flow_A" {
		t.Fatalf("result = %+v", result)
	}
	sent := api.sent(t)
	if len(sent) != 1 || sent[0].(*transferv1.GetInternalTransferBatchRequest).GetRequestId() != requestID || api.unsigned != 0 {
		t.Fatalf("sent = %v unsigned=%d", sent, api.unsigned)
	}
	for _, id := range []string{"", "0", "0123", "12a", strings.Repeat("9", 40)} {
		var validation *sdkerrors.ValidationError
		if _, err := client.InternalTransfers.GetBatch(context.Background(), id); !stderrors.As(err, &validation) {
			t.Fatalf("GetBatch(%q) err = %v, want a ValidationError", id, err)
		}
	}
	// A processing batch's legs have no flow yet.
	processing := proto.Clone(api.batchRead).(*transferv1.GetInternalTransferBatchResponse)
	processing.Status = transferv1.InternalTransferBatchStatus_BATCH_PROCESSING
	processing.Legs[0].Status, processing.Legs[0].FlowId, processing.Legs[0].CreditedAtTsNs = transferv1.InternalTransferBatchLegStatus_LEG_PROCESSING, "", 0
	if got, err := newContractClient(t, &contractAPI{batchRead: processing}, nil).InternalTransfers.GetBatch(context.Background(), requestID); err != nil || got.Status != "processing" || got.Legs[0].FlowID != "" {
		t.Fatalf("processing batch = %+v, %v", got, err)
	}
	// A reply naming another batch, holding no leg, or with a credited leg
	// without its flow ID breaks the contract.
	for name, breakReply := range map[string]func(*transferv1.GetInternalTransferBatchResponse){
		"other batch":           func(r *transferv1.GetInternalTransferBatchResponse) { r.RequestId = "1" },
		"no legs":               func(r *transferv1.GetInternalTransferBatchResponse) { r.Legs = nil },
		"credited without flow": func(r *transferv1.GetInternalTransferBatchResponse) { r.Legs[0].FlowId = "" },
	} {
		reply := proto.Clone(api.batchRead).(*transferv1.GetInternalTransferBatchResponse)
		breakReply(reply)
		bad := &contractAPI{batchRead: reply}
		var contractErr *sdkerrors.ResponseContractError
		if _, err := newContractClient(t, bad, nil).InternalTransfers.GetBatch(context.Background(), requestID); !stderrors.As(err, &contractErr) {
			t.Fatalf("%s: err = %v, want ResponseContractError", name, err)
		}
	}
}

func TestStatementExportsCreateGetAndList(t *testing.T) {
	sub := uint64(9000000001)
	api := &contractAPI{export: &ledgerrdv1.StatementExport{
		ExportId: 77, SubaccountId: &sub, IdempotencyKey: "statement-2026", Status: ledgerrdv1.StatementExportStatus_STATEMENT_EXPORT_STATUS_READY,
		Movements: 1 << 40, Bytes: 1<<63 + 5, ReadyAt: timestamppb.New(time.Unix(100, 5)), ExpiresAt: timestamppb.New(time.Unix(200, 0)),
		CompleteThrough: timestamppb.New(time.Unix(50, 123456789)),
	}}
	defaultSub := codecs.FormatID(sub)
	client := newContractClient(t, api, &defaultSub)
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(codecs.StatementExportMaxPeriod)
	export, err := client.Statements.CreateExport(context.Background(), services.CreateStatementExportOptions{StartTime: start, EndTime: end, AssetID: 3, IdempotencyKey: "statement-2026"})
	if err != nil {
		t.Fatal(err)
	}
	if export.ExportID != codecs.FormatID(77) || export.SubaccountID != defaultSub || export.Status != "ready" || export.Bytes != 1<<63+5 ||
		export.CompleteThrough == nil || !export.CompleteThrough.Equal(time.Unix(50, 123456789)) || export.FailureReason != "" {
		t.Fatalf("export = %+v", export)
	}
	if _, err := client.Statements.GetExport(context.Background(), export.ExportID); err != nil {
		t.Fatal(err)
	}
	var listed int
	for e, err := range client.Statements.Exports(context.Background(), services.ListStatementExportsOptions{Limit: 100}) {
		if err != nil {
			t.Fatal(err)
		}
		if e.ExportID != export.ExportID {
			t.Fatalf("listed %+v", e)
		}
		listed++
	}
	if listed != 2 {
		t.Fatalf("listed %d exports over two pages", listed)
	}
	sent := api.sent(t)
	create := sent[0].(*ledgerrdv1.CreateStatementExportRequest)
	if create.GetSubaccountId() != sub || !create.GetEndTime().AsTime().Equal(end) || create.GetAssetId() != 3 || create.GetIdempotencyKey() != "statement-2026" {
		t.Fatalf("create = %v", create)
	}
	if get := sent[1].(*ledgerrdv1.GetStatementExportRequest); get.GetExportId() != 77 {
		t.Fatalf("get = %v", get)
	}
	if list := sent[3].(*ledgerrdv1.ListStatementExportsRequest); list.GetPageToken() != "p2" || list.GetLimit() != 100 || list.GetSubaccountId() != sub {
		t.Fatalf("second list = %v", list)
	}
}

func TestStatementExportsRefuseOutOfContractRequestsBeforeSending(t *testing.T) {
	api := &contractAPI{}
	client := newContractClient(t, api, nil)
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	zeroSub := "1"
	for name, opts := range map[string]services.CreateStatementExportOptions{
		"no key":              {StartTime: start, EndTime: start.Add(time.Hour)},
		"key with a space":    {StartTime: start, EndTime: start.Add(time.Hour), IdempotencyKey: "statement 2026"},
		"key with a slash":    {StartTime: start, EndTime: start.Add(time.Hour), IdempotencyKey: "statement/2026"},
		"key over 64 chars":   {StartTime: start, EndTime: start.Add(time.Hour), IdempotencyKey: strings.Repeat("k", 65)},
		"empty period":        {StartTime: start, EndTime: start, IdempotencyKey: "k"},
		"end before start":    {StartTime: start, EndTime: start.Add(-time.Hour), IdempotencyKey: "k"},
		"period over 366 d":   {StartTime: start, EndTime: start.Add(codecs.StatementExportMaxPeriod + time.Nanosecond), IdempotencyKey: "k"},
		"no start":            {EndTime: start, IdempotencyKey: "k"},
		"zero sub-account ID": {StartTime: start, EndTime: start.Add(time.Hour), IdempotencyKey: "k", SubAccountID: &zeroSub},
	} {
		if _, err := client.Statements.CreateExport(context.Background(), opts); !isValidation(err) {
			t.Fatalf("%s: err = %T %v", name, err, err)
		}
	}
	if _, err := client.Statements.ListExports(context.Background(), services.ListStatementExportsOptions{Limit: 101}); !isValidation(err) {
		t.Fatalf("limit 101: %v", err)
	}
	if _, err := client.Statements.GetExport(context.Background(), ""); !isValidation(err) {
		t.Fatalf("empty export ID: %v", err)
	}
	if n := len(api.sent(t)); n != 0 {
		t.Fatalf("%d requests sent", n)
	}
	if codecs.ListStatementExportsLimitMax != 100 {
		t.Fatalf("contract limit = %d", codecs.ListStatementExportsLimitMax)
	}
}

func TestStatementExportsRefuseRepliesWithoutTheExport(t *testing.T) {
	for name, export := range map[string]*ledgerrdv1.StatementExport{
		"no export":        nil,
		"zero export ID":   {Status: ledgerrdv1.StatementExportStatus_STATEMENT_EXPORT_STATUS_QUEUED},
		"zero sub-account": {ExportId: 1, SubaccountId: proto.Uint64(0)},
	} {
		client := newContractClient(t, &contractAPI{export: export}, nil)
		_, err := client.Statements.GetExport(context.Background(), codecs.FormatID(5))
		var contractErr *sdkerrors.ResponseContractError
		if !stderrors.As(err, &contractErr) {
			t.Fatalf("%s: err = %T %v", name, err, err)
		}
	}
}

func TestDustConversionsSendSignedScopedRequests(t *testing.T) {
	api := &contractAPI{conversion: &transferv1.DustConversion{
		ConversionId: "12345", QuoteId: "dq_1", State: transferv1.DustConversionState_DUST_CONVERSION_STATE_COMPLETED, TargetAssetId: 3,
		PayoutE18: &typev1.U128{Lo: 5390000000000000000}, FeeE18: &typev1.U128{Lo: 110000000000000000}, FeeBps: 200,
	}}
	defaultSub := codecs.FormatID(9000000001)
	client := newContractClient(t, api, &defaultSub)
	conv, err := client.DustConversions.Convert(context.Background(), "dq_1", "dust-2026-11-14-1", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if conv.State != "completed" || conv.ConversionID != "12345" {
		t.Fatalf("conversion = %+v", conv)
	}
	if text, _ := conv.Payout.Format(); text != "5.39" {
		t.Fatalf("payout = %s", text)
	}
	if _, err := client.DustConversions.ListBalances(context.Background(), 0, "main", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := client.DustConversions.List(context.Background(), services.ListDustConversionsOptions{Limit: 100}); err != nil {
		t.Fatal(err)
	}
	sent := api.sent(t)
	if create := sent[0].(*transferv1.CreateDustConversionRequest); create.GetSubaccountId() != 9000000001 || create.GetQuoteId() != "dq_1" || create.GetIdempotencyKey() != "dust-2026-11-14-1" {
		t.Fatalf("create = %v", create)
	}
	if list := sent[1].(*transferv1.ListDustBalancesRequest); list.GetSubaccountId() != 0 || list.GetTargetAssetId() != 0 {
		t.Fatalf("balances = %v; want the main account and the first target", list)
	}
}

func TestDustConversionsRefuseOutOfContractRequestsBeforeSending(t *testing.T) {
	api := &contractAPI{}
	client := newContractClient(t, api, nil)
	ctx := context.Background()
	checks := map[string]error{}
	_, checks["quote without target"] = client.DustConversions.Quote(ctx, 0, nil, nil)
	_, checks["convert without quote"] = client.DustConversions.Convert(ctx, "", "k", nil, nil)
	_, checks["convert with a long quote ID"] = client.DustConversions.Convert(ctx, strings.Repeat("q", 65), "k", nil, nil)
	_, checks["convert without key"] = client.DustConversions.Convert(ctx, "dq_1", "", nil, nil)
	_, checks["convert with a long key"] = client.DustConversions.Convert(ctx, "dq_1", strings.Repeat("k", 129), nil, nil)
	_, checks["get a non-numeric conversion"] = client.DustConversions.Get(ctx, "conv-1", nil, nil)
	_, checks["get conversion 0"] = client.DustConversions.Get(ctx, "0123", nil, nil)
	_, checks["get a 40-digit conversion"] = client.DustConversions.Get(ctx, "1"+strings.Repeat("0", 39), nil, nil)
	_, checks["list 101"] = client.DustConversions.List(ctx, services.ListDustConversionsOptions{Limit: 101})
	for name, err := range checks {
		if !isValidation(err) {
			t.Fatalf("%s: err = %T %v", name, err, err)
		}
	}
	if n := len(api.sent(t)); n != 0 {
		t.Fatalf("%d requests sent", n)
	}
}

func TestDustConversionsRefuseAReplyWithoutTheConversion(t *testing.T) {
	for name, conv := range map[string]*transferv1.DustConversion{"none": nil, "no ID": {QuoteId: "dq_1"}} {
		client := newContractClient(t, &contractAPI{conversion: conv}, nil)
		_, err := client.DustConversions.Convert(context.Background(), "dq_1", "k", nil, nil)
		var contractErr *sdkerrors.ResponseContractError
		if !stderrors.As(err, &contractErr) {
			t.Fatalf("%s: err = %T %v", name, err, err)
		}
	}
}

func TestLifecycleAccountFlowsAreSignedScopedAndPaged(t *testing.T) {
	api := &contractAPI{flow: &lifecyclev1.FlowDetailView{Summary: &lifecyclev1.FlowSummaryView{
		FlowId: "flow_W", FlowKind: lifecyclev1.FlowKind_KIND_DEPOSIT, IsTerminal: true,
		LifecycleReason: lifecyclev1.LifecycleReason_ZIPPER_DEPOSIT_DELIVERED_TO_WALLET, Participation: lifecyclev1.FlowParticipation_PARTICIPATION_OWNER,
	}}}
	defaultSub := codecs.FormatID(9000000001)
	client := newContractClient(t, api, &defaultSub)
	scope, participation := "terminal_only", "recipient"
	var ids []string
	for flow, err := range client.Lifecycle.AccountFlows(context.Background(), services.ListAccountFlowsParams{Limit: 500, Scope: &scope, Participation: &participation, Reversed: true}) {
		if err != nil {
			t.Fatal(err)
		}
		if flow.Participation != "recipient" {
			t.Fatalf("flow = %+v", flow)
		}
		ids = append(ids, flow.IntentID)
	}
	if strings.Join(ids, ",") != "flow_A,flow_BA" {
		t.Fatalf("flows = %v", ids)
	}
	flow, err := client.Lifecycle.GetAccountFlow(context.Background(), "flow_W", "main", nil)
	if err != nil {
		t.Fatal(err)
	}
	if flow.LifecycleReason != "zipper_deposit_delivered_to_wallet" || !flow.IsTerminal {
		t.Fatalf("flow = %+v", flow)
	}
	sent := api.sent(t)
	for i, msg := range sent[:2] {
		req := msg.(*lifecyclev1.ListAccountFlowsRequest)
		if req.GetSubaccountId() != 9000000001 || req.GetLimit() != 500 || req.GetScope() != lifecyclev1.ListScope_LIST_TERMINAL_ONLY ||
			req.GetParticipation() != lifecyclev1.FlowParticipation_PARTICIPATION_RECIPIENT || req.GetSort() != lifecyclev1.Sort_SORT_OLDEST {
			t.Fatalf("list %d = %v", i, req)
		}
	}
	if get := sent[2].(*lifecyclev1.GetAccountFlowRequest); get.SubaccountId != nil || get.GetFlowId() != "flow_W" {
		t.Fatalf("get = %v; want the main account", get)
	}
	if _, err := client.Lifecycle.ListAccountFlows(context.Background(), services.ListAccountFlowsParams{Limit: 501}); !isValidation(err) {
		t.Fatalf("limit 501: %v", err)
	}
	if _, err := client.Lifecycle.GetAccountFlow(context.Background(), "", nil, nil); !isValidation(err) {
		t.Fatalf("empty flow ID: %v", err)
	}
	if n := len(api.sent(t)); n != 3 {
		t.Fatalf("refused calls were sent: %d requests", n)
	}
	for name, detail := range map[string]*lifecyclev1.FlowDetailView{"no flow": nil, "no summary": {}, "no flow ID": {Summary: &lifecyclev1.FlowSummaryView{}}} {
		_, err := newContractClient(t, &contractAPI{flow: detail}, nil).Lifecycle.GetAccountFlow(context.Background(), "flow_W", nil, nil)
		var contractErr *sdkerrors.ResponseContractError
		if !stderrors.As(err, &contractErr) {
			t.Fatalf("%s: err = %T %v", name, err, err)
		}
	}
}

func isValidation(err error) bool {
	var validation *sdkerrors.ValidationError
	return stderrors.As(err, &validation)
}

// A batch leg reads the destination the server resolved for it, all three of
// its fields, and nil when the reply carries none.
func TestInternalTransfersCreateBatchReadsEachLegsResolvedDestination(t *testing.T) {
	api := &contractAPI{batch: func(req *transferv1.BatchCreateInternalTransfersRequest) *transferv1.BatchCreateInternalTransfersResponse {
		resp := acceptAll(req)
		resp.Legs[0].Destination = &transferv1.ResolvedDestination{
			RootAccountPublicId: "AAo8yKqygTg", SubaccountPublicId: "AAo8yKqygTh", SmartAccountAddress: "0x2222222222222222222222222222222222222222",
		}
		resp.Legs[1].Destination = nil
		return resp
	}}
	client := newContractClient(t, api, nil)
	scale := 18
	dest := "0x2222222222222222222222222222222222222222"
	leg := models.InternalTransferBatchLeg{AssetID: 1, Quantity: models.AssetAmountFromDecimal("1"), QuantityScale: &scale, DestinationSmartAccountAddress: &dest}
	result, err := client.InternalTransfers.CreateBatch(context.Background(), []models.InternalTransferBatchLeg{leg, leg}, "k")
	if err != nil {
		t.Fatal(err)
	}
	got := result.Legs[0].Destination
	if got == nil || got.RootAccountPublicID != "AAo8yKqygTg" || got.SubaccountPublicID != "AAo8yKqygTh" || got.SmartAccountAddress != dest {
		t.Fatalf("leg 0 destination = %+v", got)
	}
	if result.Legs[1].Destination != nil {
		t.Fatalf("leg 1 destination = %+v, want nil without one", result.Legs[1].Destination)
	}
}

// ListAccountFlows sends every filter the contract has, by label, and
// refuses an ID filter the contract refuses before sending.
func TestLifecycleListAccountFlowsSendsEveryFilter(t *testing.T) {
	api := &contractAPI{}
	client := newContractClient(t, api, nil)
	kind, state, orderBy, tx := "withdraw", "PENDING_LEDGER", "started_at", "0xabc"
	_, err := client.Lifecycle.ListAccountFlows(context.Background(), services.ListAccountFlowsParams{
		FlowKind: &kind, FlowState: &state, OrderBy: &orderBy, TxRef: &tx,
		PolyesterChainIDs: []uint32{7}, ZippedAssetIDs: []uint32{3, 4}, UnifiedAssetIDs: []uint32{1},
	})
	if err != nil {
		t.Fatal(err)
	}
	req := api.sent(t)[0].(*lifecyclev1.ListAccountFlowsRequest)
	if req.GetFlowKind() != lifecyclev1.FlowKind_KIND_WITHDRAW || req.GetFlowState() != lifecyclev1.FlowState_STATE_PENDING_LEDGER ||
		req.GetOrderBy() != lifecyclev1.ListOrderBy_ORDER_BY_STARTED_AT || req.GetTxRef() != tx ||
		len(req.GetPolyesterChainIds()) != 1 || req.GetPolyesterChainIds()[0] != 7 || len(req.GetZippedAssetIds()) != 2 || req.GetUnifiedAssetIds()[0] != 1 {
		t.Fatalf("request = %v", req)
	}
	tooMany := make([]uint32, codecs.ListAccountFlowsFilterIDsMax+1)
	for i := range tooMany {
		tooMany[i] = uint32(i + 1)
	}
	bad, unspecified := "sideways", "unspecified"
	for name, params := range map[string]services.ListAccountFlowsParams{
		"unknown kind":       {FlowKind: &bad},
		"unknown state":      {FlowState: &bad},
		"unknown order":      {OrderBy: &bad},
		"unspecified kind":   {FlowKind: &unspecified},
		"zero chain ID":      {PolyesterChainIDs: []uint32{0}},
		"repeated asset ID":  {ZippedAssetIDs: []uint32{3, 3}},
		"101 unified assets": {UnifiedAssetIDs: tooMany},
	} {
		if _, err := client.Lifecycle.ListAccountFlows(context.Background(), params); !isValidation(err) {
			t.Fatalf("%s: err = %T %v", name, err, err)
		}
	}
	if n := len(api.sent(t)); n != 1 {
		t.Fatalf("refused filters were sent: %d requests", n)
	}
	if codecs.ListAccountFlowsFilterIDsMax != 100 {
		t.Fatalf("contract ID filter maximum = %d", codecs.ListAccountFlowsFilterIDsMax)
	}
}
