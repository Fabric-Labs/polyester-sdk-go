package decode

import (
	"fmt"
	"strings"

	"google.golang.org/protobuf/reflect/protoreflect"

	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	typev1 "github.com/Fabric-Labs/polyester-sdk-go/gen/polyester/type/v1"
	transferv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/transfer/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

// enumLabel names an enum value in lowercase without prefix: "" for the zero
// value, "unknown(n)" for a value newer than this SDK.
func enumLabel(enum protoreflect.Enum, prefix string) string {
	number := enum.Number()
	if number == 0 {
		return ""
	}
	desc := enum.Descriptor().Values().ByNumber(number)
	if desc == nil {
		return fmt.Sprintf("unknown(%d)", number)
	}
	return strings.ToLower(strings.TrimPrefix(string(desc.Name()), prefix))
}

// transferErrorCodeLabel names a transfer.v1 ErrorCode carried as data, such
// as a refused leg's or conversion's code, the way order reject reasons read:
// lowercase without ERROR_CODE_, "" when unspecified, and
// UNKNOWN_ERROR_CODE(n) for a code newer than this SDK.
func transferErrorCodeLabel(code transferv1.ErrorCode) string {
	raw := int32(code)
	if raw == 0 {
		return ""
	}
	if name, ok := transferv1.ErrorCode_name[raw]; ok {
		return strings.ToLower(strings.TrimPrefix(name, "ERROR_CODE_"))
	}
	return fmt.Sprintf("UNKNOWN_ERROR_CODE(%d)", raw)
}

// ledgerAmount decodes an 18-decimal U128 amount exactly; an absent value is 0.
func ledgerAmount(v *typev1.U128, assetID *uint32) models.AssetAmountScaled {
	return codecs.DecodeAssetAmountBig(codecs.U128ToBig(v.GetHi(), v.GetLo()), codecs.LedgerScale, models.QuantityDomainLedgerE18, assetID)
}

// optionalLedgerAmount decodes an 18-decimal U128 amount; nil when absent.
func optionalLedgerAmount(v *typev1.U128) *models.AssetAmountScaled {
	if v == nil {
		return nil
	}
	out := ledgerAmount(v, nil)
	return &out
}

// internalTransferBatchReply is what BatchCreateInternalTransfers,
// GetInternalTransferBatch and AbandonInternalTransferBatch answer.
type internalTransferBatchReply interface {
	GetRequestId() string
	GetStatus() transferv1.InternalTransferBatchStatus
	GetLegs() []*transferv1.InternalTransferBatchLegResult
}

// InternalTransferBatchFromProto decodes a BatchCreateInternalTransfers
// reply for a request of legs legs. It returns a *errors.ResponseContractError
// when the reply breaks the contract: a batch or leg status this SDK can't
// name, a leg count or order that doesn't match the request, an accepted or
// processing batch without its request ID, a leg whose status is not its
// batch's, a processing or credited leg without its transfer ID, or a
// credited leg without its flow ID. The batch may have run: read it back
// with GetInternalTransferBatch, or retry with the same idempotency key and
// legs.
func InternalTransferBatchFromProto(msg *transferv1.BatchCreateInternalTransfersResponse, legs int) (models.InternalTransferBatchResult, error) {
	return internalTransferBatch("BatchCreateInternalTransfers", msg, legs)
}

// InternalTransferBatchFromGet decodes a GetInternalTransferBatch reply for
// the batch requestID, with the checks of InternalTransferBatchFromProto; the
// reply must name the batch it was asked for and hold at least one leg.
func InternalTransferBatchFromGet(msg *transferv1.GetInternalTransferBatchResponse, requestID string) (models.InternalTransferBatchResult, error) {
	out, err := internalTransferBatch("GetInternalTransferBatch", msg, -1)
	if err != nil {
		return out, err
	}
	if out.RequestID != requestID {
		return models.InternalTransferBatchResult{}, &sdkerrors.ResponseContractError{Operation: "GetInternalTransferBatch", Msg: fmt.Sprintf("reply names batch %q, not %q", out.RequestID, requestID)}
	}
	return out, nil
}

// InternalTransferBatchFromAbandon decodes an AbandonInternalTransferBatch
// reply. BATCH_ABANDONED is Status "abandoned" with no RequestID and no legs:
// no funds moved under the key and none ever will. Any other reply is the
// batch submitted under the key, decoded with the checks of
// InternalTransferBatchFromProto. A *errors.ResponseContractError reports a
// reply that breaks the contract, such as an abandoned batch with a request ID
// or legs; abandoning the key again with the same key is safe.
func InternalTransferBatchFromAbandon(msg *transferv1.AbandonInternalTransferBatchResponse) (models.InternalTransferBatchResult, error) {
	const op = "AbandonInternalTransferBatch"
	if msg.GetStatus() != transferv1.InternalTransferBatchStatus_BATCH_ABANDONED {
		return internalTransferBatch(op, msg, -1)
	}
	if msg.GetRequestId() != "" || len(msg.GetLegs()) != 0 {
		return models.InternalTransferBatchResult{}, &sdkerrors.ResponseContractError{Operation: op, Msg: fmt.Sprintf("abandoned batch has request_id %q and %d leg results", msg.GetRequestId(), len(msg.GetLegs()))}
	}
	return models.InternalTransferBatchResult{Status: "abandoned", Legs: []models.InternalTransferBatchLegResult{}}, nil
}

func internalTransferBatch(op string, msg internalTransferBatchReply, legs int) (models.InternalTransferBatchResult, error) {
	fail := func(format string, args ...any) (models.InternalTransferBatchResult, error) {
		return models.InternalTransferBatchResult{}, &sdkerrors.ResponseContractError{Operation: op, Msg: fmt.Sprintf(format, args...)}
	}
	var status string
	var legStatus transferv1.InternalTransferBatchLegStatus
	switch msg.GetStatus() {
	case transferv1.InternalTransferBatchStatus_BATCH_ACCEPTED:
		status, legStatus = "accepted", transferv1.InternalTransferBatchLegStatus_LEG_CREDITED
	case transferv1.InternalTransferBatchStatus_BATCH_PROCESSING:
		status, legStatus = "processing", transferv1.InternalTransferBatchLegStatus_LEG_PROCESSING
	case transferv1.InternalTransferBatchStatus_BATCH_REFUSED:
		status, legStatus = "refused", transferv1.InternalTransferBatchLegStatus_LEG_NOT_MOVED
	default:
		return fail("batch status %d is not accepted, processing or refused", int32(msg.GetStatus()))
	}
	if status != "refused" && msg.GetRequestId() == "" {
		return fail("%s batch missing request_id", status)
	}
	switch {
	case legs >= 0 && len(msg.GetLegs()) != legs:
		return fail("%d leg results for %d legs", len(msg.GetLegs()), legs)
	case len(msg.GetLegs()) == 0:
		return fail("batch has no leg results")
	}
	out := models.InternalTransferBatchResult{RequestID: msg.GetRequestId(), Status: status, Legs: make([]models.InternalTransferBatchLegResult, 0, len(msg.GetLegs()))}
	for i, leg := range msg.GetLegs() {
		if leg.GetLegIndex() != uint32(i) {
			return fail("leg result %d has leg_index %d", i, leg.GetLegIndex())
		}
		if leg.GetStatus() != legStatus {
			return fail("%s batch has leg %d with status %d", status, i, int32(leg.GetStatus()))
		}
		if legStatus != transferv1.InternalTransferBatchLegStatus_LEG_NOT_MOVED && leg.GetTransferId() == "" {
			return fail("%s leg %d missing transfer_id", enumLabel(legStatus, "LEG_"), i)
		}
		if legStatus == transferv1.InternalTransferBatchLegStatus_LEG_CREDITED && leg.GetFlowId() == "" {
			return fail("credited leg %d missing flow_id", i)
		}
		assetID := leg.GetAssetId()
		out.Legs = append(out.Legs, models.InternalTransferBatchLegResult{
			LegIndex:     leg.GetLegIndex(),
			Status:       enumLabel(leg.GetStatus(), "LEG_"),
			TransferID:   leg.GetTransferId(),
			FlowID:       leg.GetFlowId(),
			CreditedTsNs: leg.GetCreditedAtTsNs(),
			AssetID:      assetID,
			AssetCode:    leg.GetAssetCode(),
			Quantity:     ledgerAmount(leg.GetAmountE18(), &assetID),
			Destination:  resolvedDestination(leg.GetDestination()),
			ErrorCode:    transferErrorCodeLabel(leg.GetErrorCode()),
			ErrorMessage: leg.GetErrorMessage(),
		})
	}
	return out, nil
}

// resolvedDestination decodes a transfer's resolved destination; nil when
// the reply carries none.
func resolvedDestination(msg *transferv1.ResolvedDestination) *models.ResolvedDestination {
	if msg == nil {
		return nil
	}
	return &models.ResolvedDestination{
		RootAccountPublicID: msg.GetRootAccountPublicId(),
		SubaccountPublicID:  msg.GetSubaccountPublicId(),
		SmartAccountAddress: msg.GetSmartAccountAddress(),
	}
}

func dustBalances(in []*transferv1.DustBalance) []models.DustBalance {
	out := make([]models.DustBalance, 0, len(in))
	for _, b := range in {
		assetID := b.GetAssetId()
		out = append(out, models.DustBalance{
			AssetID:     assetID,
			Amount:      ledgerAmount(b.GetAmountE18(), &assetID),
			Eligibility: enumLabel(b.GetEligibility(), "DUST_ASSET_ELIGIBILITY_"),
			USDValue:    optionalLedgerAmount(b.GetUsdValueE18()),
		})
	}
	return out
}

// DustBalancesFromProto decodes a ListDustBalances reply.
func DustBalancesFromProto(msg *transferv1.ListDustBalancesResponse) (models.DustBalances, error) {
	target := msg.GetTargetAssetId()
	return models.DustBalances{
		TargetAssetID:   target,
		Targets:         append([]uint32{}, msg.GetTargets()...),
		FeeBps:          msg.GetFeeBps(),
		Balances:        dustBalances(msg.GetBalances()),
		GrossUSD:        ledgerAmount(msg.GetGrossUsdE18(), nil),
		Payout:          ledgerAmount(msg.GetPayoutE18(), &target),
		TargetAvailable: msg.GetTargetAvailable(),
	}, nil
}

// DustQuoteFromProto decodes a QuoteDustConversion reply. A price whose scale
// this SDK can't format exactly returns a *errors.ResponseContractError.
func DustQuoteFromProto(msg *transferv1.QuoteDustConversionResponse) (models.DustQuote, error) {
	prices := make([]models.DustAssetPrice, 0, len(msg.GetPrices()))
	for _, p := range msg.GetPrices() {
		if p.GetPriceScale() > codecs.MaxProtocolScale {
			return models.DustQuote{}, &sdkerrors.ResponseContractError{Operation: "QuoteDustConversion", Msg: fmt.Sprintf("price of asset %d has scale %d above %d", p.GetAssetId(), p.GetPriceScale(), codecs.MaxProtocolScale)}
		}
		price, err := codecs.FormatQtyScaled(p.GetPriceTicks(), int(p.GetPriceScale()))
		if err != nil {
			return models.DustQuote{}, &sdkerrors.ResponseContractError{Operation: "QuoteDustConversion", Msg: fmt.Sprintf("price of asset %d: %v", p.GetAssetId(), err)}
		}
		prices = append(prices, models.DustAssetPrice{
			AssetID:    p.GetAssetId(),
			Price:      price,
			PriceTicks: p.GetPriceTicks(),
			PriceScale: p.GetPriceScale(),
			AsOf:       timestampTime(p.GetAsOf()),
		})
	}
	target := msg.GetTargetAssetId()
	return models.DustQuote{
		QuoteID:       msg.GetQuoteId(),
		ExpiresAt:     timestampTime(msg.GetExpiresAt()),
		TargetAssetID: target,
		FeeBps:        msg.GetFeeBps(),
		GrossUSD:      ledgerAmount(msg.GetGrossUsdE18(), nil),
		Payout:        ledgerAmount(msg.GetPayoutE18(), &target),
		Balances:      dustBalances(msg.GetBalances()),
		Prices:        prices,
	}, nil
}

func dustConversion(op string, msg *transferv1.DustConversion) (models.DustConversion, error) {
	if msg == nil {
		return models.DustConversion{}, &sdkerrors.ResponseContractError{Operation: op, Msg: "missing conversion"}
	}
	if err := requireStringID(op, "conversion_id", msg.GetConversionId()); err != nil {
		return models.DustConversion{}, err
	}
	target := msg.GetTargetAssetId()
	sources := make([]models.DustSource, 0, len(msg.GetSources()))
	for _, src := range msg.GetSources() {
		assetID := src.GetAssetId()
		sources = append(sources, models.DustSource{AssetID: assetID, Amount: ledgerAmount(src.GetAmountE18(), &assetID)})
	}
	return models.DustConversion{
		ConversionID:  msg.GetConversionId(),
		QuoteID:       msg.GetQuoteId(),
		State:         enumLabel(msg.GetState(), "DUST_CONVERSION_STATE_"),
		RefusalCode:   transferErrorCodeLabel(msg.GetRefusalCode()),
		TargetAssetID: target,
		Sources:       sources,
		Payout:        ledgerAmount(msg.GetPayoutE18(), &target),
		QuotedPayout:  ledgerAmount(msg.GetQuotedPayoutE18(), &target),
		Fee:           ledgerAmount(msg.GetFeeE18(), &target),
		FeeBps:        msg.GetFeeBps(),
		CreatedAt:     timestampTime(msg.GetCreatedAt()),
		CompletedAt:   timestampTime(msg.GetCompletedAt()),
	}, nil
}

// DustConversionFromCreate decodes a CreateDustConversion reply. A reply
// without its conversion or conversion ID returns a
// *errors.ResponseContractError: the conversion may have been made, so read
// it back or retry with the same idempotency key.
func DustConversionFromCreate(msg *transferv1.CreateDustConversionResponse) (models.DustConversion, error) {
	return dustConversion("CreateDustConversion", msg.GetConversion())
}

// DustConversionFromGet decodes a GetDustConversion reply. A reply without
// its conversion or conversion ID returns a *errors.ResponseContractError.
func DustConversionFromGet(msg *transferv1.GetDustConversionResponse) (models.DustConversion, error) {
	return dustConversion("GetDustConversion", msg.GetConversion())
}

// DustConversionsListFromProto decodes one page of conversions. A conversion
// without its ID returns a *errors.ResponseContractError.
func DustConversionsListFromProto(msg *transferv1.ListDustConversionsResponse) (models.DustConversionsList, error) {
	out := make([]models.DustConversion, 0, len(msg.GetConversions()))
	for _, c := range msg.GetConversions() {
		conv, err := dustConversion("ListDustConversions", c)
		if err != nil {
			return models.DustConversionsList{}, err
		}
		out = append(out, conv)
	}
	return models.DustConversionsList{Conversions: out, NextPageToken: msg.GetNextPageToken()}, nil
}
