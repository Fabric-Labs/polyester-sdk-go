package polyester

import (
	"context"
	"errors"
	"reflect"
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

func TestClientExposesDocumentedServices(t *testing.T) {
	client, err := New(Config{HydrateCatalogs: false})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })

	expected := []string{
		"Auth", "Accounts", "ChainAnalytics", "MarketData", "Candles", "MarketOverview",
		"Zipper", "Heatmap", "Lifecycle", "Balances", "Orderbook", "Orders", "Trades",
		"Triggers", "Transfers", "InternalTransfers", "Deposit", "APIKeys", "Policies",
		"SubAccounts", "Resolve", "AddressBook", "SocialVerification", "Whiteboard",
		"Polychart", "Layout", "GuardSigner", "Withdraw", "TradingWithdraws", "Realtime",
	}
	v := reflect.ValueOf(client).Elem()
	for _, name := range expected {
		if f := v.FieldByName(name); !f.IsValid() || f.IsNil() {
			t.Fatalf("missing client.%s", name)
		}
	}
	if client.Accounts != client.Resolve {
		t.Fatal("Accounts alias should point to Resolve")
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
