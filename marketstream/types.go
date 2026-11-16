// Package marketstream is a client for the public binary market-data
// WebSocket: trades, best bid and offer, and order book depth of spot pairs,
// encoded with Simple Binary Encoding (SBE).
//
// The stream is public: connecting needs no API key. Dial connects,
// subscribes and returns a Client whose Events channel delivers every
// stream's messages in order, with prices and quantities as exact decimal
// values scaled by the pair's catalog scales. The client reconnects on its own
// and resubscribes; each stream recovers as the protocol defines:
//
//   - trades: every trade arrives once or is reported in a TradesGap, which
//     the client can backfill from the trade history API (Config.History);
//   - bbo and depth20: each message is the full state, so a newer one
//     replaces the older and the client keeps the newest;
//   - depth: a DepthSnapshot opens the book and DepthUpdate messages apply
//     only to a book at exactly their base. A book that cannot follow the
//     stream is rebuilt from a fresh snapshot, never patched across a gap.
//
// Decoders are generated from the published schema with Real Logic's
// sbe-tool. Messages of templates the decoders do not know are skipped. A
// message of a newer schema version is read by its declared block lengths,
// as the SBE versioning rules allow; a message whose layout this client
// cannot read, or of another schema, is refused, never misread.
package marketstream

import (
	"fmt"
	"time"

	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

// Path is the market-data stream's path on the API's WebSocket host.
const Path = "/ws/market.sbe"

// Stream is a market-data stream of one trading pair.
type Stream uint8

const (
	// StreamTrades is every trade of the pair in real time, with a trade
	// frontier and explicit gaps.
	StreamTrades Stream = 1
	// StreamBBO is the best bid and offer in real time; a subscriber that
	// falls behind receives only the newest.
	StreamBBO Stream = 2
	// StreamDepth20 is the best 20 price levels per side as full replacement
	// snapshots.
	StreamDepth20 Stream = 3
	// StreamDepth is the live depth window: a snapshot, then exact-base
	// updates.
	StreamDepth Stream = 4
)

func (s Stream) String() string {
	switch s {
	case StreamTrades:
		return "trades"
	case StreamBBO:
		return "bbo"
	case StreamDepth20:
		return "depth20"
	case StreamDepth:
		return "depth"
	default:
		return fmt.Sprintf("Stream(%d)", uint8(s))
	}
}

func (s Stream) known() bool { return s >= StreamTrades && s <= StreamDepth }

// Topic is one stream of one trading pair, by the pair's numeric ID from the
// spot configuration.
type Topic struct {
	SymbolID uint32
	Stream   Stream
}

func (t Topic) String() string { return fmt.Sprintf("%d/%s", t.SymbolID, t.Stream) }

// Side is the side of the order that took liquidity in a trade.
type Side uint8

const (
	// SideUnknown is a side the server did not set.
	SideUnknown Side = 0
	SideBuy     Side = 1
	SideSell    Side = 2
)

func (s Side) String() string {
	switch s {
	case SideBuy:
		return "buy"
	case SideSell:
		return "sell"
	case SideUnknown:
		return "unknown"
	default:
		return fmt.Sprintf("Side(%d)", uint8(s))
	}
}

// FeedState says whether the server holds a pair's current market data.
type FeedState uint8

const (
	// FeedLive: the pair's streams are current.
	FeedLive FeedState = 1
	// FeedStale: the server lost the pair's current state. Best bid and offer
	// and depth pause until it recovers; trade gaps are reported when it
	// does.
	FeedStale FeedState = 2
)

func (s FeedState) String() string {
	switch s {
	case FeedLive:
		return "live"
	case FeedStale:
		return "stale"
	default:
		return fmt.Sprintf("FeedState(%d)", uint8(s))
	}
}

// SubscribeResult is the outcome of one subscription item.
type SubscribeResult uint8

const (
	// SubscribeOK: the stream is subscribed; its messages follow.
	SubscribeOK SubscribeResult = 1
	// SubscribeUnknownSymbol: the pair is not served.
	SubscribeUnknownSymbol SubscribeResult = 2
	// SubscribeLimitExceeded: the connection already holds the most
	// subscriptions it may; the item is not subscribed.
	SubscribeLimitExceeded SubscribeResult = 3
	// SubscribeAlreadySubscribed: the connection already holds the stream.
	SubscribeAlreadySubscribed SubscribeResult = 4
	// SubscribeNotSubscribed answers an Unsubscribe: the connection did not
	// hold the stream.
	SubscribeNotSubscribed SubscribeResult = 5
	// SubscribeSymbolDisabled: the pair is listed but disabled; subscribe
	// again once it is enabled.
	SubscribeSymbolDisabled SubscribeResult = 7
)

func (r SubscribeResult) String() string {
	switch r {
	case SubscribeOK:
		return "ok"
	case SubscribeUnknownSymbol:
		return "unknown_symbol"
	case SubscribeLimitExceeded:
		return "limit_exceeded"
	case SubscribeAlreadySubscribed:
		return "already_subscribed"
	case SubscribeNotSubscribed:
		return "not_subscribed"
	case SubscribeSymbolDisabled:
		return "symbol_disabled"
	default:
		return fmt.Sprintf("SubscribeResult(%d)", uint8(r))
	}
}

// Subscribed reports whether the stream is held on the connection after this
// result.
func (r SubscribeResult) Subscribed() bool {
	return r == SubscribeOK || r == SubscribeAlreadySubscribed
}

// Level is one price level: a price in quote units and the total resting
// quantity in base units.
type Level struct {
	Price models.PriceTicks
	Qty   models.QtyScaled
}

// Trade is one public trade.
type Trade struct {
	// TradeID is the trade's match ID, the ID the trade history API returns.
	// Trade IDs ascend per pair, across book epochs too.
	TradeID   uint64
	Price     models.PriceTicks
	Qty       models.QtyScaled
	TakerSide Side
}

// Event is one message of the stream: *Trades, *TradesGap, *TradesThrough,
// *TradesBackfill, *BestBidOffer, *DepthSnapshot, *DepthUpdate,
// *MarketStatus, *SubscribeResponse, *Pong, *Connected or *Disconnected.
type Event interface {
	event()
}

// Trades are trades of one book update, in ascending trade ID order. Each
// trade is delivered at most once per Client, across reconnects.
type Trades struct {
	SymbolID uint32
	// Symbol is the pair's symbol from the catalog.
	Symbol    string
	BookEpoch uint64
	// UpdateID is the book update that executed the trades.
	UpdateID     uint64
	TransactTime time.Time
	SendTime     time.Time
	Trades       []Trade
}

// GapCause says how a trades gap became known.
type GapCause uint8

const (
	// GapSkipped: the server named trades this subscription will not
	// receive live.
	GapSkipped GapCause = 1
	// GapDetected: the client found trades neither delivered nor named.
	GapDetected GapCause = 2
	// GapReconnect: trades between the last one accounted for before a
	// reconnect and the new subscription's baseline.
	GapReconnect GapCause = 3
)

func (c GapCause) String() string {
	switch c {
	case GapSkipped:
		return "skipped"
	case GapDetected:
		return "detected"
	case GapReconnect:
		return "reconnect"
	default:
		return fmt.Sprintf("GapCause(%d)", uint8(c))
	}
}

// TradesGap names trades of a pair that were not delivered live. Recover
// them from the trade history API by trade ID: Backfill walks the gap, and
// Config.History makes the client do so on its own.
type TradesGap struct {
	SymbolID uint32
	// AfterTradeID is the last trade ID accounted for before the gap: the
	// gap starts after it.
	AfterTradeID uint64
	// ThroughTradeID is the last missing trade ID, inclusive. It is 0 when
	// the gap is the rest of TailEpoch.
	ThroughTradeID uint64
	// TailEpoch, when not 0, is a book epoch whose every trade after
	// AfterTradeID is missing; the trade history API reports the epoch's
	// last trade once the epoch has ended.
	TailEpoch uint64
	Cause     GapCause
}

// TradesThrough is the trade frontier: every trade of the pair up to
// LastTradeID has been delivered or reported in a TradesGap. It arrives on
// subscribe and then every second.
type TradesThrough struct {
	SymbolID    uint32
	BookEpoch   uint64
	LastTradeID uint64
	SendTime    time.Time
}

// TradesBackfill is the outcome of recovering one TradesGap from the trade
// history API, when Config.History is set. Trades are the gap's trades in
// ascending ID order; Err is set when the recovery failed, for example
// because the history API broke its contract.
type TradesBackfill struct {
	Gap    TradesGap
	Trades []Trade
	Err    error
}

// BestBidOffer is a pair's best bid and offer. A side without orders is nil.
// Only states newer than the last one delivered are delivered.
type BestBidOffer struct {
	SymbolID     uint32
	Symbol       string
	BookEpoch    uint64
	UpdateID     uint64
	TransactTime time.Time
	SendTime     time.Time
	Bid          *Level
	Ask          *Level
	// Stale is set on the copy Client.BestBidOffer returns while the quote
	// may not be current: the connection that fed it was lost, or the pair's
	// feed is stale. Events always carry false.
	Stale bool
}

// DepthSnapshot replaces a pair's book of the depth20 or depth stream. Bids
// are best (highest) first, asks best (lowest) first.
type DepthSnapshot struct {
	SymbolID  uint32
	Symbol    string
	Stream    Stream
	BookEpoch uint64
	UpdateID  uint64
	// LevelsPerSide is the most levels per side the stream holds: 20 for
	// depth20; for depth, the live window each side is kept to.
	LevelsPerSide int
	TransactTime  time.Time
	SendTime      time.Time
	Bids          []Level
	Asks          []Level
}

// DepthUpdate is a change to a pair's depth book, already applied to the
// client's book (Client.OrderBook): each level is set, and a level with
// quantity 0 is deleted.
type DepthUpdate struct {
	SymbolID      uint32
	Symbol        string
	BookEpoch     uint64
	FirstUpdateID uint64
	LastUpdateID  uint64
	TransactTime  time.Time
	SendTime      time.Time
	Bids          []Level
	Asks          []Level
}

// MarketStatus is a pair's feed state; it arrives when it changes, and when a
// subscription starts while the pair is stale.
type MarketStatus struct {
	SymbolID uint32
	Feed     FeedState
	SendTime time.Time
}

// SubscribeResponse answers one Subscribe: one result per topic, in request
// order.
type SubscribeResponse struct {
	RequestID uint64
	Results   []TopicResult
}

// TopicResult is one topic's subscription outcome.
type TopicResult struct {
	Topic  Topic
	Result SubscribeResult
}

// Pong answers the client's heartbeat ping.
type Pong struct {
	RequestID uint64
	// RoundTrip is the time from sending the ping to reading the pong.
	RoundTrip time.Duration
	SendTime  time.Time
}

// Connected reports a connection whose subscriptions are re-established.
// Reconnect is false for the first connection.
type Connected struct {
	Reconnect bool
}

// Disconnected reports a lost connection; the client reconnects after Delay.
type Disconnected struct {
	Err   error
	Delay time.Duration
}

func (*Trades) event()            {}
func (*TradesGap) event()         {}
func (*TradesThrough) event()     {}
func (*TradesBackfill) event()    {}
func (*BestBidOffer) event()      {}
func (*DepthSnapshot) event()     {}
func (*DepthUpdate) event()       {}
func (*MarketStatus) event()      {}
func (*SubscribeResponse) event() {}
func (*Pong) event()              {}
func (*Connected) event()         {}
func (*Disconnected) event()      {}
