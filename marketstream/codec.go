package marketstream

import (
	"fmt"

	ms "github.com/Fabric-Labs/polyester-sdk-go/internal/sbe/marketstream"
)

// SchemaID and SchemaVersion identify the schema this client's decoders are
// generated from. A message of another schema is refused. A message of a
// newer version of this schema is read by its declared block lengths: fields
// and groups the newer version appends are skipped.
const (
	SchemaID      = ms.MessageHeaderSbeSchemaID
	SchemaVersion = ms.MessageHeaderSbeSchemaVersion
)

// SchemaError refuses a message this client cannot read: one of another
// schema. The server speaks a protocol this client does not, so the client
// stops instead of reconnecting.
type SchemaError struct {
	SchemaID uint16
	Version  uint16
}

func (e *SchemaError) Error() string {
	return fmt.Sprintf("market stream message of schema %d version %d; this client reads schema %d up to the rules of version %d", e.SchemaID, e.Version, SchemaID, SchemaVersion)
}

// FrameError refuses a message whose bytes do not match its declared layout,
// or whose values break the protocol. The client reconnects, so every stream
// restarts from a fresh baseline; without Config.AutoReconnect it stops with
// this error.
type FrameError struct {
	TemplateID uint16
	Msg        string
}

func (e *FrameError) Error() string {
	return fmt.Sprintf("invalid market stream message (template %d): %s", e.TemplateID, e.Msg)
}

const headerLength = int(ms.MessageHeaderEncodedLength)

// The decoded messages, in wire units. Template IDs are the generated
// constants.
type (
	tradesMsg struct {
		symbolID                            uint32
		epoch, updateID, transactNs, sendNs uint64
		fills                               []fillMsg
	}
	fillMsg struct {
		tradeID    uint64
		price, qty int64
		side       uint8
	}
	tradesSkippedMsg struct {
		symbolID                   uint32
		epoch, first, last, sendNs uint64
	}
	tradesThroughMsg struct {
		symbolID            uint32
		epoch, last, sendNs uint64
	}
	bboMsg struct {
		symbolID                            uint32
		epoch, updateID, transactNs, sendNs uint64
		bidPrice, bidQty, askPrice, askQty  int64
	}
	levelMsg struct {
		price, qty int64
	}
	depthSnapshotMsg struct {
		symbolID                            uint32
		stream                              uint8
		epoch, updateID, transactNs, sendNs uint64
		levelsPerSide                       uint16
		bids, asks                          []levelMsg
	}
	depthUpdateMsg struct {
		symbolID                               uint32
		epoch, first, last, transactNs, sendNs uint64
		bids, asks                             []levelMsg
	}
	marketStatusMsg struct {
		symbolID uint32
		feed     uint8
		sendNs   uint64
	}
	subscribeResponseMsg struct {
		requestID uint64
		results   []resultMsg
	}
	resultMsg struct {
		symbolID       uint32
		stream, result uint8
	}
	pongMsg struct {
		requestID, sendNs uint64
	}
)

