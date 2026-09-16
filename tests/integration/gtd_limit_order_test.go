//go:build integration

package integration_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	"github.com/Fabric-Labs/polyester-sdk-go/internal/testutil"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

func TestGTDLimitOrderRoundTrip(t *testing.T) {
	testutil.RequireAccountWideCleanup(t)
	testutil.RequireMutation(t)
	client, ctx, cleanup := testutil.RequireLiveClient(t)
	defer cleanup()

	symbol := testutil.TradeSymbol(t, client, ctx)
	testutil.RequireTradingBalanceForSymbol(t, ctx, client, symbol)

	price, qty, err := testutil.USDTFundedBuyLimitParams(ctx, client, symbol)
	if err != nil {
		t.Fatal(err)
	}
	clientOrderID := testutil.UniqueClientOrderID("gtd")
	postOnly := true
	tif := "gtd"
	expires := time.Now().UTC().Add(time.Hour).Truncate(time.Second).Format(time.RFC3339)

	created, err := client.Orders.Create(ctx, models.CreateOrderRequest{
		Symbol:        &symbol,
		Side:          "buy",
		OrderType:     "limit",
		TIF:           &tif,
		Qty:           models.QtyFromDecimal(qty),
		Price:         pricePtr(models.PriceFromDecimal(price)),
		ClientOrderID: &clientOrderID,
		PostOnly:      postOnly,
		ExpiresAt:     &expires,
	}, nil)
	if err != nil {
		if testutil.IsDevnetOrderInternalError(err) || testutil.DevnetUnavailable(err) || gtdUnsupported(err) {
			t.Skipf("devnet GTD order placement unavailable: %v", err)
		}
		var validation *sdkerrors.ValidationError
		if errors.As(err, &validation) && strings.Contains(strings.ToLower(validation.Msg), "notional") {
			t.Skipf("order sizing below devnet minimum notional: %v", err)
		}
		t.Fatal(err)
	}
	if created.ClientOrderID != clientOrderID {
		t.Fatalf("client_order_id=%q want %q", created.ClientOrderID, clientOrderID)
	}

	openOrder, err := testutil.WaitForOpenOrder(ctx, client, clientOrderID, 50, 0)
	if err != nil {
		var notIndexed *testutil.DevnetOrderNotIndexedError
		if errors.As(err, &notIndexed) {
			t.Skip("devnet order create accepted but orders read APIs never indexed the order")
		}
		t.Fatal(err)
	}
	if openOrder.TIF != "gtd" {
		t.Fatalf("open tif=%q", openOrder.TIF)
	}
	if openOrder.ExpireAt == "" {
		t.Fatal("expected expire_at on GTD open order")
	}

	detail := testutil.CallRequired(t, "orders.get", func() (models.GetOrderResult, error) {
		return client.Orders.Get(ctx, nil, models.OrderKeyByClientID(clientOrderID), nil, false, false)
	})
	if detail.Order == nil || detail.Order.TIF != "gtd" || detail.Order.ExpireAt == "" {
		t.Fatalf("detail=%+v", detail.Order)
	}

	defer func() {
		_, _ = client.Orders.CancelAll(ctx, nil, nil, &symbol, nil, false, nil)
	}()

	if _, err := client.Orders.Cancel(ctx, nil, models.OrderKeyByClientID(clientOrderID), &symbol, nil, nil); err != nil {
		t.Fatal(err)
	}
	if err := testutil.WaitForNoOpenOrder(ctx, client, clientOrderID, 50, 0); err != nil {
		t.Fatal(err)
	}
}

func gtdUnsupported(err error) bool {
	msg := strings.ToLower(err.Error())
	for _, token := range []string{"gtd", "expire_at", "expireat", "time_in_force", "timeinforce", "unimplemented", "not implemented"} {
		if strings.Contains(msg, token) {
			return true
		}
	}
	return false
}
