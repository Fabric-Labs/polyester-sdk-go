package marketstream

import (
	"encoding/binary"
	"math"

	ms "github.com/Fabric-Labs/polyester-sdk-go/internal/sbe/marketstream"
)

// Synthesized server messages, encoded with the generated encoders.

const testBufSize = 1 << 16

func finish(buf []byte, encoded uint64) []byte {
	return buf[:uint64(headerLength)+encoded]
}

func tradesFrame(symbol uint32, epoch, updateID uint64, fills ...fillMsg) []byte {
	return tradesFrameAt(symbol, epoch, updateID, 1_700_000_000_000_000_000, fills...)
}

// tradesFrameAt is tradesFrame of a book update executed at transactNs.
func tradesFrameAt(symbol uint32, epoch, updateID, transactNs uint64, fills ...fillMsg) []byte {
	buf := make([]byte, testBufSize)
	var m ms.Trades
	m.WrapAndApplyHeader(buf, 0, uint64(len(buf))).
		SetSymbolId(symbol).SetBookEpoch(epoch).SetUpdateId(updateID).SetTransactTsNs(transactNs).SetSendTsNs(1_700_000_000_000_001_000)
	g := m.EntriesCount(uint16(len(fills)))
	for _, f := range fills {
		g.Next().SetTradeId(f.tradeID).SetPriceTicks(f.price).SetQtyScaled(f.qty).SetTakerSide(ms.Side(f.side))
	}
	return finish(buf, m.EncodedLength())
}

func fill(id uint64) fillMsg {
	return fillMsg{tradeID: id, price: 65_000_000_000_000, qty: 1_500, side: 1}
}

func tradesThroughFrame(symbol uint32, epoch, last uint64) []byte {
	buf := make([]byte, testBufSize)
	var m ms.TradesThrough
	m.WrapAndApplyHeader(buf, 0, uint64(len(buf))).SetSymbolId(symbol).SetBookEpoch(epoch).SetLastTradeId(last).SetSendTsNs(1)
	return finish(buf, m.EncodedLength())
}

func tradesSkippedFrame(symbol uint32, epoch, first, last uint64) []byte {
	buf := make([]byte, testBufSize)
	var m ms.TradesSkipped
	m.WrapAndApplyHeader(buf, 0, uint64(len(buf))).SetSymbolId(symbol).SetBookEpoch(epoch).SetFirstTradeId(first).SetLastTradeId(last).SetSendTsNs(1)
	return finish(buf, m.EncodedLength())
}

const noPrice = math.MinInt64

func bboFrame(symbol uint32, epoch, updateID uint64, bidPrice, bidQty, askPrice, askQty int64) []byte {
	buf := make([]byte, testBufSize)
	var m ms.BestBidOffer
	m.WrapAndApplyHeader(buf, 0, uint64(len(buf))).SetSymbolId(symbol).SetBookEpoch(epoch).SetUpdateId(updateID).
		SetTransactTsNs(1).SetSendTsNs(2).SetBidPriceTicks(bidPrice).SetBidQtyScaled(bidQty).SetAskPriceTicks(askPrice).SetAskQtyScaled(askQty)
	return finish(buf, m.EncodedLength())
}

func lv(price, qty int64) levelMsg { return levelMsg{price: price, qty: qty} }

func depthSnapshotFrame(symbol uint32, stream Stream, epoch, updateID uint64, bids, asks []levelMsg) []byte {
	window := uint16(1000)
	if stream == StreamDepth20 {
		window = 20
	}
	return depthSnapshotFrameWindow(symbol, stream, epoch, updateID, window, bids, asks)
}

func depthSnapshotFrameWindow(symbol uint32, stream Stream, epoch, updateID uint64, window uint16, bids, asks []levelMsg) []byte {
	buf := make([]byte, testBufSize)
	var m ms.DepthSnapshot
	m.WrapAndApplyHeader(buf, 0, uint64(len(buf))).SetSymbolId(symbol).SetStream(ms.StreamType(stream)).SetBookEpoch(epoch).SetUpdateId(updateID).SetTransactTsNs(1).SetSendTsNs(2).SetLevelsPerSide(window)
	b := m.BidsCount(uint16(len(bids)))
	for _, l := range bids {
		b.Next().SetPriceTicks(l.price).SetQtyScaled(l.qty)
	}
	a := m.AsksCount(uint16(len(asks)))
	for _, l := range asks {
		a.Next().SetPriceTicks(l.price).SetQtyScaled(l.qty)
	}
	return finish(buf, m.EncodedLength())
}

