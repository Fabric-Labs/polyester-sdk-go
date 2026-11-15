package polyester_test

import (
	"context"
	"strings"
	"testing"

	polyester "github.com/Fabric-Labs/polyester-sdk-go"
	"github.com/Fabric-Labs/polyester-sdk-go/services"
)

// With SkipCatalogHydration set, a call that names a market by symbol cannot
// resolve it. The refusal tells a Go caller how to load the catalogs. A managed book needs them even with a SymbolID, for its quantity scale.
func TestSymbolRefusalWithoutCatalogsNamesBothRemedies(t *testing.T) {
	client, err := polyester.New(polyester.Config{
		// Closed loopback ports: nothing can be reached, and nothing must be.
		APIURL:               "http://127.0.0.1:9",
		WSURL:                "ws://127.0.0.1:9",
		SkipCatalogHydration: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = client.Close() }()
	ctx := context.Background()

	_, subscribeErr := client.Orderbook.CreateSubscription(ctx, services.CreateSubscriptionOptions{Symbol: "BTC-USDT", Depth: 50})
	id := uint32(1)
	_, byIDErr := client.Orderbook.CreateSubscription(ctx, services.CreateSubscriptionOptions{Symbol: "BTC-USDT", SymbolID: &id, Depth: 50})
	_, getErr := client.Orderbook.Get(ctx, "BTC-USDT", 50)
	for _, tc := range []struct {
		name     string
		err      error
		remedies []string
	}{
		{"Orderbook.CreateSubscription", subscribeErr, []string{"SkipCatalogHydration unset", "WaitForCatalogs", "SymbolID does not replace the catalog"}},
		{"Orderbook.CreateSubscription with SymbolID", byIDErr, []string{"SkipCatalogHydration unset", "WaitForCatalogs"}},
		{"Orderbook.Get", getErr, []string{"SkipCatalogHydration unset", "WaitForCatalogs"}},
	} {
		name, err := tc.name, tc.err
		if err == nil {
			t.Fatalf("%s by symbol without catalogs succeeded", name)
		}
		for _, remedy := range tc.remedies {
			if !strings.Contains(err.Error(), remedy) {
				t.Errorf("%s: %q does not name %q", name, err, remedy)
			}
		}
	}
}
