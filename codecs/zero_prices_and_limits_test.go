package codecs

import (
	"errors"
	"strings"
	"testing"

	validate "buf.build/gen/go/bufbuild/protovalidate/protocolbuffers/go/buf/validate"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"

	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

func zeroPrices() map[string]models.PriceInput {
	return map[string]models.PriceInput{
		"decimal 0":    models.PriceFromDecimal("0"),
		"decimal 0.00": models.PriceFromDecimal("0.00"),
		"ticks 0":      models.PriceFromTicksInt(0),
	}
}

func refusedNaming(t *testing.T, field string, err error) {
	t.Helper()
	var validation *sdkerrors.ValidationError
	if !errors.As(err, &validation) || !strings.Contains(validation.Msg, field) {
		t.Fatalf("err = %T %v, want a *errors.ValidationError naming %s", err, err, field)
	}
}

type triggerCreate struct {
	kind, side, orderType, tif string
	triggerPrice, limitPrice   *models.PriceInput
	opts                       CreateTriggerOptions
}

func (c triggerCreate) encode() error {
	_, err := CreateTriggerToProto(1, "BTC-USDT", c.kind, c.triggerPrice, c.side, models.QtyFromDecimal("1"), c.orderType, c.limitPrice, "", c.tif, nil, nil, false, 8, c.opts)
	return err
}

// A zero trigger, limit, activation or ladder price on a create is refused
// before sending, in every form, instead of being encoded as "no price".
func TestCreatePathsRefuseAZeroPrice(t *testing.T) {
	positive := models.PriceFromDecimal("100")
	high := models.PriceFromDecimal("200")
	levels := int32(5)
	distance := int64(1_000_000_000)
	for name, zero := range zeroPrices() {
		zero := zero
		for label, tc := range map[string]struct {
			field string
			in    triggerCreate
		}{
			"stop_loss trigger_price":         {"trigger_price", triggerCreate{kind: "stop_loss", side: "sell", orderType: "market", triggerPrice: &zero}},
			"take_profit trigger_price":       {"trigger_price", triggerCreate{kind: "take_profit", side: "sell", orderType: "market", triggerPrice: &zero}},
			"stop_loss limit child gtc":       {"limit_price", triggerCreate{kind: "stop_loss", side: "sell", orderType: "limit", triggerPrice: &positive, limitPrice: &zero}},
			"stop_loss limit child ioc":       {"limit_price", triggerCreate{kind: "stop_loss", side: "sell", orderType: "limit", tif: "ioc", triggerPrice: &positive, limitPrice: &zero}},
			"take_profit limit child fok":     {"limit_price", triggerCreate{kind: "take_profit", side: "sell", orderType: "limit", tif: "fok", triggerPrice: &positive, limitPrice: &zero}},
			"twap limit price":                {"limit_price", triggerCreate{kind: "twap", side: "buy", orderType: "limit", limitPrice: &zero, opts: CreateTriggerOptions{TwapDurationMs: ptr(int64(60_000)), TwapSliceIntervalMs: ptr(int64(1_000))}}},
			"trailing_stop activation_price":  {"activation_price", triggerCreate{kind: "trailing_stop", side: "sell", orderType: "market", opts: CreateTriggerOptions{TrailingDistanceTicks: &distance, ActivationPrice: &zero}}},
			"trailing_stop buy limit_price":   {"limit_price", triggerCreate{kind: "trailing_stop", side: "buy", orderType: "market", limitPrice: &zero, opts: CreateTriggerOptions{TrailingDistanceTicks: &distance}}},
			"ladder price_min":                {"ladder_price_min", triggerCreate{kind: "ladder", side: "buy", orderType: "limit", opts: CreateTriggerOptions{LadderPriceMin: &zero, LadderPriceMax: &high, LadderLevels: &levels}}},
			"ladder price_max":                {"ladder_price_max", triggerCreate{kind: "ladder", side: "buy", orderType: "limit", opts: CreateTriggerOptions{LadderPriceMin: &positive, LadderPriceMax: &zero, LadderLevels: &levels}}},
			"ladder price_min without a max":  {"ladder_price_min", triggerCreate{kind: "ladder", side: "buy", orderType: "limit", opts: CreateTriggerOptions{LadderPriceMin: &zero, LadderLevels: &levels}}},
			"ladder price_max without a min":  {"ladder_price_max", triggerCreate{kind: "ladder", side: "buy", orderType: "limit", opts: CreateTriggerOptions{LadderPriceMax: &zero, LadderLevels: &levels}}},
			"stop_loss market child baseline": {"", triggerCreate{kind: "stop_loss", side: "sell", orderType: "market", triggerPrice: &positive}},
		} {
			err := tc.in.encode()
			if tc.field == "" {
				if err != nil {
					t.Fatalf("%s: %v", label, err)
				}
				continue
			}
			t.Run(label+" "+name, func(t *testing.T) { refusedNaming(t, tc.field, err) })
		}
		t.Run("order limit price "+name, func(t *testing.T) {
			symbol, symbolID := "BTC-USDT", uint32(1)
			_, err := CreateOrderToProto(models.CreateOrderRequest{Symbol: &symbol, SymbolID: &symbolID, Side: "buy", OrderType: "limit", Qty: models.QtyFromDecimal("1"), Price: &zero}, 8, 2)
			refusedNaming(t, "price", err)
		})
		t.Run("oco legs "+name, func(t *testing.T) {
			for field, in := range map[string]models.CreateOcoInput{
				"take_profit.trigger_price": {Side: "sell", Qty: models.QtyFromDecimal("1"), TakeProfit: models.RiskLegInput{TriggerPrice: zero}, StopLoss: models.RiskLegInput{TriggerPrice: positive}},
				"stop_loss.limit_price":     {Side: "sell", Qty: models.QtyFromDecimal("1"), TakeProfit: models.RiskLegInput{TriggerPrice: high}, StopLoss: models.RiskLegInput{TriggerPrice: positive, LimitPrice: &zero}},
			} {
				in.Symbol = "BTC-USDT"
				_, err := CreateOcoToProto(1, in, nil, 8)
				refusedNaming(t, field, err)
			}
		})
	}
	// Positive prices still encode.
	pm, pmax := models.PriceFromDecimal("0.000000001"), models.PriceFromDecimal("100")
	req, err := CreateTriggerToProto(1, "BTC-USDT", "ladder", nil, "buy", models.QtyFromDecimal("1"), "limit", nil, "", "", nil, nil, false, 8, CreateTriggerOptions{LadderPriceMin: &pm, LadderPriceMax: &pmax, LadderLevels: &levels})
	if err != nil || req.GetTrigger().GetLadder().GetPriceMinTicks() != 1 {
		t.Fatalf("smallest positive ladder minimum: req=%v err=%v", req, err)
	}
}

// A trigger modify refuses a trailing distance or trigger price the contract
// does not take (gt 0), and a negative activation price; 0 still clears the
// activation price.
func TestModifyTriggerRefusesNonPositiveTrailingDistanceAndTriggerPrice(t *testing.T) {
	for _, d := range []int64{0, -5} {
		d := d
		_, err := ModifyTriggerToProto("2", 0, "", nil, ModifyTriggerOptions{TrailingDistanceTicks: &d})
		refusedNaming(t, "trailing_distance_ticks", err)
	}
	one := int64(1)
	req, err := ModifyTriggerToProto("2", 0, "", nil, ModifyTriggerOptions{TrailingDistanceTicks: &one})
	if err != nil || req.GetTrailingDistanceTicks() != 1 {
		t.Fatalf("trailing distance 1: req=%v err=%v", req, err)
	}
	for name, zero := range zeroPrices() {
		zero := zero
		_, err := ModifyTriggerToProto("2", 0, "", nil, ModifyTriggerOptions{TriggerPrice: &zero})
		if err == nil {
			t.Fatalf("trigger_price %s: encoded", name)
		}
		refusedNaming(t, "trigger_price", err)
		req, err := ModifyTriggerToProto("2", 0, "", nil, ModifyTriggerOptions{ActivationPrice: &zero})
		if err != nil || req.ActivationPriceTicks == nil || *req.ActivationPriceTicks != 0 {
			t.Fatalf("activation_price %s clears: req=%v err=%v", name, req, err)
		}
	}
	negative := models.PriceFromDecimal("-1")
	_, err = ModifyTriggerToProto("2", 0, "", nil, ModifyTriggerOptions{ActivationPrice: &negative})
	refusedNaming(t, "activation_price", err)
	if ModifyTrailingDistanceTicksMin != 1 {
		t.Fatalf("ModifyTrailingDistanceTicksMin = %d, want the contract's gt 0", ModifyTrailingDistanceTicksMin)
	}
}

// CheckPageLimit applies each public request's own limit rule: every request
// with a limit rule refuses one above its maximum, takes its maximum, and
// takes an unset limit; a field with presence and a minimum refuses an
// explicit value below it.
func TestCheckPageLimitFollowsEveryRequestsContractRule(t *testing.T) {
	checked := 0
	protoregistry.GlobalTypes.RangeMessages(func(mt protoreflect.MessageType) bool {
		desc := mt.Descriptor()
		field := desc.Fields().ByName("limit")
		if field == nil || field.Kind() != protoreflect.Uint32Kind || !strings.HasSuffix(string(desc.Name()), "Request") {
			return true
		}
		rules, _ := proto.GetExtension(field.Options(), validate.E_Field).(*validate.FieldRules)
		if rules.GetUint32() == nil {
			return true
		}
		checked++
		max := contractUint32Max(mt.New().Interface(), "limit")
		with := func(v uint32) proto.Message {
			msg := mt.New()
			msg.Set(field, protoreflect.ValueOfUint32(v))
			return msg.Interface()
		}
		if err := CheckPageLimit(mt.New().Interface()); err != nil {
			t.Errorf("%s unset limit: %v", desc.FullName(), err)
		}
		if err := CheckPageLimit(with(max)); err != nil {
			t.Errorf("%s limit %d: %v", desc.FullName(), max, err)
		}
		if max < ^uint32(0) {
			refusedNaming(t, "limit", CheckPageLimit(with(max+1)))
		}
		if field.HasPresence() && rules.GetUint32().HasGte() && rules.GetUint32().GetGte() > 0 {
			refusedNaming(t, "limit", CheckPageLimit(with(0)))
		}
		return true
	})
	if checked < 20 {
		t.Fatalf("checked %d request limits; the generated contract should carry more", checked)
	}
}

// Public IDs are ASCII: a decimal or base58 ID written with other digits, or
// wrapped in non-ASCII spaces, is refused rather than read as another ID.
func TestIDsRefuseNonASCIIDigitsAndSpaces(t *testing.T) {
	for _, value := range []string{"٥", "５", "٢٣", "２３", "๒๓", " 23", "23　", "２3"} {
		if _, err := IDToInt(value, "order_id"); err == nil {
			t.Errorf("IDToInt(%q) accepted", value)
		} else {
			refusedNaming(t, "order_id", err)
		}
		if _, err := ParseAccountID(value, "sub_account_id"); err == nil {
			t.Errorf("ParseAccountID(%q) accepted", value)
		}
	}
	if id, err := IDToInt("23", "order_id"); err != nil || id != 60 {
		t.Fatalf(`IDToInt("23") = %d, %v; want 60 (base58)`, id, err)
	}
	if id, err := IDToInt(" 23 ", "order_id"); err != nil || id != 60 {
		t.Fatalf(`IDToInt(" 23 ") = %d, %v; want 60`, id, err)
	}
}
