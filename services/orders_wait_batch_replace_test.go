package services

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/Fabric-Labs/polyester-sdk-go/auth"
	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	orderv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/orders/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/gen/orders/v1/ordersv1connect"
	"github.com/Fabric-Labs/polyester-sdk-go/transport"
)

// batchStatusHandler answers GetBatchReplaceStatus from a script of
// responses; the last one repeats.
type batchStatusHandler struct {
	ordersv1connect.UnimplementedOrdersReadServiceHandler
	mu        sync.Mutex
	responses []func() (*orderv1.GetBatchReplaceStatusResponse, error)
	calls     int
}

func (h *batchStatusHandler) GetBatchReplaceStatus(_ context.Context, req *connect.Request[orderv1.GetBatchReplaceStatusRequest]) (*connect.Response[orderv1.GetBatchReplaceStatusResponse], error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	next := h.responses[min(h.calls, len(h.responses)-1)]
	h.calls++
	msg, err := next()
	if err != nil {
		return nil, err
	}
	msg.BatchRequestId = req.Msg.GetBatchRequestId()
	return connect.NewResponse(msg), nil
}

func newBatchStatusService(t *testing.T, handler *batchStatusHandler) *OrdersService {
	t.Helper()
	mux := http.NewServeMux()
	path, h := ordersv1connect.NewOrdersReadServiceHandler(handler)
	mux.Handle(path, h)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	creds, err := auth.LoadCredentials("ak_test", auth.GenerateEd25519Keypair().SecretKeyHex, false)
	if err != nil {
		t.Fatal(err)
	}
	cfg := transport.DefaultConfig()
	cfg.APIURL = srv.URL
	return NewOrdersService(transport.NewFactory(cfg, creds, nil), nil, nil, nil, nil, nil, nil)
}

func batchStatus(acceptedAt time.Time, phase orderv1.BatchReplacePhase, code string) func() (*orderv1.GetBatchReplaceStatusResponse, error) {
	return func() (*orderv1.GetBatchReplaceStatusResponse, error) {
		return &orderv1.GetBatchReplaceStatusResponse{
			AdmissionStatus: orderv1.BatchReplaceAdmissionStatus_BATCH_REPLACE_ADMISSION_STATUS_ADMITTED,
			Items: []*orderv1.BatchReplaceStatusItem{
				{ItemIndex: 0, Phase: orderv1.BatchReplacePhase_BATCH_REPLACE_PHASE_WORKING, OldOrderId: 1, ReplacementOrderId: 2, ActionTaken: orderv1.ModifyActionTaken_REPLACED},
				{ItemIndex: 1, Phase: phase, OldOrderId: 3, Code: code},
			},
			AcceptedCount: 2,
			AcceptedTsNs:  uint64(acceptedAt.UnixNano()),
		}, nil
	}
}

func TestWaitBatchReplacePollsUntilNoItemIsAdmitted(t *testing.T) {
	now := time.Now()
	handler := &batchStatusHandler{responses: []func() (*orderv1.GetBatchReplaceStatusResponse, error){
		// A new batch's status reads not found until it becomes readable.
		func() (*orderv1.GetBatchReplaceStatusResponse, error) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("batch replace status not found"))
		},
		batchStatus(now, orderv1.BatchReplacePhase_BATCH_REPLACE_PHASE_ADMITTED, ""),
		func() (*orderv1.GetBatchReplaceStatusResponse, error) {
			return nil, connect.NewError(connect.CodeUnavailable, errors.New("catching up"))
		},
		batchStatus(now, orderv1.BatchReplacePhase_BATCH_REPLACE_PHASE_ADMITTED, ""),
		batchStatus(now, orderv1.BatchReplacePhase_BATCH_REPLACE_PHASE_REJECTED, "POST_ONLY_CROSS"),
	}}
	service := newBatchStatusService(t, handler)

	status, err := service.WaitBatchReplace(context.Background(), nil, codecs.FormatID(77), nil)
	if err != nil {
		t.Fatal(err)
	}
	refused := status.Items[1]
	if refused.Phase != "rejected" || refused.ErrorCode != "ERROR_CODE_POST_ONLY_CROSS" || refused.ReplacementOrderID != "" {
		t.Fatalf("refused item=%+v", refused)
	}
	if status.Items[0].Phase != "working" || status.Items[0].ReplacementOrderID == "" {
		t.Fatalf("working item=%+v", status.Items[0])
	}
	handler.mu.Lock()
	defer handler.mu.Unlock()
	if handler.calls != 5 {
		t.Fatalf("polls=%d", handler.calls)
	}
}

func TestWaitBatchReplaceStopsAtTheStatusWindow(t *testing.T) {
	// Accepted 59.7 s ago: the 60 s status window ends during the wait.
	acceptedAt := time.Now().Add(-59700 * time.Millisecond)
	handler := &batchStatusHandler{responses: []func() (*orderv1.GetBatchReplaceStatusResponse, error){
		batchStatus(acceptedAt, orderv1.BatchReplacePhase_BATCH_REPLACE_PHASE_ADMITTED, ""),
	}}
	service := newBatchStatusService(t, handler)

	started := time.Now()
	status, err := service.WaitBatchReplace(context.Background(), nil, codecs.FormatID(77), nil)
	var expired *sdkerrors.BatchReplaceWindowExpiredError
	if !errors.As(err, &expired) || expired.BatchRequestID != codecs.FormatID(77) {
		t.Fatalf("err=%T %v", err, err)
	}
	if sdkerrors.IsRetryable(err) {
		t.Fatal("waiting again after the window proves nothing")
	}
	if elapsed := time.Since(started); elapsed > 2*time.Second {
		t.Fatalf("stopped after %s", elapsed)
	}
	// The last status read is returned for reconciliation.
	if len(status.Items) != 2 || status.Items[1].Phase != "admitted" {
		t.Fatalf("last status=%+v", status)
	}
}

