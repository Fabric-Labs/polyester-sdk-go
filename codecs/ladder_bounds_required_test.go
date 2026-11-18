package codecs

import (
	"strings"
	"testing"

	validate "buf.build/gen/go/bufbuild/protovalidate/protocolbuffers/go/buf/validate"
	"google.golang.org/protobuf/reflect/protoreflect"

	orderv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/orders/v1"
	triggersv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/triggers/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

// Contract triggers.proto, LadderTrigger: `int64 price_min_ticks = 2
// [(buf.validate.field).int64.gt = 0]` and `int64 price_max_ticks = 3
// [(buf.validate.field).int64.gt = 0]`, plain proto3 scalars with no
// IGNORE_IF_ZERO_VALUE, so a ladder without either bound is on the wire as 0,
// which the contract refuses. Both bounds are required.
func TestLadderContractRequiresBothPriceBounds(t *testing.T) {
	for _, name := range []protoreflect.Name{"price_min_ticks", "price_max_ticks"} {
		field := (&triggersv1.LadderTrigger{}).ProtoReflect().Descriptor().Fields().ByName(name)
		rules := contractFieldRules(&triggersv1.LadderTrigger{}, name)
		if field.HasPresence() || rules.GetIgnore() != validate.Ignore_IGNORE_UNSPECIFIED || rules.GetInt64().GetGt() != 0 || !rules.GetInt64().HasGt() {
			t.Fatalf("%s: presence=%v rules=%v, want a required int64 gt 0", name, field.HasPresence(), rules)
		}
	}
}

// A ladder without its minimum or maximum price, or with neither, is refused
// before sending with the code the API answers for a ladder price.
func TestLadderWithoutAPriceBoundIsRefused(t *testing.T) {
	low, high := refPrice("30000"), refPrice("31000")
	unset := &models.PriceInput{}
	for _, tc := range []struct {
		name, field string
		min, max    *models.PriceInput
	}{
		{"no min", "ladder_price_min", nil, high},
		{"no max", "ladder_price_max", low, nil},
		{"neither", "ladder_price_min", nil, nil},
		{"unset min", "ladder_price_min", unset, high},
		{"unset max", "ladder_price_max", low, unset},
	} {
		err := createCodedTrigger(triggerCase{kind: "ladder", side: "buy", orderType: "limit",
			opts: CreateTriggerOptions{LadderPriceMin: tc.min, LadderPriceMax: tc.max, LadderLevels: refI32(5)}})
		requireCode(t, tc.name, err, orderv1.ErrorCode_ERROR_CODE_TRIGGER_PRICE_INVALID)
		if !strings.Contains(err.Error(), tc.field) {
			t.Fatalf("%s: %v, want it to name %s", tc.name, err, tc.field)
		}
	}
	if err := createCodedTrigger(triggerCase{kind: "ladder", side: "buy", orderType: "limit",
		opts: CreateTriggerOptions{LadderPriceMin: low, LadderPriceMax: high, LadderLevels: refI32(5)}}); err != nil {
		t.Fatalf("both bounds: %v", err)
	}
}
