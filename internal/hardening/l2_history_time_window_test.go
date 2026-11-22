package hardening_test

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"connectrpc.com/connect"
	polyester "github.com/Fabric-Labs/polyester-sdk-go"
	"github.com/Fabric-Labs/polyester-sdk-go/auth"
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	"github.com/Fabric-Labs/polyester-sdk-go/gen/chain/zipper/v1/chainzipperv1connect"
	"github.com/Fabric-Labs/polyester-sdk-go/gen/marketdata/v1/marketdatav1connect"
	orderv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/orders/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/gen/orders/v1/ordersv1connect"
	"github.com/Fabric-Labs/polyester-sdk-go/internal/hardening"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

// timeWindowServer serves the selector catalog and records the time range of
// each order history and user trades request.
type timeWindowServer struct {
	*selectorServer

	mu       sync.Mutex
	history  []timeWindow
	trades   []timeWindow
	requests atomic.Int64
}

// timeWindow is a request's start_ts_ns and end_ts_ns; nil is unset.
type timeWindow struct {
	start, end *uint64
}

func (s *timeWindowServer) GetOrderHistory(_ context.Context, req *connect.Request[orderv1.GetOrderHistoryRequest]) (*connect.Response[orderv1.GetOrderHistoryResponse], error) {
	s.requests.Add(1)
	s.mu.Lock()
	s.history = append(s.history, timeWindow{req.Msg.StartTsNs, req.Msg.EndTsNs})
	s.mu.Unlock()
	return connect.NewResponse(&orderv1.GetOrderHistoryResponse{}), nil
}

func (s *timeWindowServer) GetUserTrades(_ context.Context, req *connect.Request[orderv1.GetUserTradesRequest]) (*connect.Response[orderv1.GetUserTradesResponse], error) {
	s.requests.Add(1)
	s.mu.Lock()
	s.trades = append(s.trades, timeWindow{req.Msg.StartTsNs, req.Msg.EndTsNs})
	s.mu.Unlock()
	return connect.NewResponse(&orderv1.GetUserTradesResponse{}), nil
}

func (s *timeWindowServer) last() (history, trades timeWindow) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.history[len(s.history)-1], s.trades[len(s.trades)-1]
}

