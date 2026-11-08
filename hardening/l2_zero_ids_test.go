package hardening_test

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	polyester "github.com/Fabric-Labs/polyester-sdk-go"
	"github.com/Fabric-Labs/polyester-sdk-go/auth"
	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	"github.com/Fabric-Labs/polyester-sdk-go/hardening"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
	"github.com/Fabric-Labs/polyester-sdk-go/services"
)

// Account and sub-account IDs that decode to 0 are refused before anything
// is sent. On the wire 0 is "unset": a sub-account ID of 0 is the main
// account, and an account ID of 0 names no account. "1" is the public ID of 0
// (codecs.FormatID(0)), and "0" and "00" read as decimal 0.
var zeroIDForms = []string{"0", "00", "1", " 1 "}

// countingAPI counts every request that reaches it and fails each one.
type countingAPI struct{ calls atomic.Int64 }

func (a *countingAPI) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	a.calls.Add(1)
	w.WriteHeader(http.StatusInternalServerError)
}

func zeroIDClient(t *testing.T, mutate func(*polyester.Config)) (*polyester.Client, *countingAPI) {
	t.Helper()
	api := &countingAPI{}
	srv := hardening.NewTestServer(api)
	t.Cleanup(srv.Close)
	kp := auth.GenerateEd25519Keypair()
	account := codecs.FormatID(7)
	cfg := polyester.Config{
		APIURL:           srv.URL,
		WSURL:            "ws" + strings.TrimPrefix(srv.URL, "http") + "/connection/websocket",
		APIKeyID:         "ak_test",
		APIPrivateKey:    kp.SecretKeyHex,
		DefaultAccountID: &account,
		HydrateCatalogs:  false,
		Timeout:          5 * time.Second,
	}
	if mutate != nil {
		mutate(&cfg)
	}
	client, err := polyester.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	// A hydrated pair, so order and trigger writes get as far as the wire.
	if err := client.Catalogs.HydrateSpotConfig(map[string]any{"pairs": []any{
		map[string]any{"symbol": "BTC-USDT", "symbol_id": float64(1), "base_quantity_scale": float64(8), "quote_quantity_scale": float64(6)},
	}}); err != nil {
		t.Fatal(err)
	}
	return client, api
}

func requireZeroIDRefused(t *testing.T, name, label string, err error, api *countingAPI) {
	t.Helper()
	var validation *sdkerrors.ValidationError
	if !errors.As(err, &validation) || !strings.Contains(err.Error(), label) {
		t.Errorf("%s: err = %v; want a %s ValidationError", name, err, label)
	}
	if api != nil {
		if calls := api.calls.Swap(0); calls != 0 {
			t.Errorf("%s: sent %d requests; want none", name, calls)
		}
	}
}

func zeroIDStr(s string) *string { return &s }

func zeroIDU32(v uint32) *uint32 { return &v }

type subAccountCall struct {
	name string
	call func(ctx context.Context, c *polyester.Client, account any, sub *string) error
}

