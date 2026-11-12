package codecs

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/Fabric-Labs/polyester-sdk-go/catalogs"
	"github.com/Fabric-Labs/polyester-sdk-go/errors"
	orderv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/orders/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
	"google.golang.org/protobuf/types/known/timestamppb"
)

var (
	orderSideToProto = map[string]orderv1.Side{
		"buy":  orderv1.Side_BUY,
		"sell": orderv1.Side_SELL,
	}
	modifyBehaviorToProto = map[string]orderv1.ModifyBehavior{
		"amend_or_replace": orderv1.ModifyBehavior_AMEND_OR_REPLACE,
		"amend_only":       orderv1.ModifyBehavior_AMEND_ONLY,
		"replace_only":     orderv1.ModifyBehavior_REPLACE_ONLY,
	}
	feeAssetToProto = map[string]orderv1.FeeAsset{
		"quote": orderv1.FeeAsset_QUOTE,
		"base":  orderv1.FeeAsset_BASE,
	}
	gtdMinOffset = time.Second
	gtdMaxOffset = 30 * 24 * time.Hour
)

func parseGTDExpireAt(value string, now time.Time) (*timestamppb.Timestamp, error) {
	text := strings.TrimSpace(value)
	if text == "" {
		return nil, &errors.ValidationError{Msg: "tif='gtd' requires expires_at"}
	}
	parsed, err := time.Parse(time.RFC3339Nano, text)
	if err != nil {
		parsed, err = time.Parse(time.RFC3339, text)
	}
	if err != nil {
		return nil, &errors.ValidationError{Msg: "expires_at must be an RFC3339 UTC timestamp"}
	}
	parsed = parsed.UTC()
	delta := parsed.Sub(now.UTC())
	if delta < gtdMinOffset || delta > gtdMaxOffset {
		return nil, &errors.ValidationError{Msg: "expires_at must be between 1 second and 30 days after validation"}
	}
	return timestamppb.New(parsed), nil
}

// FormatExpireAt encodes a proto timestamp as RFC3339 UTC, or empty when unset.
func FormatExpireAt(ts *timestamppb.Timestamp) string {
	if ts == nil || !ts.IsValid() {
		return ""
	}
	t := ts.AsTime().UTC()
	if t.Nanosecond() == 0 {
		return t.Format(time.RFC3339)
	}
	return t.Format(time.RFC3339Nano)
}

// ParseOptionalSubaccountID parses an optional sub-account ID to uint64, nil
// when unset. An ID that decodes to 0 is refused, as ParseAccountID does.
func ParseOptionalSubaccountID(value *string) (*uint64, error) {
	if value == nil || *value == "" {
		return nil, nil
	}
	parsed, err := ParseAccountID(*value, "sub_account_id")
	if err != nil {
		return nil, err
	}
	return &parsed, nil
}

// QuantityScaleForSymbol returns quantity scale from the spot catalog.
//
// It does not silently fall back to scale 8 when catalogs or symbol are missing
// or unhydrated; write paths that encode decimal quantities must fail loudly.
func QuantityScaleForSymbol(c *catalogs.Manager, symbol *string) (int, error) {
	if c == nil || symbol == nil || *symbol == "" {
		return 0, &errors.ValidationError{Msg: "quantity scale requires catalogs and symbol"}
	}
	scale, ok := c.BaseQuantityScaleForSymbol(*symbol)
	if !ok {
		return 0, &errors.ValidationError{
			Msg: "quantity scale for " + *symbol + " is unavailable; call WaitForCatalogs before placing orders, or pass a scaled Quantity",
		}
	}
	return scale, nil
}

// QuoteQuantityScaleForSymbol returns quote quantity scale from the spot catalog.
//
// Quote-debit budgets must use this scale. It does not fall back to base scale.
func QuoteQuantityScaleForSymbol(c *catalogs.Manager, symbol *string) (int, error) {
	if c == nil || symbol == nil || *symbol == "" {
		return 0, &errors.ValidationError{Msg: "quote quantity scale requires catalogs and symbol"}
	}
	scale, ok := c.QuoteQuantityScaleForSymbol(*symbol)
	if !ok {
		return 0, &errors.ValidationError{
			Msg: "quote quantity scale for " + *symbol + " is unavailable; call WaitForCatalogs before placing quote-budget orders",
		}
	}
	return scale, nil
}

