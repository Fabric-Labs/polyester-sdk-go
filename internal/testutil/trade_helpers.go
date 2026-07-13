package testutil

import (
	"context"
	"errors"
	"math"
	"math/big"
	"os"
	"strconv"
	"strings"

	"github.com/Fabric-Labs/polyester-sdk-go"
	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

var farBelowBuyPriceHints = map[string]string{
	"ETH-USDT": "100",
	"BTC-USDT": "1000",
	"SOL-USDT": "10",
	"BNB-USDT": "10",
}

var farAboveBuyStopPriceHints = map[string]string{
	"ETH-USDT": "50000",
	"BTC-USDT": "500000",
	"SOL-USDT": "5000",
	"BNB-USDT": "5000",
}

// PairForSymbol returns spot pair metadata for a symbol.
func PairForSymbol(spotRaw map[string]any, symbol string) map[string]any {
	for _, key := range []string{"pairs", "symbols"} {
		raw, _ := spotRaw[key].([]any)
		for _, item := range raw {
			pair, _ := item.(map[string]any)
			if sym, _ := pair["symbol"].(string); sym == symbol {
				return pair
			}
		}
	}
	return nil
}

// SkipFundingCheck reports whether balance pre-checks are disabled.
func SkipFundingCheck() bool {
	return EnvTruthy("POLYESTER_TEST_SKIP_FUNDING_CHECK")
}

// MinTradingQuoteRequired returns the minimum quote balance for trading tests.
func MinTradingQuoteRequired() *big.Int {
	raw := strings.TrimSpace(os.Getenv("POLYESTER_TEST_MIN_TRADING_QUOTE"))
	if raw == "" {
		raw = "10"
	}
	n, ok := new(big.Int).SetString(raw, 10)
	if !ok {
		return big.NewInt(10)
	}
	return n
}

// TradingBalance returns the trading ledger balance for an asset id (u128 wire scale).
func TradingBalance(balances models.BalancesList, assetID uint32) *big.Int {
	for _, row := range balances.Balances {
		if row.AssetID == assetID {
			n, ok := new(big.Int).SetString(row.Trading, 10)
			if !ok {
				return big.NewInt(0)
			}
			return n
		}
	}
	return big.NewInt(0)
}

// FundingBalance returns the funding ledger balance for an asset id (u128 wire scale).
func FundingBalance(balances models.BalancesList, assetID uint32) *big.Int {
	for _, row := range balances.Balances {
		if row.AssetID == assetID {
			n, ok := new(big.Int).SetString(row.Funding, 10)
			if !ok {
				return big.NewInt(0)
			}
			return n
		}
	}
	return big.NewInt(0)
}

// TradingBalanceHuman returns the human-readable trading balance for an asset id.
func TradingBalanceHuman(balances models.BalancesList, assetID uint32) *big.Rat {
	raw := TradingBalance(balances, assetID)
	formatted := codecs.FormatLedgerU128(raw.String(), codecs.LedgerScale)
	rat, ok := new(big.Rat).SetString(formatted)
	if !ok {
		return new(big.Rat)
	}
	return rat
}

// BaseAssetIDForSymbol resolves the base asset ledger id for a pair.
func BaseAssetIDForSymbol(spotRaw map[string]any, symbol string, zipper models.DepositWithdrawConfig) *uint32 {
	pair := PairForSymbol(spotRaw, symbol)
	if pair == nil {
		return nil
	}
	if direct := intish(pair["base_asset_id"]); direct != nil {
		return direct
	}
	if direct := intish(pair["baseAssetId"]); direct != nil {
		return direct
	}
	baseSymbol := stringish(pair["baseAsset"])
	if baseSymbol == "" {
		baseSymbol = stringish(pair["base_asset"])
	}
	if baseSymbol == "" {
		return nil
	}
	for _, asset := range zipper.Assets {
		if asset.Asset == baseSymbol && asset.LedgerID > 0 {
			id := asset.LedgerID
			return &id
		}
	}
	return nil
}

// QuoteAssetIDForSymbol resolves the quote asset ledger id for a pair.
func QuoteAssetIDForSymbol(spotRaw map[string]any, symbol string, zipper models.DepositWithdrawConfig) *uint32 {
	pair := PairForSymbol(spotRaw, symbol)
	if pair == nil {
		return nil
	}
	if direct := intish(pair["quote_asset_id"]); direct != nil {
		return direct
	}
	if direct := intish(pair["quoteAssetId"]); direct != nil {
		return direct
	}
	quoteSymbol := stringish(pair["quoteAsset"])
	if quoteSymbol == "" {
		quoteSymbol = stringish(pair["quote_asset"])
	}
	if quoteSymbol == "" {
		return nil
	}
	for _, asset := range zipper.Assets {
		if asset.Asset == quoteSymbol && asset.LedgerID > 0 {
			id := asset.LedgerID
			return &id
		}
	}
	return nil
}

// FarAboveBuyStopPrice returns a buy-stop trigger price safely above typical devnet spot.
func FarAboveBuyStopPrice(symbol string, pair map[string]any) string {
	if override := strings.TrimSpace(os.Getenv("POLYESTER_TEST_TRIGGER_PRICE")); override != "" {
		return override
	}
	if hint, ok := farAboveBuyStopPriceHints[symbol]; ok {
		return hint
	}
	_ = pair
	return "50000"
}

// MarketRefPrice returns a client reference price for MARKET order reservation.
func MarketRefPrice(ctx context.Context, client *polyester.Client, symbol, side string, pair map[string]any) string {
	if override := strings.TrimSpace(os.Getenv("POLYESTER_TEST_TRADE_PRICE")); override != "" {
		return override
	}
	if client != nil {
		book, err := client.Orderbook.Get(ctx, symbol, 5)
		if err == nil {
			switch strings.ToLower(strings.TrimSpace(side)) {
			case "sell":
				if len(book.Bids) > 0 && book.Bids[0].Price.Ticks > 0 {
					return book.Bids[0].Price.Format()
				}
			default:
				if len(book.Asks) > 0 && book.Asks[0].Price.Ticks > 0 {
					return book.Asks[0].Price.Format()
				}
			}
			if len(book.Asks) > 0 && book.Asks[0].Price.Ticks > 0 {
				return book.Asks[0].Price.Format()
			}
			if len(book.Bids) > 0 && book.Bids[0].Price.Ticks > 0 {
				return book.Bids[0].Price.Format()
			}
		}
		overview, err := client.MarketOverview.List(ctx, []string{symbol}, 5, "", false)
		if err == nil {
			for _, row := range overview.Markets {
				if row.Symbol != symbol || row.LastPrice.Ticks <= 0 {
					continue
				}
				return row.LastPrice.Format()
			}
		}
	}
	return FarAboveBuyStopPrice(symbol, pair)
}

// ResolvePostOnlyBuyLimitPrice returns a post-only buy price slightly below the live book.
func ResolvePostOnlyBuyLimitPrice(ctx context.Context, client *polyester.Client, symbol string, pair map[string]any) string {
	if override := strings.TrimSpace(os.Getenv("POLYESTER_TEST_PRICE")); override != "" {
		return override
	}
	if override := strings.TrimSpace(os.Getenv("POLYESTER_SMOKE_PRICE")); override != "" {
		return override
	}
	tickSize := decimalString(pair["tickSize"], pair["tick_size"], "0.01")
	if client != nil {
		book, err := client.Orderbook.Get(ctx, symbol, 5)
		if err == nil {
			if price, ok := postOnlyBuyPriceFromBook(book, tickSize); ok {
				return price
			}
		}
		overview, err := client.MarketOverview.List(ctx, []string{symbol}, 5, "", false)
		if err == nil {
			for _, row := range overview.Markets {
				if row.Symbol != symbol || row.LastPrice.Ticks <= 0 {
					continue
				}
				if price, ok := postOnlyBuyPriceFromLastTicks(row.LastPrice.Ticks, tickSize, symbol); ok {
					return price
				}
			}
		}
	}
	if hint, ok := farBelowBuyPriceHints[symbol]; ok {
		return hint
	}
	return "100"
}

// FarBelowBuyLimitPrice is deprecated; use ResolvePostOnlyBuyLimitPrice for live market pricing.
func FarBelowBuyLimitPrice(symbol string, pair map[string]any) string {
	return ResolvePostOnlyBuyLimitPrice(context.Background(), nil, symbol, pair)
}

func postOnlyBuyPriceFromBook(book models.OrderbookData, tickSize string) (string, bool) {
	if len(book.Bids) == 0 {
		return "", false
	}
	tickTicks, err := codecs.ParsePriceTicks(tickSize, "tick_size")
	if err != nil || tickTicks == 0 {
		return "", false
	}
	bidTicks := book.Bids[0].Price.Ticks
	target := bidTicks - int64(tickTicks)
	if target < int64(tickTicks) {
		target = int64(tickTicks)
	}
	if len(book.Asks) > 0 && book.Asks[0].Price.Ticks > 0 {
		askTicks := book.Asks[0].Price.Ticks
		maxPostOnly := askTicks - int64(tickTicks)
		if target > maxPostOnly {
			target = maxPostOnly
		}
	}
	if target < int64(tickTicks) {
		return "", false
	}
	return codecs.FormatPriceTicks(target), true
}

func postOnlyBuyPriceFromLastTicks(lastTicks int64, tickSize, symbol string) (string, bool) {
	if lastTicks <= 0 {
		return "", false
	}
	tickTicks, err := codecs.ParsePriceTicks(tickSize, "tick_size")
	if err != nil || tickTicks == 0 {
		return "", false
	}
	target := lastTicks * 995 / 1000
	aligned := (target / int64(tickTicks)) * int64(tickTicks)
	if aligned < int64(tickTicks) {
		if hint, ok := farBelowBuyPriceHints[symbol]; ok {
			return hint, true
		}
		aligned = int64(tickTicks)
	}
	return codecs.FormatPriceTicks(aligned), true
}

// MinBaseQtyForPair returns a minimum base quantity for a limit order.
func MinBaseQtyForPair(pair map[string]any, price string) string {
	if qty := strings.TrimSpace(os.Getenv("POLYESTER_TEST_QTY")); qty != "" {
		return qty
	}
	if qty := strings.TrimSpace(os.Getenv("POLYESTER_SMOKE_QTY")); qty != "" {
		return qty
	}
	stepStr := decimalString(pair["stepSize"], pair["step_size"], "0.001")
	minNotionalStr := decimalString(pair["minNotionalQuote"], pair["min_notional_quote"], "10")
	minQtyStr := decimalString(pair["minQtyBase"], pair["min_qty_base"], stepStr)

	stepF, err := strconv.ParseFloat(stepStr, 64)
	if err != nil || stepF <= 0 {
		return stepStr
	}
	priceF, err := strconv.ParseFloat(price, 64)
	if err != nil || priceF <= 0 {
		return stepStr
	}
	minNotionalF, _ := strconv.ParseFloat(minNotionalStr, 64)
	if minNotionalF <= 0 {
		minNotionalF = 10
	}
	minQtyF, _ := strconv.ParseFloat(minQtyStr, 64)
	if minQtyF <= 0 {
		minQtyF = stepF
	}

	units := math.Ceil(minNotionalF / priceF / stepF)
	minUnits := math.Ceil(minQtyF / stepF)
	if units < minUnits {
		units = minUnits
	}
	if units < 1 {
		units = 1
	}
	return strconv.FormatFloat(units*stepF, 'f', -1, 64)
}

func decimalString(values ...any) string {
	for _, value := range values {
		if s := stringish(value); s != "" {
			return s
		}
	}
	return ""
}

func intish(v any) *uint32 {
	switch t := v.(type) {
	case float64:
		u := uint32(t)
		return &u
	case int:
		u := uint32(t)
		return &u
	case int64:
		u := uint32(t)
		return &u
	case uint32:
		return &t
	default:
		return nil
	}
}

func stringish(v any) string {
	switch t := v.(type) {
	case string:
		return t
	default:
		return ""
	}
}

// DevnetBackendUnavailable reports likely devnet backend connectivity failures.
func DevnetBackendUnavailable(err error) bool {
	if err == nil {
		return false
	}
	if DevnetUnavailable(err) || IsDevnetOrderInternalError(err) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "connection refused") ||
		strings.Contains(msg, "context deadline exceeded") ||
		strings.Contains(msg, "client.timeout exceeded")
}

// IsDevnetOrderInternalError reports likely devnet OMS internal failures.
func IsDevnetOrderInternalError(err error) bool {
	if err == nil {
		return false
	}
	var srv *sdkerrors.ServerError
	if errors.As(err, &srv) {
		return strings.Contains(strings.ToLower(srv.Msg), "internal error")
	}
	var api *sdkerrors.APIError
	if errors.As(err, &api) {
		code := strings.ToUpper(strings.TrimSpace(api.Code))
		return code == "INTERNAL" || code == "INTERNAL_ERROR" || strings.Contains(strings.ToLower(api.Msg), "internal error")
	}
	return false
}

// DevnetOrderNotIndexedError indicates create succeeded but read APIs never indexed the order.
type DevnetOrderNotIndexedError struct{ Msg string }

func (e *DevnetOrderNotIndexedError) Error() string { return e.Msg }
