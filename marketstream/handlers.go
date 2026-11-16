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
		return c.onSubscribeResponse(m)
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

// onSubscribeResponse applies the answer to a Subscribe or Unsubscribe.
// Each topic's result is applied only if the request is still the topic's
// generation: an answer to a request a later one superseded is delivered,
// never applied. Each topic answered is then reconciled with the caller's
// intent.
func (c *Client) onSubscribeResponse(m *subscribeResponseMsg) error {
	resp := &SubscribeResponse{RequestID: m.requestID, Results: make([]TopicResult, len(m.results))}
	for i, r := range m.results {
		resp.Results[i] = TopicResult{Topic: Topic{SymbolID: r.symbolID, Stream: Stream(r.stream)}, Result: SubscribeResult(r.result)}
	}
	var applied []Topic
	var broken error
	c.mu.Lock()
	req, known := c.sent[m.requestID]
	delete(c.sent, m.requestID)
	if known {
		c.state.Lock()
		for _, r := range resp.Results {
			o := c.owners[r.Topic]
			if o == nil || o.gen != m.requestID || !r.Topic.Stream.known() {
				continue
			}
			o.gen = 0
			applied = append(applied, r.Topic)
			if err := c.apply(o, r, req.unsubscribe); err != nil && broken == nil {
				broken = err
			}
		}
		for _, t := range req.topics {
			// A topic the answer leaves out is no longer awaited: its owner
			// reconciles from what it last knew.
			if o := c.owners[t]; o != nil && o.gen == m.requestID {
				o.gen = 0
				applied = append(applied, t)
			}
		}
		c.state.Unlock()
	}
	c.mu.Unlock()
	// An answer to no request of this connection is delivered, never
	// applied.
	c.answer(m.requestID, resp)
	if broken != nil {
		return broken
	}
	if len(applied) > 0 {
		_ = c.reconcileTopics(applied...)
	}
	return nil
}

// apply records the answer to a topic's current request on its owner and
// its state. c.mu and c.state must be held.
//
// An Unsubscribe answered OK or NOT_SUBSCRIBED leaves the topic not held;
// answered otherwise for a held topic, the server's state is unknown and the
// connection ends, reconnecting every stream. A Subscribe answered OK made a
// new subscription, whose snapshot or baseline the topic waits for;
// ALREADY_SUBSCRIBED keeps the one held. A topic held without the initial
// state of its subscription, which followed the answer to a superseded
// request before the topic was held, is ended and subscribed again, as a
// depth stream after a gap. A refused Subscribe ends the topic: it is not
// resubscribed. State is kept only for topics the caller wants.
func (c *Client) apply(o *owner, r TopicResult, unsubscribe bool) error {
	t := r.Topic
	want := c.wantSet[t]
	// A Subscribe sent before this request was answered before it.
	maybeHeld := o.held || o.subscribing
	o.subscribing = false
	if unsubscribe {
		if r.Result != SubscribeOK && r.Result != SubscribeNotSubscribed && maybeHeld {
			return &UnsubscribeError{Topic: t, Result: r.Result}
		}
		o.held, o.recovering = false, false
		delete(c.active, t)
		delete(c.initial, t)
		if !want {
			c.dropTopicLocked(t)
		} else if b := c.books[t]; b != nil {
			b.synced = false
		}
		return nil
	}
	if !r.Result.Subscribed() {
		o.held, o.recovering = false, false
		c.unwant(t)
		c.dropTopicLocked(t)
		return nil
	}
	fresh := r.Result == SubscribeOK
	o.held = true
	if fresh {
		o.recovering = false
	}
	if !want {
		// Held again by a request the caller has since withdrawn: reconcile
		// ends it.
		delete(c.active, t)
		return nil
	}
	c.active[t] = true
	if fresh {
		delete(c.initial, t)
	}
	missing := !fresh && !c.initial[t] && !o.recovering
	if missing {
		o.recovering = true
	}
	switch t.Stream {
	case StreamTrades:
		if c.trackers[t.SymbolID] == nil {
			c.trackers[t.SymbolID] = &tradeTracker{symbolID: t.SymbolID}
		}
	case StreamDepth20, StreamDepth:
		b := c.books[t]
		if b == nil {
			b = &book{stream: t.Stream}
			c.books[t] = b
		}
		switch {
		case fresh:
			b.synced, b.recovering = false, false
		case missing:
			b.recovering, b.synced = t.Stream == StreamDepth, false
		}
	}
	return nil
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
// again: LIVE while the book still waits is a gap, and the depth stream alone
// recovers.
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
		c.startRecovery(topic)
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
	if err == nil {
		c.initial[Topic{SymbolID: m.symbolID, Stream: StreamTrades}] = true
	}
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
	c.initial[Topic{SymbolID: m.symbolID, Stream: StreamBBO}] = true
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
	if b == nil || !c.active[topic] || b.recovering {
		// While the stream recovers, only the new subscription's snapshot
		// counts.
		c.state.Unlock()
		return nil
	}
	changed, err := b.snapshot(m)
	if err == nil {
		c.initial[topic] = true
	}
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
// cannot apply is a gap: the book goes stale and the stream alone recovers by
// an Unsubscribe and a Subscribe, whose snapshot replaces the book. Updates
// that arrive while it recovers are dropped.
func (c *Client) onDepthUpdate(m *depthUpdateMsg) error {
	topic := Topic{SymbolID: m.symbolID, Stream: StreamDepth}
	c.state.Lock()
	b := c.books[topic]
	if b == nil || !c.active[topic] || b.recovering {
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
			c.startRecovery(topic)
			return nil
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

// UnsubscribeError reports an Unsubscribe that the server answered with
// neither OK nor NOT_SUBSCRIBED for a stream it held or may hold (a
// Subscribe of it was unanswered when the Unsubscribe was sent): whether the
// connection still holds the stream is unknown, so the client reconnects,
// and every stream is resubscribed.
type UnsubscribeError struct {
	Topic  Topic
	Result SubscribeResult
}

func (e *UnsubscribeError) Error() string {
	return "market stream unsubscribe of " + e.Topic.String() + " answered " + e.Result.String()
}
