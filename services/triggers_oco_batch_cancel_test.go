package services

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"connectrpc.com/connect"
	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	orderv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/orders/v1"
	ratelimitv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/polyester/ratelimit/v1"
	triggersv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/triggers/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/gen/triggers/v1/triggersv1connect"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
	"google.golang.org/protobuf/proto"
)

// ocoHandler records OCO and batch trigger cancel requests and answers each
// with its scripted response or error.
type ocoHandler struct {
	triggersv1connect.UnimplementedTriggersServiceHandler
	mu          sync.Mutex
	creates     []*triggersv1.CreateOcoRequest
	cancels     []*triggersv1.CancelOcoRequest
	batches     []*triggersv1.BatchCancelTriggersRequest
	lists       []*triggersv1.ListTriggersRequest
	createErr   error
	cancelErr   error
	batchResp   *triggersv1.BatchCancelTriggersResponse
	listTrigger *triggersv1.Trigger
}

func (h *ocoHandler) CreateOco(_ context.Context, req *connect.Request[triggersv1.CreateOcoRequest]) (*connect.Response[triggersv1.CreateOcoResponse], error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.creates = append(h.creates, req.Msg)
	if h.createErr != nil {
		return nil, h.createErr
	}
	return connect.NewResponse(&triggersv1.CreateOcoResponse{
		OrderListId:         77,
		ClientListId:        req.Msg.GetOco().GetClientListId(),
		TakeProfitTriggerId: 78,
		StopLossTriggerId:   79,
		AcceptedAtTsNs:      1_700_000_000_000_000_123,
	}), nil
}

func (h *ocoHandler) CancelOco(_ context.Context, req *connect.Request[triggersv1.CancelOcoRequest]) (*connect.Response[triggersv1.CancelOcoResponse], error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.cancels = append(h.cancels, req.Msg)
	if h.cancelErr != nil {
		return nil, h.cancelErr
	}
	return connect.NewResponse(&triggersv1.CancelOcoResponse{OrderListId: req.Msg.GetOrderListId(), Status: triggersv1.TriggerStatus_STATUS_CANCELED}), nil
}

func (h *ocoHandler) BatchCancelTriggers(_ context.Context, req *connect.Request[triggersv1.BatchCancelTriggersRequest]) (*connect.Response[triggersv1.BatchCancelTriggersResponse], error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.batches = append(h.batches, req.Msg)
	return connect.NewResponse(h.batchResp), nil
}

func (h *ocoHandler) ListTriggers(_ context.Context, req *connect.Request[triggersv1.ListTriggersRequest]) (*connect.Response[triggersv1.ListTriggersResponse], error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.lists = append(h.lists, req.Msg)
	resp := &triggersv1.ListTriggersResponse{}
	if h.listTrigger != nil {
		resp.Triggers = []*triggersv1.Trigger{h.listTrigger}
	}
	return connect.NewResponse(resp), nil
}

func sellOco(clientListID *string) models.CreateOcoInput {
	limit := models.PriceFromDecimal("70500")
	stp := "expire_both"
	return models.CreateOcoInput{
		Symbol: "BTC-USDT",
		Side:   "sell",
		Qty:    models.QtyFromDecimal("0.25"),
		TakeProfit: models.RiskLegInput{
			TriggerPrice: models.PriceFromDecimal("71000"),
			LimitPrice:   &limit,
		},
		StopLoss: models.RiskLegInput{
			TriggerPrice:       models.PriceFromDecimal("65000"),
			TriggerPriceSource: "index",
		},
		SelfTradePreventionMode: &stp,
		ClientListID:            clientListID,
	}
}

