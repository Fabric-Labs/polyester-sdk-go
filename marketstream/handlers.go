package marketstream

import (
	"errors"
	"math"
	"time"

	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	ms "github.com/Fabric-Labs/polyester-sdk-go/internal/sbe/marketstream"
)

// handle applies one server message. An error ends the connection; the client
// reconnects unless it is a SchemaError.
func (c *Client) handle(frame []byte) error {
	decoded, err := decodeFrame(frame)
	if err != nil {
		return err
	}
	c.healthy.Store(true)
	switch m := decoded.(type) {
	case *subscribeResponseMsg:
		c.onSubscribeResponse(m)
	case *pongMsg:
		c.onPong(m)
	case *marketStatusMsg:
		return c.onMarketStatus(m)
	case *tradesThroughMsg:
		return c.onTradesThrough(m)
	case *tradesSkippedMsg:
		return c.onTradesSkipped(m)
	case *tradesMsg:
		return c.onTrades(m)
	case *bboMsg:
		return c.onBBO(m)
	case *depthSnapshotMsg:
		return c.onDepthSnapshot(m)
	case *depthUpdateMsg:
		return c.onDepthUpdate(m)
	}
	// nil: a template this client does not know is skipped.
	return nil
}

func (c *Client) onSubscribeResponse(m *subscribeResponseMsg) {
	resp := &SubscribeResponse{RequestID: m.requestID, Results: make([]TopicResult, len(m.results))}
	c.mu.Lock()
	req, known := c.sent[m.requestID]
	delete(c.sent, m.requestID)
	c.mu.Unlock()
	if !known {
		// No request of this connection: delivered, never applied.
		for i, r := range m.results {
			resp.Results[i] = TopicResult{Topic: Topic{SymbolID: r.symbolID, Stream: Stream(r.stream)}, Result: SubscribeResult(r.result)}
		}
		c.emit(resp)
		return
	}
	if req.unsubscribe {
		c.onUnsubscribeResponse(m, resp)
		return
	}
	c.state.Lock()
	for i, r := range m.results {
		topic := Topic{SymbolID: r.symbolID, Stream: Stream(r.stream)}
		result := SubscribeResult(r.result)
		resp.Results[i] = TopicResult{Topic: topic, Result: result}
		if !result.Subscribed() || !topic.Stream.known() {
			continue
		}
		c.active[topic] = true
		switch topic.Stream {
		case StreamTrades:
			if c.trackers[topic.SymbolID] == nil {
				c.trackers[topic.SymbolID] = &tradeTracker{symbolID: topic.SymbolID}
			}
		case StreamDepth20, StreamDepth:
			if c.books[topic] == nil {
				c.books[topic] = &book{stream: topic.Stream}
			}
		}
	}
	c.state.Unlock()

	c.mu.Lock()
	for _, r := range resp.Results {
		if !r.Result.Subscribed() {
			// Refused: not resubscribed on reconnect.
			c.unwant(r.Topic)
		}
	}
	c.mu.Unlock()
	c.answer(m.requestID, resp)
}

// onUnsubscribeResponse ends the streams an Unsubscribe names: whether they
// ended now or were not held, the client drops their state.
func (c *Client) onUnsubscribeResponse(m *subscribeResponseMsg, resp *SubscribeResponse) {
	var ended []Topic
	for i, r := range m.results {
		topic := Topic{SymbolID: r.symbolID, Stream: Stream(r.stream)}
		result := SubscribeResult(r.result)
		resp.Results[i] = TopicResult{Topic: topic, Result: result}
		if result == SubscribeOK || result == SubscribeNotSubscribed {
			ended = append(ended, topic)
		}
	}
	c.dropTopics(ended)
	c.answer(m.requestID, resp)
}

// answer delivers a response as an event and to the request waiting for it.
func (c *Client) answer(requestID uint64, resp *SubscribeResponse) {
	c.mu.Lock()
	waiter := c.waiting[requestID]
	delete(c.waiting, requestID)
	c.mu.Unlock()
	c.emit(resp)
	if waiter != nil {
		waiter <- subscribeOutcome{resp: resp}
	}
}

// unwant stops resubscribing a topic on reconnect. c.mu must be held.
func (c *Client) unwant(topic Topic) {
	if !c.wantSet[topic] {
		return
	}
	delete(c.wantSet, topic)
	for i, t := range c.wanted {
		if t == topic {
			c.wanted = append(c.wanted[:i], c.wanted[i+1:]...)
			return
		}
	}
}

// onMarketStatus records a pair's feed state. A stale pair's depth book waits
// for a replacement snapshot, which the server sends before the pair is live
// again: LIVE while the book still waits is a gap, recovered by reconnecting.
func (c *Client) onMarketStatus(m *marketStatusMsg) error {
	feed := FeedState(m.feed)
	topic := Topic{SymbolID: m.symbolID, Stream: StreamDepth}
	c.state.Lock()
	c.feeds[m.symbolID] = feed
	b := c.books[topic]
	held := b != nil && c.active[topic]
	waiting := held && !b.synced
	if held && feed == FeedStale {
		b.synced = false
	}
	c.state.Unlock()
	if feed == FeedLive && waiting {
		return &BookGapError{SymbolID: m.symbolID, Msg: "feed live again before the depth book's replacement snapshot"}
	}
	c.emit(&MarketStatus{SymbolID: m.symbolID, Feed: feed, SendTime: timeOf(m.sendNs)})
	return nil
}

