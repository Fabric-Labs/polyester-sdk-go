// Command marketstreamfixtures derives the market-data stream's test
// fixtures from captured frames. It reads a directory of captures, each a
// directory with a frames.tsv (columns: file, direction, message) naming one
// WebSocket binary frame per file. It selects one frame, or one sequence, per
// role, rewrites every identifier, price, quantity and time to synthetic
// values, re-encodes the frames with the generated codecs, and writes them
// with an index to the output directory. Captures stay where they are; only
// the derived frames are written.
//
//	go run ./internal/tools/marketstreamfixtures -captures DIR -out marketstream/testdata/fixtures
//
// The rewrite keeps what the tests rely on: the order and contiguity of
// update IDs and trade IDs, the epoch each trade ID encodes, the order of
// prices (bids best first, asks best first), which quantities are zero, the
// order of times, and the pairing of requests with their responses.
package main

import (
	"errors"
	"flag"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"

	ms "github.com/Fabric-Labs/polyester-sdk-go/internal/sbe/marketstream"
)

func main() {
	captures := flag.String("captures", "", "directory of captures, each with a frames.tsv")
	out := flag.String("out", "marketstream/testdata/fixtures", "output directory")
	flag.Parse()
	if *captures == "" {
		fmt.Fprintln(os.Stderr, "usage: marketstreamfixtures -captures DIR [-out DIR]")
		os.Exit(2)
	}
	if err := run(*captures, *out); err != nil {
		fmt.Fprintln(os.Stderr, "marketstreamfixtures:", err)
		os.Exit(1)
	}
}

// Synthetic bases the rewrite maps to.
const (
	symbolBase  = 101
	updateBase  = 1000
	tradeOffset = 500
	priceBase   = 100_000_000_000 // 100 quote units at the fixed 1e9 price scale
	timeBase    = 1_700_000_000_000_000_000
	epochBits   = 40
	nullPrice   = math.MinInt64
)

type level struct{ price, qty int64 }

type fill struct {
	id         uint64
	price, qty int64
	side       uint8
}

type item struct {
	symbol         uint32
	stream, result uint8
}

// msg is one decoded message, in wire units.
type msg struct {
	template         uint16
	requestID        uint64
	items            []item
	symbol           uint32
	stream, feed     uint8
	epoch, update    uint64
	first, last      uint64 // DepthUpdate range; TradesSkipped range (last 0: the rest of the epoch)
	lastTrade        uint64 // TradesThrough
	transact, send   uint64
	levelsPerSide    uint16
	bids, asks       []level
	fills            []fill
	bidPrice, bidQty int64
	askPrice, askQty int64
}

type frame struct {
	capture, file, direction, message string
	m                                 msg
}

func run(capturesDir, outDir string) error {
	dirs, err := filepath.Glob(filepath.Join(capturesDir, "*", "frames.tsv"))
	if err != nil {
		return err
	}
	sort.Strings(dirs)
	if len(dirs) == 0 {
		return fmt.Errorf("no frames.tsv under %s", capturesDir)
	}
	var all [][]frame
	for _, index := range dirs {
		frames, err := load(filepath.Dir(index))
		if err != nil {
			return err
		}
		all = append(all, frames)
	}
	d := newDeriver(all)
	return d.write(outDir)
}

func load(dir string) ([]frame, error) {
	raw, err := os.ReadFile(filepath.Join(dir, "frames.tsv"))
	if err != nil {
		return nil, err
	}
	var frames []frame
	for i, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		cols := strings.Split(line, "\t")
		if i == 0 || len(cols) < 3 {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, cols[0]))
		if err != nil {
			return nil, err
		}
		m, err := decode(data)
		if err != nil {
			return nil, fmt.Errorf("frame %d of a capture: %w", i, err)
		}
		frames = append(frames, frame{capture: filepath.Base(dir), file: cols[0], direction: cols[1], message: cols[2], m: m})
	}
	return frames, nil
}

// rewrite maps one capture's values to synthetic ones.
type rewrite struct {
	symbols   map[uint32]uint32 // shared by every capture
	requests  map[uint64]uint64
	minUpdate uint64
	minPrice  int64
	minTime   uint64
}

func (r *rewrite) symbol(s uint32) uint32 {
	if v, ok := r.symbols[s]; ok {
		return v
	}
	v := uint32(symbolBase + len(r.symbols))
	r.symbols[s] = v
	return v
}

