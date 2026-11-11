package hardening_test

// POLY-9228: no level of an SDK error's chain prints a credential.
//
// An answer can quote a URL with userinfo, such as a proxy's. The SDK error
// copied it into its message, so Error() printed the credential. Every level
// of the chain MapConnectError returns is now redacted, and errors.As still
// finds the typed error. The URL and credentials here are synthetic.

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	polyester "github.com/Fabric-Labs/polyester-sdk-go"
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	"github.com/Fabric-Labs/polyester-sdk-go/hardening"
)

const (
	chainUser   = "sdk-proxy-user"
	chainSecret = "proxy-secret-9228"
	chainQuery  = "token=query-secret"
)

var chainProxyURL = "http://" + chainUser + ":" + chainSecret + "@proxy.example.test:3128/tunnel?" + chainQuery

func chainAnswers() map[string]hardening.HttpResponse {
	return map[string]hardening.HttpResponse{
		"connect-error": hardening.Json(http.StatusServiceUnavailable,
			[]byte(`{"code":"unavailable","message":"upstream `+chainProxyURL+` refused"}`)),
		"gateway-problem": hardening.Json(http.StatusBadGateway,
			[]byte(`{"code":"EDGE_PROXY_FAILED","detail":"upstream `+chainProxyURL+` refused"}`)),
	}
}

func assertChainClean(t *testing.T, err error) {
	t.Helper()
	levels := 0
	for e := err; e != nil; e = errors.Unwrap(e) {
		levels++
		printed := e.Error() + fmt.Sprintf(" %v %+v", e, e)
		for _, secret := range []string{chainUser, chainSecret, chainQuery} {
			if strings.Contains(printed, secret) {
				t.Fatalf("level %d (%T) prints %q: %s", levels, e, secret, printed)
			}
		}
	}
}

func TestL2UnaryErrorChainPrintsNoCredential(t *testing.T) {
	for name, answer := range chainAnswers() {
		t.Run(name, func(t *testing.T) {
			srv := hardening.SpawnHTTP(func(hardening.ParsedRequest) hardening.HttpResponse { return answer })
			t.Cleanup(srv.Close)
			client, err := polyester.New(polyester.Config{APIURL: srv.BaseURL(), WSURL: "ws://127.0.0.1:1/unused", Timeout: 2 * time.Second})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = client.Close() })
			_, err = client.MarketData.GetSpotConfig(context.Background())
			var serverErr *sdkerrors.ServerError
			if !errors.As(err, &serverErr) {
				t.Fatalf("GetSpotConfig = %T %v, want *errors.ServerError", err, err)
			}
			if !strings.Contains(err.Error(), "proxy.example.test") {
				t.Fatalf("the message lost the host: %v", err)
			}
			assertChainClean(t, err)
		})
	}
}

func TestL2CatalogStartupErrorChainPrintsNoCredential(t *testing.T) {
	for name, answer := range chainAnswers() {
		t.Run(name, func(t *testing.T) {
			srv := hardening.SpawnHTTP(func(req hardening.ParsedRequest) hardening.HttpResponse {
				if strings.HasSuffix(req.Path, "/GetSpotConfig") {
					return startupSpotResponse(t)
				}
				return answer
			})
			t.Cleanup(srv.Close)
			client, err := polyester.New(polyester.Config{APIURL: srv.BaseURL(), WSURL: "ws://127.0.0.1:1/unused", HydrateCatalogs: true, Timeout: 2 * time.Second})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = client.Close() })
			if err := client.WaitForCatalogs(context.Background()); err != nil {
				t.Fatal(err)
			}
			err = client.WaitForFundingCatalog(context.Background())
			var hydrationErr *sdkerrors.CatalogHydrationError
			var serverErr *sdkerrors.ServerError
			if !errors.As(err, &hydrationErr) || !errors.As(err, &serverErr) {
				t.Fatalf("WaitForFundingCatalog = %T %v", err, err)
			}
			assertChainClean(t, err)
		})
	}
}