// subAccountCalls covers every client method that takes a sub-account scope.
func subAccountCalls() []subAccountCall {
	btc := zeroIDStr("BTC-USDT")
	key := models.OrderKeyByID(codecs.FormatID(42))
	order := func(sub *string) models.CreateOrderRequest {
		price := models.PriceFromDecimal("50000")
		return models.CreateOrderRequest{Symbol: btc, Side: "buy", OrderType: "limit", Qty: models.QtyFromDecimal("0.1"), Price: &price, SubAccountID: sub}
	}
	triggerID := codecs.FormatID(42)
	price := models.PriceFromDecimal("50000")
	call := func(name string, fn func(context.Context, *polyester.Client, any, *string) error) subAccountCall {
		return subAccountCall{name, fn}
	}
	return []subAccountCall{
		call("Orders.Create", func(ctx context.Context, c *polyester.Client, a any, s *string) error {
			_, err := c.Orders.Create(ctx, order(s), a)
			return err
		}),
		call("Orders.Preview", func(ctx context.Context, c *polyester.Client, a any, s *string) error {
			_, err := c.Orders.Preview(ctx, order(s), a)
			return err
		}),
		call("Orders.ListOpen", func(ctx context.Context, c *polyester.Client, a any, s *string) error {
			_, err := c.Orders.ListOpen(ctx, a, s, nil, nil, false, false, nil)
			return err
		}),
		call("Orders.ListHistory", func(ctx context.Context, c *polyester.Client, a any, s *string) error {
			_, err := c.Orders.ListHistory(ctx, a, s, nil, nil, nil, 0, false, false, nil)
			return err
		}),
		call("Orders.Get", func(ctx context.Context, c *polyester.Client, a any, s *string) error {
			_, err := c.Orders.Get(ctx, a, key, s, false, false)
			return err
		}),
		call("Orders.Cancel", func(ctx context.Context, c *polyester.Client, a any, s *string) error {
			_, err := c.Orders.Cancel(ctx, a, key, nil, zeroIDU32(1), s)
			return err
		}),
		call("Orders.ModifyWith", func(ctx context.Context, c *polyester.Client, a any, s *string) error {
			_, err := c.Orders.ModifyWith(ctx, a, services.ModifyParams{Symbol: "BTC-USDT", Key: key, SubAccountID: s, NewPrice: &price})
			return err
		}),
		call("Orders.CancelAllWith", func(ctx context.Context, c *polyester.Client, a any, s *string) error {
			_, err := c.Orders.CancelAllWith(ctx, a, services.CancelAllParams{SubAccountID: s})
			return err
		}),
		call("Orders.CancelAllAfter", func(ctx context.Context, c *polyester.Client, a any, s *string) error {
			_, err := c.Orders.CancelAllAfter(ctx, a, 10, s, nil, nil, nil)
			return err
		}),
		call("Orders.BatchCreate", func(ctx context.Context, c *polyester.Client, a any, s *string) error {
			_, err := c.Orders.BatchCreate(ctx, a, []models.CreateOrderRequest{order(nil)}, s, nil, nil, false)
			return err
		}),
		call("Orders.BatchCancel", func(ctx context.Context, c *polyester.Client, a any, s *string) error {
			_, err := c.Orders.BatchCancel(ctx, a, []models.BatchCancelItem{{Key: key, SymbolID: zeroIDU32(1)}}, s, nil)
			return err
		}),
		call("Orders.BatchReplace", func(ctx context.Context, c *polyester.Client, a any, s *string) error {
			_, err := c.Orders.BatchReplace(ctx, a, []models.BatchReplaceItem{{Key: key, NewPrice: &price}}, "BTC-USDT", s, nil)
			return err
		}),
		call("Orders.GetBatchReplaceStatus", func(ctx context.Context, c *polyester.Client, a any, s *string) error {
			_, err := c.Orders.GetBatchReplaceStatus(ctx, a, codecs.FormatID(42), s)
			return err
		}),
		call("Balances.List", func(ctx context.Context, c *polyester.Client, a any, s *string) error {
			_, err := c.Balances.List(ctx, a, s)
			return err
		}),
		call("Balances.GetBalanceHistory", func(ctx context.Context, c *polyester.Client, a any, s *string) error {
			_, err := c.Balances.GetBalanceHistory(ctx, a, "1d", s, 0, nil)
			return err
		}),
		call("Balances.GetEquityHistory", func(ctx context.Context, c *polyester.Client, a any, s *string) error {
			_, err := c.Balances.GetEquityHistory(ctx, a, "1d", s, nil, "asset")
			return err
		}),
		call("Fees.GetSpotFeeRates", func(ctx context.Context, c *polyester.Client, a any, s *string) error {
			_, err := c.Fees.GetSpotFeeRates(ctx, a, s, nil)
			return err
		}),
		call("RateLimits.GetTradingRateLimits", func(ctx context.Context, c *polyester.Client, a any, s *string) error {
			_, err := c.RateLimits.GetTradingRateLimits(ctx, a, s)
			return err
		}),
		call("Triggers.List", func(ctx context.Context, c *polyester.Client, a any, s *string) error {
			_, err := c.Triggers.List(ctx, a, s, nil, nil, 0, nil)
			return err
		}),
		call("Triggers.Get", func(ctx context.Context, c *polyester.Client, a any, s *string) error {
			_, err := c.Triggers.Get(ctx, a, triggerID, s)
			return err
		}),
		call("Triggers.Create", func(ctx context.Context, c *polyester.Client, a any, s *string) error {
			_, err := c.Triggers.Create(ctx, a, "BTC-USDT", "stop_loss", &price, "sell", models.QtyFromDecimal("0.1"), "market", nil, "", "", s, nil, false, codecs.CreateTriggerOptions{})
			return err
		}),
		call("Triggers.Cancel", func(ctx context.Context, c *polyester.Client, a any, s *string) error {
			_, err := c.Triggers.Cancel(ctx, a, triggerID, s)
			return err
		}),
		call("Triggers.Pause", func(ctx context.Context, c *polyester.Client, a any, s *string) error {
			_, err := c.Triggers.Pause(ctx, a, triggerID, s)
			return err
		}),
		call("Triggers.Resume", func(ctx context.Context, c *polyester.Client, a any, s *string) error {
			_, err := c.Triggers.Resume(ctx, a, triggerID, s, nil, nil)
			return err
		}),
		call("Triggers.Modify", func(ctx context.Context, c *polyester.Client, a any, s *string) error {
			_, err := c.Triggers.Modify(ctx, a, triggerID, s, nil, nil, codecs.ModifyTriggerOptions{TriggerPrice: &price})
			return err
		}),
		call("Triggers.ListEvents", func(ctx context.Context, c *polyester.Client, a any, s *string) error {
			_, err := c.Triggers.ListEvents(ctx, a, triggerID, s, 0, nil, nil)
			return err
		}),
		call("Trades.List", func(ctx context.Context, c *polyester.Client, a any, s *string) error {
			_, err := c.Trades.List(ctx, a, s, nil, nil, 0, nil, nil)
			return err
		}),
		call("Transfers.List", func(ctx context.Context, c *polyester.Client, a any, s *string) error {
			_, err := c.Transfers.List(ctx, a, s, 0, false, nil)
			return err
		}),
		call("APIKeys.List", func(ctx context.Context, c *polyester.Client, a any, s *string) error {
			_, err := c.APIKeys.List(ctx, a, s)
			return err
		}),
		call("SubAccounts.Get", func(ctx context.Context, c *polyester.Client, a any, s *string) error {
			_, err := c.SubAccounts.Get(ctx, a, s, false, false, false, false, false, "")
			return err
		}),
		call("SubAccounts.ListMembers", func(ctx context.Context, c *polyester.Client, a any, s *string) error {
			_, err := c.SubAccounts.ListMembers(ctx, a, s)
			return err
		}),
		call("SubAccounts.ListActivity", func(ctx context.Context, c *polyester.Client, a any, s *string) error {
			_, err := c.SubAccounts.ListActivity(ctx, a, s, 0, nil)
			return err
		}),
		call("AddressBook.ListEntries", func(ctx context.Context, c *polyester.Client, a any, s *string) error {
			_, err := c.AddressBook.ListEntries(ctx, a, s, nil, 0, nil)
			return err
		}),
		call("AddressBook.GetView", func(ctx context.Context, c *polyester.Client, a any, s *string) error {
			_, err := c.AddressBook.GetView(ctx, a, s, 0, 0)
			return err
		}),
		call("AddressBook.CreateTag", func(ctx context.Context, c *polyester.Client, a any, s *string) error {
			_, err := c.AddressBook.CreateTag(ctx, a, s, "desk", "blue")
			return err
		}),
		call("Deposit.ListAddresses", func(ctx context.Context, c *polyester.Client, a any, s *string) error {
			_, err := c.Deposit.ListAddresses(ctx, 1, a, s)
			return err
		}),
		call("GuardSigner.GetStatus", func(ctx context.Context, c *polyester.Client, a any, s *string) error {
			_, err := c.GuardSigner.GetStatus(ctx, a, s)
			return err
		}),
	}
}