func TestCreateOcoSendsBothLegsAndReadsTheOco(t *testing.T) {
	handler := &ocoHandler{}
	service := newTriggerControlServiceWith(t, handler)
	clientListID := "protect-btc-1"
	sub := codecs.FormatID(42)

	got, err := service.CreateOco(context.Background(), nil, sellOco(&clientListID), &sub)
	if err != nil {
		t.Fatalf("create OCO: %v", err)
	}
	want := models.CreateOcoResult{
		OrderListID:         codecs.FormatID(77),
		ClientListID:        clientListID,
		TakeProfitTriggerID: codecs.FormatID(78),
		StopLossTriggerID:   codecs.FormatID(79),
		AcceptedTsNs:        1_700_000_000_000_000_123,
	}
	if got != want {
		t.Fatalf("result = %+v, want %+v", got, want)
	}
	if len(handler.creates) != 1 {
		t.Fatalf("creates = %d, want 1", len(handler.creates))
	}
	subID := uint64(42)
	wantReq := &triggersv1.CreateOcoRequest{
		SubaccountId: &subID,
		Oco: &triggersv1.OcoIntent{
			SymbolId:                5,
			Side:                    orderv1.Side_SELL,
			QtyScaled:               25_000_000,
			SelfTradePreventionMode: orderv1.SelfTradePreventionMode_EXPIRE_BOTH,
			ClientListId:            clientListID,
			TakeProfit: &triggersv1.OcoLeg{
				TriggerPriceTicks: 71_000_000_000_000,
				Child:             &orderv1.RiskExecution{Execution: &orderv1.RiskExecution_LimitGtc{LimitGtc: &orderv1.RiskLimitGtc{PriceTicks: 70_500_000_000_000}}},
			},
			StopLoss: &triggersv1.OcoLeg{
				TriggerPriceTicks:  65_000_000_000_000,
				Child:              &orderv1.RiskExecution{Execution: &orderv1.RiskExecution_MarketIoc{MarketIoc: &orderv1.RiskMarketIoc{}}},
				TriggerPriceSource: orderv1.TriggerPriceSource_INDEX_PRICE,
			},
		},
	}
	if !proto.Equal(handler.creates[0], wantReq) {
		t.Fatalf("request = %v, want %v", handler.creates[0], wantReq)
	}
}

// A create whose outcome is unknown is sent again with the same client OCO
// ID, which makes the retry return the same OCO: the SDK sends the identical
// request, and never retries a create on its own.
func TestCreateOcoRetryWithTheSameClientListIDSendsTheSameRequest(t *testing.T) {
	handler := &ocoHandler{createErr: connect.NewError(connect.CodeUnavailable, errors.New("busy"))}
	service := newTriggerControlServiceWith(t, handler)
	clientListID := "protect-btc-2"

	if _, err := service.CreateOco(context.Background(), nil, sellOco(&clientListID), nil); err == nil {
		t.Fatal("create OCO: want the unavailable error")
	}
	if len(handler.creates) != 1 {
		t.Fatalf("creates after one failed call = %d, want 1: the SDK must not resend a create itself", len(handler.creates))
	}
	handler.createErr = nil
	if _, err := service.CreateOco(context.Background(), nil, sellOco(&clientListID), nil); err != nil {
		t.Fatalf("retried create OCO: %v", err)
	}
	if !proto.Equal(handler.creates[0], handler.creates[1]) {
		t.Fatalf("retry = %v, want the first request %v", handler.creates[1], handler.creates[0])
	}
}

func TestCreateOcoGeneratesAClientListIDWhenNoneIsGiven(t *testing.T) {
	handler := &ocoHandler{}
	service := newTriggerControlServiceWith(t, handler)
	empty := ""
	for _, id := range []*string{nil, &empty} {
		got, err := service.CreateOco(context.Background(), nil, sellOco(id), nil)
		if err != nil {
			t.Fatalf("create OCO: %v", err)
		}
		if got.ClientListID == "" || got.ClientListID != handler.creates[len(handler.creates)-1].GetOco().GetClientListId() {
			t.Fatalf("client list ID = %q, want the generated one sent", got.ClientListID)
		}
	}
	if handler.creates[0].GetOco().GetClientListId() == handler.creates[1].GetOco().GetClientListId() {
		t.Fatal("two creates without a client list ID sent the same generated ID")
	}
}

// A client OCO ID is checked as the contract checks it, 1 to 36 bytes of
// UTF-8, and sent unchanged.
func TestCreateOcoSendsAnyClientListIDOfUpTo36Bytes(t *testing.T) {
	for _, id := range []string{"oco énergie #1 ✓", strings.Repeat("é", 18), " padded "} {
		handler := &ocoHandler{}
		service := newTriggerControlServiceWith(t, handler)
		got, err := service.CreateOco(context.Background(), nil, sellOco(&id), nil)
		if err != nil {
			t.Fatalf("client list ID %q: %v", id, err)
		}
		if sent := handler.creates[0].GetOco().GetClientListId(); sent != id || got.ClientListID != id {
			t.Fatalf("client list ID %q: sent %q, read %q", id, sent, got.ClientListID)
		}
	}
	for _, id := range []string{strings.Repeat("é", 19), "\xff\xfe"} {
		handler := &ocoHandler{}
		service := newTriggerControlServiceWith(t, handler)
		_, err := service.CreateOco(context.Background(), nil, sellOco(&id), nil)
		var validation *sdkerrors.ValidationError
		if !errors.As(err, &validation) || len(handler.creates) != 0 {
			t.Fatalf("client list ID %q: err = %T %v, sent %d, want refused before sending", id, err, err, len(handler.creates))
		}
	}
}

