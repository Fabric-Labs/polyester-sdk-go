package realtime

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	"github.com/Fabric-Labs/polyester-sdk-go/hardening"
)

// POLY-8563: a refused subscribe token keeps the server's reason, as the
// Python and Rust SDKs report it: "HTTP 400: unsupported channel".
func TestSubscriptionTokenRefusalKeepsTheServerText(t *testing.T) {
	for _, tc := range []struct {
		name, contentType, body, want string
	}{
		{"plain text", "text/plain", "unsupported channel\n", "HTTP 400: unsupported channel"},
		{"json", "application/json", `{"code":"invalid_argument","message":"unsupported channel"}`, `HTTP 400: {"code":"invalid_argument","message":"unsupported channel"}`},
		{"empty", "text/plain", "", "HTTP 400"},
		{"long", "text/plain", strings.Repeat("x", 2*maxAuthErrorBodyBytes), "HTTP 400: " + strings.Repeat("x", maxAuthErrorBodyBytes) + "…"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := hardening.NewTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", tc.contentType)
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()

			_, err := fetchSubscriptionToken(context.Background(), &http.Client{Timeout: time.Second}, testCreds(t), srv.URL, "private:unknown:proto")
			var rtErr *sdkerrors.RealtimeError
			if !errors.As(err, &rtErr) {
				t.Fatalf("want RealtimeError, got %T: %v", err, err)
			}
			want := "realtime subscription token for private:unknown:proto: " + tc.want
			if err.Error() != want && rtErr.Msg != want {
				t.Fatalf("error = %q\nwant  %q", rtErr.Msg, want)
			}
		})
	}
}