// TestL2ZeroSubAccountIDIsRefusedBeforeSending passes a sub-account ID that
// decodes to 0 in every form a client method resolves: the subAccountID
// argument, an account string, an account dict and the client's default.
func TestL2ZeroSubAccountIDIsRefusedBeforeSending(t *testing.T) {
	ctx := context.Background()
	client, api := zeroIDClient(t, nil)
	for _, tc := range subAccountCalls() {
		for _, zero := range zeroIDForms {
			requireZeroIDRefused(t, tc.name+" sub_account_id "+zero, "sub_account_id",
				tc.call(ctx, client, nil, zeroIDStr(zero)), api)
			requireZeroIDRefused(t, tc.name+" account "+zero, "sub_account_id",
				tc.call(ctx, client, zero, nil), api)
			for _, field := range []string{"subaccountId", "sub_account_id"} {
				requireZeroIDRefused(t, tc.name+" account dict "+field+" "+zero, "sub_account_id",
					tc.call(ctx, client, map[string]string{field: zero}, nil), api)
			}
		}
	}
	for _, zero := range zeroIDForms {
		defaulted, defaultedAPI := zeroIDClient(t, func(cfg *polyester.Config) { cfg.DefaultSubAccountID = zeroIDStr(zero) })
		for _, tc := range subAccountCalls() {
			requireZeroIDRefused(t, tc.name+" default "+zero, "sub_account_id",
				tc.call(ctx, defaulted, nil, nil), defaultedAPI)
		}
	}
}