func (r *rewrite) request(id uint64) uint64 {
	if v, ok := r.requests[id]; ok {
		return v
	}
	v := uint64(len(r.requests) + 1)
	r.requests[id] = v
	return v
}

func (r *rewrite) update(u uint64) uint64 { return u - r.minUpdate + updateBase }

// trade shifts a trade ID within its epoch; 0 (null) stays 0.
func (r *rewrite) trade(id uint64) uint64 {
	if id == 0 {
		return 0
	}
	return id + tradeOffset
}

func (r *rewrite) price(p int64) int64 {
	if p == nullPrice {
		return p
	}
	return p - r.minPrice + priceBase
}

// qty keeps zero and maps any other quantity to a small positive one.
func qty(q int64) int64 {
	if q == 0 {
		return 0
	}
	return 1000 * (1 + q%97)
}

func (r *rewrite) time(t uint64) uint64 {
	if t == 0 {
		return 0
	}
	return t - r.minTime + timeBase
}

func (r *rewrite) apply(m msg) msg {
	m.requestID = r.request(m.requestID)
	if m.template == ms.PingSbeTemplateID || m.template == ms.SubscribeSbeTemplateID || m.template == ms.SubscribeResponseSbeTemplateID || m.template == ms.PongSbeTemplateID {
		items := make([]item, len(m.items))
		for i, it := range m.items {
			items[i] = item{symbol: r.symbol(it.symbol), stream: it.stream, result: it.result}
		}
		m.items = items
	} else {
		m.requestID = 0
	}
	if m.symbol != 0 {
		m.symbol = r.symbol(m.symbol)
	}
	if m.update != 0 {
		m.update = r.update(m.update)
	}
	if m.template == ms.DepthUpdateSbeTemplateID {
		m.first, m.last = r.update(m.first), r.update(m.last)
	}
	if m.template == ms.TradesSkippedSbeTemplateID {
		m.first, m.last = r.trade(m.first), r.trade(m.last)
	}
	if m.template == ms.TradesThroughSbeTemplateID {
		// A frontier moves with the trades it accounts for, an epoch's base
		// (0 for epoch 1) included: it is a position, never a null.
		m.lastTrade += tradeOffset
	}
	m.transact, m.send = r.time(m.transact), r.time(m.send)
	m.bids, m.asks = r.levels(m.bids), r.levels(m.asks)
	fills := make([]fill, len(m.fills))
	for i, f := range m.fills {
		fills[i] = fill{id: r.trade(f.id), price: r.price(f.price), qty: qty(f.qty), side: f.side}
	}
	m.fills = fills
	if m.template == ms.BestBidOfferSbeTemplateID {
		m.bidPrice, m.askPrice = r.price(m.bidPrice), r.price(m.askPrice)
		m.bidQty, m.askQty = qty(m.bidQty), qty(m.askQty)
	}
	return m
}

func (r *rewrite) levels(ls []level) []level {
	out := make([]level, len(ls))
	for i, l := range ls {
		out[i] = level{price: r.price(l.price), qty: qty(l.qty)}
	}
	return out
}

func newRewrite(symbols map[uint32]uint32, frames []frame) *rewrite {
	r := &rewrite{symbols: symbols, requests: map[uint64]uint64{}, minUpdate: math.MaxUint64, minPrice: math.MaxInt64, minTime: math.MaxUint64}
	price := func(p int64) {
		if p != nullPrice && p > 0 && p < r.minPrice {
			r.minPrice = p
		}
	}
	tm := func(t uint64) {
		if t != 0 && t < r.minTime {
			r.minTime = t
		}
	}
	for _, f := range frames {
		m := f.m
		for _, u := range []uint64{m.update, m.first, m.last} {
			if m.template == ms.TradesSkippedSbeTemplateID || u == 0 {
				continue
			}
			r.minUpdate = min(r.minUpdate, u)
		}
		for _, l := range append(append([]level{}, m.bids...), m.asks...) {
			price(l.price)
		}
		for _, fl := range m.fills {
			price(fl.price)
		}
		price(m.bidPrice)
		price(m.askPrice)
		tm(m.transact)
		tm(m.send)
	}
	// Synthetic positions stay above zero.
	if r.minUpdate == math.MaxUint64 {
		r.minUpdate = 0
	}
	if r.minPrice == math.MaxInt64 {
		r.minPrice = priceBase
	}
	if r.minTime == math.MaxUint64 {
		r.minTime = timeBase
	}
	return r
}

