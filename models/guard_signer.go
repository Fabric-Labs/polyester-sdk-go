package models

// GuardSignerStatus is guard signer wallet status.
type GuardSignerStatus struct {
	SignerWallet string `json:"signer_wallet,omitempty"`
	Status       string `json:"status,omitempty"`
	// OnchainSignerAddress is the signer registered on chain; empty when
	// none is. A Status of uninitialized with a different address here means
	// another signer is registered.
	OnchainSignerAddress string `json:"onchain_signer_address,omitempty"`
	// Nonce is the current on-chain nonce, a decimal string, in NonceSpace.
	Nonce      string `json:"nonce,omitempty"`
	NonceSpace uint64 `json:"nonce_space,omitempty"`
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
//
// EncryptedPrivateKey holds the guard signer wallet's private key exactly as
// the API returns it. It is a Secret, so printing, logging or encoding the
// result never reveals the key; call EncryptedPrivateKey.Reveal to read it.
type ExportGuardSignerWalletResult struct {
	EncryptedPrivateKey Secret `json:"encrypted_private_key,omitempty"`
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