// TestL2ZeroSubAccountIDControlsStillSend keeps an omitted sub-account (the
// main account) and a non-zero one on the wire.
func TestL2ZeroSubAccountIDControlsStillSend(t *testing.T) {
	ctx := context.Background()
	client, api := zeroIDClient(t, nil)
	for name, sub := range map[string]*string{"omitted": nil, "empty": zeroIDStr(""), "42": zeroIDStr(codecs.FormatID(42))} {
		_, _ = client.Balances.List(ctx, nil, sub)
		if calls := api.calls.Swap(0); calls == 0 {
			t.Errorf("Balances.List %s sent nothing", name)
		}
	}
}

// TestL2ZeroSubAccountIDRequestObjectIsRefused covers sub-account IDs carried
// by a request object or a batch item.
func TestL2ZeroSubAccountIDRequestObjectIsRefused(t *testing.T) {
	ctx := context.Background()
	client, api := zeroIDClient(t, nil)
	price := models.PriceFromDecimal("50000")
	for _, zero := range zeroIDForms {
		item := models.CreateOrderRequest{Symbol: zeroIDStr("BTC-USDT"), Side: "buy", OrderType: "limit", Qty: models.QtyFromDecimal("0.1"), Price: &price, SubAccountID: zeroIDStr(zero)}
		_, err := client.Orders.BatchCreate(ctx, nil, []models.CreateOrderRequest{item}, nil, nil, nil, false)
		requireZeroIDRefused(t, "Orders.BatchCreate item "+zero, "sub_account_id", err, api)
	}
}