// fixture is one derived frame or a TODO slot.
type fixture struct {
	name, group, note string
	f                 *frame
}

type deriver struct {
	captures [][]frame
	symbols  map[uint32]uint32
	out      []fixture
}

func newDeriver(captures [][]frame) *deriver {
	return &deriver{captures: captures, symbols: map[uint32]uint32{}}
}

// first returns the first frame of the captures that matches.
func (d *deriver) first(match func(frame) bool) (*frame, bool) {
	for _, c := range d.captures {
		for i := range c {
			if match(c[i]) {
				return &c[i], true
			}
		}
	}
	return nil, false
}

func (d *deriver) add(name, group, note string, f *frame) {
	d.out = append(d.out, fixture{name: name, group: group, note: note, f: f})
}

func (d *deriver) one(name, note string, match func(frame) bool) {
	if f, ok := d.first(match); ok {
		d.add(name, name, note, f)
	} else {
		d.add(name, name, "TODO: no captured frame yet; "+note, nil)
	}
}

func is(dir, message string) func(frame) bool {
	return func(f frame) bool { return f.direction == dir && f.message == message }
}

const (
	streamDepth20 = 3
	streamDepth   = 4
	feedLive      = 1
	feedStale     = 2
)

func (d *deriver) select_() {
	d.one("client-subscribe", "a Subscribe", is("client", "Subscribe"))
	d.one("client-ping", "a Ping", is("client", "Ping"))
	d.one("subscribe-response", "the answer to client-subscribe", is("server", "SubscribeResponse"))
	d.one("pong", "the answer to client-ping", is("server", "Pong"))
	d.one("best-bid-offer", "a BestBidOffer", is("server", "BestBidOffer"))
	d.one("market-status-stale", "MarketStatus STALE", func(f frame) bool { return f.message == "MarketStatus" && f.m.feed == feedStale })
	d.one("market-status-live", "MarketStatus LIVE", func(f frame) bool { return f.message == "MarketStatus" && f.m.feed == feedLive })
	d.trades()
	d.tradesSkipped()
	d.depth20()
	d.one("depth-snapshot", "a depth-stream DepthSnapshot", func(f frame) bool { return f.message == "DepthSnapshot" && f.m.stream == streamDepth })
	d.chains()
	d.levelsChain()
}

// trades selects a Trades message whose pair's baseline TradesThrough came
// before it in the same capture, with that baseline.
func (d *deriver) trades() {
	for _, c := range d.captures {
		through := map[uint32]int{}
		for i, f := range c {
			switch f.message {
			case "TradesThrough":
				through[f.m.symbol] = i
			case "Trades":
				if j, ok := through[f.m.symbol]; ok {
					d.add("trades-through", "trades", "the pair's baseline", &c[j])
					d.add("trades", "trades", "trades after the baseline", &c[i])
					return
				}
			}
		}
	}
	d.add("trades-through", "trades", "TODO: no captured baseline before trades yet", nil)
	d.add("trades", "trades", "TODO: no captured trades after a baseline yet", nil)
}

// tradesSkipped selects a TradesSkipped whose pair's baseline TradesThrough
// came before it in the same capture, with that baseline.
func (d *deriver) tradesSkipped() {
	for _, c := range d.captures {
		through := map[uint32]int{}
		for i, f := range c {
			switch f.message {
			case "TradesThrough":
				through[f.m.symbol] = i
			case "TradesSkipped":
				if j, ok := through[f.m.symbol]; ok {
					d.add("trades-skipped-through", "trades-skipped", "the pair's baseline", &c[j])
					d.add("trades-skipped", "trades-skipped", "trades the subscription will not receive live", &c[i])
					return
				}
			}
		}
	}
	d.add("trades-skipped", "trades-skipped", "TODO: no captured TradesSkipped after a baseline yet", nil)
}

