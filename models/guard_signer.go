package models

// GuardSignerStatus is guard signer wallet status.
type GuardSignerStatus struct {
	SignerWallet string `json:"signer_wallet,omitempty"`
	Status       string `json:"status,omitempty"`
}

// GuardApproval is a signed protected action approval.
//
// NonceSpace and DeadlineUnix are the exact signed approval fields; submit
// them unchanged with Signature. DeadlineUnix is a unix time in seconds.
type GuardApproval struct {
	ApprovalID   string `json:"approval_id,omitempty"`
	Signature    string `json:"signature,omitempty"`
	NonceSpace   uint64 `json:"nonce_space,omitempty"`
	DeadlineUnix uint64 `json:"deadline_unix,omitempty"`
}

// CreateGuardSignerWalletResult is create wallet outcome.
type CreateGuardSignerWalletResult struct {
	SignerWallet string `json:"signer_wallet,omitempty"`
}

// ExportGuardSignerWalletResult is export wallet outcome.
type ExportGuardSignerWalletResult struct {
	EncryptedPrivateKey string `json:"encrypted_private_key,omitempty"`
}

// RotateGuardSignerWalletResult is rotate wallet outcome.
//
// Approval is the signed approval needed to submit the rotation; it is nil
// when the API returns none.
type RotateGuardSignerWalletResult struct {
	SignerWallet string         `json:"signer_wallet,omitempty"`
	Approval     *GuardApproval `json:"approval,omitempty"`
}

// BatchSignProtectedActionsResult summarizes batch sign.
type BatchSignProtectedActionsResult struct {
	Approvals []GuardApproval `json:"approvals,omitempty"`
}
