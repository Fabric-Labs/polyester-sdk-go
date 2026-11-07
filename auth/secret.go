package auth

import (
	"fmt"
	"io"

	"github.com/Fabric-Labs/polyester-sdk-go/errors"
)

const redactedSecret = "[REDACTED]"

// Secret holds a secret string, such as the hex API private key in
// polyester.Config. Printing and encoding it never reveal the value: fmt
// prints [REDACTED] for every verb, encoding/json, encoding/xml and other
// encoding.TextMarshaler users write "[REDACTED]", and gob refuses it. The
// value sits behind a pointer, so fmt prints only an address when a Secret is
// reached by reflection, for example through an unexported struct field.
//
// The zero Secret is unset. Decoding JSON, XML or text into a Secret sets it,
// so configuration files keep loading. Call Reveal to read the value.
type Secret struct {
	value *string
}

// NewSecret wraps value. An empty value gives the unset Secret.
func NewSecret(value string) Secret {
	if value == "" {
		return Secret{}
	}
	return Secret{value: &value}
}

// Reveal returns the secret value, or "" when unset.
func (s Secret) Reveal() string {
	if s.value == nil {
		return ""
	}
	return *s.value
}

// IsSet reports whether the Secret holds a value.
func (s Secret) IsSet() bool {
	return s.value != nil
}

// String returns [REDACTED], or "" when unset.
func (s Secret) String() string {
	if s.value == nil {
		return ""
	}
	return redactedSecret
}

// GoString redacts the value for %#v formatting.
func (s Secret) GoString() string {
	return s.String()
}

// Format redacts the value for every fmt verb.
func (s Secret) Format(f fmt.State, verb rune) {
	if verb == 'v' && f.Flag('#') {
		_, _ = io.WriteString(f, s.GoString())
		return
	}
	_, _ = fmt.Fprintf(f, fmt.FormatString(f, verb), s.String())
}

// MarshalText returns [REDACTED], or nothing when unset. encoding/json and
// encoding/xml use it.
func (s Secret) MarshalText() ([]byte, error) {
	return []byte(s.String()), nil
}

// UnmarshalText sets the Secret to text.
func (s *Secret) UnmarshalText(text []byte) error {
	*s = NewSecret(string(text))
	return nil
}

// GobEncode refuses to encode a Secret, because gob would carry the value.
func (Secret) GobEncode() ([]byte, error) {
	return nil, &errors.AuthError{Msg: "auth.Secret cannot be gob-encoded"}
}
