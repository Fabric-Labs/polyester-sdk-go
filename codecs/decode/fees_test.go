package decode_test

import (
	"encoding/json"
	"google.golang.org/protobuf/encoding/protojson"
	"strings"
	"testing"

	"github.com/Fabric-Labs/polyester-sdk-go/codecs/decode"
	feesv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/fees/v1"
)

func TestSpotFeeRatesDecodeRows(t *testing.T) {
	msg := &feesv1.GetSpotFeeRatesResponse{
		FeeRates: []*feesv1.SpotFeeRate{
			{
				SymbolId:            7,
				MakerFeeRatePercent: "0.01",
				TakerFeeRatePercent: "0.04",
				VipTier:             2,
			},
		},
	}
	result := decode.SpotFeeRatesListFromProto(msg, nil)
	if len(result.FeeRates) != 1 {
		t.Fatalf("rows=%d", len(result.FeeRates))
	}
	row := result.FeeRates[0]
	if row.SymbolID != 7 || row.Symbol != "" || row.VIPTier != 2 {
		t.Fatalf("row=%+v", row)
	}
	if row.MakerFeeRatePercent != "0.01" || row.TakerFeeRatePercent != "0.04" {
		t.Fatalf("rates=%+v", row)
	}
}

func TestSpotFeeRateReserveBreakdownAndDefaults(t *testing.T) {
	var msg feesv1.SpotFeeRate
	if err := protojson.Unmarshal([]byte(`{"symbolId":7,"reserveFeeRatePercent":"0.09","postOnlyReserveFeeRatePercent":"0.08","nextChangeAt":"2026-11-06T00:00:00Z","breakdown":{"tierMakerFeeRatePercent":"-0.08","tierTakerFeeRatePercent":"0.135","marketMultiplier":{"makerMultiplierPercent":"90","takerMultiplierPercent":"20","effectiveFrom":"2026-11-01T00:00:00Z"},"referralDiscount":{"makerDiscountPercent":"5","takerDiscountPercent":"10","effectiveFrom":"2026-11-01T00:00:00Z"}}}`), &msg); err != nil {
		t.Fatal(err)
	}
	row := decode.SpotFeeRateFromProto(&msg, nil)
	raw, err := json.Marshal(row)
	if err != nil {
		t.Fatal(err)
	}
	var data map[string]any
	if err = json.Unmarshal(raw, &data); err != nil {
		t.Fatal(err)
	}
	if data["reserve_fee_rate_percent"] != "0.09" || data["post_only_reserve_fee_rate_percent"] != "0.08" || data["next_change_at"] != "2026-11-06T00:00:00Z" {
		t.Fatalf("reserve fields lost: %s", raw)
	}
	breakdown, ok := data["breakdown"].(map[string]any)
	if !ok {
		t.Fatalf("breakdown lost: %s", raw)
	}
	if breakdown["tier_maker_fee_rate_percent"] != "-0.08" {
		t.Fatalf("tier lost: %s", raw)
	}
	discounts, ok := breakdown["account_discounts"].([]any)
	if !ok || len(discounts) != 0 {
		t.Fatalf("absent discounts must be empty: %s", raw)
	}
	multiplier := breakdown["market_multiplier"].(map[string]any)
	if multiplier["maker_multiplier_percent"] != "90" || multiplier["effective_to"] != nil {
		t.Fatalf("multiplier=%v", multiplier)
	}
	referral := breakdown["referral_discount"].(map[string]any)
	if referral["taker_discount_percent"] != "10" {
		t.Fatalf("referral=%v", referral)
	}
	emptyRaw, _ := json.Marshal(decode.SpotFeeRateFromProto(&feesv1.SpotFeeRate{}, nil))
	if strings.Contains(string(emptyRaw), "breakdown") || strings.Contains(string(emptyRaw), "next_change_at") {
		t.Fatalf("optional fields invented: %s", emptyRaw)
	}
}
