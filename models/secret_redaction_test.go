package models_test

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

	"github.com/Fabric-Labs/polyester-sdk-go/auth"
	"github.com/Fabric-Labs/polyester-sdk-go/codecs/decode"
	guardv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/chain/guard/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

// fakeExportedKey is an obviously fake marker standing in for an exported
// guard signer private key.
const fakeExportedKey = "fake-exported-guard-key-marker"

var secretVerbs = []string{"%v", "%+v", "%#v", "%s", "%q", "%x", "%X", "%d", "%t"}

// secretMarkers are the renderings of secret that must never appear: the
// value, its hex-of-text form, and for byte secrets the raw and decimal forms.
func secretMarkers(secret string) [][]byte {
	markers := [][]byte{
		[]byte(strings.ToLower(secret[:16])),
		[]byte(hex.EncodeToString([]byte(secret[:8]))),
	}
	return markers
}

func assertNoSecret(t *testing.T, path string, out []byte, markers [][]byte) {
	t.Helper()
	lower := bytes.ToLower(out)
	for _, marker := range markers {
		if bytes.Contains(lower, marker) || bytes.Contains(out, marker) {
			t.Errorf("%s leaked secret material: %s", path, out)
			return
		}
	}
}

// renderAll returns every display, diagnostic and serialization rendering of
// subject that an application could write to a log.
func renderAll(t *testing.T, name string, subject any) map[string][]byte {
	t.Helper()
	out := map[string][]byte{}
	for _, verb := range secretVerbs {
		out[name+" "+verb] = []byte(fmt.Sprintf(verb, subject))
	}
	if encoded, err := json.Marshal(subject); err == nil {
		out[name+" json"] = encoded
	} else {
		out[name+" json error"] = []byte(err.Error())
	}
	if encoded, err := xml.Marshal(subject); err == nil {
		out[name+" xml"] = encoded
	} else {
		out[name+" xml error"] = []byte(err.Error())
	}
	for _, handler := range []string{"json", "text"} {
		var buf bytes.Buffer
		var h slog.Handler = slog.NewJSONHandler(&buf, nil)
		if handler == "text" {
			h = slog.NewTextHandler(&buf, nil)
		}
		slog.New(h).Info("result", "value", subject)
		out[name+" slog "+handler] = buf.Bytes()
	}
	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(subject); err != nil {
		out[name+" gob error"] = []byte(err.Error())
	}
	out[name+" gob"] = buf.Bytes()
	return out
}

// exportedKey reads the private key through the explicit export accessor.
func exportedKey(r models.ExportGuardSignerWalletResult) string {
	return r.EncryptedPrivateKey.Reveal()
}

func TestExportGuardSignerWalletResultRedactsPrivateKey(t *testing.T) {
	result := decode.ExportWalletFromProto(&guardv1.ExportGuardSignerWalletResponse{PrivateKey: fakeExportedKey})
	if exportedKey(result) != fakeExportedKey {
		t.Fatal("the export accessor must return the exported private key")
	}
	type exported struct {
		Result models.ExportGuardSignerWalletResult
	}
	type unexported struct {
		name   string
		result models.ExportGuardSignerWalletResult
	}
	subjects := map[string]any{
		"value":              result,
		"pointer":            &result,
		"nested":             exported{Result: result},
		"slice":              []models.ExportGuardSignerWalletResult{result},
		"unexported field":   unexported{name: "app", result: result},
		"unexported pointer": &unexported{result: result},
	}
	markers := secretMarkers(fakeExportedKey)
	for name, subject := range subjects {
		for path, rendered := range renderAll(t, name, subject) {
			assertNoSecret(t, path, rendered, markers)
		}
	}
}

// keypairSecretHex reads the generated private key through its accessor.
func keypairSecretHex(kp models.Ed25519Keypair) string {
	return kp.SecretKeyHex.Reveal()
}

func TestEd25519KeypairRedactsSecretKey(t *testing.T) {
	kp := auth.GenerateEd25519Keypair()
	secretHex := keypairSecretHex(kp)
	seed, err := hex.DecodeString(secretHex)
	if err != nil || len(seed) != 32 {
		t.Fatalf("generated secret key must be a 32-byte hex seed: %v", err)
	}
	if kp.SecretKey.Reveal() != string(seed) {
		t.Fatal("SecretKey must reveal the seed that SecretKeyHex encodes")
	}
	type exported struct{ Keypair models.Ed25519Keypair }
	type unexported struct{ keypair models.Ed25519Keypair }
	subjects := map[string]any{
		"value":            kp,
		"pointer":          &kp,
		"nested":           exported{Keypair: kp},
		"unexported field": unexported{keypair: kp},
	}
	markers := append(secretMarkers(secretHex),
		seed[:8],
		[]byte(fmt.Sprintf("[%d %d %d %d", seed[0], seed[1], seed[2], seed[3])),
	)
	for name, subject := range subjects {
		for path, rendered := range renderAll(t, name, subject) {
			assertNoSecret(t, path, rendered, markers)
		}
	}
}
