package models

// Order is a normalized open or historical order.
type Order struct {
	OrderID       string    `json:"order_id"`
	SymbolID      uint32    `json:"symbol_id"`
	ClientOrderID string    `json:"client_order_id,omitempty"`
	Side          string    `json:"side,omitempty"`
	Status        string    `json:"status,omitempty"`
	OrderType     string    `json:"order_type,omitempty"`
	TIF           string    `json:"tif,omitempty"`
	OrigQty       QtyScaled `json:"orig_qty,omitempty"`
	CumQty        QtyScaled `json:"cum_qty,omitempty"`
	LeavesQty     QtyScaled `json:"leaves_qty,omitempty"`
	Price         PriceTicks `json:"price,omitempty"`
	AvgPx         PriceTicks `json:"avg_px,omitempty"`
	CreatedTsNs   string    `json:"created_ts_ns,omitempty"`
}

// OrdersList holds paginated orders.
type OrdersList struct {
	Orders        []Order `json:"orders"`
	NextPageToken string  `json:"next_page_token,omitempty"`
}

// OrderMutationResult is the outcome of a single order mutation.
type OrderMutationResult struct {
	Status        string `json:"status"`
	OrderID       string `json:"order_id,omitempty"`
	ClientOrderID string `json:"client_order_id,omitempty"`
}

// GetOrderResult includes order detail and related fills.
type GetOrderResult struct {
	Order  *Order      `json:"order,omitempty"`
	Trades []UserTrade `json:"trades,omitempty"`
}

// UserTrade is a user fill record.
type UserTrade struct {
	SymbolID  uint32     `json:"symbol_id"`
	MatchID   string     `json:"match_id,omitempty"`
	OrderID   string     `json:"order_id,omitempty"`
	Side      string     `json:"side,omitempty"`
	IsMaker   bool       `json:"is_maker,omitempty"`
	Price     PriceTicks `json:"price,omitempty"`
	Qty       QtyScaled  `json:"qty,omitempty"`
	FeeScaled string     `json:"fee_scaled,omitempty"`
	TsNs      string     `json:"ts_ns,omitempty"`
}

// UserTradesList holds paginated user trades.
type UserTradesList struct {
	Trades        []UserTrade `json:"trades"`
	NextPageToken string      `json:"next_page_token,omitempty"`
}

// ModifyOrderResult is returned from order modify RPCs.
type ModifyOrderResult struct {
	ActionTaken  string `json:"action_taken,omitempty"`
	OldOrderID   string `json:"old_order_id,omitempty"`
	FinalOrderID string `json:"final_order_id,omitempty"`
	Code         string `json:"code,omitempty"`
}

// CancelAllOrdersResult summarizes cancel-all.
type CancelAllOrdersResult struct {
	Status           string `json:"status,omitempty"`
	MatchedOrders    int    `json:"matched_orders,omitempty"`
	SubmittedCancels int    `json:"submitted_cancels,omitempty"`
	FailedCancels    int    `json:"failed_cancels,omitempty"`
}

// BatchModifyResultItem is one batch modify outcome.
type BatchModifyResultItem struct {
	Status        string `json:"status"`
	ClientOrderID string `json:"client_order_id,omitempty"`
	FinalOrderID  string `json:"final_order_id,omitempty"`
	Code          string `json:"code,omitempty"`
}

// BatchModifyOrdersResult summarizes batch modify.
type BatchModifyOrdersResult struct {
	Results       []BatchModifyResultItem `json:"results"`
	AmendedCount  int                     `json:"amended_count,omitempty"`
	ReplacedCount int                     `json:"replaced_count,omitempty"`
	RejectedCount int                     `json:"rejected_count,omitempty"`
}

// BatchCreateResultItem is one batch create outcome.
type BatchCreateResultItem struct {
	Status        string `json:"status"`
	OrderID       string `json:"order_id,omitempty"`
	ClientOrderID string `json:"client_order_id,omitempty"`
	Code          string `json:"code,omitempty"`
}

// BatchCreateOrdersResult summarizes batch create.
type BatchCreateOrdersResult struct {
	Results       []BatchCreateResultItem `json:"results"`
	AcceptedCount int                     `json:"accepted_count,omitempty"`
	RejectedCount int                     `json:"rejected_count,omitempty"`
}

