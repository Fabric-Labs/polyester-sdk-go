package models

// InternalTransferBatchLeg is one transfer of an InternalTransfers.CreateBatch
// request: the fields of InternalTransfers.Create without the idempotency
// key, which the batch carries for all of its legs.
type InternalTransferBatchLeg struct {
	// AssetID is the asset to move (required).
	AssetID uint32
	// Quantity is the amount, converted exactly as for InternalTransfers.Create:
	// a decimal string in whole units of the asset, or an exact scaled value.
	// It is never a float.
	Quantity AssetAmountInput
	// QuantityScale is the decimal scale of an integer Quantity, as for
	// InternalTransfers.Create; nil reads it from the loaded catalogs.
	QuantityScale *int
	// Account and SubAccountID select the leg's source account as for
	// InternalTransfers.Create. Legs of one batch may debit different source
	// accounts.
	Account      any
	SubAccountID *string
	// Exactly one destination: a root account, a sub-account (public IDs) or
	// a smart account address.
	DestinationAccountID           *string
	DestinationSubaccountID        *string
	DestinationSmartAccountAddress *string
}

// InternalTransferBatchResult is the answer to InternalTransfers.CreateBatch.
//
// Status "refused" means at least one leg failed its checks, so no leg ran,
// no funds moved and no step-up was used: the failed legs are "refused" with
// their ErrorCode, the others "not_submitted". Status "submitted" means every
// leg passed its checks and the legs ran in order, independently: they are
// not atomic, so a leg can be "refused" after earlier legs were "accepted".
type InternalTransferBatchResult struct {
	// RequestID is the batch's stable server-owned identifier; empty when the
	// batch was refused.
	RequestID string `json:"request_id,omitempty"`
	// Status is "submitted" or "refused".
	Status string `json:"status"`
	// Legs holds one result per requested leg, in request order.
	Legs []InternalTransferBatchLegResult `json:"legs"`
}

// InternalTransferBatchLegResult is the outcome of one leg of a batch.
//
// Status is "accepted" (the transfer has a durable TransferID, not that it
// has settled), "pending" (the outcome is not known yet: the transfer runs to
// its outcome without the caller; retry the batch with the same idempotency
// key and the same legs to read it, which runs no accepted leg twice),
// "refused" (the leg moved no funds; ErrorCode says why) or "not_submitted"
// (the leg did not run: its batch was refused, or the request ended before
// the leg started; a retry with the same key runs it).
type InternalTransferBatchLegResult struct {
	LegIndex uint32 `json:"leg_index"`
	Status   string `json:"status"`
	// RequestID is the leg's transfer request; empty while the leg has not
	// been submitted.
	RequestID string `json:"request_id,omitempty"`
	// TransferID and FlowID are set once the leg is accepted. FlowID is the
	// transfer's lifecycle flow, for Lifecycle.GetAccountFlow.
	TransferID string `json:"transfer_id,omitempty"`
	FlowID     string `json:"flow_id,omitempty"`
	// AcceptedTsNs is when the leg was accepted, in nanoseconds since the
	// Unix epoch; 0 before.
	AcceptedTsNs uint64 `json:"accepted_ts_ns,omitempty"`
	AssetID      uint32 `json:"asset_id,omitempty"`
	// AssetCode is set once the leg is submitted.
	AssetCode string `json:"asset_code,omitempty"`
	// Quantity echoes the leg's amount at the 18-decimal ledger scale.
	Quantity AssetAmountScaled `json:"quantity"`
	// ErrorCode is why the leg was refused, or why its outcome is not known
	// yet, as a lowercase label without the ERROR_CODE_ prefix, such as
	// "insufficient_funds"; empty for an accepted or not-submitted leg, and
	// UNKNOWN_ERROR_CODE(n) for a code newer than this SDK.
	ErrorCode    string `json:"error_code,omitempty"`
	ErrorMessage string `json:"error_message,omitempty"`
}