func (c *Client) onPong(m *pongMsg) {
	c.mu.Lock()
	owed, sent := c.ping.id, c.ping.sent
	if m.requestID == owed {
		c.ping.id = 0
	}
	c.mu.Unlock()
	if m.requestID != owed || owed == 0 {
		return
	}
	c.emit(&Pong{RequestID: m.requestID, RoundTrip: time.Since(sent), SendTime: timeOf(m.sendNs)})
}

// trackerFor is the trades accounting of a pair whose trades stream the
// connection holds; nil otherwise. c.state must be held.
func (c *Client) trackerFor(symbolID uint32) *tradeTracker {
	if !c.active[Topic{SymbolID: symbolID, Stream: StreamTrades}] {
		return nil
	}
	return c.trackers[symbolID]
}

func (c *Client) onTradesThrough(m *tradesThroughMsg) error {
	c.state.Lock()
	t := c.trackerFor(m.symbolID)
	if t == nil {
		c.state.Unlock()
		return nil
	}
	gaps, err := t.through(m)
	c.state.Unlock()
	if err != nil {
		return &FrameError{TemplateID: ms.TradesThroughSbeTemplateID, Msg: err.Error()}
	}
	c.emitGaps(gaps)
	c.emit(&TradesThrough{SymbolID: m.symbolID, BookEpoch: m.epoch, LastTradeID: m.last, SendTime: timeOf(m.sendNs)})
	return nil
}

func (c *Client) onTradesSkipped(m *tradesSkippedMsg) error {
	c.state.Lock()
	t := c.trackerFor(m.symbolID)
	if t == nil {
		c.state.Unlock()
		return nil
	}
	gaps, err := t.skipped(m)
	c.state.Unlock()
	if err != nil {
		return &FrameError{TemplateID: ms.TradesSkippedSbeTemplateID, Msg: err.Error()}
	}
	c.emitGaps(gaps)
	return nil
}

func (c *Client) onTrades(m *tradesMsg) error {
	c.state.Lock()
	t := c.trackerFor(m.symbolID)
	if t == nil {
		c.state.Unlock()
		return nil
	}
	for _, f := range m.fills {
		if f.price <= 0 || f.qty <= 0 {
			c.state.Unlock()
			return &FrameError{TemplateID: ms.TradesSbeTemplateID, Msg: "trade with a price or quantity that is not positive"}
		}
	}
	fills, gaps, err := t.trades(m)
	c.state.Unlock()
	if err != nil {
		return &FrameError{TemplateID: ms.TradesSbeTemplateID, Msg: err.Error()}
	}
	c.emitGaps(gaps)
	if len(fills) == 0 {
		return nil
	}
	scale, _ := c.cfg.Pairs.BaseQuantityScaleForSymbolID(m.symbolID)
	symbol := c.symbol(m.symbolID)
	ev := &Trades{SymbolID: m.symbolID, Symbol: symbol, BookEpoch: m.epoch, UpdateID: m.updateID, TransactTime: timeOf(m.transactNs), SendTime: timeOf(m.sendNs), Trades: make([]Trade, len(fills))}
	for i, f := range fills {
		ev.Trades[i] = Trade{
			TradeID:   f.tradeID,
			Price:     codecs.DecodePriceTicks(f.price, symbol),
			Qty:       codecs.DecodeQtyScaled(f.qty, scale, symbol, &m.symbolID),
			TakerSide: Side(f.side),
		}
	}
	c.emit(ev)
	return nil
}

// emitGaps delivers gaps and, with Config.History, starts their recovery.
func (c *Client) emitGaps(gaps []*TradesGap) {
	for _, g := range gaps {
		if !c.emit(g) {
			return
		}
		if c.cfg.History == nil {
			continue
		}
		gap := *g
		c.mu.Lock()
		stopped := c.closed || c.err != nil
		if !stopped {
			c.backfills.Add(1)
		}
		c.mu.Unlock()
		if stopped {
			return
		}
		go func() {
			defer c.backfills.Done()
			trades, err := Backfill(c.ctx, c.cfg.History, gap)
			if errors.Is(err, c.ctx.Err()) && c.ctx.Err() != nil {
				return
			}
			c.emit(&TradesBackfill{Gap: gap, Trades: trades, Err: err})
		}()
	}
}