// OrderIntentToProto encodes the transport-independent OrderIntent shared by
// single and batch create. The flat public params (order_type/tif/post_only)
// are mapped onto the appropriate execution variant.
//
// quoteQuantityScale is required when MaxQuoteDebitScaled or
// QuoteProceedsScaled is set; otherwise it is ignored.
func OrderIntentToProto(req models.CreateOrderRequest, quantityScale, quoteQuantityScale int) (*orderv1.OrderIntent, error) {
	if req.SymbolID == nil || *req.SymbolID == 0 {
		return nil, &errors.ValidationError{Msg: "orders.create requires a resolved non-zero symbol_id"}
	}
	side, ok := orderSideToProto[strings.ToLower(req.Side)]
	if !ok {
		return nil, &errors.ValidationError{Msg: "side must be 'buy' or 'sell'"}
	}
	orderType := strings.ToLower(req.OrderType)
	if orderType != "limit" && orderType != "market" {
		return nil, &errors.ValidationError{Msg: "order_type must be 'limit' or 'market'"}
	}
	symbol := ""
	if req.Symbol != nil {
		symbol = *req.Symbol
	}
	intent := &orderv1.OrderIntent{
		SymbolId: *req.SymbolID,
		Side:     side,
		FeeAsset: orderv1.FeeAsset_QUOTE,
	}
	hasQty := req.Qty.IsSet()
	hasQuoteBudget := req.MaxQuoteDebitScaled.IsSet()
	hasQuoteProceeds := req.QuoteProceedsScaled.IsSet()
	sizings := 0
	for _, set := range []bool{hasQty, hasQuoteBudget, hasQuoteProceeds} {
		if set {
			sizings++
		}
	}
	if sizings != 1 {
		return nil, &errors.ValidationError{Msg: "orders.create requires exactly one of qty, max_quote_debit_scaled or quote_proceeds_scaled"}
	}
	switch {
	case hasQty:
		qty, err := ResolveQtyScaled(req.Qty, quantityScale, "qty", symbol, req.SymbolID)
		if err != nil {
			return nil, err
		}
		intent.Sizing = &orderv1.OrderIntent_BaseQtyScaled{BaseQtyScaled: qty}
	case hasQuoteBudget:
		if side != orderv1.Side_BUY {
			return nil, &errors.ValidationError{Msg: "max_quote_debit_scaled is only valid for buy orders"}
		}
		budget, err := ResolveQuoteQtyScaled(req.MaxQuoteDebitScaled, quoteQuantityScale, "max_quote_debit_scaled", symbol, req.SymbolID)
		if err != nil {
			return nil, err
		}
		intent.Sizing = &orderv1.OrderIntent_MaxQuoteDebitScaled{MaxQuoteDebitScaled: budget}
	default:
		if side != orderv1.Side_SELL {
			return nil, &errors.ValidationError{Msg: "quote_proceeds_scaled is only valid for sell orders"}
		}
		target, err := ResolveQuoteQtyScaled(req.QuoteProceedsScaled, quoteQuantityScale, "quote_proceeds_scaled", symbol, req.SymbolID)
		if err != nil {
			return nil, err
		}
		intent.Sizing = &orderv1.OrderIntent_QuoteProceedsScaled{QuoteProceedsScaled: target}
	}
	clientOrderID, err := optionalClientID(req.ClientOrderID, "client_order_id")
	if err != nil {
		return nil, err
	}
	if clientOrderID != nil {
		intent.ClientOrderId = *clientOrderID
	}
	if req.FeeAsset != nil {
		feeAsset, ok := feeAssetToProto[strings.ToLower(*req.FeeAsset)]
		if !ok {
			return nil, &errors.ValidationError{Msg: "fee_asset must be quote or base"}
		}
		if feeAsset == orderv1.FeeAsset_BASE && side != orderv1.Side_BUY {
			return nil, &errors.ValidationError{Msg: "fee_asset=base is only valid for buy orders"}
		}
		intent.FeeAsset = feeAsset
	}
	if req.SelfTradePreventionMode != nil {
		mode, ok := selfTradePreventionModeToProto[strings.ToLower(*req.SelfTradePreventionMode)]
		if !ok {
			return nil, &errors.ValidationError{Msg: "self_trade_prevention_mode must be expire_taker, expire_maker, expire_both, or decrement"}
		}
		intent.SelfTradePreventionMode = mode
	}

	hasPrice := req.Price != nil && req.Price.IsSet()
	var priceTicks int64
	if hasPrice {
		ticks, err := ResolvePriceTicks(*req.Price, "price", symbol)
		if err != nil {
			return nil, err
		}
		priceTicks = ticks
	}
	tif := ""
	if req.TIF != nil {
		tif = strings.ToLower(*req.TIF)
		if tif != "gtc" && tif != "ioc" && tif != "fok" && tif != "gtd" {
			return nil, &errors.ValidationError{Msg: "tif must be one of 'gtc', 'ioc', 'fok', or 'gtd'"}
		}
	}
	if req.ExpiresAt != nil && strings.TrimSpace(*req.ExpiresAt) != "" && tif != "gtd" {
		return nil, &errors.ValidationError{Msg: "expires_at is only valid for limit GTD orders"}
	}

	switch orderType {
	case "market":
		// The contract's only market execution is MarketIoc: "Market orders
		// always execute as immediate-or-cancel". Any other time in force is
		// refused, never sent as IOC.
		if tif != "" && tif != "ioc" {
			return nil, &errors.ValidationError{Msg: "tif='" + tif + "' is only valid for limit orders"}
		}
		if req.PostOnly {
			return nil, &errors.ValidationError{Msg: "post_only is not supported for market orders"}
		}
		if hasPrice {
			return nil, &errors.ValidationError{
				Msg: "price is not valid for market orders; use market_client_ref_price for a reservation reference",
			}
		}
		market := &orderv1.MarketIoc{}
		if req.MarketClientRefPrice != nil && req.MarketClientRefPrice.IsSet() {
			ticks, err := ResolvePriceTicks(*req.MarketClientRefPrice, "market_client_ref_price", symbol)
			if err != nil {
				return nil, err
			}
			market.ClientRefPriceTicks = ticks
		}
		if err := applyMarketMaxSlippage(market, side, req.MaxSlippageTicks, req.MaxSlippageBps); err != nil {
			return nil, err
		}
		intent.Execution = &orderv1.OrderIntent_MarketIoc{MarketIoc: market}
	case "limit":
		if req.MaxSlippageTicks != nil || req.MaxSlippageBps != nil {
			return nil, &errors.ValidationError{Msg: "max_slippage_ticks and max_slippage_bps are only valid for market orders"}
		}
		if req.MarketClientRefPrice != nil && req.MarketClientRefPrice.IsSet() {
			return nil, &errors.ValidationError{Msg: "market_client_ref_price is only valid for market orders"}
		}
		if !hasPrice {
			return nil, &errors.ValidationError{Msg: "limit orders require price"}
		}
		switch tif {
		case "ioc":
			if req.PostOnly {
				return nil, &errors.ValidationError{Msg: "post_only is not supported for ioc limit orders"}
			}
			intent.Execution = &orderv1.OrderIntent_LimitIoc{LimitIoc: &orderv1.LimitIoc{PriceTicks: priceTicks}}
		case "fok":
			if req.PostOnly {
				return nil, &errors.ValidationError{Msg: "post_only is not supported for fok limit orders"}
			}
			intent.Execution = &orderv1.OrderIntent_LimitFok{LimitFok: &orderv1.LimitFok{PriceTicks: priceTicks}}
		case "gtd":
			expires := ""
			if req.ExpiresAt != nil {
				expires = *req.ExpiresAt
			}
			expireAt, err := parseGTDExpireAt(expires, time.Now())
			if err != nil {
				return nil, err
			}
			intent.Execution = &orderv1.OrderIntent_LimitGtd{LimitGtd: &orderv1.LimitGtd{
				PriceTicks: priceTicks,
				PostOnly:   req.PostOnly,
				ExpireAt:   expireAt,
			}}
		default: // gtc or unspecified
			intent.Execution = &orderv1.OrderIntent_LimitGtc{LimitGtc: &orderv1.LimitGtc{PriceTicks: priceTicks, PostOnly: req.PostOnly}}
		}
	}
	if hasQuoteBudget && orderType == "limit" && tif != "ioc" {
		return nil, &errors.ValidationError{Msg: "max_quote_debit_scaled is only valid for buy market or limit IOC orders"}
	}
	if hasQuoteProceeds && orderType == "limit" && tif != "ioc" {
		return nil, &errors.ValidationError{Msg: "quote_proceeds_scaled is only valid for sell market or limit IOC orders"}
	}
	if req.DisplayQty != nil && req.DisplayQty.IsSet() {
		display, err := resolveDisplayQty(*req.DisplayQty, orderType, tif, quantityScale, symbol, req.SymbolID)
		if err != nil {
			return nil, err
		}
		switch execution := intent.Execution.(type) {
		case *orderv1.OrderIntent_LimitGtc:
			execution.LimitGtc.DisplayQtyScaled = display
		case *orderv1.OrderIntent_LimitGtd:
			execution.LimitGtd.DisplayQtyScaled = display
		}
	}
	if req.Peg != nil {
		peg, err := orderPegToProto(*req.Peg, orderType, hasQty, req.AttachedRisk != nil)
		if err != nil {
			return nil, err
		}
		intent.Peg = peg
	}
	risk, err := RiskPolicyToProto(req.AttachedRisk, "attached_risk", symbol)
	if err != nil {
		return nil, err
	}
	if err := validateTrailingLimitForSide(risk, side, "attached_risk"); err != nil {
		return nil, err
	}
	intent.AttachedRisk = risk
	return intent, nil
}

