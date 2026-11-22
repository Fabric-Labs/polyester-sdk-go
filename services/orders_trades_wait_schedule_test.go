package services

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"net/http"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/Fabric-Labs/polyester-sdk-go/auth"
	"github.com/Fabric-Labs/polyester-sdk-go/catalogs"
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	orderv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/orders/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/gen/orders/v1/ordersv1connect"
	"github.com/Fabric-Labs/polyester-sdk-go/internal/hardening"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
	"github.com/Fabric-Labs/polyester-sdk-go/transport"
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

// waitSchedule is one random GetOrder schedule and the outcome a wait
// without a deadline would reach.
type waitSchedule struct {
	cum     int64
	timeout time.Duration
	polls   [][]tradesPage
	// outcome is "success", "cycle" or "timeout" (a hung page, or no poll
	// whose trades add up).
	outcome      string
	wantTrades   []int64
	wantComplete bool
}

func randomWaitSchedule(seed uint64) waitSchedule {
	r := rand.New(rand.NewPCG(seed, 0))
	s := waitSchedule{cum: 100, timeout: time.Duration(30+r.IntN(270)) * time.Millisecond}
	nPolls := 1 + r.IntN(3)
	for p := range nPolls {
		nPages := 1 + r.IntN(5)
		pages := make([]tradesPage, nPages)
		// The last poll adds up unless the schedule wants it short.
		adds := p == nPolls-1 && r.IntN(5) != 0
		remaining := s.cum
		for i := range pages {
			page := &pages[i]
			if i < nPages-1 {
				page.next = fmt.Sprintf("t%d", i+1)
			}
			page.complete = r.IntN(3) != 0
			if r.IntN(3) == 0 {
				page.delay = time.Duration(r.IntN(60)) * time.Millisecond
			} else {
				page.delay = time.Duration(r.IntN(4)) * time.Millisecond
			}
			for range r.IntN(3) {
				if remaining <= 1 {
					break
				}
				qty := 1 + r.Int64N(remaining-1)
				page.trades = append(page.trades, qty)
				remaining -= qty
			}
		}
		if adds && remaining > 0 {
			last := &pages[nPages-1]
			last.trades = append(last.trades, remaining)
		}
		switch r.IntN(8) {
		case 0:
			pages[r.IntN(nPages)].hang = true
		case 1:
			if nPages > 1 {
				// The last page points back at an earlier page token.
				pages[nPages-1].next = fmt.Sprintf("t%d", 1+r.IntN(nPages-1))
			}
		}
		s.polls = append(s.polls, pages)
	}
	s.outcome = "timeout"
	for p := 0; ; p++ {
		pages := s.polls[min(p, len(s.polls)-1)]
		var trades []int64
		seen := map[string]bool{}
		token := ""
		result := ""
		for {
			idx := 0
			if token != "" {
				idx, _ = strconv.Atoi(strings.TrimPrefix(token, "t"))
			}
			page := pages[idx]
			if page.hang {
				result = "timeout"
				break
			}
			trades = append(trades, page.trades...)
			if page.next == "" {
				var sum int64
				for _, qty := range trades {
					sum += qty
				}
				if sum == s.cum {
					result = "success"
					s.wantTrades = trades
					s.wantComplete = page.complete
				}
				break
			}
			if seen[page.next] {
				result = "cycle"
				break
			}
			seen[page.next] = true
			token = page.next
		}
		if result != "" {
			s.outcome = result
			return s
		}
		if p >= len(s.polls)-1 {
			return s
		}
	}
}

// simTradesServer answers GetOrder from a schedule of polls on a simClock: a
// page's delay moves the clock instead of sleeping, and a page that hangs
// moves it to the wait's deadline. A request without a page token starts the
// next poll; the last poll repeats. The order reads FILLED.
type simTradesServer struct {
	ordersv1connect.UnimplementedOrdersReadServiceHandler
	clock *simClock
	cum   int64
	polls [][]tradesPage

	mu       sync.Mutex
	poll     int
	started  bool
	arrivals []time.Time
}

func (s *simTradesServer) GetOrder(ctx context.Context, req *connect.Request[orderv1.GetOrderRequest]) (*connect.Response[orderv1.GetOrderResponse], error) {
	s.mu.Lock()
	s.arrivals = append(s.arrivals, s.clock.Now())
	token := req.Msg.GetPageToken()
	if token == "" {
		if s.started && s.poll < len(s.polls)-1 {
			s.poll++
		}
		s.started = true
	}
	pages := s.polls[s.poll]
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
	// A page that never answers, or one whose delay reaches the wait's
	// deadline, is cut short by the client: answer only once it has gone.
	if (page.hang && s.clock.AdvanceToNextDeadline()) || (!page.hang && s.clock.Advance(page.delay)) {
		<-ctx.Done()
		return nil, connect.NewError(connect.CodeCanceled, ctx.Err())
	}
	resp := &orderv1.GetOrderResponse{
		Order:              &orderv1.Order{OrderId: 1, SymbolId: 1, Status: orderv1.OrderStatus_FILLED, CumQtyScaled: s.cum},
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

func (s *simTradesServer) requestArrivals() []time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.arrivals)
}

