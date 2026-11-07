package services

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	orderv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/orders/v1"
)

// GetBatchReplaceStatus: UNAVAILABLE is the SDK's retryable server error,
// NOT_FOUND stays a not-found refusal and never reads as a rejected batch,
// and the batch ID keeps all 64 bits whether given in decimal or base58.
func TestGetBatchReplaceStatusErrorsAndExactBatchID(t *testing.T) {
	unavailable := func() (*orderv1.GetBatchReplaceStatusResponse, error) {
		err := connect.NewError(connect.CodeUnavailable, errors.New("The batch status is temporarily unavailable; retry the request."))
		detail, detailErr := connect.NewErrorDetail(&orderv1.ErrorDetail{Code: orderv1.ErrorCode_ERROR_CODE_UNAVAILABLE})
		if detailErr != nil {
			t.Fatal(detailErr)
		}
		err.AddDetail(detail)
		return nil, err
	}
	notFound := func() (*orderv1.GetBatchReplaceStatusResponse, error) {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("batch replace status not found"))
	}
	ctx := context.Background()
	const maxID = "18446744073709551615"

	svc := newBatchStatusService(t, &batchStatusHandler{responses: []func() (*orderv1.GetBatchReplaceStatusResponse, error){unavailable}})
	_, err := svc.GetBatchReplaceStatus(ctx, nil, maxID, nil)
	var serverErr *sdkerrors.ServerError
	if !errors.As(err, &serverErr) || !sdkerrors.IsRetryable(err) || sdkerrors.MutationOutcomeUnknown(err) {
		t.Fatalf("UNAVAILABLE = %T %v, retryable=%v", err, err, sdkerrors.IsRetryable(err))
	}

	svc = newBatchStatusService(t, &batchStatusHandler{responses: []func() (*orderv1.GetBatchReplaceStatusResponse, error){notFound}})
	_, err = svc.GetBatchReplaceStatus(ctx, nil, maxID, nil)
	var apiErr *sdkerrors.APIError
	if !errors.As(err, &apiErr) || apiErr.Code != connect.CodeNotFound.String() || sdkerrors.IsRetryable(err) {
		t.Fatalf("NOT_FOUND = %T %v", err, err)
	}

	svc = newBatchStatusService(t, &batchStatusHandler{responses: []func() (*orderv1.GetBatchReplaceStatusResponse, error){
		batchStatus(time.Now(), orderv1.BatchReplacePhase_BATCH_REPLACE_PHASE_WORKING, ""),
	}})
	status, err := svc.GetBatchReplaceStatus(ctx, nil, maxID, nil)
	if err != nil || status.BatchRequestID != codecs.FormatUint64ID(math.MaxUint64) {
		t.Fatalf("status = %+v, err = %v", status, err)
	}
}