// BatchCancelResultItem is one batch cancel outcome.
type BatchCancelResultItem struct {
	Status        string `json:"status"`
	OrderID       string `json:"order_id,omitempty"`
	ClientOrderID string `json:"client_order_id,omitempty"`
	Code          string `json:"code,omitempty"`
}

// BatchCancelOrdersResult summarizes batch cancel.
type BatchCancelOrdersResult struct {
	Results       []BatchCancelResultItem `json:"results"`
	AcceptedCount int                     `json:"accepted_count,omitempty"`
	RejectedCount int                     `json:"rejected_count,omitempty"`
}

// CancelAllAfterResult is returned from cancel-all-after.
type CancelAllAfterResult struct {
	Status              string `json:"status,omitempty"`
	EffectiveTimeoutSec int    `json:"effective_timeout_sec,omitempty"`
	ExpiresAtTsNs       string `json:"expires_at_ts_ns,omitempty"`
}

// AssetBalance is a ledger balance row.
type AssetBalance struct {
	AssetID   uint32 `json:"asset_id"`
	Trading   string `json:"trading,omitempty"`
	Funding   string `json:"funding,omitempty"`
	Reserved  string `json:"reserved,omitempty"`
	Available string `json:"available,omitempty"`
}

// BalancesList holds balance rows.
type BalancesList struct {
	Balances []AssetBalance `json:"balances"`
}


// BalanceHistorySeries is one balance history series.
type BalanceHistorySeries struct {
	AssetID     uint32  `json:"asset_id"`
	AccountCode uint32  `json:"account_code"`
	BalanceQ    []int64 `json:"balance_q"`
}

// BalanceHistory is balance history response.
type BalanceHistory struct {
	Range      string                 `json:"range"`
	Bucket     string                 `json:"bucket,omitempty"`
	StartTsSec int64                  `json:"start_ts_sec,omitempty"`
	EndTsSec   int64                  `json:"end_ts_sec,omitempty"`
	Points     int                    `json:"points,omitempty"`
	Series     []BalanceHistorySeries `json:"series"`
}

// EquityHistorySeries is one equity history series.
type EquityHistorySeries struct {
	AccountCode uint32  `json:"account_code,omitempty"`
	AccountName string  `json:"account_name,omitempty"`
	AssetID     uint32  `json:"asset_id,omitempty"`
	AssetSymbol string  `json:"asset_symbol,omitempty"`
	EquityQ     []int64 `json:"equity_q"`
}

// EquityHistory is equity history response.
type EquityHistory struct {
	Range      string                `json:"range"`
	Bucket     string                `json:"bucket,omitempty"`
	StartTsSec int64                 `json:"start_ts_sec,omitempty"`
	EndTsSec   int64                 `json:"end_ts_sec,omitempty"`
	QuoteAsset string                `json:"quote_asset,omitempty"`
	Points     int                   `json:"points,omitempty"`
	Series     []EquityHistorySeries `json:"series"`
}

// Hold is an open ledger hold.
type Hold struct {
	HoldID         string `json:"hold_id"`
	AssetID        uint32 `json:"asset_id"`
	AmountReserved string `json:"amount_reserved,omitempty"`
	ExpiresAtNs    string `json:"expires_at_ns,omitempty"`
}

// HoldsList holds open holds.
type HoldsList struct {
	Holds []Hold `json:"holds"`
}

// LedgerTransfer is a ledger transfer row.
type LedgerTransfer struct {
	AssetID      uint32 `json:"asset_id,omitempty"`
	Amount       string `json:"amount,omitempty"`
	TransferType int    `json:"transfer_type,omitempty"`
	AccountCode  int    `json:"account_code,omitempty"`
	Timestamp    int64  `json:"timestamp,omitempty"`
	Pending      bool   `json:"pending,omitempty"`
	TxID         string `json:"tx_id,omitempty"`
	IsDebit      bool   `json:"is_debit,omitempty"`
}

// TransfersList holds ledger transfers.
type TransfersList struct {
	Transfers  []LedgerTransfer `json:"transfers"`
	NextCursor *int64           `json:"next_cursor,omitempty"`
}