func TestCreateOcoRefusesAnInvalidOcoBeforeSending(t *testing.T) {
	cases := map[string]func(*models.CreateOcoInput){
		"no side":              func(in *models.CreateOcoInput) { in.Side = "" },
		"unknown symbol":       func(in *models.CreateOcoInput) { in.Symbol = "DOGE-USDT" },
		"zero quantity":        func(in *models.CreateOcoInput) { in.Qty = models.QtyFromDecimal("0") },
		"no take-profit price": func(in *models.CreateOcoInput) { in.TakeProfit.TriggerPrice = models.PriceInput{} },
		"no stop-loss price":   func(in *models.CreateOcoInput) { in.StopLoss.TriggerPrice = models.PriceInput{} },
		"unknown price source": func(in *models.CreateOcoInput) { in.StopLoss.TriggerPriceSource = "bid" },
		"unknown self-trade mode": func(in *models.CreateOcoInput) {
			mode := "decrease"
			in.SelfTradePreventionMode = &mode
		},
		"client list ID over 36 bytes": func(in *models.CreateOcoInput) {
			id := strings.Repeat("a", 37)
			in.ClientListID = &id
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			handler := &ocoHandler{}
			service := newTriggerControlServiceWith(t, handler)
			in := sellOco(nil)
			mutate(&in)
			_, err := service.CreateOco(context.Background(), nil, in, nil)
			var validation *sdkerrors.ValidationError
			if !errors.As(err, &validation) {
				t.Fatalf("err = %T %v, want a validation error", err, err)
			}
			if len(handler.creates) != 0 {
				t.Fatalf("sent %d creates, want none", len(handler.creates))
			}
		})
	}
}

func TestCancelOcoSendsTheOcoIDAndReadsBothLegsCanceled(t *testing.T) {
	handler := &ocoHandler{}
	service := newTriggerControlServiceWith(t, handler)
	sub := codecs.FormatID(42)

	got, err := service.CancelOco(context.Background(), nil, codecs.FormatID(77), &sub)
	if err != nil {
		t.Fatalf("cancel OCO: %v", err)
	}
	if want := (models.CancelOcoResult{OrderListID: codecs.FormatID(77), Status: "canceled"}); got != want {
		t.Fatalf("result = %+v, want %+v", got, want)
	}
	subID := uint64(42)
	if want := (&triggersv1.CancelOcoRequest{OrderListId: 77, SubaccountId: &subID}); !proto.Equal(handler.cancels[0], want) {
		t.Fatalf("request = %v, want %v", handler.cancels[0], want)
	}
}

func TestCancelOcoRefusesAMissingOcoIDBeforeSending(t *testing.T) {
	for _, id := range []string{"", "0", "not-an-id!"} {
		handler := &ocoHandler{}
		service := newTriggerControlServiceWith(t, handler)
		_, err := service.CancelOco(context.Background(), nil, id, nil)
		var validation *sdkerrors.ValidationError
		if !errors.As(err, &validation) || len(handler.cancels) != 0 {
			t.Fatalf("id %q: err = %T %v, sent %d, want a validation error and nothing sent", id, err, err, len(handler.cancels))
		}
	}
}

func TestCancelOcoOfANonOcoIDKeepsTheContractCode(t *testing.T) {
	refusal := connect.NewError(connect.CodeNotFound, errors.New("trigger not found"))
	detail, err := connect.NewErrorDetail(&orderv1.ErrorDetail{Code: orderv1.ErrorCode_ERROR_CODE_TRIGGER_NOT_FOUND})
	if err != nil {
		t.Fatal(err)
	}
	refusal.AddDetail(detail)
	handler := &ocoHandler{cancelErr: refusal}
	service := newTriggerControlServiceWith(t, handler)

	_, err = service.CancelOco(context.Background(), nil, codecs.FormatID(78), nil)
	var apiErr *sdkerrors.APIError
	if !errors.As(err, &apiErr) || apiErr.Code != "ERROR_CODE_TRIGGER_NOT_FOUND" || sdkerrors.IsRetryable(err) {
		t.Fatalf("err = %T %+v, want a final TRIGGER_NOT_FOUND", err, err)
	}
	if len(handler.cancels) != 1 {
		t.Fatalf("cancels = %d, want 1", len(handler.cancels))
	}
}

