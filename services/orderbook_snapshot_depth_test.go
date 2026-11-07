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
	"github.com/Fabric-Labs/polyester-sdk-go/hardening"
	"github.com/Fabric-Labs/polyester-sdk-go/transport"
	"google.golang.org/protobuf/proto"
)

// POLY-8586: Get reports the depth it requested on the wire. A depth between
// snapshot depths rides the smallest one covering it (as the managed book does
// with stream depths), 0 is the default 50, and a depth below 0 or above 1000
// is refused before any request.
func TestTicket8586SnapshotReportsWireDepth(t *testing.T) {
	var wire atomic.Int32
	var calls atomic.Int32
	srv := hardening.NewTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		body, _ := io.ReadAll(r.Body)
		req := new(ob.GetOrderBookRequest)
		if err := proto.Unmarshal(body, req); err != nil {
			t.Error(err)
			return
		}
		wire.Store(int32(req.GetDepth()))
		w.Header().Set("Content-Type", "application/proto")
		data, _ := proto.Marshal(&ob.GetOrderBookResponse{SymbolId: 1, BookSeq: 7, Available: true})
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

	for _, tc := range []struct {
		depth int
		wire  ob.Depth
		want  int
	}{
		{1, ob.Depth_DEPTH_1, 1},
		{2, ob.Depth_DEPTH_5, 5},
		{5, ob.Depth_DEPTH_5, 5},
		{10, ob.Depth_DEPTH_10, 10},
		{11, ob.Depth_DEPTH_20, 20},
		{15, ob.Depth_DEPTH_20, 20},
		{20, ob.Depth_DEPTH_20, 20},
		{0, ob.Depth_DEPTH_50, 50},
		{21, ob.Depth_DEPTH_50, 50},
		{101, ob.Depth_DEPTH_200, 200},
		{501, ob.Depth_DEPTH_1000, 1000},
		{1000, ob.Depth_DEPTH_1000, 1000},
	} {
		wire.Store(-1)
		got, err := svc.Get(context.Background(), "BTC-USDT", tc.depth)
		if err != nil {
			t.Fatalf("depth %d: %v", tc.depth, err)
		}
		if ob.Depth(wire.Load()) != tc.wire || got.Depth != tc.want {
			t.Fatalf("depth %d: wire=%v reported=%d, want wire=%v reported=%d", tc.depth, ob.Depth(wire.Load()), got.Depth, tc.wire, tc.want)
		}
	}

	before := calls.Load()
	for _, depth := range []int{-1, -2, 1001, 5000} {
		_, err := svc.Get(context.Background(), "BTC-USDT", depth)
		var validation *sdkerrors.ValidationError
		if !errors.As(err, &validation) {
			t.Fatalf("depth %d: err=%v, want ValidationError", depth, err)
		}
	}
	if calls.Load() != before {
		t.Fatalf("refused depths reached the server: %d calls", calls.Load()-before)
	}
}
