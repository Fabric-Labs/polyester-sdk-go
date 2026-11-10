package realtime

import (
	"context"
	"crypto/ed25519"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"

	"github.com/Fabric-Labs/polyester-sdk-go/auth"
)

// A signed realtime token request answered with any redirect must not reach
// the redirect destination, whether the realtime client uses its own HTTP
// client or one the caller supplies.
func TestRealtimeTokenRequestIsNotForwardedAcrossOrigins(t *testing.T) {
	statuses := []int{
		http.StatusMovedPermanently, http.StatusFound, http.StatusSeeOther,
		http.StatusTemporaryRedirect, http.StatusPermanentRedirect,
	}
	clients := map[string]func() *http.Client{
		"sdk-client":    func() *http.Client { return nil },
		"caller-client": func() *http.Client { return &http.Client{} },
	}
	for _, status := range statuses {
		for clientName, newClient := range clients {
			t.Run(strconv.Itoa(status)+"/"+clientName, func(t *testing.T) {
				var mu sync.Mutex
				var requests, signed int
				destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					mu.Lock()
					requests++
					if r.Header.Get("X-API-KEY-ID") != "" || r.Header.Get("X-API-TIMESTAMP") != "" || r.Header.Get("X-API-SIGNATURE") != "" {
						signed++
					}
					mu.Unlock()
					_, _ = w.Write([]byte(`{"token":"captured"}`))
				}))
				t.Cleanup(destination.Close)
				origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					http.Redirect(w, r, destination.URL+r.URL.RequestURI(), status)
				}))
				t.Cleanup(origin.Close)

				_, private, err := ed25519.GenerateKey(nil)
				if err != nil {
					t.Fatal(err)
				}
				creds, err := auth.NewCredentials("ak_redirect_test", private)
				if err != nil {
					t.Fatal(err)
				}
				client := NewClient("ws://127.0.0.1:1", origin.URL, creds, newClient(), 10)

				token, err := fetchConnectionToken(context.Background(), client.http, creds, origin.URL)

				mu.Lock()
				defer mu.Unlock()
				if signed != 0 || requests != 0 {
					t.Fatalf("destination received %d request(s), %d signed, after a %d redirect", requests, signed, status)
				}
				if err == nil {
					t.Fatalf("token request succeeded with %q although its only answer was a redirect", token)
				}
			})
		}
	}
}
