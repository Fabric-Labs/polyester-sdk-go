//go:build integration

package integration_test

import (
	"errors"
	"strings"
	"testing"

	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	"github.com/Fabric-Labs/polyester-sdk-go/internal/testutil"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

func TestOrderRoundTripMutation(t *testing.T) {
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
	clientOrderID := testutil.UniqueClientOrderID("e2e")
	postOnly := true
	tif := "gtc"

	created, err := client.Orders.Create(ctx, models.CreateOrderRequest{
		Symbol:        &symbol,
		Side:          "buy",
		OrderType:     "limit",
		TIF:           &tif,
		Qty:           models.QtyFromDecimal(qty),
		Price:         pricePtr(models.PriceFromDecimal(price)),
		ClientOrderID: &clientOrderID,
		PostOnly:      postOnly,
	}, nil)
	if err != nil {
		if testutil.IsDevnetOrderInternalError(err) || testutil.DevnetUnavailable(err) {
			t.Skipf("devnet order placement unavailable: %v", err)
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
	if created.OrderID == "" {
		t.Fatal("expected order_id from create")
	}

	openOrder, err := testutil.WaitForOpenOrder(ctx, client, clientOrderID, 50, 0)
	if err != nil {
		var notIndexed *testutil.DevnetOrderNotIndexedError
		if errors.As(err, &notIndexed) {
			t.Skip("devnet order create accepted but orders read APIs never indexed the order; check OMS read path on devnet")
		}
		t.Fatal(err)
	}
	if openOrder.ClientOrderID != clientOrderID {
		t.Fatalf("open order client_order_id=%q", openOrder.ClientOrderID)
	}

	detail := testutil.CallRequired(t, "orders.get", func() (models.GetOrderResult, error) {
		return client.Orders.Get(ctx, nil, models.OrderKeyByClientID(clientOrderID), nil, false, false)
	})
	if detail.Order == nil || detail.Order.ClientOrderID != clientOrderID {
		t.Fatalf("detail=%+v", detail.Order)
	}
	if detail.Order.OrigQty.Scaled() <= 0 {
		t.Fatalf("expected current accepted orig_qty, got %+v", detail.Order.OrigQty)
	}

	defer func() {
		_, _ = client.Orders.CancelAll(ctx, nil, nil, &symbol, nil, false, nil)
	}()

	cancelled, err := client.Orders.Cancel(ctx, nil, models.OrderKeyByClientID(clientOrderID), &symbol, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if cancelled.Status == "" {
		t.Fatalf("cancel status empty: %+v", cancelled)
	}
	if err := testutil.WaitForNoOpenOrder(ctx, client, clientOrderID, 50, 0); err != nil {
		t.Fatal(err)
	}
}
