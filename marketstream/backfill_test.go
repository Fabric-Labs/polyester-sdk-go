package marketstream

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/Fabric-Labs/polyester-sdk-go/codecs/decode"
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	marketdatav1 "github.com/Fabric-Labs/polyester-sdk-go/gen/marketdata/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

// fakeHistory serves a trade store: every trade ID it holds, the epochs that
// ended, and a watermark W below which trades are visible. Pages follow the
// after-trade-ID walk's contract.
type fakeHistory struct {
	trades []uint64          // ascending
	ends   map[uint64]uint64 // epoch -> last trade ID (or base)
	w      func(call int) uint64
	limit  int
	calls  int
	afters []uint64
	// broken rewrites a page to break the contract.
	broken func(*HistoryPage)
}

func (h *fakeHistory) TradesAfter(_ context.Context, symbolID uint32, after uint64, limit int) (HistoryPage, error) {
	if symbolID != 7 {
		return HistoryPage{}, errors.New("wrong symbol")
	}
	if h.limit > 0 {
		limit = h.limit
	}
	h.afters = append(h.afters, after)
	w := h.w(h.calls)
	h.calls++
	page := HistoryPage{CompleteThroughTradeID: w}
	for _, id := range h.trades {
		if id > after && id <= w && len(page.Trades) < limit {
			page.Trades = append(page.Trades, Trade{TradeID: id})
		}
	}
	switch {
	case len(page.Trades) == limit:
		page.NextAfterTradeID = page.Trades[len(page.Trades)-1].TradeID
	default:
		page.NextAfterTradeID = max(after, w)
	}
	for e, last := range h.ends {
		if last >= after && last <= page.NextAfterTradeID {
			page.EpochEnds = append(page.EpochEnds, EpochEnd{Epoch: e, LastTradeID: last})
		}
	}
	if h.broken != nil {
		h.broken(&page)
	}
	return page, nil
}

func span(from, to uint64) []uint64 {
	var out []uint64
	for id := from; id <= to; id++ {
		out = append(out, id)
	}
	return out
}

func tradeIDs(trades []Trade) []uint64 {
	var out []uint64
	for _, t := range trades {
		out = append(out, t.TradeID)
	}
	return out
}

func fastRetries(t *testing.T) {
	first, maxDelay := backfillRetryFirst, backfillRetryMax
	backfillRetryFirst, backfillRetryMax = time.Millisecond, 4*time.Millisecond
	t.Cleanup(func() { backfillRetryFirst, backfillRetryMax = first, maxDelay })
}

func constW(w uint64) func(int) uint64 { return func(int) uint64 { return w } }

func TestBackfillTerminationCases(t *testing.T) {
	fastRetries(t)
	cases := []struct {
		name  string
		h     *fakeHistory
		gap   TradesGap
		want  []uint64
		pages int
	}{
		{
			name: "same epoch", h: &fakeHistory{trades: span(1, 110), w: constW(110)},
			gap: TradesGap{SymbolID: 7, AfterTradeID: 100, ThroughTradeID: 110}, want: span(101, 110), pages: 1,
		},
		{
			name: "reconnect into an empty epoch",
			h:    &fakeHistory{trades: span(1, 105), ends: map[uint64]uint64{1: 105}, w: constW(e2)},
			gap:  TradesGap{SymbolID: 7, AfterTradeID: 100, ThroughTradeID: e2}, want: span(101, 105), pages: 1,
		},
		{
			name: "a full page ends on the epoch's last trade",
			h:    &fakeHistory{trades: span(1, 105), ends: map[uint64]uint64{1: 105}, w: constW(e2), limit: 5},
			gap:  TradesGap{SymbolID: 7, AfterTradeID: 100, ThroughTradeID: e2}, want: span(101, 105), pages: 2,
		},
		{
			name: "baseline at an epoch base behind an empty epoch",
			h:    &fakeHistory{trades: span(1, 105), ends: map[uint64]uint64{1: 105, 2: e2}, w: constW(e3)},
			gap:  TradesGap{SymbolID: 7, AfterTradeID: 100, ThroughTradeID: e3}, want: span(101, 105), pages: 1,
		},
		{
			name: "across epochs with trades",
			h:    &fakeHistory{trades: append(append(span(1, 105), span(e2+1, e2+4)...), span(e3+1, e3+3)...), ends: map[uint64]uint64{1: 105, 2: e2 + 4}, w: constW(e3 + 3)},
			gap:  TradesGap{SymbolID: 7, AfterTradeID: 100, ThroughTradeID: e3 + 3},
			want: append(append(span(101, 105), span(e2+1, e2+4)...), span(e3+1, e3+3)...), pages: 1,
		},
		{
			name: "empty old tail",
			h:    &fakeHistory{trades: span(1, 105), ends: map[uint64]uint64{1: 105}, w: constW(e2)},
			gap:  TradesGap{SymbolID: 7, AfterTradeID: 105, TailEpoch: 1}, want: nil, pages: 1,
		},
		{
			name: "a tail with trades",
			h:    &fakeHistory{trades: append(span(1, 105), e2+1), ends: map[uint64]uint64{1: 105}, w: constW(e2 + 1)},
			gap:  TradesGap{SymbolID: 7, AfterTradeID: 100, TailEpoch: 1}, want: span(101, 105), pages: 1,
		},
		{
			name: "the store behind the client",
			h: &fakeHistory{trades: span(1, 105), w: func(call int) uint64 {
				if call < 2 {
					return 90
				}
				return 105
			}},
			gap: TradesGap{SymbolID: 7, AfterTradeID: 100, ThroughTradeID: 105}, want: span(101, 105), pages: 3,
		},
		{
			name: "the store behind by an epoch",
			h: &fakeHistory{trades: append(span(1, 100), span(e2+1, e2+5)...), ends: map[uint64]uint64{1: 100}, w: func(call int) uint64 {
				if call < 1 {
					return 90
				}
				return e2 + 5
			}},
			gap: TradesGap{SymbolID: 7, AfterTradeID: e2 + 3, ThroughTradeID: e2 + 5}, want: span(e2+4, e2+5), pages: 2,
		},
		{
			name: "a gap of more than two pages",
			h:    &fakeHistory{trades: span(1, 3000), w: constW(3000)},
			gap:  TradesGap{SymbolID: 7, AfterTradeID: 400, ThroughTradeID: 2900}, want: span(401, 2900), pages: 3,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Backfill(context.Background(), tc.h, tc.gap)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(tradeIDs(got), tc.want) {
				t.Fatalf("trades %v, want %v", tradeIDs(got), tc.want)
			}
			if tc.h.calls != tc.pages {
				t.Fatalf("%d pages (cursors %v), want %d", tc.h.calls, tc.h.afters, tc.pages)
			}
			for i := 1; i < len(tc.h.afters); i++ {
				if tc.h.afters[i] < tc.h.afters[i-1] {
					t.Fatalf("cursor moved backwards: %v", tc.h.afters)
				}
			}
		})
	}
}