// levelsChain selects the first depth chain whose snapshot holds levels.
func (d *deriver) levelsChain() {
	for _, c := range d.captures {
		for _, ch := range d.chainsOf(c) {
			s := ch.frames[0].m
			if len(s.bids)+len(s.asks) == 0 {
				continue
			}
			for i, f := range ch.frames[:min(len(ch.frames), 1+maxChainUpdates)] {
				note := "contiguous DepthUpdate"
				if i == 0 {
					note = "a depth-stream DepthSnapshot holding levels"
				}
				d.add(fmt.Sprintf("depth-levels-chain-%02d", i), "depth-levels-chain", note, f)
			}
			return
		}
	}
	d.add("depth-levels-chain", "depth-levels-chain", "TODO: no captured depth-stream DepthSnapshot holding levels yet", nil)
}

// depth20 selects the depth20 snapshot with the most levels.
func (d *deriver) depth20() {
	var best *frame
	for _, c := range d.captures {
		for i, f := range c {
			if f.message == "DepthSnapshot" && f.m.stream == streamDepth20 && (best == nil || len(f.m.bids)+len(f.m.asks) > len(best.m.bids)+len(best.m.asks)) {
				best = &c[i]
			}
		}
	}
	if best == nil {
		d.add("depth20-snapshot", "depth20-snapshot", "TODO: no captured depth20 snapshot yet", nil)
		return
	}
	d.add("depth20-snapshot", "depth20-snapshot", "the depth20 snapshot with the most levels", best)
}

// chain is a depth-stream snapshot and the contiguous updates of its pair
// after it in one capture.
type chain struct{ frames []*frame }

func (d *deriver) chainsOf(c []frame) []chain {
	var out []chain
	for i, f := range c {
		if f.message != "DepthSnapshot" || f.m.stream != streamDepth {
			continue
		}
		ch := chain{frames: []*frame{&c[i]}}
		pos := f.m.update
		for j := i + 1; j < len(c); j++ {
			g := c[j]
			if g.m.symbol != f.m.symbol {
				continue
			}
			if g.message == "DepthSnapshot" && g.m.stream == streamDepth {
				break
			}
			if g.message != "DepthUpdate" {
				continue
			}
			if g.m.epoch != f.m.epoch || g.m.first != pos+1 {
				break
			}
			ch.frames = append(ch.frames, &c[j])
			pos = g.m.last
		}
		if len(ch.frames) > 1 {
			out = append(out, ch)
		}
	}
	return out
}

const maxChainUpdates = 8

// chains selects the longest contiguous chain of updates with levels, and
// the first chain whose first update is a bridging range without levels.
func (d *deriver) chains() {
	var longest, bridge *chain
	for _, c := range d.captures {
		for _, ch := range d.chainsOf(c) {
			withLevels := 0
			for _, f := range ch.frames[1:] {
				if len(f.m.bids)+len(f.m.asks) > 0 {
					withLevels++
				}
			}
			if withLevels > 0 && (longest == nil || len(ch.frames) > len(longest.frames)) {
				ch := ch
				longest = &ch
			}
			u := ch.frames[1].m
			if bridge == nil && u.last > u.first && len(u.bids)+len(u.asks) == 0 {
				ch := chain{frames: ch.frames[:2]}
				bridge = &ch
			}
		}
	}
	if longest == nil {
		d.add("depth-chain", "depth-chain", "TODO: no captured depth chain with levels yet", nil)
	} else {
		frames := longest.frames[:min(len(longest.frames), 1+maxChainUpdates)]
		for i, f := range frames {
			note := "contiguous DepthUpdate"
			if i == 0 {
				note = "the chain's DepthSnapshot"
			}
			d.add(fmt.Sprintf("depth-chain-%02d", i), "depth-chain", note, f)
		}
	}
	if bridge == nil {
		d.add("depth-bridge", "depth-bridge", "TODO: no captured bridging DepthUpdate yet", nil)
	} else {
		d.add("depth-bridge-00", "depth-bridge", "the DepthSnapshot the bridging range starts after", bridge.frames[0])
		d.add("depth-bridge-01", "depth-bridge", "a bridging DepthUpdate: a range without levels", bridge.frames[1])
	}
}

