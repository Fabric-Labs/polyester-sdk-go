package hardening_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"
	polyester "github.com/Fabric-Labs/polyester-sdk-go"
	"github.com/Fabric-Labs/polyester-sdk-go/auth"
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	orderv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/orders/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/gen/orders/v1/ordersv1connect"
	"github.com/Fabric-Labs/polyester-sdk-go/hardening"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

// tradesPage is one GetOrder execution page. Page i of a poll is requested
// with page token "t<i>"; page 0 with no token.
type tradesPage struct {
	trades   []int64
	next     string
	complete bool
	delay    time.Duration
	// hang blocks the page until the client gives up on the request.
	hang bool
}

// tradesWaitServer answers GetOrder from a schedule of polls. A request
// without a page token starts the next poll; the last poll repeats.
type tradesWaitServer struct {
	ordersv1connect.UnimplementedOrdersReadServiceHandler
	cum   int64
	polls [][]tradesPage
	// statuses is the order's status on each poll, the last one repeating.
	// The order reads FILLED when statuses is empty.
	statuses []orderv1.OrderStatus

	mu       sync.Mutex
	poll     int
	reads    int // polls started after the first
	started  bool
	arrivals []time.Time
}

func (s *tradesWaitServer) GetOrder(ctx context.Context, req *connect.Request[orderv1.GetOrderRequest]) (*connect.Response[orderv1.GetOrderResponse], error) {
	s.mu.Lock()
	s.arrivals = append(s.arrivals, time.Now())
	token := req.Msg.GetPageToken()
	if token == "" {
		if s.started {
			s.reads++
			if s.poll < len(s.polls)-1 {
				s.poll++
			}
		}
		s.started = true
	}
	pages := s.polls[s.poll]
	status := orderv1.OrderStatus_FILLED
	if len(s.statuses) > 0 {
		status = s.statuses[min(s.reads, len(s.statuses)-1)]
	}
	s.mu.Unlock()
	idx := 0
	if token != "" {
		parsed, err := strconv.Atoi(strings.TrimPrefix(token, "t"))
		if err != nil || parsed <= 0 || parsed >= len(pages) {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("unknown page token %q", token))
		}
		idx = parsed
	}
	page := pages[idx]
	if page.hang {
		<-ctx.Done()
		return nil, connect.NewError(connect.CodeCanceled, ctx.Err())
	}
	if page.delay > 0 {
		timer := time.NewTimer(page.delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, connect.NewError(connect.CodeCanceled, ctx.Err())
		case <-timer.C:
		}
	}
	resp := &orderv1.GetOrderResponse{
		Order:              &orderv1.Order{OrderId: 1, SymbolId: 1, Status: status, CumQtyScaled: s.cum},
		NextPageToken:      page.next,
		ExecutionsComplete: page.complete,
	}
	for i, qty := range page.trades {
		resp.Trades = append(resp.Trades, &orderv1.UserTrade{
			SymbolId: 1, OrderId: 1, MatchId: uint64(1000*idx + i + 1), QtyScaled: qty,
		})
	}
	return connect.NewResponse(resp), nil
}

func (s *tradesWaitServer) requestArrivals() []time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.arrivals)
}

func newTradesWaitClient(t *testing.T, handler *tradesWaitServer) *polyester.Client {
	t.Helper()
	return newTradesWaitClientWith(t, handler, nil)
}