// decodeFrame decodes one WebSocket binary message. It returns nil and no
// error for a template this client does not know, which the protocol lets a
// newer server add.
func decodeFrame(frame []byte) (msg any, err error) {
	if len(frame) < headerLength {
		return nil, &FrameError{Msg: fmt.Sprintf("%d bytes, shorter than the message header", len(frame))}
	}
	var h ms.MessageHeader
	h.Wrap(frame, 0, uint64(SchemaVersion), uint64(len(frame)))
	if h.SchemaId() != SchemaID {
		return nil, &SchemaError{SchemaID: h.SchemaId(), Version: h.Version()}
	}
	template := h.TemplateId()
	// The layout is checked before any generated accessor reads, so they
	// never index past the frame; recover only guards against a generator
	// defect.
	defer func() {
		if r := recover(); r != nil {
			msg, err = nil, &FrameError{TemplateID: template, Msg: fmt.Sprint(r)}
		}
	}()
	block, version, size := uint64(h.BlockLength()), uint64(h.Version()), uint64(len(frame))
	check := func(minBlock uint16, groups ...uint64) error {
		return checkLayout(frame, h, minBlock, groups...)
	}
	switch template {
	case ms.TradesSbeTemplateID:
		var m ms.Trades
		if err := check(m.SbeBlockLength(), (&ms.TradesEntries{}).SbeBlockLength()); err != nil {
			return nil, err
		}
		m.WrapForDecode(frame, uint64(headerLength), block, version, size)
		out := &tradesMsg{symbolID: m.SymbolId(), epoch: m.BookEpoch(), updateID: m.UpdateId(), transactNs: m.TransactTsNs(), sendNs: m.SendTsNs()}
		g := m.Entries()
		out.fills = make([]fillMsg, 0, g.Count())
		for g.HasNext() {
			f := g.Next()
			out.fills = append(out.fills, fillMsg{tradeID: f.TradeId(), price: f.PriceTicks(), qty: f.QtyScaled(), side: uint8(f.TakerSide())})
		}
		return out, nil
	case ms.TradesSkippedSbeTemplateID:
		var m ms.TradesSkipped
		if err := check(m.SbeBlockLength()); err != nil {
			return nil, err
		}
		m.WrapForDecode(frame, uint64(headerLength), block, version, size)
		return &tradesSkippedMsg{symbolID: m.SymbolId(), epoch: m.BookEpoch(), first: m.FirstTradeId(), last: m.LastTradeId(), sendNs: m.SendTsNs()}, nil
	case ms.TradesThroughSbeTemplateID:
		var m ms.TradesThrough
		if err := check(m.SbeBlockLength()); err != nil {
			return nil, err
		}
		m.WrapForDecode(frame, uint64(headerLength), block, version, size)
		return &tradesThroughMsg{symbolID: m.SymbolId(), epoch: m.BookEpoch(), last: m.LastTradeId(), sendNs: m.SendTsNs()}, nil
	case ms.BestBidOfferSbeTemplateID:
		var m ms.BestBidOffer
		if err := check(m.SbeBlockLength()); err != nil {
			return nil, err
		}
		m.WrapForDecode(frame, uint64(headerLength), block, version, size)
		return &bboMsg{
			symbolID: m.SymbolId(), epoch: m.BookEpoch(), updateID: m.UpdateId(), transactNs: m.TransactTsNs(), sendNs: m.SendTsNs(),
			bidPrice: m.BidPriceTicks(), bidQty: m.BidQtyScaled(), askPrice: m.AskPriceTicks(), askQty: m.AskQtyScaled(),
		}, nil
	case ms.DepthSnapshotSbeTemplateID:
		var m ms.DepthSnapshot
		if err := check(m.SbeBlockLength(), (&ms.DepthSnapshotBids{}).SbeBlockLength(), (&ms.DepthSnapshotAsks{}).SbeBlockLength()); err != nil {
			return nil, err
		}
		m.WrapForDecode(frame, uint64(headerLength), block, version, size)
		out := &depthSnapshotMsg{symbolID: m.SymbolId(), stream: uint8(m.Stream()), epoch: m.BookEpoch(), updateID: m.UpdateId(), transactNs: m.TransactTsNs(), sendNs: m.SendTsNs(), levelsPerSide: m.LevelsPerSide()}
		bids := m.Bids()
		out.bids = make([]levelMsg, 0, bids.Count())
		for bids.HasNext() {
			l := bids.Next()
			out.bids = append(out.bids, levelMsg{price: l.PriceTicks(), qty: l.QtyScaled()})
		}
		asks := m.Asks()
		out.asks = make([]levelMsg, 0, asks.Count())
		for asks.HasNext() {
			l := asks.Next()
			out.asks = append(out.asks, levelMsg{price: l.PriceTicks(), qty: l.QtyScaled()})
		}
		return out, nil
	case ms.DepthUpdateSbeTemplateID:
		var m ms.DepthUpdate
		if err := check(m.SbeBlockLength(), (&ms.DepthUpdateBids{}).SbeBlockLength(), (&ms.DepthUpdateAsks{}).SbeBlockLength()); err != nil {
			return nil, err
		}
		m.WrapForDecode(frame, uint64(headerLength), block, version, size)
		out := &depthUpdateMsg{symbolID: m.SymbolId(), epoch: m.BookEpoch(), first: m.FirstUpdateId(), last: m.LastUpdateId(), transactNs: m.TransactTsNs(), sendNs: m.SendTsNs()}
		bids := m.Bids()
		out.bids = make([]levelMsg, 0, bids.Count())
		for bids.HasNext() {
			l := bids.Next()
			out.bids = append(out.bids, levelMsg{price: l.PriceTicks(), qty: l.QtyScaled()})
		}
		asks := m.Asks()
		out.asks = make([]levelMsg, 0, asks.Count())
		for asks.HasNext() {
			l := asks.Next()
			out.asks = append(out.asks, levelMsg{price: l.PriceTicks(), qty: l.QtyScaled()})
		}
		return out, nil
	case ms.MarketStatusSbeTemplateID:
		var m ms.MarketStatus
		if err := check(m.SbeBlockLength()); err != nil {
			return nil, err
		}
		m.WrapForDecode(frame, uint64(headerLength), block, version, size)
		return &marketStatusMsg{symbolID: m.SymbolId(), feed: uint8(m.Feed()), sendNs: m.SendTsNs()}, nil
	case ms.SubscribeResponseSbeTemplateID:
		var m ms.SubscribeResponse
		if err := check(m.SbeBlockLength(), (&ms.SubscribeResponseResults{}).SbeBlockLength()); err != nil {
			return nil, err
		}
		m.WrapForDecode(frame, uint64(headerLength), block, version, size)
		out := &subscribeResponseMsg{requestID: m.RequestId()}
		g := m.Results()
		out.results = make([]resultMsg, 0, g.Count())
		for g.HasNext() {
			r := g.Next()
			out.results = append(out.results, resultMsg{symbolID: r.SymbolId(), stream: uint8(r.Stream()), result: uint8(r.Result())})
		}
		return out, nil
	case ms.PongSbeTemplateID:
		var m ms.Pong
		if err := check(m.SbeBlockLength()); err != nil {
			return nil, err
		}
		m.WrapForDecode(frame, uint64(headerLength), block, version, size)
		return &pongMsg{requestID: m.RequestId(), sendNs: m.SendTsNs()}, nil
	default:
		return nil, nil
	}
}

