package polyester

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestNewUsesDefaults(t *testing.T) {
	client, err := New(Config{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	if client.APIURL != DefaultAPIURL {
		t.Fatalf("api url: %s", client.APIURL)
	}
	if client.Orderbook == nil || client.MarketData == nil {
		t.Fatal("expected service tree")
	}
}

func TestNewValidatesAPIBaseURL(t *testing.T) {
	for _, rawURL := range []string{
		"https://api.example.test?x=1",
		"https://api.example.test?",
		"https://api.example.test/?",
		"https://api.example.test/#fragment",
		"https://api.example.test#",
		"https://api.example.test/#",
		"api.example.test",
		"ftp://api.example.test",
		"https:///missing-host",
		"http://:8080",
		" https://api.example.test",
	} {
		t.Run(rawURL, func(t *testing.T) {
			client, err := New(Config{APIURL: rawURL})
			if client != nil {
				t.Cleanup(func() { _ = client.Close() })
			}
			if err == nil {
				t.Fatalf("New accepted invalid APIURL %q", rawURL)
			}
		})
	}

	for _, rawURL := range []string{
		"http://127.0.0.1:12345",
		"http://127.0.0.1:12345/test-base/",
		"https://api.example.test",
		"https://api.example.test/path%3Fsegment",
		"https://api.example.test/path%23segment",
		"https://proxy-user:proxy-pass@api.example.test/base",
	} {
		t.Run("valid_"+rawURL, func(t *testing.T) {
			client, err := New(Config{APIURL: rawURL})
			if err != nil {
				t.Fatalf("New rejected valid APIURL %q: %v", rawURL, err)
			}
			t.Cleanup(func() { _ = client.Close() })
		})
	}
}

func TestConfigStringRedactsURLUserinfo(t *testing.T) {
	cfg := Config{
		APIURL: "https://proxy-user:proxy-pass@api.example.test/base",
		WSURL:  "wss://socket-user:socket-pass@api.example.test/realtime",
	}
	for name, rendered := range map[string]string{
		"String":   cfg.String(),
		"GoString": cfg.GoString(),
		"format":   fmt.Sprintf("%#v", cfg),
	} {
		if strings.Contains(rendered, "proxy-user") ||
			strings.Contains(rendered, "proxy-pass") ||
			strings.Contains(rendered, "socket-user") ||
			strings.Contains(rendered, "socket-pass") {
			t.Fatalf("%s exposed URL credentials: %s", name, rendered)
		}
		if !strings.Contains(rendered, "https://api.example.test/base") ||
			!strings.Contains(rendered, "wss://api.example.test/realtime") {
			t.Fatalf("%s did not preserve the URL after removing userinfo: %s", name, rendered)
		}
	}
}

func TestConfigStringRedactsMalformedCredentialURLs(t *testing.T) {
	cfg := Config{
		APIURL: "https://api-user:api-secret@api.example.test/%zz",
		WSURL:  "wss://ws-user:ws-secret@api.example.test/%zz",
	}
	for name, rendered := range map[string]string{
		"String":   cfg.String(),
		"GoString": cfg.GoString(),
		"format":   fmt.Sprintf("%#v", cfg),
	} {
		for _, secret := range []string{"api-user", "api-secret", "ws-user", "ws-secret", "%zz"} {
			if strings.Contains(rendered, secret) {
				t.Fatalf("%s exposed malformed URL contents %q: %s", name, secret, rendered)
			}
		}
		if count := strings.Count(rendered, "[REDACTED INVALID URL]"); count != 2 {
			t.Fatalf("%s should fail closed for both malformed URLs, got %d placeholders: %s", name, count, rendered)
		}
	}
	_, err := New(Config{APIURL: cfg.APIURL})
	if err == nil {
		t.Fatal("New accepted malformed credential-bearing APIURL")
	}
	for _, secret := range []string{"api-user", "api-secret", "%zz"} {
		if strings.Contains(err.Error(), secret) {
			t.Fatalf("APIURL validation error exposed malformed URL contents %q: %v", secret, err)
		}
	}
}

func TestConfigStringRedactsOpaqueAndHostlessCredentialURLs(t *testing.T) {
	for _, test := range []struct {
		name   string
		apiURL string
		wsURL  string
	}{
		{
			name:   "opaque",
			apiURL: "https:api-user:api-secret@host",
			wsURL:  "wss:ws-user:ws-secret@host",
		},
		{
			name:   "hostless",
			apiURL: "https:/api-user:api-secret@host",
			wsURL:  "wss:/ws-user:ws-secret@host",
		},
		{
			name:   "scheme_less_hostless",
			apiURL: "/api-user:api-secret@host",
			wsURL:  "/ws-user:ws-secret@host",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			cfg := Config{APIURL: test.apiURL, WSURL: test.wsURL}
			for name, rendered := range map[string]string{
				"String":   cfg.String(),
				"GoString": cfg.GoString(),
				"format":   fmt.Sprintf("%#v", cfg),
			} {
				for _, secret := range []string{"api-user", "api-secret", "ws-user", "ws-secret", "@host"} {
					if strings.Contains(rendered, secret) {
						t.Fatalf("%s exposed malformed URL contents %q: %s", name, secret, rendered)
					}
				}
				if count := strings.Count(rendered, "[REDACTED INVALID URL]"); count != 2 {
					t.Fatalf("%s should fail closed for both malformed URLs, got %d placeholders: %s", name, count, rendered)
				}
			}
		})
	}
}

func TestConfigStringPreservesNormalURLs(t *testing.T) {
	cfg := Config{
		APIURL: "https://api.example.test/path@segment",
		WSURL:  "wss://api.example.test/realtime@stream",
	}
	for name, rendered := range map[string]string{
		"String":   cfg.String(),
		"GoString": cfg.GoString(),
		"format":   fmt.Sprintf("%#v", cfg),
	} {
		if !strings.Contains(rendered, cfg.APIURL) || !strings.Contains(rendered, cfg.WSURL) {
			t.Fatalf("%s changed a normal URL: %s", name, rendered)
		}
	}
}

func TestConfigStringRedactsPrivateKey(t *testing.T) {
	cfg := Config{
		APIKeyID:      "key_123",
		APIPrivateKey: "super-secret-private-key",
		APIURL:        DefaultAPIURL,
	}
	rendered := cfg.String()
	if !strings.Contains(rendered, "[REDACTED]") {
		t.Fatalf("expected redaction, got %s", rendered)
	}
	if strings.Contains(rendered, "super-secret-private-key") {
		t.Fatalf("private key leaked: %s", rendered)
	}
	goRendered := cfg.GoString()
	if !strings.Contains(goRendered, "[REDACTED]") {
		t.Fatalf("GoString expected redaction, got %s", goRendered)
	}
	if strings.Contains(goRendered, "super-secret-private-key") {
		t.Fatalf("private key leaked via GoString: %s", goRendered)
	}
	if strings.Contains(fmt.Sprintf("%#v", cfg), "super-secret-private-key") {
		t.Fatalf("private key leaked via %%#v: %#v", cfg)
	}
}

func TestClientExposesDocumentedServices(t *testing.T) {
	client, err := New(Config{HydrateCatalogs: false})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })

	expected := []string{
		"Auth", "ChainAnalytics", "MarketData", "Candles", "MarketOverview",
		"Zipper", "Heatmap", "Lifecycle", "Balances", "Orderbook", "Orders", "Trades",
		"Triggers", "Transfers", "InternalTransfers", "Deposit", "APIKeys", "Policies",
		"SubAccounts", "AddressBook", "SocialVerification", "Whiteboard",
		"Polychart", "Layout", "GuardSigner", "VIP", "Fees", "RateLimits",
		"Withdraw", "TradingWithdraws", "Realtime",
	}
	v := reflect.ValueOf(client).Elem()
	for _, name := range expected {
		if f := v.FieldByName(name); !f.IsValid() || f.IsNil() {
			t.Fatalf("missing client.%s", name)
		}
	}
	if client.Candles != client.MarketData {
		t.Fatal("Candles alias should point to MarketData")
	}
	if client.TradingWithdraws != client.Withdraw {
		t.Fatal("TradingWithdraws alias should point to Withdraw")
	}
	if client.Auth.Profile == nil {
		t.Fatal("expected client.Auth.Profile")
	}
}

func TestWaitForCatalogsReturnsImmediatelyWhenHydrationDisabled(t *testing.T) {
	client, err := New(Config{HydrateCatalogs: false})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := client.WaitForCatalogs(ctx); err != nil {
		t.Fatalf("wait for catalogs: %v", err)
	}
}

func TestWaitForCatalogsHonorsContextCancellation(t *testing.T) {
	client := &Client{catalogHydrationDone: make(chan struct{})}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := client.WaitForCatalogs(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context cancellation, got %v", err)
	}
}
