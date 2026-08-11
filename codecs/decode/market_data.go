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

// MarketTradesFromProtoChecked enforces response timestamp units before
// exposing successful trade rows.
func MarketTradesFromProtoChecked(msg *marketdatav1.GetTradesResponse, quantityScale int) (models.MarketTradesResult, error) {
	for _, trade := range msg.GetTrades() {
		if err := ValidateTimestampNS(trade.GetTsNs(), "GetTrades", "trades.ts_ns"); err != nil {
			return models.MarketTradesResult{}, err
		}
	}
	return MarketTradesFromProto(msg, quantityScale), nil
}

func CandlesFromProto(msg *marketdatav1.GetCandlesResponse, volumeScale int) models.CandlesResult {
	out := make([]models.Candle, 0, len(msg.GetCandles()))
	for _, c := range msg.GetCandles() {
		out = append(out, models.Candle{
			TsSec: int64(c.GetTsSec()),
			Open:  codecs.FormatPriceTicks(c.GetOpen()), High: codecs.FormatPriceTicks(c.GetHigh()),
			Low: codecs.FormatPriceTicks(c.GetLow()), Close: codecs.FormatPriceTicks(c.GetClose()),
			Volume: formatQtyScaledOrEmpty(c.GetVolume(), volumeScale),
		})
	}
	return models.CandlesResult{Candles: out}
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
func CandlesColumnsFromProto(msg *marketdatav1.GetCandlesColumnsResponse, volumeScale int) (models.CandlesResult, error) {
	rows := len(msg.GetTsSec())
	if len(msg.GetOpen()) != rows || len(msg.GetHigh()) != rows ||
		len(msg.GetLow()) != rows || len(msg.GetClose()) != rows ||
		len(msg.GetVolume()) != rows {
		return models.CandlesResult{}, &sdkerrors.TransportError{Msg: fmt.Sprintf(
			"invalid GetCandlesColumns response lengths: ts_sec=%d open=%d high=%d low=%d close=%d volume=%d",
			rows, len(msg.GetOpen()), len(msg.GetHigh()), len(msg.GetLow()),
			len(msg.GetClose()), len(msg.GetVolume()),
		)}
	}
	out := make([]models.Candle, 0, len(msg.GetTsSec()))
	for i, ts := range msg.GetTsSec() {
		candle := models.Candle{
			TsSec:  int64(ts),
			Open:   codecs.FormatPriceTicks(msg.GetOpen()[i]),
			High:   codecs.FormatPriceTicks(msg.GetHigh()[i]),
			Low:    codecs.FormatPriceTicks(msg.GetLow()[i]),
			Close:  codecs.FormatPriceTicks(msg.GetClose()[i]),
			Volume: formatQtyScaledOrEmpty(msg.GetVolume()[i], volumeScale),
		}
		out = append(out, candle)
	}
	tf := timeframeLabels[msg.GetTimeframe()]
	return models.CandlesResult{
		SymbolID: msg.GetSymbolId(), Timeframe: tf, Candles: out, NextPageToken: msg.GetNextPageToken(),
	}, nil
}

// CandlePointFromProto decodes one candle point publication.
func CandlePointFromProto(point *marketdatav1.CandlePoint, symbolID uint32, timeframe string, volumeScale int) models.Candle {
	if point == nil {
		return models.Candle{}
	}
	return models.Candle{
		TsSec:     int64(point.GetTsSec()),
		Open:      codecs.FormatPriceTicks(point.GetOpen()),
		High:      codecs.FormatPriceTicks(point.GetHigh()),
		Low:       codecs.FormatPriceTicks(point.GetLow()),
		Close:     codecs.FormatPriceTicks(point.GetClose()),
		Volume:    formatQtyScaledOrEmpty(point.GetVolume(), volumeScale),
		SymbolID:  symbolID,
		Timeframe: timeframe,
	}
}
