package polyester

import (
	"bytes"
	"encoding/gob"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/Fabric-Labs/polyester-sdk-go/chain"
)

// syntheticPrivateKeyHex is an obviously fake Ed25519 seed (0xa0, 0xa1, ...).
const syntheticPrivateKeyHex = "a0a1a2a3a4a5a6a7a8a9aaabacadaeafb0b1b2b3b4b5b6b7b8b9babbbcbdbebf"

const syntheticURLSecret = "url-userinfo-secret"

func syntheticConfig() Config {
	env := chain.PolyesterDevnetEnvironment
	account := "acct_redaction"
	return Config{
		Environment:      &env,
		APIKeyID:         "key_redaction",
		APIPrivateKey:    syntheticPrivateKeyHex,
		APIURL:           "https://user:" + syntheticURLSecret + "@api.example.test",
		WSURL:            "wss://ws.example.test",
		DefaultAccountID: &account,
		Timeout:          3 * time.Second,
		WireFormat:       "json",
	}
}

// assertNoConfigSecrets fails when out contains the synthetic key, in plain or
// hex-of-text form, or the URL userinfo secret.
func assertNoConfigSecrets(t *testing.T, path string, out []byte) {
	t.Helper()
	lower := strings.ToLower(string(out))
	for name, marker := range map[string]string{
		"key":             syntheticPrivateKeyHex[:16],
		"key as hex text": hex.EncodeToString([]byte(syntheticPrivateKeyHex[:8])),
		"url userinfo":    syntheticURLSecret,
	} {
		if strings.Contains(lower, strings.ToLower(marker)) {
			t.Errorf("%s leaked %s", path, name)
		}
	}
}

func TestConfigFormattingRedactsPrivateKeyForValueAndPointer(t *testing.T) {
	cfg := syntheticConfig()
	type exported struct {
		Config Config
	}
	type exportedPointer struct {
		Config *Config
	}
	subjects := map[string]any{
		"value":                cfg,
		"pointer":              &cfg,
		"nested value":         exported{Config: cfg},
		"nested value pointer": &exported{Config: cfg},
		"nested pointer":       exportedPointer{Config: &cfg},
		"slice of values":      []Config{cfg},
		"map of values":        map[string]Config{"k": cfg},
	}
	for name, subject := range subjects {
		for _, verb := range []string{"%v", "%+v", "%#v", "%s", "%q", "%x", "%X", "%d", "%t"} {
			rendered := fmt.Sprintf(verb, subject)
			assertNoConfigSecrets(t, name+" "+verb, []byte(rendered))
		}
	}
	if rendered := fmt.Sprintf("%v", cfg); !strings.Contains(rendered, "key_redaction") || !strings.Contains(rendered, "[REDACTED]") {
		t.Error("value formatting should keep the key ID and mark the redaction")
	}
}

func TestConfigEncodersRedactPrivateKey(t *testing.T) {
	cfg := syntheticConfig()
	type nested struct {
		Config Config `json:"config" xml:"config"`
	}
	for name, subject := range map[string]any{
		"value":   cfg,
		"pointer": &cfg,
		"nested":  nested{Config: cfg},
	} {
		encoded, err := json.Marshal(subject)
		if err != nil {
			t.Fatalf("json %s: %v", name, err)
		}
		assertNoConfigSecrets(t, "json "+name, encoded)
		if !bytes.Contains(encoded, []byte(`"APIKeyID":"key_redaction"`)) ||
			!bytes.Contains(encoded, []byte(`"APIPrivateKey":"[REDACTED]"`)) ||
			!bytes.Contains(encoded, []byte(`"DefaultAccountID":"acct_redaction"`)) {
			t.Errorf("json %s should keep the non-secret fields and mark the redaction", name)
		}

		encoded, err = xml.Marshal(subject)
		if err != nil {
			t.Fatalf("xml %s: %v", name, err)
		}
		assertNoConfigSecrets(t, "xml "+name, encoded)
	}

	// An unset key stays empty rather than claiming a redacted value.
	encoded, err := json.Marshal(Config{APIKeyID: "key_redaction"})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(encoded, []byte(`"APIPrivateKey":""`)) {
		t.Fatalf("unset key should encode as empty: %s", encoded)
	}
}

func TestConfigJSONDecodingStillReadsPrivateKey(t *testing.T) {
	var cfg Config
	if err := json.Unmarshal([]byte(`{"APIKeyID":"key_redaction","APIPrivateKey":"`+syntheticPrivateKeyHex+`"}`), &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.APIPrivateKey != syntheticPrivateKeyHex {
		t.Fatal("decoding a JSON config file must keep reading APIPrivateKey")
	}
}

func TestConfigStructuredLoggingRedactsPrivateKey(t *testing.T) {
	cfg := syntheticConfig()
	for _, handler := range []string{"json", "text"} {
		for name, subject := range map[string]any{"value": cfg, "pointer": &cfg} {
			var out bytes.Buffer
			var h slog.Handler = slog.NewJSONHandler(&out, nil)
			if handler == "text" {
				h = slog.NewTextHandler(&out, nil)
			}
			slog.New(h).Info("configured", "config", subject)
			assertNoConfigSecrets(t, "slog "+handler+" "+name, out.Bytes())
			if !strings.Contains(out.String(), "key_redaction") || !strings.Contains(out.String(), "[REDACTED]") {
				t.Errorf("slog %s %s should keep the key ID and mark the redaction", handler, name)
			}
		}
	}
}

func TestConfigGobEncodingIsRefused(t *testing.T) {
	cfg := syntheticConfig()
	for name, subject := range map[string]any{"value": cfg, "pointer": &cfg} {
		var out bytes.Buffer
		if err := gob.NewEncoder(&out).Encode(subject); err == nil {
			t.Fatalf("gob %s: encoding a config with a private key must fail", name)
		} else {
			assertNoConfigSecrets(t, "gob error "+name, []byte(err.Error()))
		}
		assertNoConfigSecrets(t, "gob "+name, out.Bytes())
	}
}