// resolveDisplayQty encodes an iceberg display quantity. Only limit GTC and
// GTD orders carry one; the API checks it against the order and the pair.
func resolveDisplayQty(in models.QtyInput, orderType, tif string, quantityScale int, symbol string, symbolID *uint32) (int64, error) {
	if orderType != "limit" || (tif != "" && tif != "gtc" && tif != "gtd") {
		return 0, &errors.ValidationError{Msg: "display_qty is only valid for limit GTC or GTD orders"}
	}
	display, err := ResolveQtyScaled(in, quantityScale, "display_qty", symbol, symbolID)
	if err != nil {
		return 0, err
	}
	if display <= 0 {
		return 0, &errors.ValidationError{Msg: "display_qty must be positive; leave it unset for a plain order"}
	}
	return display, nil
}

var pegReferenceToProto = map[string]orderv1.PegReference{
	"best_bid": orderv1.PegReference_BEST_BID,
	"best_ask": orderv1.PegReference_BEST_ASK,
}

// orderPegToProto encodes a static entry peg. The contract allows it only on
// limit orders sized by base quantity and without attached risk.
func orderPegToProto(peg models.OrderPeg, orderType string, baseSized, hasAttachedRisk bool) (*orderv1.StaticPeg, error) {
	reference, ok := pegReferenceToProto[strings.ToLower(strings.TrimSpace(peg.Reference))]
	if !ok {
		return nil, &errors.ValidationError{Msg: "peg.reference must be best_bid or best_ask"}
	}
	switch {
	case orderType != "limit":
		return nil, &errors.ValidationError{Msg: "peg is only valid for limit orders"}
	case !baseSized:
		return nil, &errors.ValidationError{Msg: "peg is only valid for orders sized by qty, not by a quote amount"}
	case hasAttachedRisk:
		return nil, &errors.ValidationError{Msg: "peg is not valid with attached_risk"}
	}
	return &orderv1.StaticPeg{Reference: reference, OffsetTicks: peg.OffsetTicks}, nil
}