func TestWaitBatchReplaceStopsAtTheWindowWhileNotFound(t *testing.T) {
	handler := &batchStatusHandler{responses: []func() (*orderv1.GetBatchReplaceStatusResponse, error){
		func() (*orderv1.GetBatchReplaceStatusResponse, error) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("batch replace status not found"))
		},
	}}
	service := newBatchStatusService(t, handler)
	// The window starts at the first read when no status was ever read.
	saved := batchReplaceStatusWindow
	batchReplaceStatusWindow = 300 * time.Millisecond
	t.Cleanup(func() { batchReplaceStatusWindow = saved })
	_, err := service.WaitBatchReplace(context.Background(), nil, codecs.FormatID(77), nil)
	var expired *sdkerrors.BatchReplaceWindowExpiredError
	if !errors.As(err, &expired) {
		t.Fatalf("err=%T %v", err, err)
	}
}

func TestWaitBatchReplaceSurfacesOtherErrors(t *testing.T) {
	handler := &batchStatusHandler{responses: []func() (*orderv1.GetBatchReplaceStatusResponse, error){
		func() (*orderv1.GetBatchReplaceStatusResponse, error) {
			return nil, connect.NewError(connect.CodePermissionDenied, errors.New("not your batch"))
		},
	}}
	service := newBatchStatusService(t, handler)
	if _, err := service.WaitBatchReplace(context.Background(), nil, codecs.FormatID(77), nil); err == nil {
		t.Fatal("expected the refusal")
	}
	handler.mu.Lock()
	defer handler.mu.Unlock()
	if handler.calls != 1 {
		t.Fatalf("polls=%d", handler.calls)
	}
}

func slowly(delay time.Duration, next func() (*orderv1.GetBatchReplaceStatusResponse, error)) func() (*orderv1.GetBatchReplaceStatusResponse, error) {
	return func() (*orderv1.GetBatchReplaceStatusResponse, error) {
		time.Sleep(delay)
		return next()
	}
}

// A status read still in flight when the window ends is abandoned: the wait
// does not outlast the window it promises.
func TestWaitBatchReplaceBoundsAnInFlightReadByTheWindow(t *testing.T) {
	acceptedAt := time.Now().Add(-59700 * time.Millisecond)
	handler := &batchStatusHandler{responses: []func() (*orderv1.GetBatchReplaceStatusResponse, error){
		batchStatus(acceptedAt, orderv1.BatchReplacePhase_BATCH_REPLACE_PHASE_ADMITTED, ""),
		slowly(1500*time.Millisecond, batchStatus(acceptedAt, orderv1.BatchReplacePhase_BATCH_REPLACE_PHASE_WORKING, "")),
	}}
	service := newBatchStatusService(t, handler)

	started := time.Now()
	status, err := service.WaitBatchReplace(context.Background(), nil, codecs.FormatID(77), nil)
	elapsed := time.Since(started)
	var expired *sdkerrors.BatchReplaceWindowExpiredError
	if !errors.As(err, &expired) {
		t.Fatalf("err=%T %v", err, err)
	}
	if elapsed > time.Second {
		t.Fatalf("returned after %s, past the window", elapsed)
	}
	if len(status.Items) != 2 || status.Items[1].Phase != "admitted" {
		t.Fatalf("last status=%+v", status)
	}
}

// The caller's cancellation of an in-flight read stays a cancellation.
func TestWaitBatchReplaceKeepsCallerCancellation(t *testing.T) {
	handler := &batchStatusHandler{responses: []func() (*orderv1.GetBatchReplaceStatusResponse, error){
		slowly(time.Second, batchStatus(time.Now(), orderv1.BatchReplacePhase_BATCH_REPLACE_PHASE_ADMITTED, "")),
	}}
	service := newBatchStatusService(t, handler)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	started := time.Now()
	_, err := service.WaitBatchReplace(ctx, nil, codecs.FormatID(77), nil)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err=%T %v", err, err)
	}
	var expired *sdkerrors.BatchReplaceWindowExpiredError
	if errors.As(err, &expired) {
		t.Fatal("the caller's deadline is not the status window")
	}
	if elapsed := time.Since(started); elapsed > 600*time.Millisecond {
		t.Fatalf("returned after %s", elapsed)
	}
}

func TestWaitBatchReplaceStopsAtTheWindowWhileUnavailable(t *testing.T) {
	acceptedAt := time.Now().Add(-59700 * time.Millisecond)
	handler := &batchStatusHandler{responses: []func() (*orderv1.GetBatchReplaceStatusResponse, error){
		batchStatus(acceptedAt, orderv1.BatchReplacePhase_BATCH_REPLACE_PHASE_ADMITTED, ""),
		func() (*orderv1.GetBatchReplaceStatusResponse, error) {
			return nil, connect.NewError(connect.CodeUnavailable, errors.New("catching up"))
		},
	}}
	service := newBatchStatusService(t, handler)

	status, err := service.WaitBatchReplace(context.Background(), nil, codecs.FormatID(77), nil)
	var expired *sdkerrors.BatchReplaceWindowExpiredError
	if !errors.As(err, &expired) {
		t.Fatalf("err=%T %v", err, err)
	}
	if len(status.Items) != 2 || status.Items[1].Phase != "admitted" {
		t.Fatalf("last status=%+v", status)
	}
	handler.mu.Lock()
	defer handler.mu.Unlock()
	if handler.calls < 3 {
		t.Fatalf("polls=%d", handler.calls)
	}
}
