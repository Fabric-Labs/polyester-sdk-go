package services

import (
	"context"
	stderrors "errors"
	"fmt"
	"math"
	"testing"

	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// capturedField reads a (possibly nested) field of a captured request.
func capturedField(msg proto.Message, path ...string) protoreflect.Value {
	m := msg.ProtoReflect()
	for i, name := range path {
		fd := m.Descriptor().Fields().ByName(protoreflect.Name(name))
		if fd == nil {
			panic("unknown field " + name)
		}
		if i == len(path)-1 {
			return m.Get(fd)
		}
		m = m.Get(fd).Message()
	}
	return protoreflect.Value{}
}

func requireRefusedBeforeTransport(t *testing.T, err error, captures chan proto.Message) {
	t.Helper()
	var validation *sdkerrors.ValidationError
	if !stderrors.As(err, &validation) {
		t.Fatalf("err = %T %v, want ValidationError", err, err)
	}
	if len(captures) != 0 {
		t.Fatalf("refused input reached transport: %v", <-captures)
	}
}

// A timeout that does not fit the wire uint32 is refused, not wrapped:
// 4294967296 would arm nothing (0 disables the switch) and 4294967306
// would arm 10 seconds.
func TestCancelAllAfterRefusesTimeoutsOutsideUint32(t *testing.T) {
	for _, timeout := range []int{-1, math.MinInt64, 1 << 32, 1<<32 + 10, math.MaxInt64} {
		t.Run(fmt.Sprint(timeout), func(t *testing.T) {
			factory, captures := scopeCaptureFactory(t)
			orders := NewOrdersService(factory, auditCatalog(t), nil, nil, nil, nil, nil)
			_, err := orders.CancelAllAfter(context.Background(), nil, timeout, nil, nil, nil, nil)
			requireRefusedBeforeTransport(t, err, captures)
		})
	}
	for _, timeout := range []int{0, 10, 120, math.MaxUint32} {
		t.Run(fmt.Sprint(timeout), func(t *testing.T) {
			factory, captures := scopeCaptureFactory(t)
			orders := NewOrdersService(factory, auditCatalog(t), nil, nil, nil, nil, nil)
			_, _ = orders.CancelAllAfter(context.Background(), nil, timeout, nil, nil, nil, nil)
			if len(captures) != 1 {
				t.Fatal("valid timeout never reached transport")
			}
			if got := capturedField(<-captures, "timeout_sec").Uint(); got != uint64(timeout) {
				t.Fatalf("timeout_sec = %d, want %d", got, timeout)
			}
		})
	}
}

// A guard chain ID that does not fit the wire uint32 is refused, not
// wrapped to another chain.
func TestSignProtectedActionRefusesChainIDsOutsideUint32(t *testing.T) {
	addresses := []string{"0x0000000000000000000000000000000000000001"}
	for _, chainID := range []int{-1, 1 << 32, 1<<32 + 1, math.MaxInt64} {
		t.Run(fmt.Sprint(chainID), func(t *testing.T) {
			factory, captures := scopeCaptureFactory(t)
			guard := NewGuardSignerService(factory, nil)
			_, err := guard.SignProtectedAction(context.Background(), "funding_add_external_whitelist", nil, nil, &chainID, addresses, nil, nil)
			requireRefusedBeforeTransport(t, err, captures)
		})
	}
	for _, chainID := range []int{0, 137, math.MaxUint32} {
		t.Run(fmt.Sprint(chainID), func(t *testing.T) {
			factory, captures := scopeCaptureFactory(t)
			guard := NewGuardSignerService(factory, nil)
			_, _ = guard.SignProtectedAction(context.Background(), "funding_add_external_whitelist", nil, nil, &chainID, addresses, nil, nil)
			if len(captures) != 1 {
				t.Fatal("valid chain ID never reached transport")
			}
			if got := capturedField(<-captures, "args", "external_whitelist", "polychain_chain_id").Uint(); got != uint64(chainID) {
				t.Fatalf("polychain_chain_id = %d, want %d", got, chainID)
			}
		})
	}
}

// A batch action's chain ID must be a whole number in the uint32 range:
// fractions, out-of-range values and non-numbers are refused instead of
// truncated, wrapped or dropped.
func TestBatchSignRefusesChainIDsOutsideUint32(t *testing.T) {
	refused := map[string]any{
		"fraction":       1.5,
		"float above":    float64(1<<32 + 1),
		"float below":    float64(-1),
		"NaN":            math.NaN(),
		"int above":      1 << 32,
		"int64 above":    int64(1<<32 + 1),
		"int negative":   -1,
		"uint64 above":   uint64(1 << 32),
		"decimal text":   "137",
		"bool":           true,
		"float32 frac":   float32(2.5),
		"int32 negative": int32(-5),
	}
	for name, chainID := range refused {
		t.Run(name, func(t *testing.T) {
			factory, captures := scopeCaptureFactory(t)
			guard := NewGuardSignerService(factory, nil)
			actions := []map[string]any{{
				"action":                      "funding_add_external_whitelist",
				"external_polychain_chain_id": chainID,
				"external_addresses":          []any{"0x0000000000000000000000000000000000000001"},
			}}
			_, err := guard.BatchSignProtectedActions(context.Background(), actions, nil, nil)
			requireRefusedBeforeTransport(t, err, captures)
		})
	}
	accepted := map[string]any{
		"float64": float64(137), "int": 137, "int64": int64(137), "uint32": uint32(137),
		"uint64 max": uint64(math.MaxUint32), "float64 max": float64(math.MaxUint32),
	}
	for name, chainID := range accepted {
		t.Run(name, func(t *testing.T) {
			factory, captures := scopeCaptureFactory(t)
			guard := NewGuardSignerService(factory, nil)
			actions := []map[string]any{{
				"action":                      "funding_add_external_whitelist",
				"external_polychain_chain_id": chainID,
				"external_addresses":          []any{"0x0000000000000000000000000000000000000001"},
			}}
			_, _ = guard.BatchSignProtectedActions(context.Background(), actions, nil, nil)
			if len(captures) != 1 {
				t.Fatal("valid chain ID never reached transport")
			}
			items := capturedField(<-captures, "actions").List()
			args := items.Get(0).Message()
			got := args.Get(args.Descriptor().Fields().ByName("args")).Message()
			ext := got.Get(got.Descriptor().Fields().ByName("external_whitelist")).Message()
			chain := ext.Get(ext.Descriptor().Fields().ByName("polychain_chain_id")).Uint()
			want := map[string]uint64{"uint64 max": math.MaxUint32, "float64 max": math.MaxUint32}[name]
			if want == 0 {
				want = 137
			}
			if chain != want {
				t.Fatalf("polychain_chain_id = %d, want %d", chain, want)
			}
		})
	}
}

// Go resolves the market before any scale: a padded symbol is an unknown
// market on create, preview, batch create and modify, and a scaled quantity
// at another scale than the market's is refused, all before transport.
func TestOrderWritesValidateScaleAgainstTheResolvedMarket(t *testing.T) {
	price := models.PriceFromDecimal("100")
	oneBTC := func(scale int) *models.QtyInput {
		q, err := models.NewQtyScaled(int64(math.Pow10(scale)))
		if err != nil {
			t.Fatal(err)
		}
		in := models.QtyFromScaled(q.WithScale(scale))
		return &in
	}
	order := func(symbol string, qty *models.QtyInput) models.CreateOrderRequest {
		return models.CreateOrderRequest{Symbol: &symbol, Side: "buy", OrderType: "limit", Price: &price, Qty: *qty}
	}
	// auditCatalog's BTC-USDT has base_quantity_scale 3.
	for _, symbol := range []string{" BTC-USDT", "BTC-USDT ", " BTC-USDT "} {
		for _, scale := range []int{1, 3, 5} {
			name := fmt.Sprintf("%q/scale=%d", symbol, scale)
			calls := map[string]func(*OrdersService) error{
				"create": func(o *OrdersService) error {
					_, err := o.Create(context.Background(), order(symbol, oneBTC(scale)), nil)
					return err
				},
				"preview": func(o *OrdersService) error {
					_, err := o.Preview(context.Background(), order(symbol, oneBTC(scale)), nil)
					return err
				},
				"batch": func(o *OrdersService) error {
					_, err := o.BatchCreate(context.Background(), nil, []models.CreateOrderRequest{order("BTC-USDT", oneBTC(3)), order(symbol, oneBTC(scale))}, nil, nil, nil, false)
					return err
				},
				"modify": func(o *OrdersService) error {
					_, err := o.ModifyWith(context.Background(), nil, ModifyParams{Symbol: symbol, Key: models.OrderKeyByID(codecs.FormatID(9)), NewQty: oneBTC(scale)})
					return err
				},
			}
			for call, run := range calls {
				t.Run(call+"/"+name, func(t *testing.T) {
					factory, captures := scopeCaptureFactory(t)
					orders := NewOrdersService(factory, auditCatalog(t), nil, nil, nil, nil, nil)
					requireRefusedBeforeTransport(t, run(orders), captures)
				})
			}
		}
	}
	for _, scale := range []int{1, 5} {
		t.Run(fmt.Sprintf("batch/plain/scale=%d", scale), func(t *testing.T) {
			factory, captures := scopeCaptureFactory(t)
			orders := NewOrdersService(factory, auditCatalog(t), nil, nil, nil, nil, nil)
			_, err := orders.BatchCreate(context.Background(), nil, []models.CreateOrderRequest{order("BTC-USDT", oneBTC(3)), order("BTC-USDT", oneBTC(scale))}, nil, nil, nil, false)
			requireRefusedBeforeTransport(t, err, captures)
		})
	}
	t.Run("batch/plain/scale=3", func(t *testing.T) {
		factory, captures := scopeCaptureFactory(t)
		orders := NewOrdersService(factory, auditCatalog(t), nil, nil, nil, nil, nil)
		_, _ = orders.BatchCreate(context.Background(), nil, []models.CreateOrderRequest{order("BTC-USDT", oneBTC(3))}, nil, nil, nil, false)
		if len(captures) != 1 {
			t.Fatal("matching scale never reached transport")
		}
		item := capturedField(<-captures, "items").List().Get(0).Message()
		qty := item.Get(item.Descriptor().Fields().ByName("base_qty_scaled")).Int()
		if qty != 1000 {
			t.Fatalf("base_qty_scaled = %d, want 1000", qty)
		}
	})
}