func applyMarketMaxSlippage(market *orderv1.MarketIoc, side orderv1.Side, ticks *int64, bps *int32) error {
	if ticks != nil && bps != nil {
		return &errors.ValidationError{Msg: "market_ioc allows at most one of max_slippage_ticks or max_slippage_bps"}
	}
	if ticks != nil {
		if *ticks <= 0 {
			return &errors.ValidationError{Msg: "max_slippage_ticks must be positive"}
		}
		market.MaxSlippage = &orderv1.MarketIoc_MaxSlippageTicks{MaxSlippageTicks: *ticks}
	}
	if bps != nil {
		maxBps := MaxSlippageBpsLimit
		if side == orderv1.Side_SELL {
			maxBps = MaxSellingSlippageBps
		}
		if err := validateBpsAtMost("max_slippage_bps", *bps, false, maxBps); err != nil {
			return err
		}
		market.MaxSlippage = &orderv1.MarketIoc_MaxSlippageBps{MaxSlippageBps: *bps}
	}
	return nil
}

// PreviewOrderToProto encodes the preview request from the same public input
// shape as create. Wire PreviewOrderRequest matches CreateOrderRequest:
// optional subaccount_id plus a full OrderIntent in order.
func PreviewOrderToProto(req models.CreateOrderRequest, quantityScale, quoteQuantityScale int) (*orderv1.PreviewOrderRequest, error) {
	intent, err := OrderIntentToProto(req, quantityScale, quoteQuantityScale)
	if err != nil {
		return nil, err
	}
	proto := &orderv1.PreviewOrderRequest{Order: intent}
	if req.SubAccountID != nil {
		sub, err := ParseAccountID(*req.SubAccountID, "sub_account_id")
		if err != nil {
			return nil, err
		}
		proto.SubaccountId = &sub
	}
	return proto, nil
}