func TestBatchCancelTriggersSendsIDsInOrderAndReadsEachOutcome(t *testing.T) {
	delay := uint64(1500)
	handler := &ocoHandler{batchResp: &triggersv1.BatchCancelTriggersResponse{
		Results: []*triggersv1.BatchCancelTriggersResultItem{
			{Status: triggersv1.BatchCancelTriggersResultItem_ACCEPTED, TriggerId: 12},
			{Status: triggersv1.BatchCancelTriggersResultItem_REJECTED, TriggerId: 11, Code: "TRIGGER_STATUS_INVALID",
				Error: &orderv1.ErrorDetail{Code: orderv1.ErrorCode_ERROR_CODE_TRIGGER_STATUS_INVALID}},
			{Status: triggersv1.BatchCancelTriggersResultItem_REJECTED, TriggerId: 13, Code: "RATE_LIMIT_EXCEEDED",
				Error: &orderv1.ErrorDetail{Code: orderv1.ErrorCode_ERROR_CODE_RATE_LIMIT_EXCEEDED, RateLimit: &ratelimitv1.RateLimitDetail{RetryAfterMs: &delay}}},
		},
		AcceptedCount: 1,
		RejectedCount: 2,
	}}
	service := newTriggerControlServiceWith(t, handler)
	sub := codecs.FormatID(42)
	requestID := "cancel-wave-1"
	ids := []string{codecs.FormatID(12), "11", codecs.FormatID(13)}

	got, err := service.BatchCancel(context.Background(), nil, ids, &sub, &requestID)
	if err != nil {
		t.Fatalf("batch cancel: %v", err)
	}
	subID := uint64(42)
	wantReq := &triggersv1.BatchCancelTriggersRequest{SubaccountId: &subID, RequestId: requestID, TriggerIds: []uint64{12, 11, 13}}
	if !proto.Equal(handler.batches[0], wantReq) {
		t.Fatalf("request = %v, want %v", handler.batches[0], wantReq)
	}
	if got.AcceptedCount != 1 || got.RejectedCount != 2 || len(got.Results) != 3 {
		t.Fatalf("result = %+v", got)
	}
	if r := got.Results[0]; r.Status != "accepted" || r.TriggerID != codecs.FormatID(12) || r.Code != "" {
		t.Fatalf("accepted item = %+v", r)
	}
	if r := got.Results[1]; r.Status != "rejected" || r.TriggerID != codecs.FormatID(11) || r.Code != "TRIGGER_STATUS_INVALID" || r.RateLimit != nil {
		t.Fatalf("ended item = %+v", r)
	}
	if r := got.Results[2]; r.Status != "rejected" || r.Code != "RATE_LIMIT_EXCEEDED" || r.RateLimit == nil || r.RateLimit.RetryAfterMs == nil || *r.RateLimit.RetryAfterMs != 1500 {
		t.Fatalf("rate-limited item = %+v", r)
	}
}

// A batch whose outcome is unknown is sent again unchanged: the same request
// ID and trigger IDs return the original results.
func TestBatchCancelTriggersRetrySendsTheSameRequest(t *testing.T) {
	handler := &ocoHandler{batchResp: &triggersv1.BatchCancelTriggersResponse{
		Results:       []*triggersv1.BatchCancelTriggersResultItem{{Status: triggersv1.BatchCancelTriggersResultItem_ACCEPTED, TriggerId: 12}},
		AcceptedCount: 1,
	}}
	service := newTriggerControlServiceWith(t, handler)
	requestID := "cancel-wave-2"
	for range 2 {
		if _, err := service.BatchCancel(context.Background(), nil, []string{"12"}, nil, &requestID); err != nil {
			t.Fatalf("batch cancel: %v", err)
		}
	}
	if !proto.Equal(handler.batches[0], handler.batches[1]) {
		t.Fatalf("retry = %v, want the first request %v", handler.batches[1], handler.batches[0])
	}

	if _, err := service.BatchCancel(context.Background(), nil, []string{"12"}, nil, nil); err != nil {
		t.Fatalf("batch cancel without a request ID: %v", err)
	}
	if generated := handler.batches[2].GetRequestId(); !strings.HasPrefix(generated, "batch-cancel-triggers-") {
		t.Fatalf("generated request ID = %q", generated)
	}
}