func TestBackfillRefusesABrokenContract(t *testing.T) {
	fastRetries(t)
	cases := map[string]func(*HistoryPage){
		"a trade that skips the next ID": func(p *HistoryPage) { p.Trades = []Trade{{TradeID: 102}} },
		"a cursor below the request":     func(p *HistoryPage) { p.Trades, p.NextAfterTradeID = nil, 99 },
		"a short page skipping trades":   func(p *HistoryPage) { p.Trades, p.NextAfterTradeID = p.Trades[:1], 105 },
		"an epoch jump without its end":  func(p *HistoryPage) { p.Trades, p.NextAfterTradeID, p.EpochEnds = nil, e2, nil },
	}
	for name, broken := range cases {
		t.Run(name, func(t *testing.T) {
			h := &fakeHistory{trades: span(1, 105), w: constW(105), broken: broken}
			_, err := Backfill(context.Background(), h, TradesGap{SymbolID: 7, AfterTradeID: 100, ThroughTradeID: 105})
			var contract *sdkerrors.ResponseContractError
			if !errors.As(err, &contract) {
				t.Fatalf("err %v, want ResponseContractError", err)
			}
		})
	}
}

func TestBackfillStopsWithItsContext(t *testing.T) {
	fastRetries(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	h := &fakeHistory{trades: span(1, 105), w: constW(90)} // never catches up
	if _, err := Backfill(ctx, h, TradesGap{SymbolID: 7, AfterTradeID: 100, ThroughTradeID: 105}); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err %v, want the context's", err)
	}
}

type failingHistory struct{ calls int }

func (f *failingHistory) TradesAfter(context.Context, uint32, uint64, int) (HistoryPage, error) {
	f.calls++
	return HistoryPage{}, errors.New("unavailable")
}

func TestBackfillGivesUpAfterRepeatedFailures(t *testing.T) {
	fastRetries(t)
	h := &failingHistory{}
	if _, err := Backfill(context.Background(), h, TradesGap{SymbolID: 7, AfterTradeID: 1, ThroughTradeID: 2}); err == nil || h.calls != backfillFetchAttempts {
		t.Fatalf("err %v after %d calls", err, h.calls)
	}
}

// A recovered trade keeps its exchange time: the trade history's ts_ns
// becomes the trade's TransactTime, as on a live trade.
func TestBackfilledTradesKeepTheirExchangeTime(t *testing.T) {
	wire := func(matchID, tsNs uint64) *marketdatav1.MarketTrade {
		return &marketdatav1.MarketTrade{SymbolId: 7, MatchId: matchID, IsBuy: true, PriceTicks: 65_000_000_000_000, QtyScaled: 1_500, TsNs: tsNs}
	}
	res := decode.MarketTradesBackfillFromProto(&marketdatav1.GetTradesResponse{
		Trades:                 []*marketdatav1.MarketTrade{wire(101, 1_700_000_000_123_456_789), wire(102, 0), wire(103, 1<<63)},
		CompleteThroughTradeId: 103,
		NextAfterTradeId:       103,
	}, 3)
	page, err := historyPage(res)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Trades) != 3 {
		t.Fatalf("trades %+v", page.Trades)
	}
	if got, want := page.Trades[0].TransactTime, time.Unix(0, 1_700_000_000_123_456_789).UTC(); !got.Equal(want) || got.Location() != time.UTC {
		t.Fatalf("trade 101 time %v, want %v", got, want)
	}
	if got := page.Trades[1].TransactTime; !got.IsZero() {
		t.Fatalf("trade 102 time %v: a time the history does not report stays zero", got)
	}
	// Unix nanoseconds are unsigned: a time past the int64 range is kept.
	if got, want := page.Trades[2].TransactTime, time.Unix(9_223_372_036, 854_775_808).UTC(); !got.Equal(want) {
		t.Fatalf("trade 103 time %v, want %v", got, want)
	}
}

func TestABackfilledTradeTimeThatIsNotANumberBreaksTheContract(t *testing.T) {
	res := models.MarketTradesResult{
		Trades:                 []models.MarketTrade{{SymbolID: 7, MatchID: "101", TsNs: "soon", Side: "buy"}},
		CompleteThroughTradeID: "101",
		NextAfterTradeID:       "101",
	}
	var contract *sdkerrors.ResponseContractError
	if _, err := historyPage(res); !errors.As(err, &contract) {
		t.Fatalf("err %v, want a ResponseContractError", err)
	}
}
