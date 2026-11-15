package services

import (
	"context"
	"fmt"
	"iter"

	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	"github.com/Fabric-Labs/polyester-sdk-go/codecs/decode"
	"github.com/Fabric-Labs/polyester-sdk-go/errors"
	transferv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/transfer/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/gen/transfer/v1/transferv1connect"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
	"github.com/Fabric-Labs/polyester-sdk-go/transport"
)

// DustConversionsService converts dust, Trading balances too small to trade,
// into one target asset at index prices less a disclosed fee. A conversion
// moves every source and credits the payout atomically, or moves nothing.
//
// A balance is dust when no open market accepts an order for it at the
// current index price and it is worth at most 10 USD; one conversion takes at
// most 20 balances worth at most 200 USD together; an account converts once
// every 6 hours (refused conversions excepted), and each target has a daily
// limit. No step-up is needed: the value stays in the account.
//
// Every call is signed. Quote, Convert, Get and List need CONVERT_DUST on an
// API key's policy (and, for a sub-account, on the sub-account's policy);
// ListBalances needs READ_BALANCES.
//
// Refusals: ERROR_CODE_DUST_QUOTE_EXPIRED, DUST_PRICE_MOVED,
// DUST_BALANCE_CHANGED, DUST_NOT_ELIGIBLE, DUST_CONVERSION_FAILED and
// DUST_TARGET_UNAVAILABLE mean quote again; DUST_WINDOW_ACTIVE and
// DUST_DAILY_LIMIT mean try much later and are not retryable;
// ERROR_CODE_DUST_BUSY is retryable shortly (errors.IsRetryable).
type DustConversionsService struct {
	transport *transport.Factory
	scoped    ScopedSubAccount
}

func NewDustConversionsService(factory *transport.Factory, defaultSubAccountID *string) *DustConversionsService {
	return &DustConversionsService{transport: factory, scoped: ScopedSubAccount{DefaultSubAccountID: defaultSubAccountID}}
}

func (s *DustConversionsService) client() transferv1connect.DustConversionServiceClient {
	return transferv1connect.NewDustConversionServiceClient(s.transport.HTTP, s.transport.Config.APIURL, s.transport.ConnectOptions(true)...)
}

// ListBalances previews the account's available Trading balances with their
// eligibility and the payout the eligible ones would bring now into
// targetAssetID; 0 judges them for the first target. It reserves nothing.
// Account and subAccountID select the account as for InternalTransfers.Create.
func (s *DustConversionsService) ListBalances(ctx context.Context, targetAssetID uint32, account AccountScope, subAccountID *string) (models.DustBalances, error) {
	req := &transferv1.ListDustBalancesRequest{TargetAssetId: targetAssetID}
	if err := s.scoped.ApplyOptionalSubaccountID(&req.SubaccountId, account, subAccountID); err != nil {
		return models.DustBalances{}, err
	}
	return UnaryAuthDecoded(ctx, s.transport, s.client().ListDustBalances, req, decode.DustBalancesFromProto)
}

// Quote prices the account's dust into targetAssetID (required, one of
// ListBalances' Targets) for 30 seconds. It reserves nothing. A quote with no
// QuoteID converts nothing; its Balances say why.
func (s *DustConversionsService) Quote(ctx context.Context, targetAssetID uint32, account AccountScope, subAccountID *string) (models.DustQuote, error) {
	if targetAssetID == 0 {
		return models.DustQuote{}, &errors.ValidationError{Msg: "target_asset_id is required"}
	}
	req := &transferv1.QuoteDustConversionRequest{TargetAssetId: targetAssetID}
	if err := s.scoped.ApplyOptionalSubaccountID(&req.SubaccountId, account, subAccountID); err != nil {
		return models.DustQuote{}, err
	}
	return UnaryAuthDecoded(ctx, s.transport, s.client().QuoteDustConversion, req, decode.DustQuoteFromProto)
}