// TestL2ZeroSubAccountIDPublicCodecsAreRefused calls the exported codecs that
// encode a sub-account ID directly.
func TestL2ZeroSubAccountIDPublicCodecsAreRefused(t *testing.T) {
	symbol := "BTC-USDT"
	sid := uint32(1)
	price := models.PriceFromDecimal("50000")
	key := models.OrderKeyByID(codecs.FormatID(42))
	order := func(sub *string) models.CreateOrderRequest {
		return models.CreateOrderRequest{Symbol: &symbol, SymbolID: &sid, Side: "buy", OrderType: "limit", Qty: models.QtyFromDecimal("0.1"), Price: &price, SubAccountID: sub}
	}
	for _, zero := range zeroIDForms {
		sub := zeroIDStr(zero)
		codecCalls := map[string]func() error{
			"ParseOptionalSubaccountID": func() error {
				_, err := codecs.ParseOptionalSubaccountID(sub)
				return err
			},
			"CreateOrderToProto": func() error {
				_, err := codecs.CreateOrderToProto(order(sub), 8, 6)
				return err
			},
			"PreviewOrderToProto": func() error {
				_, err := codecs.PreviewOrderToProto(order(sub), 8, 6)
				return err
			},
			"BatchCreateOrdersToProto": func() error {
				_, err := codecs.BatchCreateOrdersToProto([]models.CreateOrderRequest{order(nil)}, sub, nil, false, 8, 6)
				return err
			},
			"BatchCancelOrdersToProto": func() error {
				_, err := codecs.BatchCancelOrdersToProto([]models.BatchCancelItem{{Key: key, SymbolID: &sid}}, sub, nil)
				return err
			},
			"BatchReplaceOrdersToProto": func() error {
				_, err := codecs.BatchReplaceOrdersToProto([]models.BatchReplaceItem{{Key: key, NewPrice: &price}}, sid, sub, nil, 8, symbol)
				return err
			},
			"ModifyOrderToProto": func() error {
				_, err := codecs.ModifyOrderToProto(symbol, sid, key, sub, nil, &price, nil, nil, nil, nil, 8)
				return err
			},
			"CancelAllOrdersToProto": func() error {
				_, err := codecs.CancelAllOrdersToProto(sub, nil, nil, false, nil)
				return err
			},
			"CancelAllAfterToProto": func() error {
				_, err := codecs.CancelAllAfterToProto(sub, 10, nil, nil, nil)
				return err
			},
			"CreateTriggerToProto": func() error {
				_, err := codecs.CreateTriggerToProto(sid, symbol, "stop_loss", &price, "sell", models.QtyFromDecimal("0.1"), "market", nil, "", "", sub, nil, false, 8, codecs.CreateTriggerOptions{})
				return err
			},
			"ModifyTriggerToProto": func() error {
				_, err := codecs.ModifyTriggerToProto(codecs.FormatID(42), sid, symbol, sub, codecs.ModifyTriggerOptions{TriggerPrice: &price})
				return err
			},
		}
		for name, fn := range codecCalls {
			requireZeroIDRefused(t, "codecs."+name+" "+zero, "sub_account_id", fn(), nil)
		}
	}
	if got, err := codecs.ParseOptionalSubaccountID(zeroIDStr(codecs.FormatID(42))); err != nil || got == nil || *got != 42 {
		t.Errorf("ParseOptionalSubaccountID(42) = %v, %v", got, err)
	}
	if got, err := codecs.ParseOptionalSubaccountID(nil); err != nil || got != nil {
		t.Errorf("ParseOptionalSubaccountID(nil) = %v, %v", got, err)
	}
}

type accountSubscribeCall struct {
	name string
	call func(ctx context.Context, c *polyester.Client, accountID any) error
}

func closed[T interface{ Close() }](sub T, err error) error {
	if err == nil {
		sub.Close()
	}
	return err
}

