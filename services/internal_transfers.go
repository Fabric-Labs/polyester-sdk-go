package services

import (
	"context"
	stderrors "errors"
	"fmt"
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
// accounts). Only a genuine HTTP 429 RATE_LIMIT_EXCEEDED refusal (an
// *errors.RateLimitError answered 429) is a definite throttle: without a
// Retry-After it means the batch outweighs the account's burst, so split it
// into smaller batches under new keys instead of retrying it. The same code
// on a 5xx answer is an unknown outcome: keep the original key and legs. A reply
// that breaks the contract returns a *errors.ResponseContractError; the batch
// may have run, so read it back or retry it with the same key and legs.
//
// "refused" is final for its key even once the cause is gone: the same legs
// under it answer the same refusal. Any HTTP 5xx answer, whatever it names
// (ERROR_CODE_SERVICE_UNAVAILABLE, ERROR_CODE_ACCOUNT_SHARD_UNAVAILABLE and
// RATE_LIMIT_EXCEEDED included), is an unknown outcome: no answer proves that
// nothing was recorded under the key. Check it with
// errors.MutationOutcomeUnknown(err), which reports it, and
// errors.IsRetryable(err), which does not; that is the stable interface, not
// the error's type. An error that is already an unknown outcome keeps its
// type and fields (a plain unavailable *errors.ServerError, a
// *errors.ResponseContractError for a malformed answer, a
// *errors.TransportError for a deadline); any other 5xx is an
// *errors.ServerError with OutcomeUnknown set. Each keeps the answer's
// status, Retry-After and code as mapped.
// The safe next step is the same key and the same legs. Read the batch with
// GetBatch when you already know its request ID (from an earlier answer);
// otherwise send it again with the original key and legs, after its
// Retry-After when present. Under one key the batch runs at most once. To
// give the batch up instead, AbandonBatch with the key answers the batch if
// one was submitted, or makes sure none ever will be. A key that was abandoned
// is refused with an *errors.APIError for which
// errors.IsIdempotencyKeyAbandoned is true (ERROR_CODE_IDEMPOTENCY_KEY_ABANDONED):
// nothing moved under it, and it is not retryable; submit the batch under a
// new key.
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
// HTTP status, whatever error class the answer maps to and whatever code it
// names: a 5xx never proves that nothing was recorded, so it is an unknown
// outcome, a rate refusal or a retryable code included. Read the batch back
// (GetBatch) or send it again with the same key and legs. Its status,
// Retry-After, code and message are kept as mapped. Any other status, a
// genuine 429 rate refusal included, is left as it maps.
func batchAnswerCertainty(err error) error {
	status := errors.AnswerStatus(err)
	if err == nil || status < 500 || status > 599 {
		return err
	}
	// Already an unknown outcome that is not retried, such as a reply that
	// breaks the contract: kept as it is.
	if errors.MutationOutcomeUnknown(err) && !errors.IsRetryable(err) {
		return err
	}
	return &errors.ServerError{
		Msg: err.Error(), Code: answerCode(err), Status: status, RetryAfter: errors.RetryAfter(err), OutcomeUnknown: true,
	}
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

// GetBatch reads a batch's current outcome and the state of every leg by the
// RequestID CreateBatch returned, as CreateBatch answered it: for example to
// follow a "processing" batch without resubmitting it. It needs that request
// ID: a CreateBatch call that failed without an answer returns none, so
// resolve that one by sending the original key and legs again. It neither submits nor
// retries the batch and needs no fresh MFA. An API key whose policy does not
// grant READ_INTERNAL_TRANSFERS is refused for every request ID, whether the
// batch exists or not, with an *errors.AuthError whose Code is
// ERROR_CODE_PERMISSION_DENIED (Connect permission_denied) and whose message
// names the action to grant: grant it on the key's policy rather than
// treating the batch as unknown. A caller allowed to read transfers must be
// allowed to read those of every source account of the batch; any other
// batch answers ERROR_CODE_NOT_FOUND, as an unknown one does. A requestID that is
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

// AbandonBatch makes the absence of a batch under an idempotency key final,
// for example after a CreateBatch call whose answer was lost and that may never
// have reached the server. If a batch was submitted under the key, it answers
// that batch's outcome ("accepted", "processing" or "refused", as GetBatch
// reads it) and changes nothing. Otherwise it records the key as abandoned
// and answers Status "abandoned", with no RequestID and no legs: no funds
// moved under the key and none ever will. Every later CreateBatch under the
// key, a retry included, is then refused with an *errors.APIError for which
// errors.IsIdempotencyKeyAbandoned is true, and every later AbandonBatch
// under it answers "abandoned" again. A CreateBatch and an AbandonBatch that
// race on one key end in exactly one of the two outcomes.
//
// The key is scoped to the caller as CreateBatch scopes it: the API key. The
// key's policy must grant INTERNAL_TRANSFER, and to answer an existing batch
// also the read permission on every source account of the batch, refused as
// GetBatch refuses it. It needs no MFA, as it never moves funds, and counts
// as one CreateBatch against the rate limit. Calling it again with the same
// key is always safe: an error whose outcome is unknown
// (errors.MutationOutcomeUnknown, such as an unavailable answer or a timeout)
// is resolved by calling it again with the same key. An idempotencyKey outside the contract's limits (an empty one
// included) is refused with an *errors.ValidationError before sending.
func (s *InternalTransfersService) AbandonBatch(ctx context.Context, idempotencyKey string) (models.InternalTransferBatchResult, error) {
	req := &transferv1.AbandonInternalTransferBatchRequest{IdempotencyKey: idempotencyKey}
	if err := codecs.CheckContractString(req, "idempotency_key", idempotencyKey, "idempotency_key"); err != nil {
		return models.InternalTransferBatchResult{}, err
	}
	return UnaryAuthDecoded(ctx, s.transport, s.client().AbandonInternalTransferBatch, req, decode.InternalTransferBatchFromAbandon)
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