// Convert converts a quote at current index prices, or returns the conversion
// already made under idempotencyKey. Every source must still be the whole
// balance the quote showed. The answer is "completed", or "processing" when
// the outcome is not known yet: poll Get with its ConversionID. Retrying with
// the same idempotency key returns the same conversion. A reply without its
// conversion returns a *errors.ResponseContractError; retry with the same key.
func (s *DustConversionsService) Convert(ctx context.Context, quoteID, idempotencyKey string, account AccountScope, subAccountID *string) (models.DustConversion, error) {
	req := &transferv1.CreateDustConversionRequest{QuoteId: quoteID, IdempotencyKey: idempotencyKey}
	if err := codecs.CheckContractString(req, "quote_id", quoteID, "quote_id"); err != nil {
		return models.DustConversion{}, err
	}
	if err := codecs.CheckContractString(req, "idempotency_key", idempotencyKey, "idempotency_key"); err != nil {
		return models.DustConversion{}, err
	}
	if err := s.scoped.ApplyOptionalSubaccountID(&req.SubaccountId, account, subAccountID); err != nil {
		return models.DustConversion{}, err
	}
	return UnaryAuthDecoded(ctx, s.transport, s.client().CreateDustConversion, req, decode.DustConversionFromCreate)
}

// Get reads one of the account's conversions.
func (s *DustConversionsService) Get(ctx context.Context, conversionID string, account AccountScope, subAccountID *string) (models.DustConversion, error) {
	req := &transferv1.GetDustConversionRequest{ConversionId: conversionID}
	if err := codecs.CheckContractString(req, "conversion_id", conversionID, "conversion_id"); err != nil {
		return models.DustConversion{}, err
	}
	if err := s.scoped.ApplyOptionalSubaccountID(&req.SubaccountId, account, subAccountID); err != nil {
		return models.DustConversion{}, err
	}
	return UnaryAuthDecoded(ctx, s.transport, s.client().GetDustConversion, req, decode.DustConversionFromGet)
}

// ListDustConversionsOptions selects a page of conversions.
type ListDustConversionsOptions struct {
	// Account and SubAccountID select the account as for
	// InternalTransfers.Create.
	Account      AccountScope
	SubAccountID *string
	// Limit is the page size; 0 uses the server's default of 20. At most
	// codecs.ListDustConversionsLimitMax.
	Limit int
	// PageToken continues from an earlier page's NextPageToken.
	PageToken string
}

// List reads one page of the account's conversions, newest first.
func (s *DustConversionsService) List(ctx context.Context, opts ListDustConversionsOptions) (models.DustConversionsList, error) {
	limit, err := PaginationLimit(opts.Limit, "limit")
	if err != nil {
		return models.DustConversionsList{}, err
	}
	if limit > codecs.ListDustConversionsLimitMax {
		return models.DustConversionsList{}, &errors.ValidationError{Msg: fmt.Sprintf("limit %d exceeds %d", limit, codecs.ListDustConversionsLimitMax)}
	}
	req := &transferv1.ListDustConversionsRequest{Limit: limit, PageToken: opts.PageToken}
	if err := s.scoped.ApplyOptionalSubaccountID(&req.SubaccountId, opts.Account, opts.SubAccountID); err != nil {
		return models.DustConversionsList{}, err
	}
	return UnaryAuthDecoded(ctx, s.transport, s.client().ListDustConversions, req, decode.DustConversionsListFromProto)
}

// All iterates the account's conversions from opts.PageToken (or the
// newest), following each page's NextPageToken. It stops at the first error.
func (s *DustConversionsService) All(ctx context.Context, opts ListDustConversionsOptions) iter.Seq2[models.DustConversion, error] {
	return paginate(opts.PageToken, func(token string) ([]models.DustConversion, string, error) {
		page := opts
		page.PageToken = token
		list, err := s.List(ctx, page)
		return list.Conversions, list.NextPageToken, err
	})
}
