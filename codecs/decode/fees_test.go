package decode_test

import (
	"testing"

	"github.com/Fabric-Labs/polyester-sdk-go/codecs/decode"
	feesv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/fees/v1"
)

func TestSpotFeeRatesDecodeRows(t *testing.T) {
	msg := &feesv1.GetSpotFeeRatesResponse{
		FeeRates: []*feesv1.SpotFeeRate{
			{
				SymbolId:            7,
				Symbol:              "BTC-USDT",
				MakerFeeRatePercent: "0.01",
				TakerFeeRatePercent: "0.04",
				VipTier:             2,
			},
		},
	}
	result := decode.SpotFeeRatesListFromProto(msg)
	if len(result.FeeRates) != 1 {
		t.Fatalf("rows=%d", len(result.FeeRates))
	}
	row := result.FeeRates[0]
	if row.SymbolID != 7 || row.Symbol != "BTC-USDT" || row.VIPTier != 2 {
		t.Fatalf("row=%+v", row)
	}
	if row.MakerFeeRatePercent != "0.01" || row.TakerFeeRatePercent != "0.04" {
		t.Fatalf("rates=%+v", row)
	}
}
