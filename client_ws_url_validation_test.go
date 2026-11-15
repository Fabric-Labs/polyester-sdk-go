package polyester

import (
	"errors"
	"testing"

	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
)

// WSURL follows the APIURL rule: a query string or fragment is refused, so
// nothing put in one by mistake is sent on every connect and reconnect.
func TestNewRefusesWSURLQueryOrFragment(t *testing.T) {
	for _, tc := range []struct {
		wsURL, want string
	}{
		{"wss://example.test/connection/websocket?token=SECRET123", "WSURL base must not contain a query string"},
		{"wss://example.test?", "WSURL base must not contain a query string"},
		{"wss://example.test/connection/websocket#frag", "WSURL base must not contain a fragment"},
		{"wss://example.test#", "WSURL base must not contain a fragment"},
	} {
		_, err := New(Config{WSURL: tc.wsURL, SkipCatalogHydration: true})
		var validation *sdkerrors.ValidationError
		if !errors.As(err, &validation) || validation.Msg != tc.want {
			t.Errorf("WSURL %q: got %v, want ValidationError %q", tc.wsURL, err, tc.want)
		}
	}
	// The APIURL control gives the same error wording.
	_, err := New(Config{APIURL: "https://example.test/api?token=SECRET123", SkipCatalogHydration: true})
	var validation *sdkerrors.ValidationError
	if !errors.As(err, &validation) || validation.Msg != "APIURL base must not contain a query string" {
		t.Fatalf("APIURL control: %v", err)
	}
}

func TestNewAcceptsPlainWSURL(t *testing.T) {
	for _, wsURL := range []string{"wss://example.test", "wss://example.test/connection/websocket", "ws://127.0.0.1:1/"} {
		client, err := New(Config{WSURL: wsURL, SkipCatalogHydration: true})
		if err != nil {
			t.Errorf("WSURL %q: %v", wsURL, err)
			continue
		}
		_ = client.Close()
	}
}