func (d *deriver) write(outDir string) error {
	d.select_()
	// One rewrite per capture, over all its frames; symbols are shared.
	rewrites := map[string]*rewrite{}
	for _, c := range d.captures {
		if len(c) > 0 {
			rewrites[c[0].capture] = newRewrite(d.symbols, c)
		}
	}
	// Symbols are numbered in the order the fixtures name them.
	if err := os.RemoveAll(outDir); err != nil {
		return err
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}
	var index strings.Builder
	index.WriteString("# Synthetic market-data stream frames derived from captures by internal/tools/marketstreamfixtures.\n")
	index.WriteString("# Every identifier, price, quantity and time is synthetic. Rows with no file are TODO slots.\n")
	index.WriteString("file\tdirection\tmessage\tgroup\tnote\n")
	for i, fx := range d.out {
		if fx.f == nil {
			fmt.Fprintf(&index, "-\t-\t-\t%s\t%s\n", fx.group, fx.note)
			continue
		}
		m := rewrites[fx.f.capture].apply(fx.f.m)
		data, err := encode(m)
		if err != nil {
			return fmt.Errorf("%s: %w", fx.name, err)
		}
		again, err := decode(data)
		if err != nil {
			return fmt.Errorf("%s re-encoded does not decode: %w", fx.name, err)
		}
		_ = again
		name := fmt.Sprintf("%02d-%s.bin", i+1, fx.name)
		if err := os.WriteFile(filepath.Join(outDir, name), data, 0o644); err != nil {
			return err
		}
		fmt.Fprintf(&index, "%s\t%s\t%s\t%s\t%s\n", name, fx.f.direction, fx.f.message, fx.group, fx.note)
	}
	return os.WriteFile(filepath.Join(outDir, "index.tsv"), []byte(index.String()), 0o644)
}

// decode reads a frame with the generated decoders.
func decode(b []byte) (m msg, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("undecodable frame: %v", r)
		}
	}()
	if len(b) < int(ms.MessageHeaderEncodedLength) {
		return msg{}, errors.New("frame shorter than its header")
	}
	var h ms.MessageHeader
	h.Wrap(b, 0, 0, uint64(len(b)))
	if h.SchemaId() != ms.MessageHeaderSbeSchemaID {
		return msg{}, fmt.Errorf("schema %d", h.SchemaId())
	}
	off, block, ver, size := ms.MessageHeaderEncodedLength, uint64(h.BlockLength()), uint64(h.Version()), uint64(len(b))
	m.template = h.TemplateId()
	switch m.template {
	case ms.SubscribeSbeTemplateID:
		var x ms.Subscribe
		x.WrapForDecode(b, off, block, ver, size)
		m.requestID = x.RequestId()
		g := x.Items()
		for g.HasNext() {
			it := g.Next()
			m.items = append(m.items, item{symbol: it.SymbolId(), stream: uint8(it.Stream())})
		}
	case ms.UnsubscribeSbeTemplateID:
		var x ms.Unsubscribe
		x.WrapForDecode(b, off, block, ver, size)
		m.requestID = x.RequestId()
		g := x.Items()
		for g.HasNext() {
			it := g.Next()
			m.items = append(m.items, item{symbol: it.SymbolId(), stream: uint8(it.Stream())})
		}
	case ms.PingSbeTemplateID:
		var x ms.Ping
		x.WrapForDecode(b, off, block, ver, size)
		m.requestID = x.RequestId()
	case ms.SubscribeResponseSbeTemplateID:
		var x ms.SubscribeResponse
		x.WrapForDecode(b, off, block, ver, size)
		m.requestID = x.RequestId()
		g := x.Results()
		for g.HasNext() {
			r := g.Next()
			m.items = append(m.items, item{symbol: r.SymbolId(), stream: uint8(r.Stream()), result: uint8(r.Result())})
		}
	case ms.PongSbeTemplateID:
		var x ms.Pong
		x.WrapForDecode(b, off, block, ver, size)
		m.requestID, m.send = x.RequestId(), x.SendTsNs()
	case ms.TradesSbeTemplateID:
		var x ms.Trades
		x.WrapForDecode(b, off, block, ver, size)
		m.symbol, m.epoch, m.update, m.transact, m.send = x.SymbolId(), x.BookEpoch(), x.UpdateId(), x.TransactTsNs(), x.SendTsNs()
		g := x.Entries()
		for g.HasNext() {
			f := g.Next()
			m.fills = append(m.fills, fill{id: f.TradeId(), price: f.PriceTicks(), qty: f.QtyScaled(), side: uint8(f.TakerSide())})
		}
	case ms.TradesSkippedSbeTemplateID:
		var x ms.TradesSkipped
		x.WrapForDecode(b, off, block, ver, size)
		m.symbol, m.epoch, m.first, m.last, m.send = x.SymbolId(), x.BookEpoch(), x.FirstTradeId(), x.LastTradeId(), x.SendTsNs()
	case ms.TradesThroughSbeTemplateID:
		var x ms.TradesThrough
		x.WrapForDecode(b, off, block, ver, size)
		m.symbol, m.epoch, m.lastTrade, m.send = x.SymbolId(), x.BookEpoch(), x.LastTradeId(), x.SendTsNs()
	case ms.BestBidOfferSbeTemplateID:
		var x ms.BestBidOffer
		x.WrapForDecode(b, off, block, ver, size)
		m.symbol, m.epoch, m.update, m.transact, m.send = x.SymbolId(), x.BookEpoch(), x.UpdateId(), x.TransactTsNs(), x.SendTsNs()
		m.bidPrice, m.bidQty, m.askPrice, m.askQty = x.BidPriceTicks(), x.BidQtyScaled(), x.AskPriceTicks(), x.AskQtyScaled()
	case ms.DepthSnapshotSbeTemplateID:
		var x ms.DepthSnapshot
		x.WrapForDecode(b, off, block, ver, size)
		m.symbol, m.stream, m.epoch, m.update = x.SymbolId(), uint8(x.Stream()), x.BookEpoch(), x.UpdateId()
		m.transact, m.send, m.levelsPerSide = x.TransactTsNs(), x.SendTsNs(), x.LevelsPerSide()
		bids := x.Bids()
		for bids.HasNext() {
			l := bids.Next()
			m.bids = append(m.bids, level{l.PriceTicks(), l.QtyScaled()})
		}
		asks := x.Asks()
		for asks.HasNext() {
			l := asks.Next()
			m.asks = append(m.asks, level{l.PriceTicks(), l.QtyScaled()})
		}
	case ms.DepthUpdateSbeTemplateID:
		var x ms.DepthUpdate
		x.WrapForDecode(b, off, block, ver, size)
		m.symbol, m.epoch, m.first, m.last, m.transact, m.send = x.SymbolId(), x.BookEpoch(), x.FirstUpdateId(), x.LastUpdateId(), x.TransactTsNs(), x.SendTsNs()
		bids := x.Bids()
		for bids.HasNext() {
			l := bids.Next()
			m.bids = append(m.bids, level{l.PriceTicks(), l.QtyScaled()})
		}
		asks := x.Asks()
		for asks.HasNext() {
			l := asks.Next()
			m.asks = append(m.asks, level{l.PriceTicks(), l.QtyScaled()})
		}
	case ms.MarketStatusSbeTemplateID:
		var x ms.MarketStatus
		x.WrapForDecode(b, off, block, ver, size)
		m.symbol, m.feed, m.send = x.SymbolId(), uint8(x.Feed()), x.SendTsNs()
	default:
		return msg{}, fmt.Errorf("template %d", m.template)
	}
	return m, nil
}

