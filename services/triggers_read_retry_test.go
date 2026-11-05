package services

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	orderv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/orders/v1"
	triggersv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/triggers/v1"
	"google.golang.org/protobuf/proto"
)

// triggerReadHandler answers the first unavailable reads with the
// catching-up UNAVAILABLE, then fails with failCode or succeeds.
type triggerReadHandler struct {
	triggerControlHandler
	mu          sync.Mutex
	unavailable int
	failCode    connect.Code
	gets        []*triggersv1.GetTriggerRequest
	lists       []*triggersv1.ListTriggersRequest
	events      []*triggersv1.ListTriggerEventsRequest
}

func catchingUp() error {
	err := connect.NewError(connect.CodeUnavailable, errors.New("Triggers are catching up with recent activity; retry the request."))
	if detail, detailErr := connect.NewErrorDetail(&orderv1.ErrorDetail{Code: orderv1.ErrorCode_ERROR_CODE_UNAVAILABLE}); detailErr == nil {
		err.AddDetail(detail)
	}
	return err
}

func (h *triggerReadHandler) answer() error {
	if h.unavailable > 0 {
		h.unavailable--
		return catchingUp()
	}
	if h.failCode != 0 {
		return connect.NewError(h.failCode, errors.New("refused"))
	}
	return nil
}

func (h *triggerReadHandler) GetTrigger(_ context.Context, req *connect.Request[triggersv1.GetTriggerRequest]) (*connect.Response[triggersv1.GetTriggerResponse], error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.gets = append(h.gets, req.Msg)
	if err := h.answer(); err != nil {
		return nil, err
	}
	return connect.NewResponse(&triggersv1.GetTriggerResponse{Trigger: &triggersv1.Trigger{
		TriggerId: req.Msg.GetTriggerId(), Status: triggersv1.TriggerStatus_STATUS_PAUSED,
	}}), nil
}

func (h *triggerReadHandler) ListTriggers(_ context.Context, req *connect.Request[triggersv1.ListTriggersRequest]) (*connect.Response[triggersv1.ListTriggersResponse], error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.lists = append(h.lists, req.Msg)
	if err := h.answer(); err != nil {
		return nil, err
	}
	return connect.NewResponse(&triggersv1.ListTriggersResponse{Triggers: []*triggersv1.Trigger{{
		TriggerId: 9, Status: triggersv1.TriggerStatus_STATUS_PAUSED,
	}}}), nil
}

func newTriggerReadService(t *testing.T, handler *triggerReadHandler) *TriggersService {
	t.Helper()
	return newTriggerControlServiceWith(t, handler)
}

func TestTriggerReadsRetryCatchingUpWithTheSameRequest(t *testing.T) {
	handler := &triggerReadHandler{unavailable: 3}
	service := newTriggerReadService(t, handler)

	trigger, err := service.Get(context.Background(), nil, codecs.FormatID(9), nil)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if trigger == nil || trigger.Status != "paused" {
		t.Fatalf("trigger=%+v", trigger)
	}

	handler.mu.Lock()
	handler.unavailable = 2
	handler.mu.Unlock()
	symbol := "BTC-USDT"
	list, err := service.List(context.Background(), nil, nil, &symbol, []string{"paused"}, 10, nil)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list.Triggers) != 1 {
		t.Fatalf("list=%+v", list)
	}

	handler.mu.Lock()
	defer handler.mu.Unlock()
	if len(handler.gets) != 4 || len(handler.lists) != 3 {
		t.Fatalf("gets=%d lists=%d", len(handler.gets), len(handler.lists))
	}
	for _, req := range handler.gets[1:] {
		if !proto.Equal(req, handler.gets[0]) {
			t.Fatalf("retry changed the request: %v vs %v", req, handler.gets[0])
		}
	}
	for _, req := range handler.lists[1:] {
		if !proto.Equal(req, handler.lists[0]) {
			t.Fatalf("retry changed the request: %v vs %v", req, handler.lists[0])
		}
	}
}