func accountSubscribeCalls() []accountSubscribeCall {
	return []accountSubscribeCall{
		{"APIKeys.Subscribe", func(ctx context.Context, c *polyester.Client, id any) error {
			return closed(c.APIKeys.Subscribe(ctx, id))
		}},
		{"AddressBook.Subscribe", func(ctx context.Context, c *polyester.Client, id any) error {
			return closed(c.AddressBook.Subscribe(ctx, id))
		}},
		{"AddressBook.SubscribeViewInvalidations", func(ctx context.Context, c *polyester.Client, id any) error {
			return closed(c.AddressBook.SubscribeViewInvalidations(ctx, id))
		}},
		{"Balances.Subscribe", func(ctx context.Context, c *polyester.Client, id any) error {
			return closed(c.Balances.Subscribe(ctx, id))
		}},
		{"Lifecycle.SubscribeOpenFlows", func(ctx context.Context, c *polyester.Client, id any) error {
			return closed(c.Lifecycle.SubscribeOpenFlows(ctx, id))
		}},
		{"Orders.Subscribe", func(ctx context.Context, c *polyester.Client, id any) error {
			return closed(c.Orders.Subscribe(ctx, id))
		}},
		{"Policies.Subscribe", func(ctx context.Context, c *polyester.Client, id any) error {
			return closed(c.Policies.Subscribe(ctx, id))
		}},
		{"Policies.SubscribeAPIPolicies", func(ctx context.Context, c *polyester.Client, id any) error {
			return closed(c.Policies.SubscribeAPIPolicies(ctx, id))
		}},
		{"SubAccounts.Subscribe", func(ctx context.Context, c *polyester.Client, id any) error {
			return closed(c.SubAccounts.Subscribe(ctx, id))
		}},
		{"SubAccounts.SubscribeAPIKeys", func(ctx context.Context, c *polyester.Client, id any) error {
			return closed(c.SubAccounts.SubscribeAPIKeys(ctx, id))
		}},
		{"Trades.Subscribe", func(ctx context.Context, c *polyester.Client, id any) error {
			return closed(c.Trades.Subscribe(ctx, id))
		}},
		{"Transfers.Subscribe", func(ctx context.Context, c *polyester.Client, id any) error {
			return closed(c.Transfers.Subscribe(ctx, id))
		}},
		{"Triggers.Subscribe", func(ctx context.Context, c *polyester.Client, id any) error {
			return closed(c.Triggers.Subscribe(ctx, id))
		}},
		{"Triggers.SubscribeEvents", func(ctx context.Context, c *polyester.Client, id any) error {
			return closed(c.Triggers.SubscribeEvents(ctx, id))
		}},
	}
}

// TestL2ZeroAccountIDIsRefusedBeforeSubscribing passes an account ID that
// decodes to 0 to every private Subscribe, as an argument and as the
// client's default account.
func TestL2ZeroAccountIDIsRefusedBeforeSubscribing(t *testing.T) {
	ctx := context.Background()
	client, api := zeroIDClient(t, nil)
	for _, tc := range accountSubscribeCalls() {
		for _, zero := range zeroIDForms {
			requireZeroIDRefused(t, tc.name+" "+zero, "account_id", tc.call(ctx, client, zero), api)
			requireZeroIDRefused(t, tc.name+" pointer "+zero, "account_id", tc.call(ctx, client, zeroIDStr(zero)), api)
		}
	}
	if _, err := services.ResolveAccountID(nil, "1"); err == nil {
		t.Error(`ResolveAccountID(nil, "1") accepted account 0`)
	}
	for _, zero := range zeroIDForms {
		defaulted, defaultedAPI := zeroIDClient(t, func(cfg *polyester.Config) { cfg.DefaultAccountID = zeroIDStr(zero) })
		for _, tc := range accountSubscribeCalls() {
			if tc.name == "Lifecycle.SubscribeOpenFlows" {
				continue // an unset account is the public open-flows channel
			}
			requireZeroIDRefused(t, tc.name+" default "+zero, "account_id", tc.call(ctx, defaulted, nil), defaultedAPI)
		}
	}
}

// TestL2ZeroAccountIDArgumentsAreRefused covers the other account IDs a
// method takes: an internal transfer's destination and a flow list's owner.
func TestL2ZeroAccountIDArgumentsAreRefused(t *testing.T) {
	ctx := context.Background()
	client, api := zeroIDClient(t, nil)
	scale := 18
	for _, zero := range zeroIDForms {
		_, err := client.InternalTransfers.Create(ctx, 1, models.AssetAmountFromDecimal("1"), "idem-1", nil, nil, zeroIDStr(zero), nil, nil, &scale)
		requireZeroIDRefused(t, "InternalTransfers.Create destination account "+zero, "destination_account_id", err, api)
		_, err = client.InternalTransfers.Create(ctx, 1, models.AssetAmountFromDecimal("1"), "idem-1", nil, nil, nil, zeroIDStr(zero), nil, &scale)
		requireZeroIDRefused(t, "InternalTransfers.Create destination sub-account "+zero, "destination_subaccount_id", err, api)
		_, err = client.Lifecycle.ListFlows(ctx, 0, nil, nil, zeroIDStr(zero), nil, false)
		requireZeroIDRefused(t, "Lifecycle.ListFlows owner "+zero, "owner_account_id", err, api)
	}
}
