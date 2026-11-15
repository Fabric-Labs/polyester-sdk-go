package codecs

import (
	"fmt"
	"math"
	"testing"

	validate "buf.build/gen/go/bufbuild/protovalidate/protocolbuffers/go/buf/validate"
	"google.golang.org/protobuf/proto"

	orderv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/orders/v1"
	triggersv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/triggers/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

func twapRequest(duration, interval *int64) (*triggersv1.CreateTriggerRequest, error) {
	return CreateTriggerToProto(
		1, "BTC-USDT", "twap", nil, "buy", models.QtyFromDecimal("0.0005"),
		"market", nil, "", "", nil, nil, false, 8,
		CreateTriggerOptions{TwapDurationMs: duration, TwapSliceIntervalMs: interval},
	)
}

func ladderRequest(levels *int32) (*triggersv1.CreateTriggerRequest, error) {
	low, high := models.PriceFromDecimal("30000"), models.PriceFromDecimal("31000")
	return CreateTriggerToProto(
		1, "BTC-USDT", "ladder", nil, "buy", models.QtyFromDecimal("0.001"),
		"limit", nil, "", "", nil, nil, false, 8,
		CreateTriggerOptions{LadderPriceMin: &low, LadderPriceMax: &high, LadderLevels: levels},
	)
}

func messageCEL(msg proto.Message) [][2]string {
	rules, _ := proto.GetExtension(msg.ProtoReflect().Descriptor().Options(), validate.E_Message).(*validate.MessageRules)
	var out [][2]string
	for _, rule := range rules.GetCel() {
		out = append(out, [2]string{rule.GetId(), rule.GetExpression()})
	}
	return out
}

// The limits are the published contract's: its validation rules, and for the
// dead-man switch the CEL rule rebuilt from the SDK's constants.
func TestTriggerAndTimeoutLimitsAreTheContracts(t *testing.T) {
	if TwapDurationMsMin != 1000 || TwapSliceIntervalMsMin != 100 {
		t.Fatalf("TWAP minimums = %d, %d; want the contract's 1000, 100", TwapDurationMsMin, TwapSliceIntervalMsMin)
	}
	if TwapDurationMsMax != math.MaxInt64 || TwapSliceIntervalMsMax != math.MaxInt64 {
		t.Fatalf("TWAP maximums = %d, %d; the contract sets none", TwapDurationMsMax, TwapSliceIntervalMsMax)
	}
	if LadderLevelsMin != 2 || LadderLevelsMax != 100 {
		t.Fatalf("ladder levels = %d..%d; want the contract's 2..100", LadderLevelsMin, LadderLevelsMax)
	}
	window := messageCEL(&triggersv1.TwapTrigger{})
	if len(window) != 1 || window[0] != [2]string{"twap_trigger.window", "this.slice_interval_ms <= this.duration_ms"} {
		t.Fatalf("TwapTrigger CEL = %v", window)
	}
	want := fmt.Sprintf(
		"this.timeout_sec == 0u || (this.timeout_sec >= %du && this.timeout_sec <= %du)",
		CancelAllAfterMinTimeoutSec, CancelAllAfterMaxTimeoutSec,
	)
	timeout := messageCEL(&orderv1.CancelAllAfterRequest{})
	if len(timeout) != 1 || timeout[0] != [2]string{"cancel_all_after.timeout_range", want} {
		t.Fatalf("CancelAllAfterRequest CEL = %v, want %q", timeout, want)
	}
	if CancelAllAfterMinTimeoutSec != 10 || CancelAllAfterMaxTimeoutSec != 120 {
		t.Fatalf("timeout range = %d..%d; want 10..120", CancelAllAfterMinTimeoutSec, CancelAllAfterMaxTimeoutSec)
	}
}

func TestTwapRefusesValuesOutsideTheContract(t *testing.T) {
	for _, tc := range []struct {
		duration, interval *int64
		mention            string
	}{
		{ptr64(500), ptr64(100), "twap_duration_ms"},
		{ptr64(0), ptr64(100), "twap_duration_ms"},
		{ptr64(-1000), ptr64(100), "twap_duration_ms"},
		{ptr64(999), ptr64(100), "twap_duration_ms"},
		{ptr64(60_000), ptr64(0), "twap_slice_interval_ms"},
		{ptr64(60_000), ptr64(-5), "twap_slice_interval_ms"},
		{ptr64(60_000), ptr64(10), "twap_slice_interval_ms"},
		{ptr64(60_000), ptr64(99), "twap_slice_interval_ms"},
		{ptr64(1_000), ptr64(1_001), "must not exceed twap_duration_ms"},
		{nil, ptr64(100), "twap_duration_ms is required"},
		{ptr64(60_000), nil, "twap_slice_interval_ms is required"},
	} {
		_, err := twapRequest(tc.duration, tc.interval)
		wantValidation(t, err, tc.mention)
	}
}

func TestTwapSendsValuesInsideTheContract(t *testing.T) {
	for _, tc := range [][2]int64{{60_000, 1_000}, {1_000, 100}, {1_000, 1_000}, {math.MaxInt64, 100}, {math.MaxInt64, math.MaxInt64}} {
		req, err := twapRequest(ptr64(tc[0]), ptr64(tc[1]))
		if err != nil {
			t.Fatalf("%v: %v", tc, err)
		}
		twap := req.GetTrigger().GetTwap()
		if twap.GetDurationMs() != tc[0] || twap.GetSliceIntervalMs() != tc[1] {
			t.Fatalf("%v: sent %d, %d", tc, twap.GetDurationMs(), twap.GetSliceIntervalMs())
		}
	}
}

func TestLadderRefusesLevelsOutsideTheContract(t *testing.T) {
	for _, levels := range []*int32{ptr32(1), ptr32(0), ptr32(101), ptr32(-3), ptr32(math.MaxInt32), nil} {
		_, err := ladderRequest(levels)
		wantValidation(t, err, "ladder_levels")
	}
}

func TestLadderSendsLevelsInsideTheContract(t *testing.T) {
	for _, levels := range []int32{2, 3, 99, 100} {
		req, err := ladderRequest(ptr32(levels))
		if err != nil {
			t.Fatalf("%d: %v", levels, err)
		}
		if got := req.GetTrigger().GetLadder().GetLevels(); got != levels {
			t.Fatalf("levels sent %d, want %d", got, levels)
		}
	}
}

func TestCancelAllAfterRefusesTimeoutsOutsideTheContract(t *testing.T) {
	for _, timeout := range []int{5, 9, 121, 1, -1, math.MaxUint32, 1 << 32} {
		_, err := CancelAllAfterToProto(nil, timeout, nil, nil, nil)
		wantValidation(t, err, "timeout_sec")
	}
	for _, timeout := range []int{0, 10, 11, 119, 120} {
		req, err := CancelAllAfterToProto(nil, timeout, nil, nil, nil)
		if err != nil {
			t.Fatalf("%d: %v", timeout, err)
		}
		if req.GetTimeoutSec() != uint32(timeout) {
			t.Fatalf("timeout sent %d, want %d", req.GetTimeoutSec(), timeout)
		}
	}
}

func ptr64(v int64) *int64 { return &v }
func ptr32(v int32) *int32 { return &v }

func trailingRequest(ticks int64) (*triggersv1.CreateTriggerRequest, error) {
	return CreateTriggerToProto(
		1, "BTC-USDT", "trailing_stop", nil, "sell", models.QtyFromDecimal("0.001"),
		"market", nil, "", "", nil, nil, false, 8,
		CreateTriggerOptions{TrailingDistanceTicks: &ticks},
	)
}

// The contract's trailing_distance_ticks rule is gt 0: zero and negative
// distances are refused before sending.
func TestTrailingStopRefusesDistanceTicksOutsideTheContract(t *testing.T) {
	if TrailingDistanceTicksMin != 1 {
		t.Fatalf("TrailingDistanceTicksMin = %d, want the contract's gt 0", TrailingDistanceTicksMin)
	}
	for _, ticks := range []int64{-5, 0, math.MinInt64} {
		_, err := trailingRequest(ticks)
		wantValidation(t, err, "trailing_distance_ticks")
	}
	for _, ticks := range []int64{1, 50_000_000_000, math.MaxInt64} {
		req, err := trailingRequest(ticks)
		if err != nil {
			t.Fatalf("%d: %v", ticks, err)
		}
		if got := req.GetTrigger().GetTrailingStop().GetTrailingDistanceTicks(); got != ticks {
			t.Fatalf("trailing ticks sent %d, want %d", got, ticks)
		}
	}
}
