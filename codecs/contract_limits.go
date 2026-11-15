package codecs

import (
	"fmt"
	"math"

	validate "buf.build/gen/go/bufbuild/protovalidate/protocolbuffers/go/buf/validate"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"

	"github.com/Fabric-Labs/polyester-sdk-go/errors"
	chainwithdrawv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/chain/withdraw/v1"
	triggersv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/triggers/v1"
)

// TWAP and ladder ranges, read from the public contract's validation rules
// on TwapTrigger and LadderTrigger. A bound the contract leaves out is the
// wire type's own. A value outside them is refused before sending.
var (
	TwapDurationMsMin, TwapDurationMsMax           = contractInt64Range(&triggersv1.TwapTrigger{}, "duration_ms")
	TwapSliceIntervalMsMin, TwapSliceIntervalMsMax = contractInt64Range(&triggersv1.TwapTrigger{}, "slice_interval_ms")
	LadderLevelsMin, LadderLevelsMax               = contractInt32Range(&triggersv1.LadderTrigger{}, "levels")
	// TrailingDistanceTicksMin is the smallest trailing stop distance in
	// ticks the contract takes (its gt 0 rule on trailing_distance_ticks).
	TrailingDistanceTicksMin, _ = contractInt64Range(&triggersv1.TrailingStopTrigger{}, "trailing_distance_ticks")
)

// withdrawIdempotencyKeyMinLen is the contract's min_len on a trading
// withdraw's idempotency_key, in characters. The key is signed as given.
var withdrawIdempotencyKeyMinLen = int32(contractFieldRules(&chainwithdrawv1.TradingWithdrawIntentPayload{}, "idempotency_key").GetString().GetMinLen())

// Dead-man-switch timeouts the contract accepts besides 0, which disarms
// it. The contract states this range only as a CEL rule on
// CancelAllAfterRequest; a unit test rebuilds that rule from these constants.
const (
	CancelAllAfterMinTimeoutSec = 10
	CancelAllAfterMaxTimeoutSec = 120
)

func contractFieldRules(msg proto.Message, name protoreflect.Name) *validate.FieldRules {
	field := msg.ProtoReflect().Descriptor().Fields().ByName(name)
	rules, _ := proto.GetExtension(field.Options(), validate.E_Field).(*validate.FieldRules)
	return rules
}

func contractInt64Range(msg proto.Message, name protoreflect.Name) (low, high int64) {
	rules := contractFieldRules(msg, name).GetInt64()
	low, high = math.MinInt64, math.MaxInt64
	switch bound := rules.GetGreaterThan().(type) {
	case *validate.Int64Rules_Gte:
		low = bound.Gte
	case *validate.Int64Rules_Gt:
		low = bound.Gt + 1
	}
	switch bound := rules.GetLessThan().(type) {
	case *validate.Int64Rules_Lte:
		high = bound.Lte
	case *validate.Int64Rules_Lt:
		high = bound.Lt - 1
	}
	return low, high
}

func contractInt32Range(msg proto.Message, name protoreflect.Name) (low, high int32) {
	rules := contractFieldRules(msg, name).GetInt32()
	low, high = math.MinInt32, math.MaxInt32
	switch bound := rules.GetGreaterThan().(type) {
	case *validate.Int32Rules_Gte:
		low = bound.Gte
	case *validate.Int32Rules_Gt:
		low = bound.Gt + 1
	}
	switch bound := rules.GetLessThan().(type) {
	case *validate.Int32Rules_Lte:
		high = bound.Lte
	case *validate.Int32Rules_Lt:
		high = bound.Lt - 1
	}
	return low, high
}

// requiredInRange returns *value when it is set and within low..high.
// Left out, a field is 0 on the wire, which these contract ranges refuse.
func requiredInRange[T int32 | int64](value *T, name, strategy string, low, high T) (T, error) {
	if value == nil {
		return 0, &errors.ValidationError{Msg: fmt.Sprintf("%s is required for a %s", name, strategy)}
	}
	if *value < low || *value > high {
		return 0, &errors.ValidationError{Msg: fmt.Sprintf("%s %d is outside %d..%d", name, *value, low, high)}
	}
	return *value, nil
}
