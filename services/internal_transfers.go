package services

import (
	"context"
	stderrors "errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/Fabric-Labs/polyester-sdk-go/catalogs"
	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	"github.com/Fabric-Labs/polyester-sdk-go/codecs/decode"
	"github.com/Fabric-Labs/polyester-sdk-go/errors"
	typev1 "github.com/Fabric-Labs/polyester-sdk-go/gen/polyester/type/v1"
	transferv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/transfer/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/gen/transfer/v1/transferv1connect"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
	"github.com/Fabric-Labs/polyester-sdk-go/transport"
)

type InternalTransfersService struct {
	transport *transport.Factory
	catalogs  *catalogs.Manager
	scoped    ScopedSubAccount
}

func internalTransferAmountE18(quantity models.AssetAmountInput, quantityScale *int, assetID uint32) (*typev1.U128, error) {
	aid := assetID
	qtyBig, err := codecs.ResolveAssetAmountScaledToScale(quantity, quantityScale, codecs.LedgerScale, "quantity", models.QuantityDomainLedgerE18, &aid)
	if err != nil {
		return nil, err
	}
	return codecs.BigIntToU128Proto(qtyBig), nil
}

func NewInternalTransfersService(factory *transport.Factory, cats *catalogs.Manager, defaultSubAccountID *string) *InternalTransfersService {
	return &InternalTransfersService{transport: factory, catalogs: cats, scoped: ScopedSubAccount{DefaultSubAccountID: defaultSubAccountID}}
}

func (s *InternalTransfersService) client() transferv1connect.InternalTransferServiceClient {
	return transferv1connect.NewInternalTransferServiceClient(s.transport.HTTP, s.transport.Config.APIURL, s.transport.ConnectOptions(true)...)
}

func (s *InternalTransfersService) Create(ctx context.Context, assetID uint32, quantity models.AssetAmountInput, idempotencyKey string, account AccountScope, subAccountID *string, destinationAccountID, destinationSubaccountID, destinationSmartAccountAddress *string, quantityScale *int) (models.InternalTransferResult, error) {
	dest, err := internalTransferDestination(destinationAccountID, destinationSubaccountID, destinationSmartAccountAddress)
	if err != nil {
		return models.InternalTransferResult{}, err
	}
	if strings.TrimSpace(idempotencyKey) == "" {
		return models.InternalTransferResult{}, &errors.ValidationError{Msg: "idempotency_key is required"}
	}
	amountE18, err := internalTransferAmountE18(quantity, quantityScale, assetID)
	if err != nil {
		return models.InternalTransferResult{}, err
	}
	req := &transferv1.CreateInternalTransferRequest{
		AssetId:        assetID,
		AmountE18:      amountE18,
		IdempotencyKey: idempotencyKey,
	}
	if err := s.scoped.ApplyOptionalSubaccountID(&req.SubaccountId, account, subAccountID); err != nil {
		return models.InternalTransferResult{}, err
	}
	switch {
	case dest.accountID != 0:
		req.Destination = &transferv1.CreateInternalTransferRequest_DestinationAccountId{DestinationAccountId: dest.accountID}
	case dest.subaccountID != 0:
		req.Destination = &transferv1.CreateInternalTransferRequest_DestinationSubaccountId{DestinationSubaccountId: dest.subaccountID}
	default:
		req.Destination = &transferv1.CreateInternalTransferRequest_DestinationSmartAccountAddress{DestinationSmartAccountAddress: dest.address}
	}
	return UnaryAuthDecoded(ctx, s.transport, s.client().CreateInternalTransfer, req, decode.InternalTransferFromProto)
}

