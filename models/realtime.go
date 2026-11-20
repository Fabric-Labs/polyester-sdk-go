package models

import "time"

// PriceQtyPair is a scaled price/qty tuple from realtime deltas.
type PriceQtyPair struct {
	PriceTicks int64
	QtyScaled  int64
}

// OrderBookDeltaUpdate is a decoded orderbook delta publication.
type OrderBookDeltaUpdate struct {
	SymbolID     uint32
	BookSeqStart string
	BookSeqEnd   string
	Reset        bool
	Bids         []PriceQtyPair
	Asks         []PriceQtyPair
	// Ts is the server's best-effort timestamp for this delta; nil when the
	// server supplied none, never the local receipt time.
	Ts *time.Time
}
