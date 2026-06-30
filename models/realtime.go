package models

// PriceQtyPair is a scaled price/qty tuple from realtime deltas.
type PriceQtyPair struct {
	PriceTicks string
	QtyScaled  string
}

// OrderBookDeltaUpdate is a decoded orderbook delta publication.
type OrderBookDeltaUpdate struct {
	SymbolID     uint32
	BookSeqStart string
	BookSeqEnd   string
	Reset        bool
	Bids         []PriceQtyPair
	Asks         []PriceQtyPair
}