// CreateBatch moves Trading to Trading transfers together under one
// idempotency key and one MFA check: every leg is credited, or none moves.
// Each leg is checked and converted as InternalTransfers.Create would,
// exactly, before anything is sent. The API takes up to 500 legs, debiting at
// most 50 distinct source account and asset pairs from at most 10 source
// accounts, each source, destination and asset combination at most once; it
// refuses a batch over those limits with ERROR_CODE_INVALID_REQUEST, or
// ERROR_CODE_BATCH_TOO_MANY_SOURCES for more than 10 sources.
//
// The result's Status is "accepted" once every leg is credited, "refused"
// when nothing moved (the legs that caused it carry their ErrorCode; nothing
// will move under this key, so submit again under a new one), or
// "processing" when the outcome is not known within the request: the batch
// completes on its own, and GetBatch with the result's RequestID (or this
// call again with the same key and the same legs in the same order) reads
// it. Different legs under the key are refused with
// ERROR_CODE_IDEMPOTENCY_CONFLICT. A batch counts once against the
// internal-transfer rate limit, with a weight of max(ceil(legs/50), source
// accounts); a RATE_LIMIT_EXCEEDED refusal without a Retry-After means the
// batch outweighs the account's burst: split it instead of retrying. A reply
// that breaks the contract returns a *errors.ResponseContractError; the batch
// may have run, so read it back or retry it with the same key and legs.
//
// "refused" is final for its key even once the cause is gone: the same legs
// under it answer the same refusal. A check that cannot be decided now fails
// the whole call with Connect code unavailable (ERROR_CODE_SERVICE_UNAVAILABLE
// or ERROR_CODE_ACCOUNT_SHARD_UNAVAILABLE) and records nothing under the key:
// that *errors.ServerError is AdmissionRefused, so errors.IsRetryable reports
// it and errors.MutationOutcomeUnknown does not. Retry with the same key and
// legs, after its Retry-After when present. Any other HTTP 502 or 503 answer,
// whatever it names, is an *errors.ServerError with an unknown outcome: read
// the batch back with GetBatch, or retry with the same key and legs.
func (s *InternalTransfersService) CreateBatch(ctx context.Context, legs []models.InternalTransferBatchLeg, idempotencyKey string) (models.InternalTransferBatchResult, error) {
	if len(legs) < codecs.BatchInternalTransferLegsMin || len(legs) > codecs.BatchInternalTransferLegsMax {
		return models.InternalTransferBatchResult{}, &errors.ValidationError{Msg: fmt.Sprintf("a batch takes at least %d leg, not %d", codecs.BatchInternalTransferLegsMin, len(legs))}
	}
	req := &transferv1.BatchCreateInternalTransfersRequest{IdempotencyKey: idempotencyKey, Legs: make([]*transferv1.InternalTransferBatchLeg, 0, len(legs))}
	if err := codecs.CheckContractString(req, "idempotency_key", idempotencyKey, "idempotency_key"); err != nil {
		return models.InternalTransferBatchResult{}, err
	}
	for i, leg := range legs {
		wire, err := s.batchLeg(leg)
		if err != nil {
			var invalid *errors.ValidationError
			if stderrors.As(err, &invalid) {
				return models.InternalTransferBatchResult{}, &errors.ValidationError{Msg: fmt.Sprintf("legs[%d]: %s", i, invalid.Msg)}
			}
			return models.InternalTransferBatchResult{}, err
		}
		req.Legs = append(req.Legs, wire)
	}
	result, err := UnaryAuthDecoded(ctx, s.transport, s.client().BatchCreateInternalTransfers, req, func(msg *transferv1.BatchCreateInternalTransfersResponse) (models.InternalTransferBatchResult, error) {
		return decode.InternalTransferBatchFromProto(msg, len(legs))
	})
	return result, batchAnswerCertainty(err)
}

// batchAnswerCertainty decides a failed batch's outcome certainty from its
// HTTP status, whatever error class the answer maps to. A 502 or 503 proves
// nothing was recorded only as the verified refusal below; any other 502 or
// 503, a rate refusal or a retryable code included, is an unknown outcome:
// read the batch back (GetBatch) or retry it with the same key and legs. Its
// status, Retry-After, code and message are kept for diagnosis. Any other
// status, a 429 rate refusal included, is left as it maps.
func batchAnswerCertainty(err error) error {
	status := errors.AnswerStatus(err)
	if err == nil || (status != http.StatusBadGateway && status != http.StatusServiceUnavailable) {
		return err
	}
	if markBatchRefusedBeforeAdmission(err) {
		return err
	}
	if errors.MutationOutcomeUnknown(err) && !errors.IsRetryable(err) {
		return err
	}
	unknown := &errors.ServerError{Msg: err.Error(), Code: answerCode(err), Status: status, RetryAfter: errors.RetryAfter(err)}
	if !errors.MutationOutcomeUnknown(unknown) {
		// A code that reads as an admission refusal stays in the message.
		unknown.Code = ""
	}
	return unknown
}

// answerCode is the service code a mapped error carries, for diagnosis.
func answerCode(err error) string {
	var (
		rate       *errors.RateLimitError
		api        *errors.APIError
		server     *errors.ServerError
		auth       *errors.AuthError
		validation *errors.ValidationError
	)
	switch {
	case stderrors.As(err, &rate):
		return rate.Code
	case stderrors.As(err, &api):
		return api.Code
	case stderrors.As(err, &server):
		return server.Code
	case stderrors.As(err, &auth):
		return auth.Code
	case stderrors.As(err, &validation):
		return validation.Code
	}
	return ""
}

