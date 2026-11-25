package polyester

import (
	"context"
	stderrors "errors"
	"strings"
	"testing"

	"connectrpc.com/connect"
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	transferv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/transfer/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
	"google.golang.org/protobuf/proto"
)

func (a *contractAPI) AbandonInternalTransferBatch(_ context.Context, req *connect.Request[transferv1.AbandonInternalTransferBatchRequest]) (*connect.Response[transferv1.AbandonInternalTransferBatchResponse], error) {
	a.record(req.Header(), req.Msg)
	resp, err := a.abandon(req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

func abandonAnswer(resp *transferv1.AbandonInternalTransferBatchResponse) func(*transferv1.AbandonInternalTransferBatchRequest) (*transferv1.AbandonInternalTransferBatchResponse, error) {
	return func(*transferv1.AbandonInternalTransferBatchRequest) (*transferv1.AbandonInternalTransferBatchResponse, error) {
		return proto.Clone(resp).(*transferv1.AbandonInternalTransferBatchResponse), nil
	}
}

func transferRefusal(code connect.Code, errorCode transferv1.ErrorCode) error {
	err := connect.NewError(code, stderrors.New(errorCode.String()))
	detail, detailErr := connect.NewErrorDetail(&transferv1.ErrorDetail{Code: errorCode})
	if detailErr != nil {
		panic(detailErr)
	}
	err.AddDetail(detail)
	return err
}

// AbandonBatch of a key no batch was submitted under answers "abandoned",
// with no request ID and no legs, from a signed request naming the key.
func TestInternalTransfersAbandonBatchOfAnUnusedKey(t *testing.T) {
	api := &contractAPI{abandon: abandonAnswer(&transferv1.AbandonInternalTransferBatchResponse{Status: transferv1.InternalTransferBatchStatus_BATCH_ABANDONED})}
	result, err := newContractClient(t, api, nil).InternalTransfers.AbandonBatch(context.Background(), "rebalance-1")
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "abandoned" || result.RequestID != "" || result.Legs == nil || len(result.Legs) != 0 {
		t.Fatalf("result = %+v", result)
	}
	sent := api.sent(t)
	if len(sent) != 1 || sent[0].(*transferv1.AbandonInternalTransferBatchRequest).GetIdempotencyKey() != "rebalance-1" {
		t.Fatalf("sent = %v", sent)
	}
}

// AbandonBatch of a key a batch was submitted under answers that batch, as
// GetBatch reads it, and checks it the same way.
func TestInternalTransfersAbandonBatchAnswersTheSubmittedBatch(t *testing.T) {
	const requestID = "92640625967207661803151833376978068587"
	accepted := &transferv1.AbandonInternalTransferBatchResponse{
		RequestId: requestID, Status: transferv1.InternalTransferBatchStatus_BATCH_ACCEPTED,
		Legs: []*transferv1.InternalTransferBatchLegResult{{
			LegIndex: 0, Status: transferv1.InternalTransferBatchLegStatus_LEG_CREDITED, TransferId: "tr-a", FlowId: "flow_A", AssetId: 7, AssetCode: "BTC",
		}},
	}
	result, err := newContractClient(t, &contractAPI{abandon: abandonAnswer(accepted)}, nil).InternalTransfers.AbandonBatch(context.Background(), "k")
	if err != nil {
		t.Fatal(err)
	}
	if result.RequestID != requestID || result.Status != "accepted" || len(result.Legs) != 1 || result.Legs[0].FlowID != "flow_A" {
		t.Fatalf("result = %+v", result)
	}
	refused := &transferv1.AbandonInternalTransferBatchResponse{
		Status: transferv1.InternalTransferBatchStatus_BATCH_REFUSED,
		Legs: []*transferv1.InternalTransferBatchLegResult{{
			LegIndex: 0, Status: transferv1.InternalTransferBatchLegStatus_LEG_NOT_MOVED, ErrorCode: transferv1.ErrorCode_ERROR_CODE_INSUFFICIENT_FUNDS,
		}},
	}
	result, err = newContractClient(t, &contractAPI{abandon: abandonAnswer(refused)}, nil).InternalTransfers.AbandonBatch(context.Background(), "k")
	if err != nil || result.Status != "refused" || result.Legs[0].ErrorCode != "insufficient_funds" {
		t.Fatalf("refused batch = %+v, %v", result, err)
	}
}

// A key outside the contract's limits is refused before anything is sent.
func TestInternalTransfersAbandonBatchRefusesAnOutOfContractKeyBeforeSending(t *testing.T) {
	api := &contractAPI{abandon: abandonAnswer(&transferv1.AbandonInternalTransferBatchResponse{Status: transferv1.InternalTransferBatchStatus_BATCH_ABANDONED})}
	client := newContractClient(t, api, nil)
	for _, key := range []string{"", strings.Repeat("k", 129)} {
		var validation *sdkerrors.ValidationError
		if _, err := client.InternalTransfers.AbandonBatch(context.Background(), key); !stderrors.As(err, &validation) {
			t.Fatalf("AbandonBatch(%d chars) err = %v, want a ValidationError", len(key), err)
		}
	}
	if sent := api.sent(t); len(sent) != 0 {
		t.Fatalf("sent = %v", sent)
	}
	if _, err := client.InternalTransfers.AbandonBatch(context.Background(), strings.Repeat("k", 128)); err != nil {
		t.Fatalf("a 128-character key: %v", err)
	}
}

// An abandoned answer with a request ID or legs, a status this SDK can't
// name, or a batch without legs breaks the contract.
func TestInternalTransfersAbandonBatchFailsClosedOnAMalformedReply(t *testing.T) {
	leg := &transferv1.InternalTransferBatchLegResult{LegIndex: 0, Status: transferv1.InternalTransferBatchLegStatus_LEG_NOT_MOVED}
	for name, reply := range map[string]*transferv1.AbandonInternalTransferBatchResponse{
		"abandoned with request ID": {Status: transferv1.InternalTransferBatchStatus_BATCH_ABANDONED, RequestId: "1"},
		"abandoned with legs":       {Status: transferv1.InternalTransferBatchStatus_BATCH_ABANDONED, Legs: []*transferv1.InternalTransferBatchLegResult{leg}},
		"unknown status":            {Status: transferv1.InternalTransferBatchStatus(99), RequestId: "1", Legs: []*transferv1.InternalTransferBatchLegResult{leg}},
		"unspecified status":        {},
		"accepted without legs":     {Status: transferv1.InternalTransferBatchStatus_BATCH_ACCEPTED, RequestId: "1"},
	} {
		var contractErr *sdkerrors.ResponseContractError
		_, err := newContractClient(t, &contractAPI{abandon: abandonAnswer(reply)}, nil).InternalTransfers.AbandonBatch(context.Background(), "k")
		if !stderrors.As(err, &contractErr) || contractErr.Operation != "AbandonInternalTransferBatch" {
			t.Fatalf("%s: err = %v, want ResponseContractError", name, err)
		}
	}
}

// An unavailable answer leaves the abandon's outcome unknown, as for any
// write; abandoning again with the same key resolves it.
func TestInternalTransfersAbandonBatchUnknownOutcomeIsResolvedByAbandoningAgain(t *testing.T) {
	calls := 0
	api := &contractAPI{abandon: func(*transferv1.AbandonInternalTransferBatchRequest) (*transferv1.AbandonInternalTransferBatchResponse, error) {
		calls++
		if calls == 1 {
			return nil, transferRefusal(connect.CodeUnavailable, transferv1.ErrorCode_ERROR_CODE_SERVICE_UNAVAILABLE)
		}
		return &transferv1.AbandonInternalTransferBatchResponse{Status: transferv1.InternalTransferBatchStatus_BATCH_ABANDONED}, nil
	}}
	client := newContractClient(t, api, nil)
	_, err := client.InternalTransfers.AbandonBatch(context.Background(), "k")
	var server *sdkerrors.ServerError
	if !stderrors.As(err, &server) || server.Code != "ERROR_CODE_SERVICE_UNAVAILABLE" || !sdkerrors.MutationOutcomeUnknown(err) {
		t.Fatalf("err = %T %v", err, err)
	}
	result, err := client.InternalTransfers.AbandonBatch(context.Background(), "k")
	if err != nil || result.Status != "abandoned" {
		t.Fatalf("second abandon = %+v, %v", result, err)
	}
	if calls != 2 {
		t.Fatalf("calls = %d", calls)
	}
}

// Only AbandonBatch answers an abandoned key: a create or a read by request
// ID that answers BATCH_ABANDONED breaks the contract.
func TestInternalTransfersAbandonedStatusOnlyFromAbandonBatch(t *testing.T) {
	scale, dest := 18, "0x2222222222222222222222222222222222222222"
	legs := []models.InternalTransferBatchLeg{{AssetID: 1, Quantity: models.AssetAmountFromDecimal("1"), QuantityScale: &scale, DestinationSmartAccountAddress: &dest}}
	create := &contractAPI{batch: func(*transferv1.BatchCreateInternalTransfersRequest) *transferv1.BatchCreateInternalTransfersResponse {
		return &transferv1.BatchCreateInternalTransfersResponse{Status: transferv1.InternalTransferBatchStatus_BATCH_ABANDONED}
	}}
	var contractErr *sdkerrors.ResponseContractError
	if _, err := newContractClient(t, create, nil).InternalTransfers.CreateBatch(context.Background(), legs, "k"); !stderrors.As(err, &contractErr) {
		t.Fatalf("CreateBatch err = %v, want ResponseContractError", err)
	}
	read := &contractAPI{batchRead: &transferv1.GetInternalTransferBatchResponse{Status: transferv1.InternalTransferBatchStatus_BATCH_ABANDONED}}
	if _, err := newContractClient(t, read, nil).InternalTransfers.GetBatch(context.Background(), "1"); !stderrors.As(err, &contractErr) {
		t.Fatalf("GetBatch err = %v, want ResponseContractError", err)
	}
}

// A batch under an abandoned key is refused for good: a typed, final
// *errors.APIError, not retryable and not an unknown outcome.
func TestInternalTransfersCreateBatchUnderAnAbandonedKeyIsFinal(t *testing.T) {
	refusing := newContractClient(t, &contractAPI{batchErr: transferRefusal(connect.CodeFailedPrecondition, transferv1.ErrorCode_ERROR_CODE_IDEMPOTENCY_KEY_ABANDONED)}, nil)
	scale, dest := 18, "0x2222222222222222222222222222222222222222"
	_, err := refusing.InternalTransfers.CreateBatch(context.Background(), []models.InternalTransferBatchLeg{
		{AssetID: 1, Quantity: models.AssetAmountFromDecimal("1"), QuantityScale: &scale, DestinationSmartAccountAddress: &dest},
	}, "abandoned-key")
	var apiErr *sdkerrors.APIError
	if !stderrors.As(err, &apiErr) || apiErr.Code != sdkerrors.CodeIdempotencyKeyAbandoned || !sdkerrors.IsIdempotencyKeyAbandoned(err) {
		t.Fatalf("err = %T %+v", err, err)
	}
	if sdkerrors.IsRetryable(err) || sdkerrors.MutationOutcomeUnknown(err) {
		t.Fatalf("retryable=%v unknown=%v", sdkerrors.IsRetryable(err), sdkerrors.MutationOutcomeUnknown(err))
	}
	conflict := newContractClient(t, &contractAPI{batchErr: transferRefusal(connect.CodeAlreadyExists, transferv1.ErrorCode_ERROR_CODE_IDEMPOTENCY_CONFLICT)}, nil)
	_, err = conflict.InternalTransfers.CreateBatch(context.Background(), []models.InternalTransferBatchLeg{
		{AssetID: 1, Quantity: models.AssetAmountFromDecimal("1"), QuantityScale: &scale, DestinationSmartAccountAddress: &dest},
	}, "used-key")
	if err == nil || sdkerrors.IsIdempotencyKeyAbandoned(err) {
		t.Fatalf("an idempotency conflict is not an abandoned key: %v", err)
	}
}
