package auth

import (
	"bytes"
	"encoding/gob"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"strings"
	"testing"
)

const syntheticSecretText = "synthetic-secret-a0a1a2a3"

func TestSecretNeverPrintsItsValue(t *testing.T) {
	secret := NewSecret(syntheticSecretText)
	type holder struct {
		secret Secret
	}
	type exported struct {
		Secret Secret
	}
	subjects := map[string]any{
		"value":             secret,
		"pointer":           &secret,
		"exported field":    exported{Secret: secret},
		"unexported field":  holder{secret: secret},
		"pointer to holder": &holder{secret: secret},
		"slice":             []Secret{secret},
		"map":               map[string]Secret{"k": secret},
	}
	for name, subject := range subjects {
		for _, verb := range []string{"%v", "%+v", "%#v", "%s", "%q", "%x", "%X", "%d"} {
			rendered := fmt.Sprintf(verb, subject)
			if strings.Contains(rendered, syntheticSecretText) ||
				strings.Contains(strings.ToLower(rendered), fmt.Sprintf("%x", syntheticSecretText[:8])) {
				t.Errorf("%s %s leaked the secret", name, verb)
			}
		}
	}
	if got := fmt.Sprint(secret); got != "[REDACTED]" {
		t.Errorf("Sprint = %q, want [REDACTED]", got)
	}
	if got := secret.Reveal(); got != syntheticSecretText {
		t.Error("Reveal must return the value")
	}
}

func TestSecretEncodersRedactAndDecodersSet(t *testing.T) {
	type doc struct {
		Key Secret `json:"key" xml:"key"`
	}
	encoded, err := json.Marshal(doc{Key: NewSecret(syntheticSecretText)})
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded) != `{"key":"[REDACTED]"}` {
		t.Errorf("json = %s", encoded)
	}
	encoded, err = xml.Marshal(doc{Key: NewSecret(syntheticSecretText)})
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(encoded, []byte(syntheticSecretText)) || !bytes.Contains(encoded, []byte("[REDACTED]")) {
		t.Error("xml must redact the secret")
	}
	encoded, err = json.Marshal(doc{})
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded) != `{"key":""}` {
		t.Errorf("unset json = %s", encoded)
	}

	var decoded doc
	if err := json.Unmarshal([]byte(`{"key":"`+syntheticSecretText+`"}`), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Key.Reveal() != syntheticSecretText {
		t.Error("json decoding must set the secret")
	}
	decoded = doc{}
	if err := xml.Unmarshal([]byte(`<doc><key>`+syntheticSecretText+`</key></doc>`), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Key.Reveal() != syntheticSecretText {
		t.Error("xml decoding must set the secret")
	}
}

func TestSecretZeroValueAndEmptyAreUnset(t *testing.T) {
	for name, secret := range map[string]Secret{"zero": {}, "empty": NewSecret("")} {
		if secret.IsSet() || secret.Reveal() != "" || secret.String() != "" {
			t.Errorf("%s secret must be unset", name)
		}
	}
	if !NewSecret("x").IsSet() {
		t.Error("non-empty secret must be set")
	}
}

func TestSecretGobEncodingIsRefused(t *testing.T) {
	var out bytes.Buffer
	if err := gob.NewEncoder(&out).Encode(struct{ Key Secret }{NewSecret(syntheticSecretText)}); err == nil {
		t.Fatal("gob must refuse a Secret")
	}
	if bytes.Contains(out.Bytes(), []byte(syntheticSecretText)) {
		t.Fatal("gob output leaked the secret")
	}
}