func newTimeWindowClient(t *testing.T, wire string) (*polyester.Client, *timeWindowServer) {
	t.Helper()
	server := &timeWindowServer{selectorServer: &selectorServer{}}
	mux := http.NewServeMux()
	mux.Handle(marketdatav1connect.NewMarketDataServiceHandler(server))
	mux.Handle(chainzipperv1connect.NewZipperServiceHandler(server))
	mux.Handle(ordersv1connect.NewOrdersReadServiceHandler(server))
	srv := hardening.NewTestServer(mux)
	t.Cleanup(srv.Close)
	client, err := polyester.New(polyester.Config{
		APIURL:           srv.URL,
		APIKeyID:         "ak_time_window_test",
		APIPrivateKey:    auth.NewSecret(auth.GenerateEd25519Keypair().SecretKeyHex.Reveal()),
		DefaultAccountID: new("7"),
		WireFormat:       wire,
		Timeout:          5 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	if err := client.WaitForCatalogs(context.Background()); err != nil {
		t.Fatal(err)
	}
	return client, server
}

func sameBound(got, want *uint64) bool {
	if got == nil || want == nil {
		return got == nil && want == nil
	}
	return *got == *want
}

func boundString(v *uint64) string {
	if v == nil {
		return "unset"
	}
	return "set"
}

// Orders.ListHistory and Trades.List send start_ts_ns and end_ts_ns as given:
// an unset bound stays unset and an explicit 0 is sent.
func TestL2HistoryAndTradesSendTheTimeWindowAsGiven(t *testing.T) {
	start, end, zero := uint64(1_700_000_000_000_000_000), uint64(1_700_000_003_600_000_000), uint64(0)
	cases := []timeWindow{
		{nil, nil},
		{&start, nil},
		{nil, &end},
		{&start, &end},
		{&start, &start},
		{&zero, nil},
		{nil, &zero},
		// An explicit end of 0 is the contract's empty page, whatever the start.
		{&start, &zero},
	}
	for _, wire := range []string{"json", "binary"} {
		t.Run(wire, func(t *testing.T) {
			client, server := newTimeWindowClient(t, wire)
			ctx := context.Background()
			for i, tc := range cases {
				if _, err := client.Orders.ListHistory(ctx, nil, nil, nil, nil, nil, 10, false, false, nil,
					models.ListOrderHistoryOptions{StartTsNs: tc.start, EndTsNs: tc.end}); err != nil {
					t.Fatalf("case %d: ListHistory: %v", i, err)
				}
				if _, err := client.Trades.List(ctx, nil, nil, nil, nil, 10, nil, nil,
					models.ListUserTradesOptions{StartTsNs: tc.start, EndTsNs: tc.end}); err != nil {
					t.Fatalf("case %d: Trades.List: %v", i, err)
				}
				history, trades := server.last()
				for name, got := range map[string]timeWindow{"history": history, "trades": trades} {
					if !sameBound(got.start, tc.start) || !sameBound(got.end, tc.end) {
						t.Fatalf("case %d %s: sent start %s end %s, want start %s end %s", i, name,
							boundString(got.start), boundString(got.end), boundString(tc.start), boundString(tc.end))
					}
				}
			}
			// Without options, neither bound is sent.
			if _, err := client.Orders.ListHistory(ctx, nil, nil, nil, nil, nil, 10, false, false, nil); err != nil {
				t.Fatal(err)
			}
			if _, err := client.Trades.List(ctx, nil, nil, nil, nil, 10, nil, nil); err != nil {
				t.Fatal(err)
			}
			history, trades := server.last()
			if history.start != nil || history.end != nil || trades.start != nil || trades.end != nil {
				t.Fatalf("no options sent a bound: history %+v trades %+v", history, trades)
			}
		})
	}
}

// A start after a non-zero end is refused with *errors.ValidationError before
// sending, as the contract's ordered_time_range rule refuses it.
func TestL2HistoryAndTradesRefuseAStartAfterTheEnd(t *testing.T) {
	refused := [][2]uint64{
		{1_700_000_003_600_000_001, 1_700_000_003_600_000_000},
		{2, 1},
		{^uint64(0), 1},
	}
	for _, wire := range []string{"json", "binary"} {
		t.Run(wire, func(t *testing.T) {
			client, server := newTimeWindowClient(t, wire)
			ctx := context.Background()
			for _, r := range refused {
				start, end := r[0], r[1]
				_, err := client.Orders.ListHistory(ctx, nil, nil, nil, nil, nil, 10, false, false, nil,
					models.ListOrderHistoryOptions{StartTsNs: &start, EndTsNs: &end})
				assertTimeRangeRefused(t, err, "orders.ListHistory")
				_, err = client.Trades.List(ctx, nil, nil, nil, nil, 10, nil, nil,
					models.ListUserTradesOptions{StartTsNs: &start, EndTsNs: &end})
				assertTimeRangeRefused(t, err, "trades.List")
			}
			if n := server.requests.Load(); n != 0 {
				t.Fatalf("a refused range sent %d requests", n)
			}
		})
	}
}

func assertTimeRangeRefused(t *testing.T, err error, method string) {
	t.Helper()
	var validation *sdkerrors.ValidationError
	if !errors.As(err, &validation) {
		t.Fatalf("%s: error = %T %v, want *errors.ValidationError", method, err, err)
	}
	if msg := err.Error(); !strings.Contains(msg, method) || !strings.Contains(msg, "start_ts_ns") || !strings.Contains(msg, "end_ts_ns") {
		t.Fatalf("%s: error %q does not name the method and range", method, msg)
	}
}