// newTradesWaitClientWith builds the client over httpClient (the SDK default
// when nil).
func newTradesWaitClientWith(t *testing.T, handler *tradesWaitServer, httpClient *http.Client) *polyester.Client {
	t.Helper()
	mux := http.NewServeMux()
	path, h := ordersv1connect.NewOrdersReadServiceHandler(handler)
	mux.Handle(path, h)
	srv := hardening.NewTestServer(mux)
	t.Cleanup(srv.Close)
	kp := auth.GenerateEd25519Keypair()
	client, err := polyester.New(polyester.Config{
		APIURL:               srv.URL,
		APIKeyID:             "ak_test",
		APIPrivateKey:        kp.SecretKeyHex,
		SkipCatalogHydration: true,
		// Far above every wait timeout below: a page read that outlives
		// its wait shows up as a multi-second return.
		Timeout:    5 * time.Second,
		HTTPClient: httpClient,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	return client
}

func waitTrades(ctx context.Context, client *polyester.Client, timeout time.Duration) (models.GetOrderResult, error) {
	return client.Orders.WaitForOrderTradesComplete(ctx, nil, models.OrderKeyByID("2"), nil, timeout)
}

// isTradesWaitTimeout reports the error WaitForOrderTradesComplete returns
// when its own timeout passes.
func isTradesWaitTimeout(err error) bool {
	return err != nil && strings.Contains(err.Error(), "order trades not complete within")
}

func tradeQtys(result models.GetOrderResult) []int64 {
	out := make([]int64, 0, len(result.Trades))
	for _, trade := range result.Trades {
		out = append(out, trade.Qty.Scaled())
	}
	return out
}

// The assembled result carries the server's executions_complete from the
// read it was assembled from, whether that read had one page or several.
func TestWaitForOrderTradesCompleteKeepsExecutionsComplete(t *testing.T) {
	cases := []struct {
		name  string
		polls [][]tradesPage
		want  bool
	}{
		{"single page", [][]tradesPage{{{trades: []int64{100}, complete: true}}}, true},
		{"terminal page of three", [][]tradesPage{{
			{trades: []int64{40}, next: "t1", complete: true},
			{trades: []int64{20}, next: "t2", complete: true},
			{trades: []int64{40}, complete: true},
		}}, true},
		{"recorded on a later poll", [][]tradesPage{
			{{trades: []int64{40}, next: "t1"}, {trades: nil}},
			{{trades: []int64{40}, next: "t1", complete: true}, {trades: []int64{60}, complete: true}},
		}, true},
		{"server still reports incomplete", [][]tradesPage{{
			{trades: []int64{50}, next: "t1"},
			{trades: []int64{50}},
		}}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := newTradesWaitClient(t, &tradesWaitServer{cum: 100, polls: tc.polls})
			result, err := waitTrades(context.Background(), client, 2*time.Second)
			if err != nil {
				t.Fatal(err)
			}
			var sum int64
			for _, qty := range tradeQtys(result) {
				sum += qty
			}
			if sum != 100 {
				t.Fatalf("trade qtys=%v, want sum 100", tradeQtys(result))
			}
			if result.ExecutionsComplete != tc.want {
				t.Fatalf("ExecutionsComplete=%v, want %v (the final read's executions_complete)", result.ExecutionsComplete, tc.want)
			}
		})
	}
}

// The wait reports completion only for a terminal order (filled, canceled or
// rejected) whose recorded trades add up to its cum_qty. executions_complete
// alone is not enough: it is also true for a working order whose fills so far
// are all recorded. The Python and Rust SDKs run the same cases.
func TestWaitForOrderTradesCompleteRequiresATerminalOrder(t *testing.T) {
	const (
		working  = orderv1.OrderStatus_WORKING
		filled   = orderv1.OrderStatus_FILLED
		canceled = orderv1.OrderStatus_CANCELED
		rejected = orderv1.OrderStatus_REJECTED
	)
	cases := []struct {
		name     string
		cum      int64
		trades   []int64
		statuses []orderv1.OrderStatus
		// wantStatus is the status of the read the wait returns.
		wantStatus string
		wantDone   bool
	}{
		{"partially filled working order", 40, []int64{40}, []orderv1.OrderStatus{working}, "working", false},
		{"terminal fully filled order", 100, []int64{60, 40}, []orderv1.OrderStatus{filled}, "filled", true},
		{"terminal canceled order with partial fills", 40, []int64{40}, []orderv1.OrderStatus{canceled}, "canceled", true},
		{"terminal order with no fills", 0, nil, []orderv1.OrderStatus{rejected}, "rejected", true},
		{"partially filled order canceled on a later poll", 40, []int64{40}, []orderv1.OrderStatus{working, working, canceled}, "canceled", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := newTradesWaitClient(t, &tradesWaitServer{
				cum:      tc.cum,
				polls:    [][]tradesPage{{{trades: tc.trades, complete: true}}},
				statuses: tc.statuses,
			})
			result, err := waitTrades(context.Background(), client, 500*time.Millisecond)
			if tc.wantDone && err != nil {
				t.Fatalf("want completion, got %v", err)
			}
			if !tc.wantDone && !isTradesWaitTimeout(err) {
				t.Fatalf("want the wait's timeout error for a working order, got %v (status %q)", err, orderStatus(result))
			}
			if got := orderStatus(result); got != tc.wantStatus {
				t.Fatalf("returned read has status %q, want %q", got, tc.wantStatus)
			}
			if !result.ExecutionsComplete {
				t.Fatal("returned read lost executions_complete")
			}
		})
	}
}

func orderStatus(result models.GetOrderResult) string {
	if result.Order == nil {
		return ""
	}
	return result.Order.Status
}

