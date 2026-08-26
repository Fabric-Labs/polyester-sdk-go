package services

import (
	"errors"
	"math"
	"testing"

	"github.com/Fabric-Labs/polyester-sdk-go/catalogs"
	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

func auditCatalog(t *testing.T) *catalogs.Manager {
	t.Helper()
	manager := catalogs.NewManager()
	err := manager.HydrateSpotConfig(map[string]any{"pairs": []any{map[string]any{
		"symbol":               "BTC-USDT",
		"symbol_id":            float64(1),
		"base_quantity_scale":  float64(3),
		"quote_quantity_scale": float64(2),
		"tick_size":            "0.01",
		"step_size":            "0.001",
		"min_qty_base":         "0.002",
		"min_notional_quote":   "10",
	}}})
	if err != nil {
		t.Fatal(err)
	}
	return manager
}

func TestOptionalSymbolFiltersFailClosedAndOmitEmpty(t *testing.T) {
	manager := auditCatalog(t)
	unknown := "NOPE-USDT"
	if _, err := ResolveOptionalSymbolID(manager, &unknown, nil, "cancel_all"); err == nil {
		t.Fatal("unknown display symbol must fail closed")
	}

	empty := "   "
	got, err := ResolveOptionalSymbolID(manager, &empty, nil, "cancel_all")
	if err != nil || got != 0 {
		t.Fatalf("empty/whitespace symbol should mean all symbols, got=%d err=%v", got, err)
	}

	known := "BTC-USDT"
	got, err = ResolveOptionalSymbolID(manager, &known, nil, "cancel_all")
	if err != nil || got != 1 {
		t.Fatalf("known symbol resolve got=%d err=%v", got, err)
	}

	proto, err := codecs.CancelAllOrdersToProto(nil, nil, nil, true, nil)
	if err != nil {
		t.Fatalf("cancel-all encode failed: %v", err)
	}
	if proto.GetSymbolId() != 0 {
		t.Fatalf("omitted filter should encode symbol_id=0, got %d", proto.GetSymbolId())
	}
	id := uint32(1)
	proto, err = codecs.CancelAllOrdersToProto(nil, &id, nil, true, nil)
	if err != nil {
		t.Fatalf("cancel-all encode failed: %v", err)
	}
	if proto.GetSymbolId() != 1 {
		t.Fatalf("symbol_id not encoded: got %d", proto.GetSymbolId())
	}
}

func TestResolveSymbolIDStillFailsClosedForWireSymbolIDPaths(t *testing.T) {
	manager := auditCatalog(t)
	unknown := "NOPE-USDT"
	_, err := ResolveSymbolID(manager, &unknown, nil, "list_history")
	var validationErr *sdkerrors.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected ValidationError, got %T: %v", err, err)
	}
	id := uint32(1)
	got, err := ResolveSymbolID(manager, nil, &id, "list_history")
	if err != nil || got != 1 {
		t.Fatalf("explicit symbol_id resolve got=%d err=%v", got, err)
	}
}

func TestPaginationLimitRejectsWrappingInputs(t *testing.T) {
	for _, limit := range []int{-1, int(uint64(math.MaxUint32) + 1)} {
		if _, err := PaginationLimit(limit, "limit"); err == nil {
			t.Fatalf("limit %d should fail", limit)
		}
	}
	if got, err := PaginationLimitOrDefault(0, 50, "limit"); err != nil || got != 50 {
		t.Fatalf("default limit got=%d err=%v", got, err)
	}
	if _, err := ExplicitPaginationLimit(nil, "limit"); err != nil {
		t.Fatalf("nil explicit limit should omit: %v", err)
	}
}

func TestOrderCreateDoesNotPreflightOffTickPrices(t *testing.T) {
	symbol := "BTC-USDT"
	tif := "gtc"
	// 5000.000001 is not aligned to tick_size 0.01; SDK must still encode and
	// leave admission to the API.
	price := models.PriceFromDecimal("5000.000001")
	sid := uint32(1)
	req := models.CreateOrderRequest{
		Symbol: &symbol, SymbolID: &sid, Side: "buy", OrderType: "limit", TIF: &tif,
		Qty: models.QtyFromDecimal("0.001"), Price: &price,
	}
	proto, err := codecs.CreateOrderToProto(req, 3, 2)
	if err != nil {
		t.Fatalf("off-tick order should encode without SDK preflight: %v", err)
	}
	if proto.GetOrder() == nil || proto.GetOrder().GetLimitGtc() == nil {
		t.Fatal("expected limit GTC intent")
	}
}

func TestPairConstraintsTreatZeroOptionalRulesAsUnset(t *testing.T) {
	manager := catalogs.NewManager()
	err := manager.HydrateSpotConfig(map[string]any{"pairs": []any{map[string]any{
		"symbol":               "BTC-USDT",
		"symbol_id":            float64(1),
		"base_quantity_scale":  float64(3),
		"quote_quantity_scale": float64(2),
		"tick_size":            "0.01",
		"step_size":            "0.001",
		"min_qty_base":         "0",
		"min_notional_quote":   "0",
	}}})
	if err != nil {
		t.Fatal(err)
	}
	constraints, ok := manager.PairConstraintsForSymbol("BTC-USDT")
	if !ok {
		t.Fatal("zero-valued optional protobuf constraints should remain usable")
	}
	if constraints.TickSizeTicks == 0 || constraints.StepSizeScaled == 0 ||
		constraints.MinQtyScaled != 0 || constraints.MinNotionalComputable {
		t.Fatalf("zero-valued optional constraints were enforced: %+v", constraints)
	}
	if scale, ok := manager.BaseQuantityScaleForSymbol("BTC-USDT"); !ok || scale != 3 {
		t.Fatalf("scale hydration broken with zero optional minima: scale=%d ok=%v", scale, ok)
	}
}
