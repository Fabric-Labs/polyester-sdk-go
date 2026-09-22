package decode

import (
	"fmt"
	"strconv"

	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	marketdatav1 "github.com/Fabric-Labs/polyester-sdk-go/gen/marketdata/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
	"github.com/Fabric-Labs/polyester-sdk-go/wire"
)

func SpotConfigFromProto(msg *marketdatav1.GetSpotConfigResponse) models.SpotConfig {
	raw, err := wire.ProtoToMap(msg)
	if err != nil {
		return models.SpotConfig{}
	}
	// proto3 omits scalar zeroes during proto-JSON conversion. Scale zero is
	// valid, so restore the typed wire value instead of treating it as missing.
	if assets, ok := raw["assets"].([]any); ok {
		for i, typed := range msg.GetAssets() {
			if i >= len(assets) {
				break
			}
			if asset, ok := assets[i].(map[string]any); ok {
				// proto3 omits scalar zeroes. Scale zero is valid for public
				// candle and market-overview base volume.
				delete(asset, "market_data_volume_scale")
				delete(asset, "marketDataVolumeScale")
				asset["market_data_volume_scale"] = float64(typed.GetMarketDataVolumeScale())
			}
		}
	}
	if pairs, ok := raw["pairs"].([]any); ok {
		for i, typed := range msg.GetPairs() {
			if i >= len(pairs) {
				break
			}
			if pair, ok := pairs[i].(map[string]any); ok {
				pair["base_quantity_scale"] = float64(typed.GetBaseQuantityScale())
				// proto3 omits scalar zeroes; restore typed quote scale when present
				// on the wire (including valid zero).
				delete(pair, "quote_quantity_scale")
				delete(pair, "quoteQuantityScale")
				pair["quote_quantity_scale"] = float64(typed.GetQuoteQuantityScale())
				delete(pair, "reference_price_scale")
				delete(pair, "referencePriceScale")
				pair["reference_price_scale"] = float64(typed.GetReferencePriceScale())
			}
		}
	}
	return models.SpotConfig{Raw: raw}
}

func MarketTradesFromProto(msg *marketdatav1.GetTradesResponse, quantityScale int) models.MarketTradesResult {
	out := make([]models.MarketTrade, 0, len(msg.GetTrades()))
	for _, t := range msg.GetTrades() {
		side := "sell"
		if t.GetIsBuy() {
			side = "buy"
		}
		out = append(out, models.MarketTrade{
			SymbolID: t.GetSymbolId(), MatchID: strconv.FormatUint(t.GetMatchId(), 10),
			Price: codecs.DecodePriceTicks(t.GetPriceTicks(), ""),
			Qty:   codecs.DecodeQtyScaled(t.GetQtyScaled(), quantityScale, "", nil),
			TsNs:  strconv.FormatUint(t.GetTsNs(), 10), Side: side,
		})
	}
	return models.MarketTradesResult{Trades: out, NextPageToken: msg.GetNextPageToken()}
}

func CandlesFromProto(msg *marketdatav1.GetCandlesResponse, volumeScale int, referencePriceScale int) (models.CandlesResult, error) {
	if err := codecs.ValidateProtocolScale(volumeScale); err != nil {
		return models.CandlesResult{}, err
	}
	out := make([]models.Candle, 0, len(msg.GetCandles()))
	for _, c := range msg.GetCandles() {
		out = append(out, candleFromPoint(c, volumeScale, codecs.PriceTickScale, true))
	}
	reference, err := candlesFromPoints(msg.GetReferenceCandles(), volumeScale, referencePriceScale, false)
	if err != nil {
		return models.CandlesResult{}, err
	}
	return models.CandlesResult{Candles: out, ReferenceCandles: reference}, nil
}

func candlesFromPoints(points []*marketdatav1.CandlePoint, volumeScale int, priceScale int, primary bool) ([]models.Candle, error) {
	if len(points) == 0 {
		return nil, nil
	}
	if err := codecs.ValidateProtocolScale(priceScale); err != nil {
		return nil, err
	}
	out := make([]models.Candle, 0, len(points))
	for _, point := range points {
		out = append(out, candleFromPoint(point, volumeScale, priceScale, primary))
	}
	return out, nil
}

func candleFromPoint(point *marketdatav1.CandlePoint, volumeScale int, priceScale int, primary bool) models.Candle {
	if point == nil {
		return models.Candle{}
	}
	formatPrice := func(ticks int64) string {
		if primary {
			return codecs.FormatPriceTicks(ticks)
		}
		return formatQtyScaledOrEmpty(ticks, priceScale)
	}
	return models.Candle{
		TsSec:       int64(point.GetTsSec()),
		Open:        formatPrice(point.GetOpen()),
		High:        formatPrice(point.GetHigh()),
		Low:         formatPrice(point.GetLow()),
		Close:       formatPrice(point.GetClose()),
		Volume:      formatQtyScaledOrEmpty(point.GetVolume(), volumeScale),
		QuoteVolume: point.GetQuoteVolume(),
	}
}

var timeframeLabels = map[marketdatav1.Timeframe]string{
	marketdatav1.Timeframe_SEC_1: "1s", marketdatav1.Timeframe_MIN_1: "1m",
	marketdatav1.Timeframe_MIN_5: "5m", marketdatav1.Timeframe_MIN_15: "15m",
	marketdatav1.Timeframe_MIN_30: "30m", marketdatav1.Timeframe_HOUR_1: "1h",
	marketdatav1.Timeframe_HOUR_4: "4h", marketdatav1.Timeframe_HOUR_12: "12h",
	marketdatav1.Timeframe_DAY_1: "1d", marketdatav1.Timeframe_WEEK_1: "1w",
	marketdatav1.Timeframe_MONTH_1: "1mo",
}

