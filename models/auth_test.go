package models

import (
	"fmt"
	"strings"
	"testing"
)

func TestEd25519KeypairStringRedactsSecret(t *testing.T) {
	kp := Ed25519Keypair{
		PublicKeyHex: "abcd",
		SecretKeyHex: "super-secret-seed-hex",
		PublicKey:    []byte{1, 2, 3},
		SecretKey:    []byte("super-secret-seed-bytes"),
	}
	for _, rendered := range []string{kp.String(), kp.GoString(), fmt.Sprintf("%v", kp), fmt.Sprintf("%#v", kp)} {
		if !strings.Contains(rendered, "abcd") {
			t.Fatalf("expected public key in %q", rendered)
		}
		if !strings.Contains(rendered, "[REDACTED]") {
			t.Fatalf("expected redaction in %q", rendered)
		}
		if strings.Contains(rendered, "super-secret-seed-hex") {
			t.Fatalf("secret hex leaked via %q", rendered)
		}
		if strings.Contains(rendered, "super-secret-seed-bytes") {
			t.Fatalf("secret bytes leaked via %q", rendered)
		}
	}
}