func TestBatchCancelTriggersRefusesAnInvalidBatchBeforeSending(t *testing.T) {
	tooMany := make([]string, codecs.MaxBatchCancelTriggers+1)
	for i := range tooMany {
		tooMany[i] = codecs.FormatID(uint64(i + 1))
	}
	badRequestID := "has space"
	cases := map[string]struct {
		ids       []string
		requestID *string
	}{
		"no trigger":         {ids: nil},
		"over the limit":     {ids: tooMany},
		"repeated trigger":   {ids: []string{"12", codecs.FormatID(12)}},
		"zero trigger ID":    {ids: []string{"0"}},
		"invalid trigger ID": {ids: []string{"not-an-id!"}},
		"invalid request ID": {ids: []string{"12"}, requestID: &badRequestID},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			handler := &ocoHandler{}
			service := newTriggerControlServiceWith(t, handler)
			_, err := service.BatchCancel(context.Background(), nil, tc.ids, nil, tc.requestID)
			var validation *sdkerrors.ValidationError
			if !errors.As(err, &validation) || len(handler.batches) != 0 {
				t.Fatalf("err = %T %v, sent %d, want a validation error and nothing sent", err, err, len(handler.batches))
			}
		})
	}
	if codecs.MaxBatchCancelTriggers != 50 {
		t.Fatalf("MaxBatchCancelTriggers = %d, want the contract's 50", codecs.MaxBatchCancelTriggers)
	}
}

func TestBatchCancelTriggersRefusesAnInconsistentResponse(t *testing.T) {
	cases := map[string]*triggersv1.BatchCancelTriggersResponse{
		"unknown status": {
			Results:       []*triggersv1.BatchCancelTriggersResultItem{{TriggerId: 12}},
			AcceptedCount: 1,
		},
		"counts disagree": {
			Results:       []*triggersv1.BatchCancelTriggersResultItem{{Status: triggersv1.BatchCancelTriggersResultItem_ACCEPTED, TriggerId: 12}},
			RejectedCount: 1,
		},
	}
	for name, resp := range cases {
		t.Run(name, func(t *testing.T) {
			service := newTriggerControlServiceWith(t, &ocoHandler{batchResp: resp})
			_, err := service.BatchCancel(context.Background(), nil, []string{"12"}, nil, nil)
			var contract *sdkerrors.ResponseContractError
			if !errors.As(err, &contract) {
				t.Fatalf("err = %T %v, want a response contract error", err, err)
			}
		})
	}
}

func TestTriggerListFiltersByOcoAndReadsItsLegs(t *testing.T) {
	listID := uint64(77)
	handler := &ocoHandler{listTrigger: &triggersv1.Trigger{TriggerId: 78, SymbolId: 5, OrderListId: &listID, ClientListId: "protect-btc-1"}}
	service := newTriggerControlServiceWith(t, handler)
	ocoID := codecs.FormatID(77)

	got, err := service.List(context.Background(), nil, nil, nil, nil, 0, nil, models.ListTriggersOptions{OrderListID: &ocoID})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if handler.lists[0].OrderListId == nil || *handler.lists[0].OrderListId != 77 {
		t.Fatalf("order_list_id filter = %v, want 77", handler.lists[0].OrderListId)
	}
	if len(got.Triggers) != 1 || got.Triggers[0].OrderListID != ocoID || got.Triggers[0].ClientListID != "protect-btc-1" {
		t.Fatalf("triggers = %+v, want the leg with its OCO", got.Triggers)
	}

	clientListID := "oco énergie #1"
	if _, err := service.List(context.Background(), nil, nil, nil, nil, 0, nil, models.ListTriggersOptions{ClientListID: &clientListID}); err != nil {
		t.Fatalf("list by client OCO ID: %v", err)
	}
	if sent := handler.lists[1].ClientListId; sent == nil || *sent != clientListID || handler.lists[1].OrderListId != nil {
		t.Fatalf("client_list_id filter = %v, want %q alone", sent, clientListID)
	}

	zero, empty, long := "0", "", strings.Repeat("a", 37)
	for name, opts := range map[string]models.ListTriggersOptions{
		"zero OCO ID":                 {OrderListID: &zero},
		"empty client OCO ID":         {ClientListID: &empty},
		"client OCO ID over 36 bytes": {ClientListID: &long},
	} {
		if _, err := service.List(context.Background(), nil, nil, nil, nil, 0, nil, opts); err == nil || len(handler.lists) != 2 {
			t.Fatalf("%s: err = %v, lists sent = %d, want refused before sending", name, err, len(handler.lists))
		}
	}
}
