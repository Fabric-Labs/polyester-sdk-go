package auth

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/gob"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"log/slog"
	"strings"
	"testing"
)

// syntheticSeed is an obviously fake Ed25519 seed (0xa0, 0xa1, ...). Its
// distinct bytes make every rendering of the key recognisable.
func syntheticSeed() []byte {
	seed := make([]byte, ed25519.SeedSize)
	for i := range seed {
		seed[i] = byte(0xa0 + i)
	}
	return seed
}

func syntheticCredentials(t *testing.T) Credentials {
	t.Helper()
	creds, err := LoadCredentials("key_redaction", hex.EncodeToString(syntheticSeed()), false)
	if err != nil {
		t.Fatal(err)
	}
	return *creds
}

// assertNoKeyMaterial fails when out contains the synthetic key in any
// encoding a formatter or encoder produces: raw bytes, hex, base64, decimal
// byte lists, or quoted escapes.
func assertNoKeyMaterial(t *testing.T, path string, out []byte) {
	t.Helper()
	seed := syntheticSeed()
	private := ed25519.NewKeyFromSeed(seed)
	lower := strings.ToLower(string(out))
	markers := map[string]string{
		"hex":            hex.EncodeToString(seed[:8]),
		"base64 key":     base64.StdEncoding.EncodeToString(private[:12]),
		"base64 seed":    base64.StdEncoding.EncodeToString(seed[:12]),
		"decimal bytes":  "160 161 162 163",
		"quoted escapes": `\xa0\xa1\xa2\xa3`,
	}
	for name, marker := range markers {
		if strings.Contains(string(out), marker) || strings.Contains(lower, strings.ToLower(marker)) {
			t.Errorf("%s leaked private key (%s)", path, name)
			return
		}
	}
	if bytes.Contains(out, seed[:8]) {
		t.Errorf("%s leaked raw private key bytes", path)
	}
}

func TestCredentialsFormattingRedactsPrivateKeyForValueAndPointer(t *testing.T) {
	creds := syntheticCredentials(t)
	type exported struct {
		Creds Credentials
	}
	type exportedPointer struct {
		Creds *Credentials
	}
	subjects := map[string]any{
		"value":                  creds,
		"pointer":                &creds,
		"nested value":           exported{Creds: creds},
		"nested value pointer":   &exported{Creds: creds},
		"nested pointer":         exportedPointer{Creds: &creds},
		"slice of values":        []Credentials{creds},
		"map of values":          map[string]Credentials{"k": creds},
		"interface holding copy": any(creds),
	}
	for name, subject := range subjects {
		for _, verb := range []string{"%v", "%+v", "%#v", "%s", "%q", "%x", "%X", "%d"} {
			rendered := fmt.Sprintf(verb, subject)
			assertNoKeyMaterial(t, name+" "+verb, []byte(rendered))
		}
	}
	if rendered := fmt.Sprintf("%v", creds); !strings.Contains(rendered, "key_redaction") || !strings.Contains(rendered, "[REDACTED]") {
		t.Error("value formatting should keep the key ID and mark the redaction")
	}
}

func TestCredentialsEncodersRedactPrivateKey(t *testing.T) {
	creds := syntheticCredentials(t)
	type nested struct {
		Creds Credentials `json:"creds" xml:"creds"`
	}
	for name, subject := range map[string]any{
		"value":   creds,
		"pointer": &creds,
		"nested":  nested{Creds: creds},
	} {
		encoded, err := json.Marshal(subject)
		if err != nil {
			t.Fatalf("json %s: %v", name, err)
		}
		assertNoKeyMaterial(t, "json "+name, encoded)
		if !bytes.Contains(encoded, []byte("key_redaction")) || !bytes.Contains(encoded, []byte("[REDACTED]")) {
			t.Errorf("json %s should keep the key ID and mark the redaction", name)
		}

		encoded, err = xml.Marshal(subject)
		if err != nil {
			t.Fatalf("xml %s: %v", name, err)
		}
		assertNoKeyMaterial(t, "xml "+name, encoded)
	}
}

func TestCredentialsStructuredLoggingRedactsPrivateKey(t *testing.T) {
	creds := syntheticCredentials(t)
	for _, handler := range []string{"json", "text"} {
		for name, subject := range map[string]any{"value": creds, "pointer": &creds} {
			var out bytes.Buffer
			var h slog.Handler = slog.NewJSONHandler(&out, nil)
			if handler == "text" {
				h = slog.NewTextHandler(&out, nil)
			}
			slog.New(h).Info("configured", "credentials", subject)
			assertNoKeyMaterial(t, "slog "+handler+" "+name, out.Bytes())
			if !strings.Contains(out.String(), "key_redaction") || !strings.Contains(out.String(), "[REDACTED]") {
				t.Errorf("slog %s %s should keep the key ID and mark the redaction", handler, name)
			}
		}
	}
}

func TestCredentialsGobEncodingIsRefused(t *testing.T) {
	creds := syntheticCredentials(t)
	for name, subject := range map[string]any{"value": creds, "pointer": &creds} {
		var out bytes.Buffer
		if err := gob.NewEncoder(&out).Encode(subject); err == nil {
			t.Fatalf("gob %s: encoding credentials must fail", name)
		} else {
			assertNoKeyMaterial(t, "gob error "+name, []byte(err.Error()))
		}
		assertNoKeyMaterial(t, "gob "+name, out.Bytes())
	}
}

// fmt cannot call methods on unexported struct fields, so Credentials must
// hold the key where reflection-based printing cannot reach it.
func TestCredentialsInUnexportedFieldDoesNotLeakPrivateKey(t *testing.T) {
	creds := syntheticCredentials(t)
	type holder struct {
		name  string
		creds Credentials
	}
	type deeper struct {
		inner holder
		list  []Credentials
		byKey map[string]Credentials
	}
	subjects := map[string]any{
		"unexported value":  holder{name: "app", creds: creds},
		"unexported deeper": deeper{inner: holder{creds: creds}, list: []Credentials{creds}, byKey: map[string]Credentials{"k": creds}},
		"pointer to holder": &holder{creds: creds},
	}
	for name, subject := range subjects {
		for _, verb := range []string{"%v", "%+v", "%#v", "%s", "%q", "%x", "%X", "%d"} {
			assertNoKeyMaterial(t, name+" "+verb, []byte(fmt.Sprintf(verb, subject)))
		}
	}
}

func TestNewCredentialsValidatesAndCopiesTheKey(t *testing.T) {
	private := ed25519.NewKeyFromSeed(syntheticSeed())
	creds, err := NewCredentials(" key_redaction ", private)
	if err != nil {
		t.Fatal(err)
	}
	if creds.KeyID != "key_redaction" {
		t.Errorf("KeyID = %q", creds.KeyID)
	}
	private[0] ^= 0xff
	if creds.PrivateKey()[0] == private[0] {
		t.Error("NewCredentials must copy the key")
	}
	returned := creds.PrivateKey()
	returned[1] ^= 0xff
	if creds.PrivateKey()[1] == returned[1] {
		t.Error("PrivateKey must return a copy")
	}
	if _, err := NewCredentials("", private); err == nil {
		t.Error("an empty key ID must be refused")
	}
	if _, err := NewCredentials("key_redaction", private[:32]); err == nil {
		t.Error("a key that is not 64 bytes must be refused")
	}
	var empty Credentials
	if empty.PrivateKey() != nil {
		t.Error("zero Credentials have no key")
	}
	if _, err := SignRequest(&empty, "GET", "https://api.example.test/x", nil, "1"); err == nil {
		t.Error("signing without a key must fail instead of panicking")
	}
}
