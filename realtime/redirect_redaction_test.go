package realtime

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Fabric-Labs/polyester-sdk-go/auth"
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	"github.com/Fabric-Labs/polyester-sdk-go/hardening"
)

// Synthetic markers placed in URLs. None is a real credential.
const (
	syntheticUser     = "synthetic-rt-user"
	syntheticPassword = "synthetic-rt-password"
	syntheticToken    = "synthetic-rt-token"
	syntheticFragment = "synthetic-rt-fragment"
)

var syntheticURLSecrets = []string{syntheticUser, syntheticPassword, syntheticToken, syntheticFragment}

func assertNoURLSecrets(t *testing.T, label string, err error, raw string) {
	t.Helper()
	if err == nil {
		t.Fatalf("%s: want an error", label)
	}
	forbidden := append([]string{raw}, syntheticURLSecrets...)
	for e := err; e != nil; e = errors.Unwrap(e) {
		for _, rendered := range []string{e.Error(), fmt.Sprintf("%v", e), fmt.Sprintf("%+v", e), fmt.Sprintf("%#v", e)} {
			for _, secret := range forbidden {
				if strings.Contains(rendered, secret) {
					t.Errorf("%s: error exposes %q: %s", label, secret, rendered)
					return
				}
			}
		}
	}
}

// A realtime token request answered with a redirect whose Location does not
// parse returns an error without the Location's userinfo, query or fragment:
// a sanitized refusal on the client the realtime client builds, and a
// sanitized transport error on any other client.
func TestTokenMalformedRedirectErrorsHaveNoURLSecrets(t *testing.T) {
	locations := map[string]string{
		"invalid-port": "http://" + syntheticUser + ":" + syntheticPassword + "@127.0.0.1:notaport/v1/rt/token?token=" + syntheticToken + "#" + syntheticFragment,
		"invalid-ipv6": "http://" + syntheticUser + ":" + syntheticPassword + "@[::1/v1/rt/token?token=" + syntheticToken + "#" + syntheticFragment,
	}
	for _, status := range []int{http.StatusMovedPermanently, http.StatusTemporaryRedirect, http.StatusPermanentRedirect} {
		for kind, location := range locations {
			srv := hardening.NewTestServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Location", location)
				w.WriteHeader(status)
			}))
			for _, guarded := range []bool{true, false} {
				httpClient := &http.Client{Timeout: time.Second}
				if guarded {
					httpClient = auth.RefuseSignedRedirects(httpClient)
				}
				label := strconv.Itoa(status) + "/" + kind + "/guarded=" + strconv.FormatBool(guarded)
				_, connErr := fetchConnectionToken(context.Background(), httpClient, testCreds(t), srv.URL)
				_, subErr := fetchSubscriptionToken(context.Background(), httpClient, testCreds(t), srv.URL, "orders:1")
				for name, err := range map[string]error{"connection": connErr, "subscription": subErr} {
					assertNoURLSecrets(t, label+"/"+name, err, location)
					var refused *sdkerrors.RedirectRefusedError
					if guarded && (!errors.As(err, &refused) || !refused.Malformed || refused.StatusCode != status) {
						t.Errorf("%s/%s: error = %T %v, want a malformed *RedirectRefusedError", label, name, err, err)
					}
				}
			}
			srv.Close()
		}
	}
}

// Token and WebSocket errors name an endpoint without its query or fragment,
// as well as without its userinfo.
func TestRealtimeURLErrorsHaveNoQueryOrFragment(t *testing.T) {
	const badWSURL = "ws://" + syntheticUser + ":" + syntheticPassword + "@127.0.0.1:notaport/connection?token=" + syntheticToken + "#" + syntheticFragment
	const unreachableWSURL = "ws://" + syntheticUser + ":" + syntheticPassword + "@127.0.0.1:1/connection?token=" + syntheticToken + "#" + syntheticFragment
	errs := map[string]error{}

	for name, raw := range map[string]string{"private dial invalid url": badWSURL, "private dial unreachable": unreachableWSURL} {
		stream := newPrivateOrderStream(privateStreamConfig{url: raw, timing: testTiming()})
		_, errs[name] = stream.dial(context.Background())
	}
	client := NewClient(badWSURL, "http://127.0.0.1:1", nil, &http.Client{Timeout: time.Second}, 10)
	defer client.Close()
	reconnect := false
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, errs["public dial"] = SubscribeProtoWithOptions(ctx, client, "trades:BTC-USDT", func([]byte) (int, error) { return 0, nil }, &SubscribeProtoOptions{AutoReconnect: &reconnect})

	for name, err := range errs {
		raw := badWSURL
		if strings.Contains(name, "unreachable") {
			raw = unreachableWSURL
		}
		assertNoURLSecrets(t, name, err, raw)
	}

	srv := hardening.NewTestServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()
	_, err := fetchSubscriptionToken(context.Background(), &http.Client{Timeout: time.Second}, testCreds(t), srv.URL, syntheticToken)
	var authErr *sdkerrors.AuthError
	if !errors.As(err, &authErr) {
		t.Fatalf("subscription token: want AuthError, got %T %v", err, err)
	}
	if authErr.Endpoint != srv.URL+"/v1/rt/subscribe" || strings.Contains(err.Error(), "?") {
		t.Errorf("endpoint = %q, error = %q; want the endpoint without its query", authErr.Endpoint, err)
	}
	if !strings.Contains(err.Error(), syntheticToken) {
		t.Errorf("the label should still name the channel: %s", err)
	}
}
