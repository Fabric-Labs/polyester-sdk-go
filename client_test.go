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
