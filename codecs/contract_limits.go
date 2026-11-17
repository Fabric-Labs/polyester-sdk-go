package codecs

import (
	"fmt"
	"math"
	"regexp"
	"sync"
	"time"
	"unicode/utf8"

	validate "buf.build/gen/go/bufbuild/protovalidate/protocolbuffers/go/buf/validate"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"

	"github.com/Fabric-Labs/polyester-sdk-go/errors"
	chainlifecyclev1 "github.com/Fabric-Labs/polyester-sdk-go/gen/chain/lifecycle/v1"
	chainwithdrawv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/chain/withdraw/v1"
	ledgerrdv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/ledger/read/v1"
	transferv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/transfer/v1"
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
	// ModifyTrailingDistanceTicksMin is the smallest trailing distance in
	// ticks a trigger modify takes (its gt 0 rule on trailing_distance_ticks).
	ModifyTrailingDistanceTicksMin, _ = contractInt64Range(&triggersv1.ModifyTriggerRequest{}, "trailing_distance_ticks")
)

// Page sizes and batch sizes, read from the public contract's validation
// rules. A value outside them is refused before sending.
var (
	// BatchInternalTransferLegsMin and BatchInternalTransferLegsMax bound the
	// number of legs of one BatchCreateInternalTransfers request.
	BatchInternalTransferLegsMin, BatchInternalTransferLegsMax = contractRepeatedItems(&transferv1.BatchCreateInternalTransfersRequest{}, "legs")
	// ListAccountFlowsLimitMax is the largest ListAccountFlows page.
	ListAccountFlowsLimitMax = contractUint32Max(&chainlifecyclev1.ListAccountFlowsRequest{}, "limit")
	// ListAccountFlowsFilterIDsMax is the most IDs one ListAccountFlows ID
	// filter (Polyester Chain, zipped asset or unified asset IDs) takes.
	_, ListAccountFlowsFilterIDsMax = contractRepeatedItems(&chainlifecyclev1.ListAccountFlowsRequest{}, "unified_asset_ids")
	// ListDustConversionsLimitMax is the largest ListDustConversions page.
	ListDustConversionsLimitMax = contractUint32Max(&transferv1.ListDustConversionsRequest{}, "limit")
	// ListStatementExportsLimitMax is the largest ListStatementExports page.
	ListStatementExportsLimitMax = contractUint32Max(&ledgerrdv1.ListStatementExportsRequest{}, "limit")
)

// StatementExportMaxPeriod is the longest period one statement export
// covers. The contract states it only as a CEL rule on
// CreateStatementExportRequest; a unit test rebuilds that rule from it.
const StatementExportMaxPeriod = 8784 * time.Hour

// CheckContractString refuses value with a *errors.ValidationError when it
// breaks the contract's string rules on msg's field: its min_len and max_len
// in characters and its pattern. label names the field in the message.
func CheckContractString(msg proto.Message, field protoreflect.Name, value, label string) error {
	rules := contractFieldRules(msg, field).GetString()
	if rules == nil {
		return nil
	}
	n := uint64(utf8.RuneCountInString(value))
	if rules.HasMinLen() && n < rules.GetMinLen() {
		if rules.GetMinLen() == 1 {
			return &errors.ValidationError{Msg: label + " is required"}
		}
		return &errors.ValidationError{Msg: fmt.Sprintf("%s must be at least %d characters", label, rules.GetMinLen())}
	}
	if rules.HasMaxLen() && n > rules.GetMaxLen() {
		return &errors.ValidationError{Msg: fmt.Sprintf("%s must be at most %d characters", label, rules.GetMaxLen())}
	}
	if rules.HasPattern() && !contractPattern(rules.GetPattern()).MatchString(value) {
		return &errors.ValidationError{Msg: fmt.Sprintf("%s must match %s", label, rules.GetPattern())}
	}
	return nil
}

var contractPatterns sync.Map

func contractPattern(pattern string) *regexp.Regexp {
	if re, ok := contractPatterns.Load(pattern); ok {
		return re.(*regexp.Regexp)
	}
	re := regexp.MustCompile(pattern)
	contractPatterns.Store(pattern, re)
	return re
}

// CheckPageLimit refuses req with a *errors.ValidationError when its page
// limit breaks the public contract's rule on the request's limit field: above
// the rule's maximum, or below its minimum when the field is set. A limit
// left unset, which a field without presence also is at 0, is not checked:
// the server then applies its default page size. A request without a limit
// field, or whose limit has no rule, passes.
func CheckPageLimit(req proto.Message) error {
	msg := req.ProtoReflect()
	field := msg.Descriptor().Fields().ByName("limit")
	if field == nil || field.Kind() != protoreflect.Uint32Kind || !msg.Has(field) {
		return nil
	}
	rules := contractFieldRules(req, "limit").GetUint32()
	if rules == nil {
		return nil
	}
	value := uint32(msg.Get(field).Uint())
	low := uint32(0)
	switch bound := rules.GetGreaterThan().(type) {
	case *validate.UInt32Rules_Gte:
		low = bound.Gte
	case *validate.UInt32Rules_Gt:
		low = bound.Gt + 1
	}
	high := contractUint32Max(req, "limit")
	switch {
	case value > high:
		return &errors.ValidationError{Msg: fmt.Sprintf("limit %d exceeds %d", value, high)}
	case value < low:
		return &errors.ValidationError{Msg: fmt.Sprintf("limit %d is below %d; leave it unset for the server's default", value, low)}
	}
	return nil
}

func contractUint32Max(msg proto.Message, name protoreflect.Name) uint32 {
	rules := contractFieldRules(msg, name).GetUint32()
	switch bound := rules.GetLessThan().(type) {
	case *validate.UInt32Rules_Lte:
		return bound.Lte
	case *validate.UInt32Rules_Lt:
		return bound.Lt - 1
	}
	return math.MaxUint32
}

func contractRepeatedItems(msg proto.Message, name protoreflect.Name) (low, high int) {
	rules := contractFieldRules(msg, name).GetRepeated()
	low, high = 0, math.MaxInt
	if rules.HasMinItems() {
		low = int(rules.GetMinItems())
	}
	if rules.HasMaxItems() {
		high = int(rules.GetMaxItems())
	}
	return low, high
}

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
