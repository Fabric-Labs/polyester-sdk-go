package realtime

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	"github.com/Fabric-Labs/polyester-sdk-go/hardening"
)

// fakeURLPassword is an obviously fake password placed in URL userinfo.
const fakeURLPassword = "fake-url-password-marker"

// Both token paths turn 401 and 403 into AuthError. Neither its endpoint nor
// its text may carry the password from the API URL's userinfo, while both keep
// the endpoint without it.
func TestTokenAuthErrorsRedactURLUserinfo(t *testing.T) {
	for _, status := range []int{http.StatusUnauthorized, http.StatusForbidden} {
		srv := hardening.NewTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(status)
			_, _ = w.Write([]byte(`{"code":"permission_denied","message":"refused"}`))
		}))
		host := strings.TrimPrefix(srv.URL, "http://")
		for _, withUserinfo := range []bool{true, false} {
			apiURL := srv.URL
			if withUserinfo {
				apiURL = "http://fake-user:" + fakeURLPassword + "@" + host
			}
			paths := map[string]struct {
				fetch    func() error
				endpoint string
			}{
				"connection token": {
					fetch: func() error {
						_, err := fetchConnectionToken(context.Background(), &http.Client{Timeout: time.Second}, testCreds(t), apiURL)
						return err
					},
					endpoint: srv.URL + "/v1/rt/token",
				},
				"subscription token": {
					fetch: func() error {
						_, err := fetchSubscriptionToken(context.Background(), &http.Client{Timeout: time.Second}, testCreds(t), apiURL, "orders:1")
						return err
					},
					endpoint: srv.URL + "/v1/rt/subscribe",
				},
			}
			for name, path := range paths {
				label := fmt.Sprintf("%s HTTP %d userinfo=%v", name, status, withUserinfo)
				err := path.fetch()
				var authErr *sdkerrors.AuthError
				if !errors.As(err, &authErr) {
					t.Fatalf("%s: want AuthError, got %T: %v", label, err, err)
				}
				for _, rendered := range []string{err.Error(), authErr.Endpoint, authErr.Msg, fmt.Sprintf("%+v", authErr), fmt.Sprintf("%#v", authErr)} {
					if strings.Contains(rendered, fakeURLPassword) || strings.Contains(rendered, "fake-user") {
						t.Errorf("%s: error leaked URL userinfo: %s", label, rendered)
					}
				}
				if authErr.Status != status || authErr.Endpoint != path.endpoint {
					t.Errorf("%s: status=%d endpoint=%q, want %d %q", label, authErr.Status, authErr.Endpoint, status, path.endpoint)
				}
				if !strings.Contains(err.Error(), "["+path.endpoint+"]") {
					t.Errorf("%s: error text should name the endpoint without userinfo: %s", label, err)
				}
			}
		}
		srv.Close()
	}
}

// A URL that does not parse must not reach an error with its userinfo: the
// standard parser quotes the whole URL in its message.
func TestRealtimeURLErrorsRedactUserinfo(t *testing.T) {
	const badAPIURL = "http://fake-user:" + fakeURLPassword + "@127.0.0.1:notaport"
	const badWSURL = "ws://fake-user:" + fakeURLPassword + "@127.0.0.1:notaport"
	errs := map[string]error{}

	_, errs["signed get"] = signedGet(context.Background(), &http.Client{Timeout: time.Second}, testCreds(t), badAPIURL+"/v1/rt/token", "realtime connection token", maxTokenResponseBytes)
	_, errs["private order stream URL"] = privateWSURL(badAPIURL)

	client := NewClient(badWSURL, badAPIURL, nil, &http.Client{Timeout: time.Second}, 10)
	defer client.Close()
	reconnect := false
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, errs["dial"] = SubscribeProtoWithOptions(ctx, client, "trades:BTC-USDT", func([]byte) (int, error) { return 0, nil }, &SubscribeProtoOptions{AutoReconnect: &reconnect})

	for name, err := range errs {
		if err == nil {
			t.Fatalf("%s: want an error", name)
		}
		for _, rendered := range []string{err.Error(), fmt.Sprintf("%+v", err), fmt.Sprintf("%#v", err)} {
			if strings.Contains(rendered, fakeURLPassword) {
				t.Errorf("%s: error leaked URL userinfo: %s", name, rendered)
			}
		}
	}
}

// failingTransport fails every request without reaching a server.
type failingTransport struct{}

func (failingTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errors.New("connection failed")
}

// The standard client masks the password in a transport error but keeps the
// user name; neither may reach the error from either token path.
func TestTokenTransportErrorsRedactURLUserinfo(t *testing.T) {
	const apiURL = "https://fake-user:" + fakeURLPassword + "@api.example.test"
	client := &http.Client{Transport: failingTransport{}}
	errs := map[string]error{}
	_, errs["connection token"] = fetchConnectionToken(context.Background(), client, testCreds(t), apiURL)
	_, errs["subscription token"] = fetchSubscriptionToken(context.Background(), client, testCreds(t), apiURL, "orders:1")
	for name, err := range errs {
		if err == nil {
			t.Fatalf("%s: want an error", name)
		}
		for _, rendered := range []string{err.Error(), fmt.Sprintf("%+v", err), fmt.Sprintf("%#v", err)} {
			if strings.Contains(rendered, fakeURLPassword) || strings.Contains(rendered, "fake-user") {
				t.Errorf("%s: error leaked URL userinfo: %s", name, rendered)
			}
		}
		if !strings.Contains(err.Error(), "api.example.test") || !strings.Contains(err.Error(), "connection failed") {
			t.Errorf("%s: error should keep the host and cause: %s", name, err)
		}
	}
}
