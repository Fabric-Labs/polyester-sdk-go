package services

import (
	"errors"
	"math"
	"testing"

	"github.com/Fabric-Labs/polyester-sdk-go/catalogs"
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
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

func TestRawSymbolFiltersFailClosed(t *testing.T) {
	manager := auditCatalog(t)
	unknown := "NOPE-USDT"
	err := ValidateSymbolFilter(manager, &unknown, "test")
	var validationErr *sdkerrors.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected ValidationError, got %T: %v", err, err)
	}
	empty := ""
	if err := ValidateSymbolFilter(manager, &empty, "test"); err != nil {
		t.Fatalf("empty filter must preserve unfiltered semantics: %v", err)
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
}

func TestPairPreflightChecksTickStepMinimumsAndNotional(t *testing.T) {
	constraints, ok := auditCatalog(t).PairConstraintsForSymbol("BTC-USDT")
	if !ok {
		t.Fatal("constraints unavailable")
	}
	validPrice := int64(5_000_000_000) // 5000 quote; 0.002 base = 10 quote.
	if err := preflightPairValues(constraints, 2, []int64{validPrice}, &validPrice); err != nil {
		t.Fatalf("valid boundary rejected: %v", err)
	}
	badTick := validPrice + 1
	if err := preflightPairValues(constraints, 2, []int64{badTick}, &badTick); err == nil {
		t.Fatal("misaligned tick should fail")
	}
	if err := preflightPairValues(constraints, 1, []int64{validPrice}, &validPrice); err == nil {
		t.Fatal("quantity below minimum should fail")
	}
	lowPrice := int64(4_000_000_000)
	if err := preflightPairValues(constraints, 2, []int64{lowPrice}, &lowPrice); err == nil {
		t.Fatal("computable notional below minimum should fail")
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
}