// CreateOrderToProto encodes create order request.
func CreateOrderToProto(req models.CreateOrderRequest, quantityScale, quoteQuantityScale int) (*orderv1.CreateOrderRequest, error) {
	intent, err := OrderIntentToProto(req, quantityScale, quoteQuantityScale)
	if err != nil {
		return nil, err
	}
	proto := &orderv1.CreateOrderRequest{Order: intent}
	if req.SubAccountID != nil {
		sub, err := ParseAccountID(*req.SubAccountID, "sub_account_id")
		if err != nil {
			return nil, err
		}
		proto.SubaccountId = &sub
	}
	return proto, nil
}

// ModifyOrderToProto encodes modify order request.
func ModifyOrderToProto(
	symbol string,
	symbolID uint32,
	key models.OrderKey,
	subAccountID *string,
	requestID *string,
	newPrice *models.PriceInput,
	newQty *models.QtyInput,
	newAttachedRisk *models.AttachedRiskInput,
	behavior *string,
	newClientOrderID *string,
	quantityScale int,
) (*orderv1.ModifyOrderRequest, error) {
	if symbolID == 0 {
		return nil, &errors.ValidationError{Msg: "modify requires a resolved non-zero symbol_id"}
	}
	if (newPrice == nil || !newPrice.IsSet()) && (newQty == nil || !newQty.IsSet()) && newAttachedRisk == nil {
		return nil, &errors.ValidationError{Msg: "modify requires new_price, new_qty, and/or new_attached_risk"}
	}
	resolvedRequestID, err := coalesceRequestID(requestID, "mod")
	if err != nil {
		return nil, err
	}
	proto := &orderv1.ModifyOrderRequest{
		RequestId: resolvedRequestID,
		SymbolId:  symbolID,
	}
	if err := ApplyOrderKeyToModify(proto, key); err != nil {
		return nil, err
	}
	if subAccountID != nil {
		sub, err := ParseAccountID(*subAccountID, "sub_account_id")
		if err != nil {
			return nil, err
		}
		proto.SubaccountId = &sub
	}
	if newPrice != nil && newPrice.IsSet() {
		ticks, err := ResolvePriceTicks(*newPrice, "new_price", symbol)
		if err != nil {
			return nil, err
		}
		proto.NewPriceTicks = &ticks
	}
	if newQty != nil && newQty.IsSet() {
		qty, err := ResolveQtyScaled(*newQty, quantityScale, "new_qty", symbol, &symbolID)
		if err != nil {
			return nil, err
		}
		proto.NewQtyScaled = &qty
	}
	risk, err := RiskPolicyToProto(newAttachedRisk, "new_attached_risk", symbol)
	if err != nil {
		return nil, err
	}
	proto.NewAttachedRisk = risk
	if behavior != nil {
		b, ok := modifyBehaviorToProto[strings.ToLower(*behavior)]
		if !ok {
			return nil, &errors.ValidationError{Msg: "behavior must be amend_or_replace, amend_only, or replace_only"}
		}
		proto.Behavior = b
	}
	validatedNewID, err := optionalClientID(newClientOrderID, "new_client_order_id")
	if err != nil {
		return nil, err
	}
	if validatedNewID != nil {
		proto.NewClientOrderId = *validatedNewID
	}
	return proto, nil
}

