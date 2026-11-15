package marketstream

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	"github.com/Fabric-Labs/polyester-sdk-go/services"
)

// TradeHistory reads a pair's public trades gap-free by trade ID, as the
// trade history API's after-trade-ID walk does.
type TradeHistory interface {
	// TradesAfter returns the pair's trades with an ID above afterTradeID,
	// ascending, up to limit.
	TradesAfter(ctx context.Context, symbolID uint32, afterTradeID uint64, limit int) (HistoryPage, error)
}

// HistoryPage is one page of a gap-free trade walk.
type HistoryPage struct {
	Trades []Trade
	// CompleteThroughTradeID: every trade with an ID at or below it exists
	// and is returned by the walk.
	CompleteThroughTradeID uint64
	// NextAfterTradeID is the cursor of the next call; equal to the
	// request's, nothing newer is available yet.
	NextAfterTradeID uint64
	// EpochEnds lists every book epoch that ended between the request's
	// cursor and NextAfterTradeID, both inclusive.
	EpochEnds []EpochEnd
}

// EpochEnd is a book epoch's last trade ID, or the epoch's base when it
// ended without trades.
type EpochEnd struct {
	Epoch       uint64
	LastTradeID uint64
}

// HistoryFromMarketData reads trades with MarketDataService.GetTradesAfter.
func HistoryFromMarketData(svc *services.MarketDataService) TradeHistory {
	return marketDataHistory{svc: svc}
}

type marketDataHistory struct{ svc *services.MarketDataService }

func (h marketDataHistory) TradesAfter(ctx context.Context, symbolID uint32, after uint64, limit int) (HistoryPage, error) {
	res, err := h.svc.GetTradesAfter(ctx, nil, &symbolID, strconv.FormatUint(after, 10), limit)
	if err != nil {
		return HistoryPage{}, err
	}
	page := HistoryPage{}
	if page.CompleteThroughTradeID, err = codecs.ParseRequiredUint64Decimal(res.CompleteThroughTradeID, "complete_through_trade_id"); err != nil {
		return HistoryPage{}, err
	}
	if page.NextAfterTradeID, err = codecs.ParseRequiredUint64Decimal(res.NextAfterTradeID, "next_after_trade_id"); err != nil {
		return HistoryPage{}, err
	}
	for _, end := range res.EpochEnds {
		last, err := codecs.ParseRequiredUint64Decimal(end.LastTradeID, "last_trade_id")
		if err != nil {
			return HistoryPage{}, err
		}
		page.EpochEnds = append(page.EpochEnds, EpochEnd{Epoch: end.Epoch, LastTradeID: last})
	}
	for _, t := range res.Trades {
		id, err := codecs.ParseRequiredUint64Decimal(t.MatchID, "match_id")
		if err != nil {
			return HistoryPage{}, err
		}
		side := SideSell
		if t.Side == "buy" {
			side = SideBuy
		}
		page.Trades = append(page.Trades, Trade{TradeID: id, Price: t.Price, Qty: t.Qty, TakerSide: side})
	}
	return page, nil
}

// backfillPageLimit is the page size of a backfill walk.
const backfillPageLimit = 1000

// Backfill timing: a walk retries a page without progress after a delay
// doubling from the first to the last, for as long as it takes; the history
// lagging the live stream is normal. A history call that fails is retried the
// same way, up to backfillFetchAttempts in a row.
var (
	backfillRetryFirst = 100 * time.Millisecond
	backfillRetryMax   = 2 * time.Second
)

const backfillFetchAttempts = 10

