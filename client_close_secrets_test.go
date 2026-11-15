package polyester

import (
	"strings"
	"testing"

	"github.com/Fabric-Labs/polyester-sdk-go/auth"
	"github.com/Fabric-Labs/polyester-sdk-go/internal/redact"
)

func newKeyedTestClient(t *testing.T, privateKeyHex string) *Client {
	t.Helper()
	client, err := New(Config{
		APIURL:               "http://127.0.0.1:1",
		APIKeyID:             "ak_test_0",
		APIPrivateKey:        auth.NewSecret(privateKeyHex),
		SkipCatalogHydration: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	return client
}

// Close forgets the client's private key: the process no longer holds a hex
// copy of it in the redaction registry once every client using it is closed.
func TestCloseRemovesClientPrivateKeyFromRedaction(t *testing.T) {
	keyA := strings.Repeat("c3", 32)
	keyB := strings.Repeat("d4", 32)
	line := "leak " + keyA + " and " + keyB

	if got := redact.Text(line); got != line {
		t.Fatalf("keys redacted before any client: %s", got)
	}
	clientA := newKeyedTestClient(t, keyA)
	clientB := newKeyedTestClient(t, keyB)
	if got, want := redact.Text(line), "leak [REDACTED] and [REDACTED]"; got != want {
		t.Fatalf("with both clients open: got %q, want %q", got, want)
	}

	_ = clientA.Close()
	if got, want := redact.Text(line), "leak "+keyA+" and [REDACTED]"; got != want {
		t.Fatalf("after closing A: got %q, want %q", got, want)
	}
	_ = clientB.Close()
	if got := redact.Text(line); got != line {
		t.Fatalf("keys still registered after both clients closed: %s", got)
	}
	// Close is idempotent: a second Close does not release anything again.
	_ = clientA.Close()
	if got := redact.Text(line); got != line {
		t.Fatalf("after second Close: %s", got)
	}
}

// Two clients holding the same key: closing one keeps the key redacted for the
// other until it closes too.
func TestCloseKeepsSharedPrivateKeyRedactedForOpenClient(t *testing.T) {
	key := strings.Repeat("e5", 32)
	line := "leak " + key

	first := newKeyedTestClient(t, key)
	second := newKeyedTestClient(t, key)
	_ = first.Close()
	_ = first.Close()
	if got, want := redact.Text(line), "leak [REDACTED]"; got != want {
		t.Fatalf("shared key with one client open: got %q, want %q", got, want)
	}
	_ = second.Close()
	if got := redact.Text(line); got != line {
		t.Fatalf("shared key still registered after both clients closed: %s", got)
	}
}

// URL userinfo is released on Close as well, and still redacted while open.
func TestCloseRemovesClientURLUserinfoFromRedaction(t *testing.T) {
	client, err := New(Config{
		APIURL:               "http://closeuser:close-url-secret-Q7@127.0.0.1:1",
		SkipCatalogHydration: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	line := "saw close-url-secret-Q7"
	if got := redact.Text(line); got != "saw [REDACTED]" {
		t.Fatalf("open client: %s", got)
	}
	_ = client.Close()
	if got := redact.Text(line); got != line {
		t.Fatalf("closed client: %s", got)
	}
}
