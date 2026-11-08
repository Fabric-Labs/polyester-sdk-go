package auth

import (
	"crypto/ed25519"
	"encoding/hex"
	"errors"
	"testing"

	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
)

// The window's decimal string is its own line, right after the timestamp.
func TestCanonicalSigningStringWithRecvWindowInsertsTheWindowLine(t *testing.T) {
	got, err := CanonicalSigningStringWithRecvWindow("123", 5000, "post", "https://api.example.test/foo/bar?b=2&a=1", []byte("{}"))
	if err != nil {
		t.Fatal(err)
	}
	want := "123\n5000\nPOST\n/foo/bar\na=1&b=2\n44136fa355b3678a1146ad16f7e8649e94fb4fc21fe77e8310c060f61caaff8a"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
	without, err := CanonicalSigningStringWithRecvWindow("123", 0, "post", "https://api.example.test/foo/bar?b=2&a=1", []byte("{}"))
	if err != nil {
		t.Fatal(err)
	}
	plain, err := CanonicalSigningString("123", "post", "https://api.example.test/foo/bar?b=2&a=1", []byte("{}"))
	if err != nil || without != plain {
		t.Fatalf("window 0 = %q, want the unchanged payload %q (%v)", without, plain, err)
	}
}

func TestSignRequestWithRecvWindowSignsTheHeader(t *testing.T) {
	public, private, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	creds, err := NewCredentials("ak_test", private)
	if err != nil {
		t.Fatal(err)
	}
	const rawURL = "https://api.example.test/orders.v1.OrdersService/CreateOrder"
	headers, err := SignRequestWithRecvWindow(creds, "POST", rawURL, []byte("body"), "1762553700123", 5000)
	if err != nil {
		t.Fatal(err)
	}
	if headers["X-API-RECV-WINDOW"] != "5000" || headers["X-API-TIMESTAMP"] != "1762553700123" {
		t.Fatalf("headers = %v", headers)
	}
	sig, err := hex.DecodeString(headers["X-API-SIGNATURE"])
	if err != nil {
		t.Fatal(err)
	}
	signed := "1762553700123\n5000\nPOST\n/orders.v1.OrdersService/CreateOrder\n\n230d8358dc8e8890b4c58deeb62912ee2f20357ae92a5cc861b98e68fe31acb5"
	if !ed25519.Verify(public, []byte(signed), sig) {
		t.Fatal("signature does not cover the receive window line")
	}
	plain, err := SignRequest(creds, "POST", rawURL, []byte("body"), "1762553700123")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := plain["X-API-RECV-WINDOW"]; ok {
		t.Fatal("SignRequest sends no receive window")
	}
	if plain["X-API-SIGNATURE"] == headers["X-API-SIGNATURE"] {
		t.Fatal("the window must change what is signed")
	}
}

func TestRecvWindowOutsideItsRangeIsRefusedBeforeSigning(t *testing.T) {
	_, private, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	creds, err := NewCredentials("ak_test", private)
	if err != nil {
		t.Fatal(err)
	}
	for _, window := range []int{-1, 60001, 1 << 40} {
		_, err := SignRequestWithRecvWindow(creds, "GET", "https://api.example.test/x", nil, "1", window)
		var validation *sdkerrors.ValidationError
		if !errors.As(err, &validation) {
			t.Fatalf("window %d: error = %T %v, want *errors.ValidationError", window, err, err)
		}
		if _, err := CanonicalSigningStringWithRecvWindow("1", window, "GET", "https://api.example.test/x", nil); !errors.As(err, &validation) {
			t.Fatalf("window %d: canonical error = %v", window, err)
		}
	}
	for _, window := range []int{MinRecvWindowMS, MaxRecvWindowMS} {
		if _, err := SignRequestWithRecvWindow(creds, "GET", "https://api.example.test/x", nil, "1", window); err != nil {
			t.Fatalf("window %d: %v", window, err)
		}
	}
}