// Backfill recovers the trades of a gap from history: it walks the pair's
// trades by ID from gap.AfterTradeID until it holds every trade through
// gap.ThroughTradeID, or, for an epoch tail, through the epoch's last trade.
// It returns the gap's trades in ascending ID order.
//
// A page without progress is retried with backoff until ctx ends: the
// history lags the live stream. A page that breaks the walk's contract (a
// trade that does not continue the cursor, or a cursor that moves backwards
// or skips trades) is a *errors.ResponseContractError.
func Backfill(ctx context.Context, history TradeHistory, gap TradesGap) ([]Trade, error) {
	if gap.ThroughTradeID == 0 && gap.TailEpoch == 0 {
		return nil, &sdkerrors.ValidationError{Msg: "trades gap has neither a last trade ID nor a tail epoch"}
	}
	w := walk{cursor: gap.AfterTradeID, ends: map[uint64]uint64{}}
	delay := backfillRetryFirst
	failures := 0
	for !w.done(gap) {
		page, err := history.TradesAfter(ctx, gap.SymbolID, w.cursor, backfillPageLimit)
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			failures++
			if failures >= backfillFetchAttempts {
				return nil, err
			}
		} else {
			failures = 0
			before := w.cursor
			if err := w.take(page, gap); err != nil {
				return nil, &sdkerrors.ResponseContractError{Msg: "trade history backfill: " + err.Error()}
			}
			if w.cursor != before {
				delay = backfillRetryFirst
				continue
			}
		}
		if w.done(gap) {
			break
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(delay):
		}
		delay = min(delay*2, backfillRetryMax)
	}
	return w.trades, nil
}

// walk is the state of one backfill: the cursor (every trade up to it is
// held), the epoch ends seen and the gap's trades collected.
type walk struct {
	cursor uint64
	ends   map[uint64]uint64
	trades []Trade
}

func (w *walk) done(gap TradesGap) bool {
	if gap.TailEpoch != 0 {
		if last, ok := w.ends[gap.TailEpoch]; ok && last <= w.cursor {
			return true
		}
		return epochOf(w.cursor) > gap.TailEpoch
	}
	return w.cursor >= gap.ThroughTradeID
}

// closedThrough reports whether the epoch ends seen close the cursor's epoch
// at the cursor and every epoch before target's empty.
func (w *walk) closedThrough(cursor, targetEpoch uint64) bool {
	from := epochOf(cursor)
	if last, ok := w.ends[from]; !ok || last != cursor {
		return false
	}
	for e := from + 1; e < targetEpoch; e++ {
		base, ok := epochBase(e)
		if last, seen := w.ends[e]; !ok || !seen || last != base {
			return false
		}
	}
	return true
}

// take validates one page against the cursor and advances it.
func (w *walk) take(page HistoryPage, gap TradesGap) error {
	request := w.cursor
	for _, end := range page.EpochEnds {
		w.ends[end.Epoch] = end.LastTradeID
	}
	for _, t := range page.Trades {
		switch {
		case t.TradeID == w.cursor+1 && epochOf(t.TradeID) == epochOf(w.cursor):
		case epochOf(t.TradeID) > epochOf(w.cursor) && t.TradeID == mustEpochBase(epochOf(t.TradeID))+1 && w.closedThrough(w.cursor, epochOf(t.TradeID)):
		default:
			return fmt.Errorf("trade %d does not continue cursor %d", t.TradeID, w.cursor)
		}
		w.cursor = t.TradeID
		if gap.TailEpoch != 0 && epochOf(t.TradeID) == gap.TailEpoch || gap.TailEpoch == 0 && t.TradeID <= gap.ThroughTradeID {
			w.trades = append(w.trades, t)
		}
	}
	next := page.NextAfterTradeID
	switch {
	case next < request:
		return fmt.Errorf("next cursor %d below the request's %d", next, request)
	case next == w.cursor:
	case next < w.cursor:
		return fmt.Errorf("next cursor %d below the last trade %d", next, w.cursor)
	case epochOf(next) == epochOf(w.cursor):
		return fmt.Errorf("next cursor %d skips trades after %d", next, w.cursor)
	case next != mustEpochBase(epochOf(next)) || !w.closedThrough(w.cursor, epochOf(next)):
		return fmt.Errorf("next cursor %d crosses epochs not closed at %d", next, w.cursor)
	default:
		w.cursor = next
	}
	return nil
}

func mustEpochBase(epoch uint64) uint64 {
	base, _ := epochBase(epoch)
	return base
}
