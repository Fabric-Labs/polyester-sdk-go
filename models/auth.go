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
// The private key is held in Secret fields, so printing, logging or encoding the
// pair never reveals it: JSON writes "[REDACTED]" for secret_key_hex and gob
// refuses the pair. Read it explicitly with SecretKeyHex.Reveal (64 hex
// characters, the form polyester.Config.APIPrivateKey takes) or
// []byte(SecretKey.Reveal()) (the 32-byte seed).
type Ed25519Keypair struct {
	PublicKeyHex string `json:"public_key_hex"`
	SecretKeyHex Secret `json:"secret_key_hex"`
	PublicKey    []byte `json:"-"`
	SecretKey    Secret `json:"-"`
}

// String redacts secret material for %v / %+v formatting.
func (k Ed25519Keypair) String() string {
	return fmt.Sprintf(
		"Ed25519Keypair{PublicKeyHex:%q SecretKeyHex:%s PublicKey:%v SecretKey:%s}",
		k.PublicKeyHex,
		k.SecretKeyHex,
		k.PublicKey,
		k.SecretKey,
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
