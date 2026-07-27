package services

import (
	"fmt"
	"testing"

	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

func TestResolveCandleChannelTimeframe(t *testing.T) {
	for _, timeframe := range []string{"1m", "MIN_1", "min1"} {
		got, err := resolveCandleChannelTimeframe(timeframe)
		if err != nil {
			t.Fatalf("resolveCandleChannelTimeframe(%q): %v", timeframe, err)
		}
		if got != "1m" {
			t.Fatalf("resolveCandleChannelTimeframe(%q) = %q, want %q", timeframe, got, "1m")
		}
		channel := fmt.Sprintf("public:spot:market:candles:%s:%d:proto", got, 101)
		if channel != "public:spot:market:candles:1m:101:proto" {
			t.Fatalf("channel for %q = %q", timeframe, channel)
		}
	}
}

func TestResolveCandleChannelTimeframeUnknown(t *testing.T) {
	if _, err := resolveCandleChannelTimeframe("not-a-tf"); err == nil {
		t.Fatal("expected validation error")
	}
}

func TestCurrentCandleSelectsFirstNewestRow(t *testing.T) {
	candles := []models.Candle{
		{TsSec: 200, Close: "2"},
		{TsSec: 100, Close: "1"},
	}
	got := currentCandle(candles)
	if got.TsSec != 200 || got.Close != "2" {
		t.Fatalf("currentCandle() = %+v, want first/newest row", got)
	}
}
