package auth

import (
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/Fabric-Labs/polyester-sdk-go/errors"
)

const (
	APIKeyIDEnv      = "POLYESTER_API_KEY_ID"
	APIPrivateKeyEnv = "POLYESTER_API_PRIVATE_KEY"
	AccountIDEnv     = "POLYESTER_ACCOUNT_ID"
)

const redactedSecret = "[REDACTED]"

// Credentials holds API-key authentication material.
//
// Formatting and encoding Credentials, as a value or a pointer, with fmt,
// encoding/json, encoding/xml or log/slog redact PrivateKey; gob refuses to
// encode them. fmt cannot call methods on unexported struct fields, so keep
// Credentials in an exported field or behind a pointer when printing a struct
// that holds them.
type Credentials struct {
	KeyID      string
	PrivateKey ed25519.PrivateKey
}

// String redacts the private key.
func (c *Credentials) String() string {
	if c == nil {
		return "Credentials<nil>"
	}
	return c.redactedString()
}

// GoString redacts the private key for %#v formatting.
func (c *Credentials) GoString() string {
	return c.String()
}

func (c Credentials) redactedString() string {
	return fmt.Sprintf("Credentials{KeyID:%q PrivateKey:%s}", c.KeyID, redactedSecret)
}

// Format redacts the private key for every fmt verb, for values as well as
// pointers. A nil pointer formats as <nil>.
func (c Credentials) Format(f fmt.State, verb rune) {
	if verb == 'v' && f.Flag('#') {
		_, _ = io.WriteString(f, c.redactedString())
		return
	}
	_, _ = fmt.Fprintf(f, fmt.FormatString(f, verb), c.redactedString())
}

// redactedCredentials is the shape encoders see.
type redactedCredentials struct {
	KeyID      string
	PrivateKey string
}

func (c Credentials) redacted() redactedCredentials {
	private := ""
	if len(c.PrivateKey) > 0 {
		private = redactedSecret
	}
	return redactedCredentials{KeyID: c.KeyID, PrivateKey: private}
}

// MarshalJSON encodes Credentials with the private key redacted. This also
// covers log/slog's JSON handler.
func (c Credentials) MarshalJSON() ([]byte, error) {
	return json.Marshal(c.redacted())
}

// MarshalXML encodes Credentials with the private key redacted.
func (c Credentials) MarshalXML(e *xml.Encoder, start xml.StartElement) error {
	return e.EncodeElement(c.redacted(), start)
}

// GobEncode refuses to encode Credentials, because gob would carry the private
// key.
func (Credentials) GobEncode() ([]byte, error) {
	return nil, &errors.AuthError{Msg: "auth.Credentials holds an API private key and cannot be gob-encoded"}
}

// SignPayload signs exact deterministic protobuf payload bytes.
func (c *Credentials) SignPayload(payload []byte) ([]byte, error) {
	if c == nil || len(c.PrivateKey) != ed25519.PrivateKeySize {
		return nil, &errors.AuthError{Msg: "valid API-key credentials are required to sign payload"}
	}
	return ed25519.Sign(c.PrivateKey, payload), nil
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
