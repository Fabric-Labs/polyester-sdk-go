package models

import "fmt"

// MeResult is the authenticated session summary.
type MeResult struct {
	AccountID               string         `json:"account_id,omitempty"`
	APIKeyID                string         `json:"api_key_id,omitempty"`
	Username                string         `json:"username,omitempty"`
	RootSmartAccountAddress string         `json:"root_smart_account_address,omitempty"`
	Session                 map[string]any `json:"session,omitempty"`
}

// UserProfile is the user profile record.
type UserProfile struct {
	Username  string `json:"username,omitempty"`
	Bio       string `json:"bio,omitempty"`
	Website   string `json:"website,omitempty"`
	Twitter   string `json:"twitter,omitempty"`
	AvatarURL string `json:"avatar_url,omitempty"`
}

// UsernameHistoryEntry is one username change.
type UsernameHistoryEntry struct {
	Username  string `json:"username,omitempty"`
	ChangedAt int64  `json:"changed_at,omitempty"`
}

// UsernameHistoryList lists username history.
type UsernameHistoryList struct {
	Entries []UsernameHistoryEntry `json:"entries"`
}

// Ed25519Keypair is a locally generated API key pair.
//
// String/GoString redact secret material so accidental logging cannot leak the
// private key. Read SecretKeyHex / SecretKey explicitly when you need the secret.
// JSON still includes secret_key_hex for intentional key-export flows.
type Ed25519Keypair struct {
	PublicKeyHex string `json:"public_key_hex"`
	SecretKeyHex string `json:"secret_key_hex"`
	PublicKey    []byte `json:"-"`
	SecretKey    []byte `json:"-"`
}

// String redacts secret material for %v / %+v formatting.
func (k Ed25519Keypair) String() string {
	return fmt.Sprintf(
		"Ed25519Keypair{PublicKeyHex:%q SecretKeyHex:%q PublicKey:%v SecretKey:%s}",
		k.PublicKeyHex,
		"[REDACTED]",
		k.PublicKey,
		"[REDACTED]",
	)
}

// GoString redacts secret material for %#v formatting.
func (k Ed25519Keypair) GoString() string {
	return k.String()
}

// AccountIdentity is a public identity update payload.
type AccountIdentity struct {
	AccountID               string `json:"account_id,omitempty"`
	Username                string `json:"username,omitempty"`
	AvatarURL               string `json:"avatar_url,omitempty"`
	RootSmartAccountAddress string `json:"root_smart_account_address,omitempty"`
}
