package services

import (
	"context"
	"errors"
	"io"
	"math"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/Fabric-Labs/polyester-sdk-go/catalogs"
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	ob "github.com/Fabric-Labs/polyester-sdk-go/gen/orderbook/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/hardening"
	"github.com/Fabric-Labs/polyester-sdk-go/orderbook"
	"github.com/Fabric-Labs/polyester-sdk-go/transport"
	"google.golang.org/protobuf/proto"
)

const testMaxDepth = 5000

// snapshotDepthServer answers GetOrderBook as the contract describes: depth is
// a level count per side up to max_depth, 0 means 50, and a deeper request is
// refused with invalid_argument, never truncated. Every response reports
// max_depth, and an available one its book_epoch.
func snapshotDepthServer(t *testing.T, wire *atomic.Int64, calls *atomic.Int32) *OrderbookService {
	t.Helper()
	srv := hardening.NewTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		body, _ := io.ReadAll(r.Body)
		req := new(ob.GetOrderBookRequest)
		if err := proto.Unmarshal(body, req); err != nil {
			t.Error(err)
			return
		}
		wire.Store(int64(req.GetDepth()))
		if req.GetDepth() > testMaxDepth {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"code":"invalid_argument","message":"depth 5001 exceeds max_depth 5000"}`))
			return
		}
		w.Header().Set("Content-Type", "application/proto")
		data, _ := proto.Marshal(&ob.GetOrderBookResponse{
			SymbolId: 1, BookSeq: 7, BookEpoch: 3, MaxDepth: testMaxDepth, Available: true,
			Bids: []*ob.PriceLevel{{PriceTicks: 65_000_000_000_000, QtyScaled: 125_000_000}},
		})
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

// Get sends depth as the level count it is: no rounding up to a fixed list of
// depths and no local maximum, since the service reports its own. 0 is sent as
// 0 and reported as the default 50.
func TestOrderbookGetSendsDepthAsGiven(t *testing.T) {
	var wire atomic.Int64
	var calls atomic.Int32
	svc := snapshotDepthServer(t, &wire, &calls)
	for _, tc := range []struct {
		depth    int
		wire     int64
		reported int
	}{
		{0, 0, 50},
		{1, 1, 1},
		{2, 2, 2},
		{20, 20, 20},
		{1001, 1001, 1001},
		{2500, 2500, 2500},
		{testMaxDepth, testMaxDepth, testMaxDepth},
	} {
		wire.Store(-1)
		got, err := svc.Get(context.Background(), "BTC-USDT", tc.depth)
		if err != nil {
			t.Fatalf("depth %d: %v", tc.depth, err)
		}
		if wire.Load() != tc.wire || got.Depth != tc.reported {
			t.Fatalf("depth %d: wire=%d reported=%d, want wire=%d reported=%d", tc.depth, wire.Load(), got.Depth, tc.wire, tc.reported)
		}
		if got.BookEpoch != "3" || got.BookSeq != "7" || got.MaxDepth != testMaxDepth || !got.Available {
			t.Fatalf("depth %d: epoch=%q seq=%q max=%d available=%v", tc.depth, got.BookEpoch, got.BookSeq, got.MaxDepth, got.Available)
		}
		if len(got.Bids) != 1 || got.Bids[0].Price.Ticks() != 65_000_000_000_000 {
			t.Fatalf("depth %d: bids %+v", tc.depth, got.Bids)
		}
		if qty, err := got.Bids[0].Qty.Format(); err != nil || qty != "1.25" {
			t.Fatalf("depth %d: bid qty %q, %v", tc.depth, qty, err)
		}
	}
}

// A depth above the service's maximum reaches the service, which refuses it
// with invalid_argument; the SDK surfaces that refusal and invents no cap.
func TestOrderbookGetDepthAboveMaxIsRefusedByService(t *testing.T) {
	var wire atomic.Int64
	var calls atomic.Int32
	svc := snapshotDepthServer(t, &wire, &calls)
	_, err := svc.Get(context.Background(), "BTC-USDT", testMaxDepth+1)
	var validation *sdkerrors.ValidationError
	if !errors.As(err, &validation) || validation.Code != connect.CodeInvalidArgument.String() {
		t.Fatalf("err=%#v, want the service's invalid_argument as a ValidationError", err)
	}
	if calls.Load() != 1 || wire.Load() != testMaxDepth+1 {
		t.Fatalf("calls=%d wire=%d, want the depth sent once as given", calls.Load(), wire.Load())
	}
	if sdkerrors.IsRetryable(err) || sdkerrors.MutationOutcomeUnknown(err) {
		t.Fatal("a refused depth is final")
	}
}

// Only what the uint32 field cannot hold is refused before any request.
func TestOrderbookGetRefusesDepthOutsideUint32(t *testing.T) {
	var wire atomic.Int64
	var calls atomic.Int32
	svc := snapshotDepthServer(t, &wire, &calls)
	depths := []int{-1, -50}
	if math.MaxInt > math.MaxUint32 {
		depths = append(depths, int(uint64(math.MaxUint32)+1))
	}
	for _, depth := range depths {
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

// An unavailable snapshot reports max_depth and epoch 0, so it orders before
// any available snapshot.
func TestOrderbookGetUnavailableSnapshotReportsMaxDepth(t *testing.T) {
	srv := hardening.NewTestServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/proto")
		data, _ := proto.Marshal(&ob.GetOrderBookResponse{SymbolId: 1, MaxDepth: testMaxDepth})
		_, _ = w.Write(data)
	}))
	t.Cleanup(srv.Close)
	cats := catalogs.NewManager()
	if err := cats.HydrateSpotConfig(map[string]any{"pairs": []any{
		map[string]any{"symbol": "BTC-USDT", "symbol_id": 1, "base_quantity_scale": 8},
	}}); err != nil {
		t.Fatal(err)
	}
	svc := NewOrderbookService(transport.NewFactory(transport.Config{APIURL: srv.URL, Timeout: 10 * time.Second}, nil, nil), cats, nil)
	got, err := svc.Get(context.Background(), "BTC-USDT", 0)
	if err != nil {
		t.Fatal(err)
	}
	if got.Available || got.BookEpoch != "0" || got.MaxDepth != testMaxDepth || len(got.Bids)+len(got.Asks) != 0 {
		t.Fatalf("unavailable snapshot: %+v", got)
	}
	newer := got
	newer.BookEpoch, newer.BookSeq, newer.Available = "1", "0", true
	if c, err := orderbook.CompareSnapshots(got, newer); err != nil || c != -1 {
		t.Fatalf("CompareSnapshots(unavailable, epoch 1) = %d, %v; want -1", c, err)
	}
}