func TestTriggerReadRetryGivesUpAfterItsBudget(t *testing.T) {
	handler := &triggerReadHandler{unavailable: 1 << 30}
	service := newTriggerReadService(t, handler)

	started := time.Now()
	_, err := service.Get(context.Background(), nil, codecs.FormatID(9), nil)
	elapsed := time.Since(started)
	var serverErr *sdkerrors.ServerError
	if !errors.As(err, &serverErr) || serverErr.Code != "ERROR_CODE_UNAVAILABLE" || !sdkerrors.IsRetryable(err) {
		t.Fatalf("err=%T %v", err, err)
	}
	// 25 ms doubling to a 500 ms cap: the waits add up to 1.775 s, the
	// last one that fits the 2 s budget.
	if elapsed < 1700*time.Millisecond || elapsed > 3*time.Second {
		t.Fatalf("gave up after %s", elapsed)
	}
	handler.mu.Lock()
	defer handler.mu.Unlock()
	if len(handler.gets) != 8 {
		t.Fatalf("attempts=%d", len(handler.gets))
	}
}

func TestTriggerReadRetryLeavesOtherErrorsAlone(t *testing.T) {
	handler := &triggerReadHandler{failCode: connect.CodeFailedPrecondition}
	service := newTriggerReadService(t, handler)
	if _, err := service.Get(context.Background(), nil, codecs.FormatID(9), nil); err == nil {
		t.Fatal("expected the refusal")
	}
	handler.mu.Lock()
	defer handler.mu.Unlock()
	if len(handler.gets) != 1 {
		t.Fatalf("attempts=%d", len(handler.gets))
	}
}

func TestTriggerReadRetryStopsWhenCanceled(t *testing.T) {
	handler := &triggerReadHandler{unavailable: 1 << 30}
	service := newTriggerReadService(t, handler)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Millisecond)
	defer cancel()
	started := time.Now()
	if _, err := service.Get(ctx, nil, codecs.FormatID(9), nil); err == nil {
		t.Fatal("expected an error")
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("cancellation took %s", elapsed)
	}
}

func (h *triggerReadHandler) ListTriggerEvents(_ context.Context, req *connect.Request[triggersv1.ListTriggerEventsRequest]) (*connect.Response[triggersv1.ListTriggerEventsResponse], error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.events = append(h.events, proto.Clone(req.Msg).(*triggersv1.ListTriggerEventsRequest))
	if err := h.answer(); err != nil {
		return nil, err
	}
	return connect.NewResponse(&triggersv1.ListTriggerEventsResponse{}), nil
}
func TestTriggerEventsRetryAndNotFound(t *testing.T) {
	handler := &triggerReadHandler{unavailable: 2}
	service := newTriggerReadService(t, handler)
	sub, page, event := "12", "next-page", "fired"
	result, err := service.ListEvents(context.Background(), nil, codecs.FormatID(9), &sub, 17, &event, &page)
	if err != nil {
		t.Fatalf("events read did not recover: %v", err)
	}
	if len(result.Events) != 0 {
		t.Fatalf("events=%v", result.Events)
	}
	handler.mu.Lock()
	if len(handler.events) != 3 {
		t.Fatalf("events attempts=%d", len(handler.events))
	}
	first := handler.events[0]
	if first.GetTriggerId() != 9 || first.GetSubaccountId() != 12 || first.GetLimit() != 17 || first.GetPageToken() != page || first.GetEventType() != triggersv1.TriggerEventType_EVENT_FIRED {
		t.Fatalf("request=%v", first)
	}
	for _, req := range handler.events[1:] {
		if !proto.Equal(first, req) {
			t.Fatalf("retry changed request: %v", req)
		}
	}
	handler.failCode = connect.CodeNotFound
	handler.mu.Unlock()
	_, getErr := service.Get(context.Background(), nil, codecs.FormatID(9), nil)
	_, eventsErr := service.ListEvents(context.Background(), nil, codecs.FormatID(9), nil, 17, nil, nil)
	var getAPI, eventsAPI *sdkerrors.APIError
	if !errors.As(getErr, &getAPI) || !errors.As(eventsErr, &eventsAPI) || getAPI.Code != eventsAPI.Code || getAPI.Code != "not_found" {
		t.Fatalf("get=%T %v events=%T %v", getErr, getErr, eventsErr, eventsErr)
	}
	handler.mu.Lock()
	defer handler.mu.Unlock()
	if len(handler.gets) != 1 || len(handler.events) != 4 {
		t.Fatalf("not-found was retried: gets=%d events=%d", len(handler.gets), len(handler.events))
	}
}