// checkLayout checks that the frame holds the fixed block its header declares
// and every repeating group in order, each entry at least as long as this
// version's entries. A message of this client's version must end with its
// last group; a newer version may append groups or data after them, which
// are skipped.
func checkLayout(frame []byte, h ms.MessageHeader, minBlock uint16, groups ...uint64) error {
	template := h.TemplateId()
	size := uint64(len(frame))
	block := uint64(h.BlockLength())
	if h.Version() >= SchemaVersion && block < uint64(minBlock) {
		return &FrameError{TemplateID: template, Msg: fmt.Sprintf("block length %d, shorter than this version's %d", block, minBlock)}
	}
	pos := uint64(headerLength) + block
	if pos > size {
		return &FrameError{TemplateID: template, Msg: fmt.Sprintf("block length %d past the end of a %d-byte message", block, size)}
	}
	for i, minEntry := range groups {
		var d ms.GroupSizeEncoding
		if pos+d.EncodedLength() > size {
			return &FrameError{TemplateID: template, Msg: fmt.Sprintf("group %d header past the end of the message", i+1)}
		}
		d.Wrap(frame, pos, uint64(h.Version()), size)
		entry, count := uint64(d.BlockLength()), uint64(d.NumInGroup())
		if entry < minEntry {
			return &FrameError{TemplateID: template, Msg: fmt.Sprintf("group %d entry length %d, shorter than this version's %d", i+1, entry, minEntry)}
		}
		pos += d.EncodedLength() + entry*count
		if pos > size {
			return &FrameError{TemplateID: template, Msg: fmt.Sprintf("group %d of %d entries past the end of the message", i+1, count)}
		}
	}
	if pos != size && h.Version() <= SchemaVersion {
		return &FrameError{TemplateID: template, Msg: fmt.Sprintf("%d bytes after the last group", size-pos)}
	}
	return nil
}

// maxSubscribeItems bounds the topics of one Subscribe message, so it stays
// well within the server's inbound message limit.
const maxSubscribeItems = 128

func encodeSubscribe(requestID uint64, topics []Topic) []byte {
	var m ms.Subscribe
	entry := (&ms.SubscribeItems{}).SbeBlockLength()
	buf := make([]byte, m.SbeBlockAndHeaderLength()+uint64(ms.GroupSizeEncodingEncodedLength)+entry*uint64(len(topics)))
	m.WrapAndApplyHeader(buf, 0, uint64(len(buf)))
	m.SetRequestId(requestID)
	items := m.ItemsCount(uint16(len(topics)))
	for _, t := range topics {
		items.Next().SetSymbolId(t.SymbolID).SetStream(ms.StreamType(t.Stream))
	}
	return buf[:uint64(headerLength)+m.EncodedLength()]
}

func encodeUnsubscribe(requestID uint64, topics []Topic) []byte {
	var m ms.Unsubscribe
	entry := (&ms.UnsubscribeItems{}).SbeBlockLength()
	buf := make([]byte, m.SbeBlockAndHeaderLength()+uint64(ms.GroupSizeEncodingEncodedLength)+entry*uint64(len(topics)))
	m.WrapAndApplyHeader(buf, 0, uint64(len(buf)))
	m.SetRequestId(requestID)
	items := m.ItemsCount(uint16(len(topics)))
	for _, t := range topics {
		items.Next().SetSymbolId(t.SymbolID).SetStream(ms.StreamType(t.Stream))
	}
	return buf[:uint64(headerLength)+m.EncodedLength()]
}

func encodePing(requestID uint64) []byte {
	var m ms.Ping
	buf := make([]byte, m.SbeBlockAndHeaderLength())
	m.WrapAndApplyHeader(buf, 0, uint64(len(buf)))
	m.SetRequestId(requestID)
	return buf
}
