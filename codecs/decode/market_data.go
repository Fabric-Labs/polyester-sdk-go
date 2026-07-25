package decode

import (
	"strconv"

	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	marketdatav1 "github.com/Fabric-Labs/polyester-sdk-go/gen/marketdata/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
	"github.com/Fabric-Labs/polyester-sdk-go/wire"
)

func SpotConfigFromProto(msg *marketdatav1.GetSpotConfigResponse) models.SpotConfig {
	raw, err := wire.ProtoToMap(msg)
	if err != nil {
		return models.SpotConfig{}
	}
	return models.SpotConfig{Raw: raw}
}

func MarketTradesFromProto(msg *marketdatav1.GetTradesResponse) models.MarketTradesResult {
	out := make([]models.MarketTrade, 0, len(msg.GetTrades()))
	for _, t := range msg.GetTrades() {
		side := "sell"
		if t.GetIsBuy() {
			side = "buy"
		}
		out = append(out, models.MarketTrade{
			SymbolID: t.GetSymbolId(), MatchID: strconv.FormatUint(t.GetMatchId(), 10),
			Price: codecs.DecodePriceTicks(t.GetPriceTicks(), ""),
			Qty:   codecs.DecodeQtyScaled(t.GetQtyScaled(), -1, "", nil),
			TsNs:  strconv.FormatUint(t.GetTsNs(), 10), Side: side,
		})
	}
	return models.MarketTradesResult{Trades: out, NextPageToken: msg.GetNextPageToken()}
}

func CandlesFromProto(msg *marketdatav1.GetCandlesResponse, volumeScale int) models.CandlesResult {
	out := make([]models.Candle, 0, len(msg.GetCandles()))
	for _, c := range msg.GetCandles() {
		out = append(out, models.Candle{
			TsSec: int64(c.GetTsSec()),
			Open:  codecs.FormatPriceTicks(c.GetOpen()), High: codecs.FormatPriceTicks(c.GetHigh()),
			Low: codecs.FormatPriceTicks(c.GetLow()), Close: codecs.FormatPriceTicks(c.GetClose()),
			Volume: codecs.FormatQtyScaled(c.GetVolume(), volumeScale),
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
func CandlesColumnsFromProto(msg *marketdatav1.GetCandlesColumnsResponse, volumeScale int) models.CandlesResult {
	out := make([]models.Candle, 0, len(msg.GetTsSec()))
	for i, ts := range msg.GetTsSec() {
		candle := models.Candle{TsSec: int64(ts)}
		if i < len(msg.GetOpen()) {
			candle.Open = codecs.FormatPriceTicks(msg.GetOpen()[i])
		}
		if i < len(msg.GetHigh()) {
			candle.High = codecs.FormatPriceTicks(msg.GetHigh()[i])
		}
		if i < len(msg.GetLow()) {
			candle.Low = codecs.FormatPriceTicks(msg.GetLow()[i])
		}
		if i < len(msg.GetClose()) {
			candle.Close = codecs.FormatPriceTicks(msg.GetClose()[i])
		}
		if i < len(msg.GetVolume()) {
			candle.Volume = codecs.FormatQtyScaled(msg.GetVolume()[i], volumeScale)
		}
		out = append(out, candle)
	}
	tf := timeframeLabels[msg.GetTimeframe()]
	return models.CandlesResult{
		SymbolID: msg.GetSymbolId(), Timeframe: tf, Candles: out, NextPageToken: msg.GetNextPageToken(),
	}
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
		Volume:    codecs.FormatQtyScaled(point.GetVolume(), volumeScale),
		SymbolID:  symbolID,
		Timeframe: timeframe,
	}
}