// Four fast pages within the timeout succeed with every page's trades.
func TestWaitForOrderTradesCompleteFollowsFastPages(t *testing.T) {
	client := newTradesWaitClient(t, &tradesWaitServer{cum: 100, polls: [][]tradesPage{{
		{trades: []int64{10}, next: "t1", complete: true},
		{trades: []int64{20}, next: "t2", complete: true},
		{trades: []int64{30}, next: "t3", complete: true},
		{trades: []int64{40}, complete: true},
	}}})
	result, err := waitTrades(context.Background(), client, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if got := tradeQtys(result); !slices.Equal(got, []int64{10, 20, 30, 40}) {
		t.Fatalf("trade qtys=%v", got)
	}
}

// Four 35 ms pages need about 140 ms; a 50 ms wait must time out instead of
// returning success after its deadline.
func TestWaitForOrderTradesCompleteTimesOutAcrossSlowPages(t *testing.T) {
	const timeout = 50 * time.Millisecond
	pages := make([]tradesPage, 4)
	for i := range pages {
		pages[i] = tradesPage{trades: []int64{25}, complete: true, delay: 35 * time.Millisecond}
		if i < len(pages)-1 {
			pages[i].next = fmt.Sprintf("t%d", i+1)
		}
	}
	client := newTradesWaitClient(t, &tradesWaitServer{cum: 100, polls: [][]tradesPage{pages}})
	started := time.Now()
	result, err := waitTrades(context.Background(), client, timeout)
	elapsed := time.Since(started)
	if err == nil {
		t.Fatalf("wait with a %s timeout succeeded after %s with %d trades", timeout, elapsed, len(result.Trades))
	}
	if !isTradesWaitTimeout(err) {
		t.Fatalf("want a timeout error, got %T: %v", err, err)
	}
}

// A page read still in flight when the wait's timeout passes is abandoned:
// the wait returns near its timeout, not when the HTTP timeout ends the read.
func TestWaitForOrderTradesCompleteAbandonsAPageAtItsTimeout(t *testing.T) {
	const timeout = 100 * time.Millisecond
	client := newTradesWaitClient(t, &tradesWaitServer{cum: 100, polls: [][]tradesPage{{
		{trades: []int64{40}, next: "t1"},
		{hang: true},
	}}})
	started := time.Now()
	_, err := waitTrades(context.Background(), client, timeout)
	elapsed := time.Since(started)
	if elapsed > timeout+time.Second {
		t.Fatalf("wait with a %s timeout returned after %s: %v", timeout, elapsed, err)
	}
	if !isTradesWaitTimeout(err) {
		t.Fatalf("want a timeout error, got %T: %v", err, err)
	}
}

// lateTransport answers each request in full, then holds the answer until
// the request's deadline has passed: a page that arrived in time but whose
// handling finished after the wait's timeout.
type lateTransport struct{ base http.RoundTripper }

func (l lateTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := l.base.RoundTrip(req)
	if err != nil {
		return nil, err
	}
	body, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil {
		return nil, err
	}
	deadline, _ := req.Context().Deadline()
	resp.Body = &lateEOFBody{Reader: bytes.NewReader(body), deadline: deadline}
	return resp, nil
}

// lateEOFBody hands over the whole body at once, then reports its end only
// once the deadline has passed.
type lateEOFBody struct {
	*bytes.Reader
	deadline time.Time
}

func (b *lateEOFBody) Read(p []byte) (int, error) {
	n, err := b.Reader.Read(p)
	if errors.Is(err, io.EOF) && !b.deadline.IsZero() {
		time.Sleep(time.Until(b.deadline) + 20*time.Millisecond)
	}
	return n, err
}

func (b *lateEOFBody) Close() error { return nil }

// A complete read whose handling ends after the timeout is no success: the
// wait times out and still returns that read as its last result. The
// deadline is checked on the clock, not only through the read's context.
func TestWaitForOrderTradesCompleteRefusesASuccessFinishedAfterItsTimeout(t *testing.T) {
	handler := &tradesWaitServer{cum: 100, polls: [][]tradesPage{{{trades: []int64{100}, complete: true}}}}
	httpClient := &http.Client{Transport: lateTransport{base: http.DefaultTransport}, Timeout: 5 * time.Second}
	client := newTradesWaitClientWith(t, handler, httpClient)
	result, err := waitTrades(context.Background(), client, 50*time.Millisecond)
	if err == nil {
		t.Fatalf("wait succeeded with a read handled after its timeout: %d trades", len(result.Trades))
	}
	if !isTradesWaitTimeout(err) || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("want the wait's timeout error, got %T: %v", err, err)
	}
	if got := tradeQtys(result); !slices.Equal(got, []int64{100}) || !result.ExecutionsComplete {
		t.Fatalf("late read not kept as the last result: trades=%v complete=%v", got, result.ExecutionsComplete)
	}
}

// A next_page_token that repeats an earlier page token would page forever:
// the wait stops with a response contract error.
func TestWaitForOrderTradesCompleteRejectsARepeatedPageToken(t *testing.T) {
	handler := &tradesWaitServer{cum: 100, polls: [][]tradesPage{{
		{trades: []int64{10}, next: "t1"},
		{trades: []int64{10}, next: "t2"},
		{trades: []int64{10}, next: "t1"},
	}}}
	client := newTradesWaitClient(t, handler)
	// An independent outer bound, so a looping wait fails the test instead
	// of hanging it.
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, err := waitTrades(ctx, client, 5*time.Second)
	var contractErr *sdkerrors.ResponseContractError
	if !errors.As(err, &contractErr) {
		t.Fatalf("want *errors.ResponseContractError, got %T: %v (after %d page reads)", err, err, len(handler.requestArrivals()))
	}
	if got := len(handler.requestArrivals()); got != 3 {
		t.Fatalf("page reads=%d, want 3", got)
	}
}
