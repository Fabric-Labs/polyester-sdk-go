package hardening_test

// POLY-9228: no level of an SDK error's chain prints a credential.
//
// An answer can quote a URL with userinfo, such as a proxy's. The SDK error
// copied it into its message, so Error() printed the credential. Every level
// of the chain MapConnectError returns is now redacted, and errors.As still
// finds the typed error. The URL and credentials here are synthetic.

import (
	"context"
	"encoding/json"
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

// Round 2: URLs whose userinfo or query holds a parenthesis or an
// apostrophe, and structured diagnostics behind pointers.

func TestL2URLWithParenthesesOrQuotesIsRedacted(t *testing.T) {
	urls := map[string]string{
		"paren-userinfo": "http://" + chainUser + ":one(two-" + chainSecret + ")@proxy.example.test/x",
		"quote-userinfo": "http://" + chainUser + ":one'two-" + chainSecret + "@proxy.example.test/x",
		"paren-query":    "http://proxy.example.test/x?" + chainQuery + "one(two-" + chainSecret + ")",
		"quote-query":    "http://proxy.example.test/x?" + chainQuery + "one'two-" + chainSecret,
	}
	for name, url := range urls {
		t.Run(name, func(t *testing.T) {
			body, _ := json.Marshal(map[string]string{"code": "unavailable", "message": "upstream " + url + " refused"})
			srv := hardening.SpawnHTTP(func(hardening.ParsedRequest) hardening.HttpResponse {
				return hardening.Json(http.StatusServiceUnavailable, body)
			})
			t.Cleanup(srv.Close)
			client, err := polyester.New(polyester.Config{APIURL: srv.BaseURL(), WSURL: "ws://127.0.0.1:1/unused", Timeout: 2 * time.Second})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = client.Close() })
			_, err = client.MarketData.GetSpotConfig(context.Background())
			if err == nil || !strings.Contains(err.Error(), "proxy.example.test") {
				t.Fatalf("GetSpotConfig = %v", err)
			}
			assertChainClean(t, err)
		})
	}
}

func TestL2StructuredDiagnosticsAreRedacted(t *testing.T) {
	cases := map[string]struct {
		status int
		body   map[string]any
		get    func(error) any
	}{
		"timestamp-skew-problem": {
			status: http.StatusUnauthorized,
			body:   map[string]any{"code": "TIMESTAMP_SKEW", "detail": "upstream " + chainProxyURL + " refused"},
			get: func(err error) any {
				var skew *sdkerrors.TimestampSkewError
				if !errors.As(err, &skew) || skew.Problem == nil {
					return nil
				}
				return skew.Problem
			},
		},
		"rate-limit-detail": {
			status: http.StatusTooManyRequests,
			body: map[string]any{
				"code":       "RATE_LIMIT_EXCEEDED",
				"detail":     "upstream " + chainProxyURL,
				"rate_limit": map[string]any{"reason": "ip", "operation_id": chainProxyURL},
			},
			get: func(err error) any {
				var limited *sdkerrors.RateLimitError
				if !errors.As(err, &limited) || limited.Detail == nil {
					return nil
				}
				return limited.Detail
			},
		},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			body, _ := json.Marshal(c.body)
			srv := hardening.SpawnHTTP(func(hardening.ParsedRequest) hardening.HttpResponse {
				return hardening.Raw(c.status, [][2]string{{"Content-Type", "application/problem+json"}}, body)
			})
			t.Cleanup(srv.Close)
			client, err := polyester.New(polyester.Config{APIURL: srv.BaseURL(), WSURL: "ws://127.0.0.1:1/unused", Timeout: 2 * time.Second})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = client.Close() })
			_, err = client.MarketData.GetSpotConfig(context.Background())
			diagnostic := c.get(err)
			if diagnostic == nil {
				t.Fatalf("no structured diagnostic in %T %v", err, err)
			}
			printed := fmt.Sprintf("%+v %#v", diagnostic, diagnostic)
			for _, secret := range []string{chainUser, chainSecret, chainQuery} {
				if strings.Contains(printed, secret) {
					t.Fatalf("diagnostic prints %q: %s", secret, printed)
				}
			}
			assertChainClean(t, err)
		})
	}
}
