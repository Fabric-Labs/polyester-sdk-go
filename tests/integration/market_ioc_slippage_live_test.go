//go:build integration

package integration_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	"github.com/Fabric-Labs/polyester-sdk-go/internal/testutil"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

func int32PtrLive(v int32) *int32 { return &v }

func marketPreviewRequest(t *testing.T, symbol, qty, refPrice string, ticks, bps *int32) models.CreateOrderRequest {
	t.Helper()
	tif := "ioc"
	return models.CreateOrderRequest{
		Symbol:               &symbol,
		Side:                 "buy",
		OrderType:            "market",
		TIF:                  &tif,
		Qty:                  models.QtyFromDecimal(qty),
		MarketClientRefPrice: pricePtr(models.PriceFromDecimal(refPrice)),
		MaxSlippageTicks:     ticks,
		MaxSlippageBps:       bps,
	}
}

func previewFingerprint(p models.PreviewOrderResult) string {
	var bound any
	if p.ProtectedPriceBound != nil {
		bound = p.ProtectedPriceBound.Ticks()
	}
	var code string
	if p.Rejection != nil {
		code = p.Rejection.Code
	}
	var admissible any
	if p.Admissible != nil {
		admissible = *p.Admissible
	}
	return fmt.Sprintf("admissible=%v bound=%v code=%q", admissible, bound, code)
}

func assertHostAcceptedSlippage(t *testing.T, label string, preview models.PreviewOrderResult) {
	t.Helper()
	if preview.Rejection == nil {
		return
	}
	blob := strings.ToLower(preview.Rejection.Code)
	for _, v := range preview.Rejection.Violations {
		blob += " " + strings.ToLower(v.FieldPath+" "+v.Message)
	}
	for _, hint := range []string{"unknown field", "unknown argument", "no such field", "unrecognized field", "unsupported create"} {
		if strings.Contains(blob, hint) {
			t.Fatalf("%s treated slippage as unknown/unsupported: %s", label, blob)
		}
	}
}

func TestPreviewMarketIocAcceptsSlippageOverrides(t *testing.T) {
	client, ctx, cleanup := testutil.RequireLiveClient(t)
	defer cleanup()

	symbol := testutil.SmokeSymbol(t, client, ctx)
	spotRaw, err := testutil.HydrateSpotRaw(ctx, client)
	if err != nil {
		t.Fatal(err)
	}
	pair := testutil.PairForSymbol(spotRaw, symbol)
	if pair == nil {
		t.Skipf("symbol %s is not in spot config", symbol)
	}
	refPrice := testutil.MarketRefPrice(ctx, client, symbol, "buy", pair)
	qty := testutil.MinBaseQtyForPair(pair, refPrice)

	preview := func(label string, ticks, bps *int32) models.PreviewOrderResult {
		t.Helper()
		return testutil.CallOptional(t, label, func() (models.PreviewOrderResult, error) {
			return client.Orders.Preview(ctx, marketPreviewRequest(t, symbol, qty, refPrice, ticks, bps), nil)
		})
	}

	unset := preview("orders.preview unset", nil, nil)
	bps := preview("orders.preview max_slippage_bps=25", nil, int32PtrLive(25))
	ticks := preview("orders.preview max_slippage_ticks=1", int32PtrLive(1), nil)
	assertHostAcceptedSlippage(t, "unset", unset)
	assertHostAcceptedSlippage(t, "bps", bps)
	assertHostAcceptedSlippage(t, "ticks", ticks)

	fpUnset := previewFingerprint(unset)
	fpBps := previewFingerprint(bps)
	fpTicks := previewFingerprint(ticks)
	t.Logf("preview fingerprints unset=%s bps=%s ticks=%s", fpUnset, fpBps, fpTicks)
	if unset.ProtectedPriceBound != nil && fpUnset == fpBps && fpUnset == fpTicks {
		t.Fatalf("slippage overrides did not change preview vs pair default; host may still be ignoring the field: %s", fpUnset)
	}
}

func TestCreateMarketIocWithSlippageBps(t *testing.T) {
	testutil.RequireMutation(t)
	client, ctx, cleanup := testutil.RequireLiveClient(t)
	defer cleanup()

	symbol := testutil.TradeSymbol(t, client, ctx)
	testutil.RequireTradingBalanceForSymbol(t, ctx, client, symbol)

	spotRaw, err := testutil.HydrateSpotRaw(ctx, client)
	if err != nil {
		t.Fatal(err)
	}
	pair := testutil.PairForSymbol(spotRaw, symbol)
	if pair == nil {
		t.Skipf("symbol %s is not in spot config", symbol)
	}
	refPrice := testutil.MarketRefPrice(ctx, client, symbol, "buy", pair)
	qty := testutil.MinBaseQtyForPair(pair, refPrice)
	clientOrderID := testutil.UniqueClientOrderID("mkt-slip")
	tif := "ioc"
	bps := int32(25)
	created, err := client.Orders.Create(ctx, models.CreateOrderRequest{
		Symbol:               &symbol,
		Side:                 "buy",
		OrderType:            "market",
		TIF:                  &tif,
		Qty:                  models.QtyFromDecimal(qty),
		ClientOrderID:        &clientOrderID,
		MarketClientRefPrice: pricePtr(models.PriceFromDecimal(refPrice)),
		MaxSlippageBps:       &bps,
	}, nil)
	if err != nil {
		if testutil.IsDevnetOrderInternalError(err) || testutil.DevnetUnavailable(err) {
			t.Skipf("devnet order placement unavailable: %v", err)
		}
		var validation *sdkerrors.ValidationError
		if errors.As(err, &validation) && strings.Contains(strings.ToLower(validation.Msg), "notional") {
			t.Skipf("order sizing below min notional: %v", err)
		}
		t.Fatal(err)
	}
	if created.ClientOrderID != clientOrderID || created.OrderID == "" {
		t.Fatalf("create=%+v", created)
	}
	t.Logf("create status=%s order_id=%s client_order_id=%s", created.Status, created.OrderID, created.ClientOrderID)
	defer func() {
		_, _ = client.Orders.Cancel(ctx, nil, models.OrderKeyByClientID(clientOrderID), nil, nil, nil)
	}()
	switch created.Status {
	case "canceled", "rejected", "filled", "accepted":
		return
	}
	detail, waitErr := testutil.WaitForTerminalOrder(ctx, client, clientOrderID, 0)
	if waitErr != nil {
		msg := waitErr.Error()
		if strings.Contains(msg, "did not reach terminal status") && !strings.Contains(msg, "stuck in status") {
			return
		}
		t.Fatal(waitErr)
	}
	if detail.Order == nil {
		t.Fatal("expected order detail")
	}
	status := detail.Order.Status
	if status != "canceled" && status != "rejected" && status != "filled" {
		t.Fatalf("unexpected terminal status %q", status)
	}
}
