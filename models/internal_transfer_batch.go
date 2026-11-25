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

// InternalTransferBatchResult is the answer to InternalTransfers.CreateBatch,
// InternalTransfers.GetBatch and InternalTransfers.AbandonBatch. A batch moves
// all of its legs or none.
//
// Status "accepted" means every leg is "credited". Status "processing" means
// the outcome is not known yet: the batch completes on its own and ends
// accepted or refused, every leg is "processing", and GetBatch with its
// RequestID (or CreateBatch again with the same idempotency key and legs)
// reads the outcome. Status "refused" means nothing moved and nothing will
// under this idempotency key: every leg is "not_moved", and the legs that
// caused the refusal carry their ErrorCode. Submit a new batch under a new
// key to try again. Status "abandoned", which only AbandonBatch answers, means
// no batch was ever submitted under the idempotency key and none ever will
// be: nothing moved, RequestID is empty and Legs is empty.
type InternalTransferBatchResult struct {
	// RequestID is the batch's stable server-owned identifier, a decimal
	// string; set on an accepted or processing batch, and empty when the
	// batch was refused before it was accepted for processing or its key was
	// abandoned.
	RequestID string `json:"request_id,omitempty"`
	// Status is "accepted", "processing", "refused" or "abandoned".
	Status string `json:"status"`
	// Legs holds one result per leg, in request order.
	Legs []InternalTransferBatchLegResult `json:"legs"`
}

// InternalTransferBatchLegResult is the state of one leg of a batch: its
// batch's outcome, leg by leg.
//
// Status is "credited" (the leg is credited to its destination; its
// TransferID, FlowID and CreditedTsNs are set), "processing" (the batch's
// outcome is not known yet; TransferID is set) or "not_moved" (the batch was
// refused and the leg moved nothing; ErrorCode is set when this leg caused
// the refusal).
type InternalTransferBatchLegResult struct {
	LegIndex uint32 `json:"leg_index"`
	Status   string `json:"status"`
	// TransferID is the leg's stable transfer identifier, set while the batch
	// is processing and once it is accepted. A batch leg is not readable with
	// InternalTransfers.Get: read the batch with GetBatch.
	TransferID string `json:"transfer_id,omitempty"`
	// FlowID is the leg's lifecycle flow, for Lifecycle.GetAccountFlow by its
	// sender or its recipient; set once the leg is credited.
	FlowID string `json:"flow_id,omitempty"`
	// CreditedTsNs is when the leg was credited, in nanoseconds since the
	// Unix epoch (server time); 0 before.
	CreditedTsNs uint64 `json:"credited_ts_ns,omitempty"`
	AssetID      uint32 `json:"asset_id,omitempty"`
	// AssetCode is set once the leg passed its checks.
	AssetCode string `json:"asset_code,omitempty"`
	// Quantity echoes the leg's amount at the 18-decimal ledger scale.
	Quantity AssetAmountScaled `json:"quantity"`
	// Destination is the destination the server resolved for the leg, set
	// once the leg passed its checks; nil when the reply carries none.
	Destination *ResolvedDestination `json:"destination,omitempty"`
	// ErrorCode is why this leg caused its batch's refusal, as a lowercase
	// label without the ERROR_CODE_ prefix, such as "insufficient_funds" or
	// "service_unavailable" (the batch could not complete: submit it again
	// under a new idempotency key); empty on every other leg, and
	// UNKNOWN_ERROR_CODE(n) for a code newer than this SDK.
	ErrorCode    string `json:"error_code,omitempty"`
	ErrorMessage string `json:"error_message,omitempty"`
}

// ResolvedDestination is the destination the server resolved for a transfer:
// the destination's root account, its sub-account when the transfer targets
// one, and its smart account address. Each is empty when the server does not
// name it.
type ResolvedDestination struct {
	RootAccountPublicID string `json:"root_account_public_id,omitempty"`
	SubaccountPublicID  string `json:"subaccount_public_id,omitempty"`
	SmartAccountAddress string `json:"smart_account_address,omitempty"`
}