// timedOutcome is the outcome the wait reaches on schedule s, on a clock that
// moves only by the page delays and the 100 ms waits between polls: the
// deadline ends a page read that reaches it, a read that finishes at it, and
// a wait that reaches it.
func timedOutcome(s waitSchedule) (outcome string, trades []int64, complete bool) {
	elapsed := time.Duration(0)
	for p := 0; ; p++ {
		pages := s.polls[min(p, len(s.polls)-1)]
		var read []int64
		requested := map[string]bool{}
		token := ""
		for {
			idx := 0
			if token != "" {
				idx, _ = strconv.Atoi(strings.TrimPrefix(token, "t"))
			}
			page := pages[idx]
			if page.hang {
				return "timeout", nil, false
			}
			elapsed += page.delay
			if elapsed >= s.timeout {
				return "timeout", nil, false
			}
			read = append(read, page.trades...)
			if page.next == "" {
				complete = page.complete
				break
			}
			if requested[page.next] {
				return "cycle", nil, false
			}
			requested[page.next] = true
			token = page.next
		}
		var sum int64
		for _, qty := range read {
			sum += qty
		}
		if sum == s.cum {
			return "success", read, complete
		}
		elapsed += 100 * time.Millisecond
		if elapsed >= s.timeout {
			return "timeout", nil, false
		}
	}
}

// Random schedules vary the number of polls before trades add up (1-3), pages
// per poll (1-5), trades per page (0-3), per-page delays (0-60 ms), the wait
// timeout (30-300 ms), each terminal page's executions_complete, a page that
// never answers and a repeated next_page_token. Time is a simClock that moves
// only by the page delays and the wait's own waits, so each schedule has one
// outcome, whatever the machine's load:
//   - a success, with that read's trades and its terminal page's
//     executions_complete, exactly when the trades add up before the
//     deadline;
//   - a response contract error exactly when a repeated page token comes
//     before the deadline;
//   - otherwise a timeout, and no page read starts at or after the deadline.
//
// WAIT_SEEDS sets the seed count (default 64); WAIT_SEED replays one seed.
func TestWaitForOrderTradesCompleteRandomSchedules(t *testing.T) {
	seeds := []uint64{}
	if env := os.Getenv("WAIT_SEED"); env != "" {
		seed, err := strconv.ParseUint(env, 10, 64)
		if err != nil {
			t.Fatal(err)
		}
		seeds = append(seeds, seed)
	} else {
		count := uint64(64)
		if env := os.Getenv("WAIT_SEEDS"); env != "" {
			parsed, err := strconv.ParseUint(env, 10, 64)
			if err != nil {
				t.Fatal(err)
			}
			count = parsed
		}
		for seed := range count {
			seeds = append(seeds, seed)
		}
	}
	for _, seed := range seeds {
		t.Run(fmt.Sprint(seed), func(t *testing.T) {
			runWaitSchedule(t, seed)
		})
	}
}

func runWaitSchedule(t *testing.T, seed uint64) {
	clock := useSimClock(t)
	s := randomWaitSchedule(seed)
	handler := &simTradesServer{clock: clock, cum: s.cum, polls: s.polls}
	mux := http.NewServeMux()
	path, h := ordersv1connect.NewOrdersReadServiceHandler(handler)
	mux.Handle(path, h)
	srv := hardening.NewTestServer(mux)
	t.Cleanup(srv.Close)
	creds, err := auth.LoadCredentials("ak_test", auth.GenerateEd25519Keypair().SecretKeyHex.Reveal(), false)
	if err != nil {
		t.Fatal(err)
	}
	cfg := transport.DefaultConfig()
	cfg.APIURL = srv.URL
	orders := NewOrdersService(transport.NewFactory(cfg, creds, nil), catalogs.NewManager(), nil, nil, nil, nil, nil)
	// A hang guard only: no schedule waits on the wall clock.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	deadline := clock.Now().Add(s.timeout)
	result, err := orders.WaitForOrderTradesComplete(ctx, nil, models.OrderKeyByID("2"), nil, s.timeout)
	want, wantTrades, wantComplete := timedOutcome(s)
	replay := fmt.Sprintf("WAIT_SEED=%d (timeout %s, %d polls, want %s)", seed, s.timeout, len(s.polls), want)
	if ctx.Err() != nil {
		t.Fatalf("%s: the wait hung: %v", replay, err)
	}
	for i, at := range handler.requestArrivals() {
		if !at.Before(deadline) {
			t.Fatalf("%s: page read %d started %s after the deadline", replay, i, at.Sub(deadline))
		}
	}
	var contractErr *sdkerrors.ResponseContractError
	switch want {
	case "success":
		if err != nil {
			t.Fatalf("%s: %v", replay, err)
		}
		got := make([]int64, 0, len(result.Trades))
		for _, trade := range result.Trades {
			got = append(got, trade.Qty.Scaled())
		}
		if !slices.Equal(got, wantTrades) || result.ExecutionsComplete != wantComplete {
			t.Fatalf("%s: trades=%v complete=%v, want %v %v", replay, got, result.ExecutionsComplete, wantTrades, wantComplete)
		}
	case "cycle":
		if !errors.As(err, &contractErr) {
			t.Fatalf("%s: err=%v, want a response contract error", replay, err)
		}
	default:
		if err == nil || !errors.Is(err, context.DeadlineExceeded) || !strings.Contains(err.Error(), "order trades not complete within") {
			t.Fatalf("%s: err=%v, want the wait's timeout", replay, err)
		}
	}
}