// markBatchRefusedBeforeAdmission marks a batch the API could not decide
// now as refused before admission: the contract records nothing under its
// key on SERVICE_UNAVAILABLE or ACCOUNT_SHARD_UNAVAILABLE, so it is retryable
// with the same key and legs. Only an actual 503 proves it, with the code its
// transfer.v1.ErrorDetail or a contract REST problem carries: a 504 or 502, a
// foreign detail, a gateway's JSON or the message keep their unknown outcome.
// The answer's status and Retry-After stay as they are. It reports whether
// it marked err.
func markBatchRefusedBeforeAdmission(err error) bool {
	var server *errors.ServerError
	if !stderrors.As(err, &server) || server.Status != http.StatusServiceUnavailable {
		return false
	}
	switch server.CodeSource {
	case batchRefusalDetail, errors.CodeSourceProblem:
	default:
		return false
	}
	switch strings.TrimPrefix(server.Code, "ERROR_CODE_") {
	case "SERVICE_UNAVAILABLE", "ACCOUNT_SHARD_UNAVAILABLE":
		server.AdmissionRefused = true
		return true
	}
	return false
}

// batchRefusalDetail is the ConnectRPC detail that carries a batch's typed
// refusal code.
var batchRefusalDetail = string((&transferv1.ErrorDetail{}).ProtoReflect().Descriptor().FullName())

// GetBatch reads a batch's current outcome and the state of every leg by the
// RequestID CreateBatch returned, as CreateBatch answered it: for example to
// follow a "processing" batch without resubmitting it. It neither submits nor
// retries the batch and needs no fresh MFA. The caller must be allowed to
// read the transfers of every source account of the batch; any other batch
// answers ERROR_CODE_NOT_FOUND, as an unknown one does. A requestID that is
// not a decimal batch ID (1 to 39 digits, no leading zero) is refused with an
// *errors.ValidationError before sending.
func (s *InternalTransfersService) GetBatch(ctx context.Context, requestID string) (models.InternalTransferBatchResult, error) {
	req := &transferv1.GetInternalTransferBatchRequest{RequestId: requestID}
	if err := codecs.CheckContractString(req, "request_id", requestID, "request_id"); err != nil {
		return models.InternalTransferBatchResult{}, err
	}
	return UnaryAuthDecoded(ctx, s.transport, s.client().GetInternalTransferBatch, req, func(msg *transferv1.GetInternalTransferBatchResponse) (models.InternalTransferBatchResult, error) {
		return decode.InternalTransferBatchFromGet(msg, requestID)
	})
}

func (s *InternalTransfersService) batchLeg(leg models.InternalTransferBatchLeg) (*transferv1.InternalTransferBatchLeg, error) {
	if leg.AssetID == 0 {
		return nil, &errors.ValidationError{Msg: "asset_id is required"}
	}
	dest, err := internalTransferDestination(leg.DestinationAccountID, leg.DestinationSubaccountID, leg.DestinationSmartAccountAddress)
	if err != nil {
		return nil, err
	}
	amountE18, err := internalTransferAmountE18(leg.Quantity, leg.QuantityScale, leg.AssetID)
	if err != nil {
		return nil, err
	}
	wire := &transferv1.InternalTransferBatchLeg{AssetId: leg.AssetID, AmountE18: amountE18}
	if err := s.scoped.ApplyOptionalSubaccountID(&wire.SubaccountId, leg.Account, leg.SubAccountID); err != nil {
		return nil, err
	}
	switch {
	case dest.accountID != 0:
		wire.Destination = &transferv1.InternalTransferBatchLeg_DestinationAccountId{DestinationAccountId: dest.accountID}
	case dest.subaccountID != 0:
		wire.Destination = &transferv1.InternalTransferBatchLeg_DestinationSubaccountId{DestinationSubaccountId: dest.subaccountID}
	default:
		wire.Destination = &transferv1.InternalTransferBatchLeg_DestinationSmartAccountAddress{DestinationSmartAccountAddress: dest.address}
	}
	return wire, nil
}

type transferDestination struct {
	accountID    uint64
	subaccountID uint64
	address      string
}

// internalTransferDestination parses the one destination of a transfer.
func internalTransferDestination(destinationAccountID, destinationSubaccountID, destinationSmartAccountAddress *string) (transferDestination, error) {
	// A supplied destination ID counts as given whatever it holds, and is
	// then read strictly: a blank one is refused, not dropped in favour of
	// another destination.
	hasAccount := destinationAccountID != nil
	hasSubaccount := destinationSubaccountID != nil
	hasAddress := destinationSmartAccountAddress != nil && strings.TrimSpace(*destinationSmartAccountAddress) != ""
	destinations := 0
	for _, has := range []bool{hasAccount, hasSubaccount, hasAddress} {
		if has {
			destinations++
		}
	}
	if destinations != 1 {
		return transferDestination{}, &errors.ValidationError{Msg: "create requires exactly one destination"}
	}
	switch {
	case hasAccount:
		id, err := codecs.ParseAccountID(*destinationAccountID, "destination_account_id")
		return transferDestination{accountID: id}, err
	case hasSubaccount:
		id, err := codecs.ParseAccountID(*destinationSubaccountID, "destination_subaccount_id")
		return transferDestination{subaccountID: id}, err
	default:
		return transferDestination{address: *destinationSmartAccountAddress}, nil
	}
}