func (c *Client) onBBO(m *bboMsg) error {
	bid, err := c.quoteSide(m.symbolID, m.bidPrice, m.bidQty)
	if err != nil {
		return err
	}
	ask, err := c.quoteSide(m.symbolID, m.askPrice, m.askQty)
	if err != nil {
		return err
	}
	c.state.Lock()
	if !c.active[Topic{SymbolID: m.symbolID, Stream: StreamBBO}] {
		c.state.Unlock()
		return nil
	}
	prev := c.quotes[m.symbolID]
	if prev != nil && (m.epoch < prev.BookEpoch || m.epoch == prev.BookEpoch && m.updateID < prev.UpdateID ||
		!prev.Stale && m.epoch == prev.BookEpoch && m.updateID == prev.UpdateID) {
		// An older state, or a re-send of the current one. A quote kept,
		// stale, from a lost connection stays until one at least as new
		// confirms or replaces it.
		c.state.Unlock()
		return nil
	}
	q := &BestBidOffer{
		SymbolID: m.symbolID, Symbol: c.symbol(m.symbolID), BookEpoch: m.epoch, UpdateID: m.updateID,
		TransactTime: timeOf(m.transactNs), SendTime: timeOf(m.sendNs), Bid: bid, Ask: ask,
	}
	c.quotes[m.symbolID] = q
	c.state.Unlock()
	ev := *q
	c.emit(&ev)
	return nil
}

// quoteSide decodes one side of a best bid and offer: none when its price is
// null, which requires quantity 0.
func (c *Client) quoteSide(symbolID uint32, price, qty int64) (*Level, error) {
	if price == math.MinInt64 {
		if qty != 0 {
			return nil, &FrameError{TemplateID: ms.BestBidOfferSbeTemplateID, Msg: "side without a price has a quantity"}
		}
		return nil, nil
	}
	if price <= 0 || qty <= 0 {
		return nil, &FrameError{TemplateID: ms.BestBidOfferSbeTemplateID, Msg: "side with a price or quantity that is not positive"}
	}
	scale, _ := c.cfg.Pairs.BaseQuantityScaleForSymbolID(symbolID)
	symbol := c.symbol(symbolID)
	return &Level{Price: codecs.DecodePriceTicks(price, symbol), Qty: codecs.DecodeQtyScaled(qty, scale, symbol, &symbolID)}, nil
}

func (c *Client) onDepthSnapshot(m *depthSnapshotMsg) error {
	topic := Topic{SymbolID: m.symbolID, Stream: Stream(m.stream)}
	c.state.Lock()
	b := c.books[topic]
	if b == nil || !c.active[topic] {
		c.state.Unlock()
		return nil
	}
	changed, err := b.snapshot(m)
	c.state.Unlock()
	if err != nil {
		return &FrameError{TemplateID: ms.DepthSnapshotSbeTemplateID, Msg: err.Error()}
	}
	if !changed {
		return nil
	}
	scale, _ := c.cfg.Pairs.BaseQuantityScaleForSymbolID(m.symbolID)
	symbol := c.symbol(m.symbolID)
	c.emit(&DepthSnapshot{
		SymbolID: m.symbolID, Symbol: symbol, Stream: topic.Stream, BookEpoch: m.epoch, UpdateID: m.updateID,
		LevelsPerSide: int(m.levelsPerSide), TransactTime: timeOf(m.transactNs), SendTime: timeOf(m.sendNs),
		Bids: levelsOf(m.bids, scale, symbol, m.symbolID), Asks: levelsOf(m.asks, scale, symbol, m.symbolID),
	})
	return nil
}

// onDepthUpdate applies an update at exactly its base. An update the book
// cannot apply ends the connection: the protocol has no unsubscribe, so the
// book is rebuilt from the snapshot of a new connection's subscription.
func (c *Client) onDepthUpdate(m *depthUpdateMsg) error {
	topic := Topic{SymbolID: m.symbolID, Stream: StreamDepth}
	c.state.Lock()
	b := c.books[topic]
	if b == nil || !c.active[topic] {
		c.state.Unlock()
		return nil
	}
	err := b.update(m)
	if err != nil {
		b.synced = false
	}
	c.state.Unlock()
	if err != nil {
		var gap *errBookGap
		if errors.As(err, &gap) {
			return &BookGapError{SymbolID: m.symbolID, Msg: gap.msg}
		}
		return &FrameError{TemplateID: ms.DepthUpdateSbeTemplateID, Msg: err.Error()}
	}
	scale, _ := c.cfg.Pairs.BaseQuantityScaleForSymbolID(m.symbolID)
	symbol := c.symbol(m.symbolID)
	c.emit(&DepthUpdate{
		SymbolID: m.symbolID, Symbol: symbol, BookEpoch: m.epoch, FirstUpdateID: m.first, LastUpdateID: m.last,
		TransactTime: timeOf(m.transactNs), SendTime: timeOf(m.sendNs),
		Bids: levelsOf(m.bids, scale, symbol, m.symbolID), Asks: levelsOf(m.asks, scale, symbol, m.symbolID),
	})
	return nil
}

// BookGapError reports a depth update the client's book could not apply
// because the book is not at its base. The client reconnects, and the new
// subscription's snapshot replaces the book.
type BookGapError struct {
	SymbolID uint32
	Msg      string
}

func (e *BookGapError) Error() string { return "market stream depth book gap: " + e.Msg }
