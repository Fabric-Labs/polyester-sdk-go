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
	triggersv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/triggers/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/gen/triggers/v1/triggersv1connect"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type cancelAllTriggersHandler struct {
	triggersv1connect.UnimplementedTriggersServiceHandler
	mu       sync.Mutex
	requests []*triggersv1.CancelAllTriggersRequest
	resp     *triggersv1.CancelAllTriggersResponse
}

func (h *cancelAllTriggersHandler) CancelAllTriggers(_ context.Context, req *connect.Request[triggersv1.CancelAllTriggersRequest]) (*connect.Response[triggersv1.CancelAllTriggersResponse], error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.requests = append(h.requests, proto.Clone(req.Msg).(*triggersv1.CancelAllTriggersRequest))
	if h.resp != nil {
		return connect.NewResponse(h.resp), nil
	}
	status := triggersv1.CancelAllTriggersResponse_SUBMITTED
	if req.Msg.GetDryRun() {
		status = triggersv1.CancelAllTriggersResponse_DRY_RUN
	}
	return connect.NewResponse(&triggersv1.CancelAllTriggersResponse{Status: status, MatchedTriggers: 4, TsNs: 1_700_000_000_000_000_123}), nil
}

// CancelAll sends the selected symbols, side and trigger types once each,
// with the caller's request ID, and reads the count and acceptance time.
func TestTriggersCancelAllSendsCriteriaAndReadsTheCount(t *testing.T) {
	handler := &cancelAllTriggersHandler{}
	service := newTriggerControlServiceWith(t, handler)
	sub := codecs.FormatID(42)
	side := "Sell"
	requestID := "ui-tpsl-cancel-all-7f3a"

	got, err := service.CancelAll(context.Background(), nil, CancelAllTriggersParams{
		SubAccountID: &sub,
		Symbols:      []string{"BTC-USDT", "BTC-USDT"},
		Side:         &side,
		TriggerTypes: []string{"stop_loss", "TAKE_PROFIT", "stop-loss"},
		RequestID:    &requestID,
	})
	if err != nil {
		t.Fatal(err)
	}
	want := models.CancelAllTriggersResult{Status: "submitted", MatchedTriggers: 4, TsNs: 1_700_000_000_000_000_123}
	if got != want {
		t.Fatalf("result = %+v, want %+v", got, want)
	}
	subID := uint64(42)
	wantReq := &triggersv1.CancelAllTriggersRequest{
		SubaccountId: &subID,
		SymbolIds:    []uint32{5},
		Side:         orderv1.Side_SELL,
		RequestId:    requestID,
		TriggerTypes: []triggersv1.TriggerType{triggersv1.TriggerType_STOP_LOSS, triggersv1.TriggerType_TAKE_PROFIT},
	}
	if len(handler.requests) != 1 || !proto.Equal(handler.requests[0], wantReq) {
		t.Fatalf("requests = %v, want %v", handler.requests, wantReq)
	}
}

// With no filters every trigger of the account matches; a request ID is
// generated, and a dry run only counts.
func TestTriggersCancelAllDryRunWithoutFiltersGeneratesARequestID(t *testing.T) {
	handler := &cancelAllTriggersHandler{}
	service := newTriggerControlServiceWith(t, handler)

	got, err := service.CancelAll(context.Background(), nil, CancelAllTriggersParams{DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "dry_run" || got.MatchedTriggers != 4 {
		t.Fatalf("result = %+v", got)
	}
	req := handler.requests[0]
	if !req.GetDryRun() || req.SubaccountId != nil || len(req.GetSymbolIds()) != 0 || len(req.GetTriggerTypes()) != 0 ||
		req.GetSide() != orderv1.Side_SIDE_UNSPECIFIED || !strings.HasPrefix(req.GetRequestId(), "cancel-all-triggers-") {
		t.Fatalf("request = %v", req)
	}
}

// A request the contract refuses is refused before sending.
func TestTriggersCancelAllRefusesInvalidCriteriaBeforeSending(t *testing.T) {
	handler := &cancelAllTriggersHandler{}
	service := newTriggerControlServiceWith(t, handler)
	side := "both"
	badID := "has space"
	blank := "  "
	tooManySymbols := make([]uint32, codecs.MaxCancelAllTriggerSymbols+1)
	for i := range tooManySymbols {
		tooManySymbols[i] = uint32(i + 1)
	}
	cases := map[string]CancelAllTriggersParams{
		"unknown type":     {TriggerTypes: []string{"oco"}},
		"bad side":         {Side: &side},
		"zero symbol":      {SymbolIDs: []uint32{0}},
		"too many symbols": {SymbolIDs: tooManySymbols},
		"bad request id":   {RequestID: &badID},
		"two selectors":    {Symbols: []string{"BTC-USDT"}, SymbolIDs: []uint32{5}},
		"blank symbol":     {Symbol: &blank, DryRun: true},
		"empty symbol":     {Symbols: []string{""}, DryRun: true},
		"blank in symbols": {Symbols: []string{"BTC-USDT", " "}},
	}
	for name, params := range cases {
		_, err := service.CancelAll(context.Background(), nil, params)
		var validation *sdkerrors.ValidationError
		if !errors.As(err, &validation) {
			t.Errorf("%s: err = %v, want a ValidationError", name, err)
		}
	}
	if len(handler.requests) != 0 {
		t.Fatalf("sent %d requests", len(handler.requests))
	}
	if codecs.MaxCancelAllTriggerSymbols != 100 || codecs.MaxCancelAllTriggerTypes != 5 {
		t.Fatalf("limits = %d symbols, %d types", codecs.MaxCancelAllTriggerSymbols, codecs.MaxCancelAllTriggerTypes)
	}
}

// The acceptance time falls back to ts when ts_ns is absent, and a reply
// without a known status is refused as a contract error.
func TestTriggersCancelAllReadsTsAndRefusesUnknownStatus(t *testing.T) {
	handler := &cancelAllTriggersHandler{resp: &triggersv1.CancelAllTriggersResponse{
		Status: triggersv1.CancelAllTriggersResponse_SUBMITTED, MatchedTriggers: 1, Ts: &timestamppb.Timestamp{Seconds: 1_700_000_000, Nanos: 5},
	}}
	service := newTriggerControlServiceWith(t, handler)
	got, err := service.CancelAll(context.Background(), nil, CancelAllTriggersParams{})
	if err != nil || got.TsNs != 1_700_000_000_000_000_005 {
		t.Fatalf("result = %+v, %v", got, err)
	}
	handler.resp = &triggersv1.CancelAllTriggersResponse{MatchedTriggers: 1}
	_, err = service.CancelAll(context.Background(), nil, CancelAllTriggersParams{})
	var contract *sdkerrors.ResponseContractError
	if !errors.As(err, &contract) {
		t.Fatalf("err = %v, want a ResponseContractError", err)
	}
}

// Leaving every symbol filter unset still matches every pair: only a
// supplied blank symbol is refused.
func TestTriggersCancelAllOmittedSymbolsMatchEveryPair(t *testing.T) {
	handler := &cancelAllTriggersHandler{}
	service := newTriggerControlServiceWith(t, handler)
	for _, params := range []CancelAllTriggersParams{{}, {Symbols: []string{}}} {
		if _, err := service.CancelAll(context.Background(), nil, params); err != nil {
			t.Fatal(err)
		}
	}
	if len(handler.requests) != 2 || len(handler.requests[0].GetSymbolIds()) != 0 || len(handler.requests[1].GetSymbolIds()) != 0 {
		t.Fatalf("requests = %v", handler.requests)
	}
}
