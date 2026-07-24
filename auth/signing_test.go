package auth

import (
	"crypto/ed25519"
	"encoding/hex"
	"testing"

	"github.com/Fabric-Labs/polyester-sdk-go/errors"
)

func TestCanonicalQuerySortsAndEncodesValues(t *testing.T) {
	got := CanonicalQuery("https://api.example.test/path?b=2&a=hello world")
	want := "a=hello%20world&b=2"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestCanonicalQueryPreservesHyphensInChannelParam(t *testing.T) {
	got := CanonicalQuery("https://api.example.test/v1/rt/subscribe?channel=private:auth:api-keys:account:proto")
	want := "channel=private%3Aauth%3Aapi-keys%3Aaccount%3Aproto"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestCanonicalQuerySharedVectors(t *testing.T) {
	cases := []struct {
		url, want string
	}{
		{"https://api.example.test/x?z=1&a=hello world&m=a+b", "a=hello%20world&m=a%20b&z=1"},
		{"https://api.example.test/x?z=1&a=hello%20world&m=a%2Bb", "a=hello%20world&m=a%2Bb&z=1"},
		{"https://api.example.test/x?b=&a=1", "a=1&b="},
		{"https://api.example.test/x?a=1&a=2&b=0", "a=1&a=2&b=0"},
		{"https://api.example.test/x?path=foo/bar&name=a_b.c~d-e", "name=a_b.c~d-e&path=foo%2Fbar"},
		{"https://api.example.test/x?msg=%E2%9C%93&plain=ok", "msg=%E2%9C%93&plain=ok"},
	}
	for _, tc := range cases {
		if got := CanonicalQuery(tc.url); got != tc.want {
			t.Fatalf("url=%q got %q want %q", tc.url, got, tc.want)
		}
	}
}

func TestCanonicalSigningStringMatchesContract(t *testing.T) {
	got := CanonicalSigningString(
		"123",
		"post",
		"https://api.example.test/foo/bar?b=2&a=1",
		[]byte("{}"),
	)
	want := "123\nPOST\n/foo/bar\na=1&b=2\n44136fa355b3678a1146ad16f7e8649e94fb4fc21fe77e8310c060f61caaff8a"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestSignRequestReturnsPolyesterHeaders(t *testing.T) {
	_, private, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	creds := &Credentials{KeyID: "key_123", PrivateKey: private}
	headers := SignRequest(creds, "POST", "https://api.example.test/foo", []byte("{}"), "123")
	if headers["X-API-KEY-ID"] != "key_123" {
		t.Fatalf("key id: %s", headers["X-API-KEY-ID"])
	}
	if headers["X-API-TIMESTAMP"] != "123" {
		t.Fatalf("timestamp: %s", headers["X-API-TIMESTAMP"])
	}
	if len(headers["X-API-SIGNATURE"]) != 128 {
		t.Fatalf("signature length: %d", len(headers["X-API-SIGNATURE"]))
	}
}

func TestLoadCredentialsFromEnv(t *testing.T) {
	_, private, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("POLYESTER_API_KEY_ID", "ak_test")
	t.Setenv("POLYESTER_API_PRIVATE_KEY", hex.EncodeToString(private.Seed()))
	creds, err := LoadCredentials("", "", true)
	if err != nil {
		t.Fatal(err)
	}
	if creds == nil {
		t.Fatal("expected credentials")
	}
	if string(private.Seed()) != string(creds.PrivateKey.Seed()) {
		t.Fatal("private key mismatch")
	}
}

func TestLoadCredentialsRequiresBoth(t *testing.T) {
	t.Setenv("POLYESTER_API_KEY_ID", "ak_test")
	t.Setenv("POLYESTER_API_PRIVATE_KEY", "")
	_, err := LoadCredentials("", "", true)
	if err == nil {
		t.Fatal("expected error")
	}
	if _, ok := err.(*errors.AuthError); !ok {
		t.Fatalf("expected AuthError, got %T", err)
	}
}