// InternalTransferResult is an internal transfer outcome.
type InternalTransferResult struct {
	RequestID  string            `json:"request_id,omitempty"`
	TransferID string            `json:"transfer_id,omitempty"`
	AssetID    uint32            `json:"asset_id,omitempty"`
	AssetCode  string            `json:"asset_code,omitempty"`
	Quantity   AssetAmountScaled `json:"quantity,omitempty"`
}

// DepositAddress is a chain deposit address.
type DepositAddress struct {
	ChainID        uint32 `json:"chain_id,omitempty"`
	DepositAddress string `json:"deposit_address,omitempty"`
}

// DepositAddressesList lists deposit addresses.
type DepositAddressesList struct {
	Addresses []DepositAddress `json:"addresses"`
}

// WithdrawIntentResult is a withdraw intent outcome.
type WithdrawIntentResult struct {
	IntentID string `json:"intent_id,omitempty"`
	Status   string `json:"status,omitempty"`
	FlowID   string `json:"flow_id,omitempty"`
}

// LifecycleFlowSummary is a lifecycle flow header.
type LifecycleFlowSummary struct {
	IntentID            string `json:"intent_id"`
	FlowKind            string `json:"flow_kind,omitempty"`
	LatestStep          string `json:"latest_step,omitempty"`
	IsOpen              bool   `json:"is_open,omitempty"`
	IsTerminal          bool   `json:"is_terminal,omitempty"`
	OwnerAccountID      string `json:"owner_account_id,omitempty"`
	SmartAccountAddress string `json:"smart_account_address,omitempty"`
}

// LifecycleFlowsList lists lifecycle flows.
type LifecycleFlowsList struct {
	Flows         []LifecycleFlowSummary `json:"flows"`
	NextPageToken string                 `json:"next_page_token,omitempty"`
}

// Trigger is a conditional order trigger.
type Trigger struct {
	TriggerID       string     `json:"trigger_id,omitempty"`
	SymbolID        uint32     `json:"symbol_id,omitempty"`
	Symbol          string     `json:"symbol,omitempty"`
	TriggerType     string     `json:"trigger_type,omitempty"`
	Status          string     `json:"status,omitempty"`
	Side            string     `json:"side,omitempty"`
	Qty             QtyScaled  `json:"qty,omitempty"`
	TriggerPrice    PriceTicks `json:"trigger_price,omitempty"`
	ClientTriggerID string     `json:"client_trigger_id,omitempty"`
}

// TriggersList lists triggers.
type TriggersList struct {
	Triggers []Trigger `json:"triggers"`
	Total    int       `json:"total,omitempty"`
}

// TriggerMutationResult is a trigger mutation outcome.
type TriggerMutationResult struct {
	TriggerID string `json:"trigger_id,omitempty"`
	Status    string `json:"status,omitempty"`
}

// TriggerEvent is a trigger lifecycle event.
type TriggerEvent struct {
	TriggerID string `json:"trigger_id,omitempty"`
	EventType string `json:"event_type,omitempty"`
	TsNs      string `json:"ts_ns,omitempty"`
}

// TriggerEventsList lists trigger events.
type TriggerEventsList struct {
	Events []TriggerEvent `json:"events"`
}

// ApiKeySummary summarizes an API key.
type ApiKeySummary struct {
	KeyID            string `json:"key_id,omitempty"`
	Label            string `json:"label,omitempty"`
	Status           string `json:"status,omitempty"`
	PublicKeyEd25519 string `json:"public_key_ed25519,omitempty"`
}

// ApiKeysList lists API keys.
type ApiKeysList struct {
	Keys []ApiKeySummary `json:"keys"`
}

// ResolvedAccount is a resolved account record.
type ResolvedAccount struct {
	AccountID           string `json:"account_id,omitempty"`
	Username            string `json:"username,omitempty"`
	SmartAccountAddress string `json:"smart_account_address,omitempty"`
}

// ResolvedAccountsList lists resolved accounts.
type ResolvedAccountsList struct {
	Accounts []ResolvedAccount `json:"accounts"`
}