const maxCancelAllSymbolIDs = 100

// CancelAllOrdersToProto encodes cancel-all request.
// An empty symbolIDs slice means all symbols.
func CancelAllOrdersToProto(subAccountID *string, symbolIDs []uint32, side *string, dryRun bool, requestID *string) (*orderv1.CancelAllOrdersRequest, error) {
	resolvedRequestID, err := coalesceRequestID(requestID, "cancel-all")
	if err != nil {
		return nil, err
	}
	unique := make([]uint32, 0, len(symbolIDs))
	seen := make(map[uint32]struct{}, len(symbolIDs))
	for _, id := range symbolIDs {
		if id == 0 {
			return nil, &errors.ValidationError{Msg: "cancel_all symbol_ids must be positive"}
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		unique = append(unique, id)
	}
	if len(unique) > maxCancelAllSymbolIDs {
		return nil, &errors.ValidationError{Msg: "cancel_all accepts at most 100 symbol_ids"}
	}
	proto := &orderv1.CancelAllOrdersRequest{
		RequestId: resolvedRequestID,
		DryRun:    dryRun,
		SymbolIds: unique,
	}
	if subAccountID != nil {
		sub, err := ParseAccountID(*subAccountID, "sub_account_id")
		if err != nil {
			return nil, err
		}
		proto.SubaccountId = &sub
	}
	if side != nil {
		s, ok := orderSideToProto[strings.ToLower(*side)]
		if !ok {
			return nil, &errors.ValidationError{Msg: "side must be buy or sell"}
		}
		proto.Side = s
	}
	return proto, nil
}

// CancelAllAfterToProto encodes cancel-all-after request.
// symbolID 0 (or nil) means all symbols.
func CancelAllAfterToProto(subAccountID *string, timeoutSec int, symbolID *uint32, side *string, requestID *string) (*orderv1.CancelAllAfterRequest, error) {
	resolvedRequestID, err := coalesceRequestID(requestID, "cancel-after")
	if err != nil {
		return nil, err
	}
	// The wire field is uint32: a wrapped value would arm a different
	// timeout, and 1<<32 would disable the switch (0).
	if timeoutSec < 0 || uint64(timeoutSec) > math.MaxUint32 {
		return nil, &errors.ValidationError{Msg: fmt.Sprintf("cancel_all_after timeout_sec %d is outside 0..%d", timeoutSec, uint32(math.MaxUint32))}
	}
	proto := &orderv1.CancelAllAfterRequest{
		RequestId:  resolvedRequestID,
		TimeoutSec: uint32(timeoutSec),
	}
	if subAccountID != nil {
		sub, err := ParseAccountID(*subAccountID, "sub_account_id")
		if err != nil {
			return nil, err
		}
		proto.SubaccountId = &sub
	}
	if symbolID != nil {
		proto.SymbolId = *symbolID
	}
	if side != nil {
		s, ok := orderSideToProto[strings.ToLower(*side)]
		if !ok {
			return nil, &errors.ValidationError{Msg: "side must be buy or sell"}
		}
		proto.Side = s
	}
	return proto, nil
}