// encode writes a message with the generated encoders.
func encode(m msg) ([]byte, error) {
	buf := make([]byte, 1<<20)
	n := uint64(len(buf))
	var length uint64
	switch m.template {
	case ms.SubscribeSbeTemplateID:
		var x ms.Subscribe
		x.WrapAndApplyHeader(buf, 0, n).SetRequestId(m.requestID)
		g := x.ItemsCount(uint16(len(m.items)))
		for _, it := range m.items {
			g.Next().SetSymbolId(it.symbol).SetStream(ms.StreamType(it.stream))
		}
		length = x.EncodedLength()
	case ms.UnsubscribeSbeTemplateID:
		var x ms.Unsubscribe
		x.WrapAndApplyHeader(buf, 0, n).SetRequestId(m.requestID)
		g := x.ItemsCount(uint16(len(m.items)))
		for _, it := range m.items {
			g.Next().SetSymbolId(it.symbol).SetStream(ms.StreamType(it.stream))
		}
		length = x.EncodedLength()
	case ms.PingSbeTemplateID:
		var x ms.Ping
		x.WrapAndApplyHeader(buf, 0, n).SetRequestId(m.requestID)
		length = x.EncodedLength()
	case ms.SubscribeResponseSbeTemplateID:
		var x ms.SubscribeResponse
		x.WrapAndApplyHeader(buf, 0, n).SetRequestId(m.requestID)
		g := x.ResultsCount(uint16(len(m.items)))
		for _, it := range m.items {
			g.Next().SetSymbolId(it.symbol).SetStream(ms.StreamType(it.stream)).SetResult(ms.SubscribeResult(it.result))
		}
		length = x.EncodedLength()
	case ms.PongSbeTemplateID:
		var x ms.Pong
		x.WrapAndApplyHeader(buf, 0, n).SetRequestId(m.requestID).SetSendTsNs(m.send)
		length = x.EncodedLength()
	case ms.TradesSbeTemplateID:
		var x ms.Trades
		x.WrapAndApplyHeader(buf, 0, n).SetSymbolId(m.symbol).SetBookEpoch(m.epoch).SetUpdateId(m.update).SetTransactTsNs(m.transact).SetSendTsNs(m.send)
		g := x.EntriesCount(uint16(len(m.fills)))
		for _, f := range m.fills {
			g.Next().SetTradeId(f.id).SetPriceTicks(f.price).SetQtyScaled(f.qty).SetTakerSide(ms.Side(f.side))
		}
		length = x.EncodedLength()
	case ms.TradesSkippedSbeTemplateID:
		var x ms.TradesSkipped
		x.WrapAndApplyHeader(buf, 0, n).SetSymbolId(m.symbol).SetBookEpoch(m.epoch).SetFirstTradeId(m.first).SetLastTradeId(m.last).SetSendTsNs(m.send)
		length = x.EncodedLength()
	case ms.TradesThroughSbeTemplateID:
		var x ms.TradesThrough
		x.WrapAndApplyHeader(buf, 0, n).SetSymbolId(m.symbol).SetBookEpoch(m.epoch).SetLastTradeId(m.lastTrade).SetSendTsNs(m.send)
		length = x.EncodedLength()
	case ms.BestBidOfferSbeTemplateID:
		var x ms.BestBidOffer
		x.WrapAndApplyHeader(buf, 0, n).SetSymbolId(m.symbol).SetBookEpoch(m.epoch).SetUpdateId(m.update).SetTransactTsNs(m.transact).SetSendTsNs(m.send).
			SetBidPriceTicks(m.bidPrice).SetBidQtyScaled(m.bidQty).SetAskPriceTicks(m.askPrice).SetAskQtyScaled(m.askQty)
		length = x.EncodedLength()
	case ms.DepthSnapshotSbeTemplateID:
		var x ms.DepthSnapshot
		x.WrapAndApplyHeader(buf, 0, n).SetSymbolId(m.symbol).SetStream(ms.StreamType(m.stream)).SetBookEpoch(m.epoch).SetUpdateId(m.update).
			SetTransactTsNs(m.transact).SetSendTsNs(m.send).SetLevelsPerSide(m.levelsPerSide)
		bids := x.BidsCount(uint16(len(m.bids)))
		for _, l := range m.bids {
			bids.Next().SetPriceTicks(l.price).SetQtyScaled(l.qty)
		}
		asks := x.AsksCount(uint16(len(m.asks)))
		for _, l := range m.asks {
			asks.Next().SetPriceTicks(l.price).SetQtyScaled(l.qty)
		}
		length = x.EncodedLength()
	case ms.DepthUpdateSbeTemplateID:
		var x ms.DepthUpdate
		x.WrapAndApplyHeader(buf, 0, n).SetSymbolId(m.symbol).SetBookEpoch(m.epoch).SetFirstUpdateId(m.first).SetLastUpdateId(m.last).
			SetTransactTsNs(m.transact).SetSendTsNs(m.send)
		bids := x.BidsCount(uint16(len(m.bids)))
		for _, l := range m.bids {
			bids.Next().SetPriceTicks(l.price).SetQtyScaled(l.qty)
		}
		asks := x.AsksCount(uint16(len(m.asks)))
		for _, l := range m.asks {
			asks.Next().SetPriceTicks(l.price).SetQtyScaled(l.qty)
		}
		length = x.EncodedLength()
	case ms.MarketStatusSbeTemplateID:
		var x ms.MarketStatus
		x.WrapAndApplyHeader(buf, 0, n).SetSymbolId(m.symbol).SetFeed(ms.FeedState(m.feed)).SetSendTsNs(m.send)
		length = x.EncodedLength()
	default:
		return nil, fmt.Errorf("template %d", m.template)
	}
	return buf[:ms.MessageHeaderEncodedLength+length], nil
}
