package auth

import (
	"crypto/ed25519"
	"encoding/hex"
	"os"
	"strings"

	"github.com/Fabric-Labs/polyester-sdk-go/errors"
)

const (
	APIKeyIDEnv      = "POLYESTER_API_KEY_ID"
	APIPrivateKeyEnv = "POLYESTER_API_PRIVATE_KEY"
	AccountIDEnv     = "POLYESTER_ACCOUNT_ID"
)

// Credentials holds API-key authentication material.
type Credentials struct {
	KeyID      string
	PrivateKey ed25519.PrivateKey
}

// LoadCredentials loads credentials from explicit values and optionally the environment.
func LoadCredentials(apiKeyID, apiPrivateKey string, fromEnv bool) (*Credentials, error) {
	keyID := strings.TrimSpace(apiKeyID)
	private := strings.TrimSpace(apiPrivateKey)
	if fromEnv {
		if keyID == "" {
			keyID = strings.TrimSpace(os.Getenv(APIKeyIDEnv))
		}
		if private == "" {
			private = strings.TrimSpace(os.Getenv(APIPrivateKeyEnv))
		}
	}
	if keyID == "" && private == "" {
		return nil, nil
	}
	if keyID == "" || private == "" {
		msg := "Both api_key_id and api_private_key are required"
		if fromEnv {
			msg = "Both POLYESTER_API_KEY_ID and POLYESTER_API_PRIVATE_KEY are required"
		}
		return nil, &errors.AuthError{Msg: msg}
	}
	seed, err := NormalizePrivateKey(private)
	if err != nil {
		return nil, err
	}
	return &Credentials{KeyID: keyID, PrivateKey: ed25519.NewKeyFromSeed(seed)}, nil
}

// NormalizePrivateKey accepts a 64-char hex Ed25519 seed.
func NormalizePrivateKey(value string) ([]byte, error) {
	private, err := hex.DecodeString(strings.TrimSpace(value))
	if err != nil {
		return nil, &errors.AuthError{Msg: "API private key must be a valid hex string or raw bytes"}
	}
	if len(private) != ed25519.SeedSize {
		return nil, &errors.AuthError{Msg: "Ed25519 API private key must be exactly 32 bytes"}
	}
	return private, nil
}

// AccountIDFromEnv returns POLYESTER_ACCOUNT_ID when set.
func AccountIDFromEnv() string {
	return strings.TrimSpace(os.Getenv(AccountIDEnv))
}
