package services

import (
	"context"
	"errors"
	"io"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Fabric-Labs/polyester-sdk-go/catalogs"
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	ob "github.com/Fabric-Labs/polyester-sdk-go/gen/orderbook/v1"
	typev1 "github.com/Fabric-Labs/polyester-sdk-go/gen/polyester/type/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/internal/hardening"
	"github.com/Fabric-Labs/polyester-sdk-go/orderbook"
	"github.com/Fabric-Labs/polyester-sdk-go/transport"
	"google.golang.org/protobuf/proto"
)

// snapshotDepthServer answers GetOrderBook as the contract describes: depth is
// a DepthTier, DEPTH_UNSPECIFIED serves DEPTH_50, and the reply names the tier
// served. served, when set, replaces the tier the reply names.
func snapshotDepthServer(t *testing.T, wire *atomic.Int32, calls *atomic.Int32, served *typev1.DepthTier, available bool) *OrderbookService {
	t.Helper()
	srv := hardening.NewTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		body, _ := io.ReadAll(r.Body)
		req := new(ob.GetOrderBookRequest)
		if err := proto.Unmarshal(body, req); err != nil {
			t.Error(err)
			return
		}
		wire.Store(int32(req.GetDepth()))
		depth := req.GetDepth()
		if depth == typev1.DepthTier_DEPTH_UNSPECIFIED {
			depth = typev1.DepthTier_DEPTH_50
		}
		if served != nil {
			depth = *served
		}
		response := &ob.GetOrderBookResponse{SymbolId: 1, BookSeq: 7, Depth: depth, Available: available}
		if available {
			response.BookEpoch = 3
			response.Bids = []*ob.PriceLevel{{PriceTicks: 65_000_000_000_000, QtyScaled: 125_000_000}}
		}
		w.Header().Set("Content-Type", "application/proto")
		data, _ := proto.Marshal(response)
		_, _ = w.Write(data)
	}))
	t.Cleanup(srv.Close)
	cats := catalogs.NewManager()
	if err := cats.HydrateSpotConfig(map[string]any{"pairs": []any{
		map[string]any{"symbol": "BTC-USDT", "symbol_id": 1, "base_quantity_scale": 8},
	}}); err != nil {
		t.Fatal(err)
	}
	return NewOrderbookService(transport.NewFactory(transport.Config{APIURL: srv.URL, Timeout: 10 * time.Second}, nil, nil), cats, nil)
}

// Get sends a depth tier as the contract's DepthTier, 0 as DEPTH_50, and
// reports the tier the service served.
func TestOrderbookGetSendsTheDepthTier(t *testing.T) {
	var wire, calls atomic.Int32
	svc := snapshotDepthServer(t, &wire, &calls, nil, true)
	for _, tc := range []struct {
		depth    int
		wire     typev1.DepthTier
		reported int
	}{
		{0, typev1.DepthTier_DEPTH_50, 50},
		{1, typev1.DepthTier_DEPTH_1, 1},
		{5, typev1.DepthTier_DEPTH_5, 5},
		{20, typev1.DepthTier_DEPTH_20, 20},
		{1000, typev1.DepthTier_DEPTH_1000, 1000},
		{5000, typev1.DepthTier_DEPTH_5000, 5000},
	} {
		wire.Store(-1)
		got, err := svc.Get(context.Background(), "BTC-USDT", tc.depth)
		if err != nil {
			t.Fatalf("depth %d: %v", tc.depth, err)
		}
		if typev1.DepthTier(wire.Load()) != tc.wire || got.Depth != tc.reported {
			t.Fatalf("depth %d: wire=%v reported=%d, want wire=%v reported=%d", tc.depth, typev1.DepthTier(wire.Load()), got.Depth, tc.wire, tc.reported)
		}
		if got.BookEpoch != "3" || got.BookSeq != "7" || !got.Available || len(got.Bids) != 1 {
			t.Fatalf("depth %d: snapshot %+v", tc.depth, got)
		}
		if qty, err := got.Bids[0].Qty.Format(); err != nil || qty != "1.25" {
			t.Fatalf("depth %d: bid qty %q, %v", tc.depth, qty, err)
		}
	}
}

// A depth that is no tier is refused before any request: the contract
// serves the tiers only.
func TestOrderbookGetRefusesADepthThatIsNoTier(t *testing.T) {
	var wire, calls atomic.Int32
	svc := snapshotDepthServer(t, &wire, &calls, nil, true)
	for _, depth := range []int{-1, 2, 37, 1001, 2500, 5001} {
		_, err := svc.Get(context.Background(), "BTC-USDT", depth)
		var validation *sdkerrors.ValidationError
		if !errors.As(err, &validation) {
			t.Fatalf("depth %d: err=%v, want ValidationError", depth, err)
		}
	}
	if calls.Load() != 0 {
		t.Fatalf("refused depths reached the server: %d calls", calls.Load())
	}
}

// The contract serves the requested tier and names it in every reply,
// available or not: a reply naming none, an unknown one or another tier is
// an *errors.ResponseContractError, never a snapshot at a depth it did not
// serve.
func TestOrderbookGetRefusesAReplyNotAtTheRequestedTier(t *testing.T) {
	for name, served := range map[string]typev1.DepthTier{
		"unset":        typev1.DepthTier_DEPTH_UNSPECIFIED,
		"unknown":      typev1.DepthTier(99),
		"another tier": typev1.DepthTier_DEPTH_200,
	} {
		for _, available := range []bool{true, false} {
			var wire, calls atomic.Int32
			svc := snapshotDepthServer(t, &wire, &calls, &served, available)
			got, err := svc.Get(context.Background(), "BTC-USDT", 100)
			var contract *sdkerrors.ResponseContractError
			if !errors.As(err, &contract) {
				t.Fatalf("%s, available %v: got %+v, %v; want a ResponseContractError", name, available, got, err)
			}
		}
	}
}

// An unavailable snapshot reports epoch 0, so it orders before any available
// snapshot.
func TestOrderbookGetUnavailableSnapshot(t *testing.T) {
	var wire, calls atomic.Int32
	svc := snapshotDepthServer(t, &wire, &calls, nil, false)
	got, err := svc.Get(context.Background(), "BTC-USDT", 0)
	if err != nil {
		t.Fatal(err)
	}
	if got.Available || got.BookEpoch != "0" || got.Depth != 50 || len(got.Bids)+len(got.Asks) != 0 {
		t.Fatalf("unavailable snapshot: %+v", got)
	}
	newer := got
	newer.BookEpoch, newer.BookSeq, newer.Available = "1", "0", true
	if c, err := orderbook.CompareSnapshots(got, newer); err != nil || c != -1 {
		t.Fatalf("CompareSnapshots(unavailable, epoch 1) = %d, %v; want -1", c, err)
	}
}