func depthUpdateFrame(symbol uint32, epoch, first, last uint64, bids, asks []levelMsg) []byte {
	buf := make([]byte, testBufSize)
	var m ms.DepthUpdate
	m.WrapAndApplyHeader(buf, 0, uint64(len(buf))).SetSymbolId(symbol).SetBookEpoch(epoch).SetFirstUpdateId(first).SetLastUpdateId(last).SetTransactTsNs(1).SetSendTsNs(2)
	b := m.BidsCount(uint16(len(bids)))
	for _, l := range bids {
		b.Next().SetPriceTicks(l.price).SetQtyScaled(l.qty)
	}
	a := m.AsksCount(uint16(len(asks)))
	for _, l := range asks {
		a.Next().SetPriceTicks(l.price).SetQtyScaled(l.qty)
	}
	return finish(buf, m.EncodedLength())
}

func marketStatusFrame(symbol uint32, feed FeedState) []byte {
	buf := make([]byte, testBufSize)
	var m ms.MarketStatus
	m.WrapAndApplyHeader(buf, 0, uint64(len(buf))).SetSymbolId(symbol).SetFeed(ms.FeedState(feed)).SetSendTsNs(3)
	return finish(buf, m.EncodedLength())
}

func subscribeResponseFrame(requestID uint64, results ...resultMsg) []byte {
	buf := make([]byte, testBufSize)
	var m ms.SubscribeResponse
	m.WrapAndApplyHeader(buf, 0, uint64(len(buf))).SetRequestId(requestID)
	g := m.ResultsCount(uint16(len(results)))
	for _, r := range results {
		g.Next().SetSymbolId(r.symbolID).SetStream(ms.StreamType(r.stream)).SetResult(ms.SubscribeResult(r.result))
	}
	return finish(buf, m.EncodedLength())
}

func pongFrame(requestID uint64) []byte {
	buf := make([]byte, testBufSize)
	var m ms.Pong
	m.WrapAndApplyHeader(buf, 0, uint64(len(buf))).SetRequestId(requestID).SetSendTsNs(4)
	return finish(buf, m.EncodedLength())
}

// withHeader rewrites a frame's header fields.
func withHeader(frame []byte, blockLength, template, schema, version uint16) []byte {
	out := append([]byte(nil), frame...)
	var h ms.MessageHeader
	h.Wrap(out, 0, 0, uint64(len(out)))
	h.SetBlockLength(blockLength).SetTemplateId(template).SetSchemaId(schema).SetVersion(version)
	return out
}

// newerVersion re-encodes a frame as a later schema version would: extra
// bytes appended to the fixed block, extra bytes appended to each group
// entry, and a trailing group the decoder does not know.
func newerVersion(frame []byte, extraBlock, extraEntry int, groups int) []byte {
	var h ms.MessageHeader
	h.Wrap(frame, 0, 0, uint64(len(frame)))
	block := int(h.BlockLength())
	out := append([]byte(nil), frame[:headerLength+block]...)
	out = append(out, make([]byte, extraBlock)...)
	pos := headerLength + block
	for range groups {
		entry := int(binary.LittleEndian.Uint16(frame[pos:]))
		count := int(binary.LittleEndian.Uint16(frame[pos+2:]))
		out = binary.LittleEndian.AppendUint16(out, uint16(entry+extraEntry))
		out = binary.LittleEndian.AppendUint16(out, uint16(count))
		pos += 4
		for range count {
			out = append(out, frame[pos:pos+entry]...)
			out = append(out, make([]byte, extraEntry)...)
			pos += entry
		}
	}
	// A group appended by the newer version: one 3-byte entry.
	out = binary.LittleEndian.AppendUint16(out, 3)
	out = binary.LittleEndian.AppendUint16(out, 1)
	out = append(out, 7, 7, 7)
	return withHeader(out, uint16(block+extraBlock), h.TemplateId(), h.SchemaId(), h.Version()+1)
}
