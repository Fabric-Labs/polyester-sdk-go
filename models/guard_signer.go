package models

// GuardSignerStatus is guard signer wallet status.
type GuardSignerStatus struct {
	SignerWallet string `json:"signer_wallet,omitempty"`
	Status       string `json:"status,omitempty"`
}

// GuardApproval is a signed protected action approval.
type GuardApproval struct {
	ApprovalID string `json:"approval_id,omitempty"`
	Signature  string `json:"signature,omitempty"`
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
type RotateGuardSignerWalletResult struct {
	SignerWallet string `json:"signer_wallet,omitempty"`
}

// BatchSignProtectedActionsResult summarizes batch sign.
type BatchSignProtectedActionsResult struct {
	Approvals []GuardApproval `json:"approvals,omitempty"`
}