// CandlesColumnsFromProto decodes columnar candle responses into rows.
// Primary OHLC stays on price scale 6. Reference OHLC uses referencePriceScale.
// Both volume series use the base asset market_data_volume_scale.
func CandlesColumnsFromProto(msg *marketdatav1.GetCandlesColumnsResponse, volumeScale int, referencePriceScale int) (models.CandlesResult, error) {
	if err := codecs.ValidateProtocolScale(volumeScale); err != nil {
		return models.CandlesResult{}, err
	}
	rows := len(msg.GetTsSec())
	quoteVolumes := msg.GetQuoteVolume()
	if len(msg.GetOpen()) != rows || len(msg.GetHigh()) != rows ||
		len(msg.GetLow()) != rows || len(msg.GetClose()) != rows ||
		len(msg.GetVolume()) != rows || (len(quoteVolumes) > 0 && len(quoteVolumes) != rows) {
		return models.CandlesResult{}, &sdkerrors.TransportError{Msg: fmt.Sprintf(
			"invalid GetCandlesColumns response lengths: ts_sec=%d open=%d high=%d low=%d close=%d volume=%d quote_volume=%d",
			rows, len(msg.GetOpen()), len(msg.GetHigh()), len(msg.GetLow()),
			len(msg.GetClose()), len(msg.GetVolume()), len(quoteVolumes),
		)}
	}
	reference, err := referenceCandlesFromColumns(msg, volumeScale, referencePriceScale)
	if err != nil {
		return models.CandlesResult{}, err
	}
	out := make([]models.Candle, 0, len(msg.GetTsSec()))
	for i, ts := range msg.GetTsSec() {
		quoteVolume := ""
		if i < len(quoteVolumes) {
			quoteVolume = quoteVolumes[i]
		}
		candle := models.Candle{
			TsSec:       int64(ts),
			Open:        codecs.FormatPriceTicks(msg.GetOpen()[i]),
			High:        codecs.FormatPriceTicks(msg.GetHigh()[i]),
			Low:         codecs.FormatPriceTicks(msg.GetLow()[i]),
			Close:       codecs.FormatPriceTicks(msg.GetClose()[i]),
			Volume:      formatQtyScaledOrEmpty(msg.GetVolume()[i], volumeScale),
			QuoteVolume: quoteVolume,
		}
		out = append(out, candle)
	}
	tf := timeframeLabels[msg.GetTimeframe()]
	return models.CandlesResult{
		SymbolID: msg.GetSymbolId(), Timeframe: tf, Candles: out, ReferenceCandles: reference, NextPageToken: msg.GetNextPageToken(),
	}, nil
}

func referenceCandlesFromColumns(msg *marketdatav1.GetCandlesColumnsResponse, volumeScale int, referencePriceScale int) ([]models.Candle, error) {
	rows := len(msg.GetReferenceTsSec())
	if rows == 0 && len(msg.GetReferenceOpen()) == 0 && len(msg.GetReferenceHigh()) == 0 &&
		len(msg.GetReferenceLow()) == 0 && len(msg.GetReferenceClose()) == 0 && len(msg.GetReferenceVolume()) == 0 {
		return nil, nil
	}
	if len(msg.GetReferenceOpen()) != rows || len(msg.GetReferenceHigh()) != rows ||
		len(msg.GetReferenceLow()) != rows || len(msg.GetReferenceClose()) != rows ||
		len(msg.GetReferenceVolume()) != rows {
		return nil, &sdkerrors.TransportError{Msg: fmt.Sprintf(
			"invalid GetCandlesColumns reference lengths: reference_ts_sec=%d reference_open=%d reference_high=%d reference_low=%d reference_close=%d reference_volume=%d",
			rows, len(msg.GetReferenceOpen()), len(msg.GetReferenceHigh()), len(msg.GetReferenceLow()),
			len(msg.GetReferenceClose()), len(msg.GetReferenceVolume()),
		)}
	}
	if err := codecs.ValidateProtocolScale(referencePriceScale); err != nil {
		return nil, err
	}
	out := make([]models.Candle, 0, rows)
	for i, ts := range msg.GetReferenceTsSec() {
		out = append(out, models.Candle{
			TsSec:  int64(ts),
			Open:   formatQtyScaledOrEmpty(msg.GetReferenceOpen()[i], referencePriceScale),
			High:   formatQtyScaledOrEmpty(msg.GetReferenceHigh()[i], referencePriceScale),
			Low:    formatQtyScaledOrEmpty(msg.GetReferenceLow()[i], referencePriceScale),
			Close:  formatQtyScaledOrEmpty(msg.GetReferenceClose()[i], referencePriceScale),
			Volume: formatQtyScaledOrEmpty(msg.GetReferenceVolume()[i], volumeScale),
		})
	}
	return out, nil
}

// CandlePointFromProto decodes one candle point publication.
func CandlePointFromProto(point *marketdatav1.CandlePoint, symbolID uint32, timeframe string, volumeScale int) models.Candle {
	if point == nil {
		return models.Candle{}
	}
	return models.Candle{
		TsSec:       int64(point.GetTsSec()),
		Open:        codecs.FormatPriceTicks(point.GetOpen()),
		High:        codecs.FormatPriceTicks(point.GetHigh()),
		Low:         codecs.FormatPriceTicks(point.GetLow()),
		Close:       codecs.FormatPriceTicks(point.GetClose()),
		Volume:      formatQtyScaledOrEmpty(point.GetVolume(), volumeScale),
		QuoteVolume: point.GetQuoteVolume(),
		SymbolID:    symbolID,
		Timeframe:   timeframe,
	}
}
