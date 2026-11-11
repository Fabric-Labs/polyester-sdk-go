package services

import (
	"context"
	stderrors "errors"
	"fmt"
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Fabric-Labs/polyester-sdk-go/auth"
	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	orderv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/orders/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/gen/orders/v1/ordersv1connect"
	triggersv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/triggers/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/gen/triggers/v1/triggersv1connect"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
	"github.com/Fabric-Labs/polyester-sdk-go/transport"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/dynamicpb"
)

func scopeCaptureFactory(t *testing.T) (*transport.Factory, chan proto.Message) {
	t.Helper()
	captures := make(chan proto.Message, 10)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/"), "/")
		if len(parts) != 2 {
			t.Errorf("unexpected RPC path %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		desc, err := protoregistry.GlobalFiles.FindDescriptorByName(protoreflect.FullName(parts[0]))
		if err != nil {
			t.Error(err)
			http.Error(w, "unknown service", 500)
			return
		}
		method := desc.(protoreflect.ServiceDescriptor).Methods().ByName(protoreflect.Name(parts[1]))
		if method == nil {
			t.Errorf("unknown method %s", parts[1])
			http.Error(w, "unknown method", 500)
			return
		}
		request := dynamicpb.NewMessage(method.Input())
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
			return
		}
		if err = proto.Unmarshal(body, request); err != nil {
			t.Error(err)
			return
		}
		captures <- request
		// Stop after request capture so response decoding cannot mask scope behavior.
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"code":"invalid_argument","message":"capture complete"}`))
	}))
	t.Cleanup(server.Close)
	creds, err := auth.LoadCredentials("ak_local", auth.GenerateEd25519Keypair().SecretKeyHex.Reveal(), false)
	if err != nil {
		t.Fatal(err)
	}
	cfg := transport.DefaultConfig()
	cfg.APIURL = server.URL
	factory := transport.NewFactory(cfg, creds, server.Client())
	return factory, captures
}

func TestExplicitEmptyScopePublicRequests(t *testing.T) {
	ptr := func(s string) *string { return &s }
	for _, defaultID := range []uint64{42, 43} {
		defaultSub := codecs.FormatID(defaultID)
		for _, control := range []struct {
			name    string
			account AccountScope
			sub     *string
			want    uint64
			refused bool
		}{
			{name: "empty", sub: ptr("")}, {name: "omitted", want: defaultID},
			{name: "main", account: "main", want: defaultID}, {name: "active", account: "active", want: defaultID},
			{name: "explicit", sub: ptr(codecs.FormatID(44)), want: 44},
			{name: "selector_id", account: codecs.FormatID(44), want: 44},
			{name: "selector_dict", account: map[string]string{"subaccountId": codecs.FormatID(44)}, want: 44},
			{name: "selector_empty", account: ""},
			{name: "conflict_main_empty", account: "main", sub: ptr(""), refused: true},
			{name: "conflict_active_explicit", account: "active", sub: ptr(codecs.FormatID(44)), refused: true},
			{name: "conflict_dict_default", account: map[string]string{"subaccountId": codecs.FormatID(44)}, sub: ptr(defaultSub), refused: true},
		} {
			t.Run(defaultSub+"/"+control.name, func(t *testing.T) {
				factory, captures := scopeCaptureFactory(t)
				cats := auditCatalog(t)
				balances := NewBalancesService(factory, cats, &defaultSub, nil, nil)
				orders := NewOrdersService(factory, cats, &defaultSub, nil, nil, nil, nil)
				guard := NewGuardSignerService(factory, &defaultSub)
				book := NewAddressBookService(factory, &defaultSub, nil, nil)
				deposit := NewDepositService(factory, &defaultSub)
				triggers := NewTriggersService(factory, cats, &defaultSub, nil, nil, nil, nil)
				ctx := context.Background()
				a, s := control.account, control.sub
				intent := scopeOrder(s)
				qty := models.QtyFromDecimal("0.01")
				price := models.PriceFromDecimal("100")
				key := models.OrderKeyByID(codecs.FormatID(9))
				var callErr error
				calls := map[string]func(){
					"balances.List":    func() { _, callErr = balances.List(ctx, a, s) },
					"balances.History": func() { _, callErr = balances.GetBalanceHistory(ctx, a, "1d", s, 0, nil) },
					"balances.Equity":  func() { _, callErr = balances.GetEquityHistory(ctx, a, "1d", s, nil, "account") },
					"orders.Create":    func() { _, callErr = orders.Create(ctx, intent, a) },
					"orders.Preview":   func() { _, callErr = orders.Preview(ctx, intent, a) },
					"orders.BatchCreate": func() {
						_, callErr = orders.BatchCreate(ctx, a, []models.CreateOrderRequest{scopeOrder(nil)}, s, nil, nil, false)
					},
					"orders.BatchCancel": func() { _, callErr = orders.BatchCancel(ctx, a, []models.BatchCancelItem{{Key: key}}, s, nil) },
					"orders.BatchReplace": func() {
						_, callErr = orders.BatchReplace(ctx, a, []models.BatchReplaceItem{{Key: key, NewQty: &qty}}, "BTC-USDT", s, nil)
					},
					"orders.Modify": func() { _, callErr = orders.Modify(ctx, a, "BTC-USDT", key, s, nil, &price, nil, nil, nil) },
					"orders.ModifyWith": func() {
						_, callErr = orders.ModifyWith(ctx, a, ModifyParams{Symbol: "BTC-USDT", Key: key, SubAccountID: s, NewPrice: &price})
					},
					"triggers.Create": func() {
						_, callErr = triggers.Create(ctx, a, "BTC-USDT", "trailing_stop", nil, "sell", qty, "market", nil, "", "", s, nil, false, codecs.CreateTriggerOptions{TrailingDistanceBps: new(int32(100))})
					},
					"triggers.Modify": func() {
						_, callErr = triggers.Modify(ctx, a, codecs.FormatID(9), s, nil, nil, codecs.ModifyTriggerOptions{TrailingDistanceBps: new(int32(100))})
					},
					"orders.ListOpen":    func() { _, callErr = orders.ListOpen(ctx, a, s, nil, nil, false, false, nil) },
					"orders.ListHistory": func() { _, callErr = orders.ListHistory(ctx, a, s, nil, nil, nil, 50, false, false, nil) },
					"orders.Cancel":      func() { _, callErr = orders.Cancel(ctx, a, models.OrderKeyByID(codecs.FormatID(9)), nil, nil, s) },
					"orders.CancelAll":   func() { _, callErr = orders.CancelAll(ctx, a, s, nil, nil, false, nil) },
					"withdraw.CreateWalletTradingWithdraw": func() {
						deadline := uint64(1000)
						_, callErr = NewWithdrawService(factory, &defaultSub).CreateWalletTradingWithdraw(ctx, "to_funding", 7, requestBuilderAmount(), "local", []byte("local"), "0xlocal", a, s, 0, &deadline, "42", "", 0)
					},
					"triggers.Cancel":     func() { _, callErr = triggers.Cancel(ctx, a, codecs.FormatID(9), s) },
					"triggers.Pause":      func() { _, callErr = triggers.Pause(ctx, a, codecs.FormatID(9), s) },
					"triggers.Resume":     func() { _, callErr = triggers.Resume(ctx, a, codecs.FormatID(9), s, nil, nil) },
					"triggers.ListEvents": func() { _, callErr = triggers.ListEvents(ctx, a, codecs.FormatID(9), s, 50, nil, nil) },
					"orders.CancelAllWith": func() {
						_, callErr = orders.CancelAllWith(ctx, a, CancelAllParams{SubAccountID: s})
					},
					"book.CreateEntry": func() {
						_, callErr = book.CreateEntry(ctx, a, s, "local", "", ptr("0xlocal"), new(uint32(1)), nil, nil, nil)
					},
					"guard.SignAction": func() {
						_, callErr = guard.SignProtectedAction(ctx, "withdraw", a, s, nil, nil, nil, nil)
					},
					"orders.CancelAllAfter":        func() { _, callErr = orders.CancelAllAfter(ctx, a, 60, s, nil, nil, nil) },
					"orders.GetBatchReplaceStatus": func() { _, callErr = orders.GetBatchReplaceStatus(ctx, a, codecs.FormatID(9), s) },
					"deposit.CreateAddress":        func() { _, callErr = deposit.CreateAddress(ctx, 1, a, s) },
					"deposit.GetAddress":           func() { _, callErr = deposit.ListAddresses(ctx, 1, a, s) },
					"guard.CreateWallet":           func() { _, callErr = guard.CreateWallet(ctx, a, s) },
					"guard.GetStatus":              func() { _, callErr = guard.GetStatus(ctx, a, s) },
					"guard.RotateWallet":           func() { _, callErr = guard.RotateWallet(ctx, a, s) },
					"guard.ExportWallet":           func() { _, callErr = guard.ExportWallet(ctx, a, s) },
					"guard.BatchSign":              func() { _, callErr = guard.BatchSignProtectedActions(ctx, nil, a, s) },
					"book.ListEntries":             func() { _, callErr = book.ListEntries(ctx, a, s, nil, 50, nil) },
					"book.ListCounterparties":      func() { _, callErr = book.ListTransferCounterparties(ctx, a, s, nil, nil, 50) },
					"book.ListDestinations":        func() { _, callErr = book.ListTransferDestinations(ctx, a, s, "", 50, nil) },
					"book.ListWhitelist":           func() { _, callErr = book.ListInternalTransferWhitelistEntries(ctx, a, s, 50, nil) },
					"book.GetWhitelist":            func() { _, callErr = book.GetWithdrawWhitelistView(ctx, a, s) },
					"book.GetView":                 func() { _, callErr = book.GetView(ctx, a, s, 50, 0) },
					"book.CreateTag":               func() { _, callErr = book.CreateTag(ctx, a, s, "local", "blue") },
					"apiKeys.List":                 func() { _, callErr = NewApiKeysService(factory, &defaultSub, nil, nil).List(ctx, a, s) },
					"fees.Get":                     func() { _, callErr = NewFeeService(factory, cats, &defaultSub).GetSpotFeeRates(ctx, a, s, nil) },
					"limits.Get":                   func() { _, callErr = NewRateLimitService(factory, &defaultSub).GetTradingRateLimits(ctx, a, s) },
					"transfers.List": func() {
						_, callErr = NewTransfersService(factory, &defaultSub, nil, nil).List(ctx, a, s, 50, false, nil)
					},
					"triggers.List": func() { _, callErr = triggers.List(ctx, a, s, nil, nil, 50, nil) },
					"triggers.Get":  func() { _, callErr = triggers.Get(ctx, a, codecs.FormatID(9), s) },
					"internalTransfers.Create": func() {
						_, callErr = NewInternalTransfersService(factory, cats, &defaultSub).Create(ctx, 7, requestBuilderAmount(), "local", a, s, ptr(codecs.FormatID(8)), nil, nil, nil)
					},
				}

				for name, call := range calls {
					t.Run(name, func(t *testing.T) {
						call()
						if control.refused {
							var validation *sdkerrors.ValidationError
							if !stderrors.As(callErr, &validation) || !strings.Contains(callErr.Error(), "not both") {
								t.Errorf("got %v, want selector conflict refusal", callErr)
							}
							if len(captures) != 0 {
								<-captures
								t.Error("conflicting scope reached transport")
							}
							return
						}
						if len(captures) != 1 {
							t.Fatalf("captured %d requests, want one; public helper did not reach transport", len(captures))
						}
						msg := (<-captures).ProtoReflect()
						field := msg.Descriptor().Fields().ByName("subaccount_id")
						if field == nil {
							t.Fatal("request has no subaccount_id")
						}
						if got := msg.Get(field).Uint(); got != control.want {
							t.Fatalf("subaccount_id=%d, want %d", got, control.want)
						}
						if control.want == 0 && msg.Has(field) {
							t.Fatal("explicit empty must omit subaccount_id")
						}
					})
				}
			})
		}
	}
}

func TestExplicitEmptyRequiredSubaccount(t *testing.T) {
	defaultSub, empty := codecs.FormatID(42), ""
	factory, captures := scopeCaptureFactory(t)
	service := NewSubAccountsService(factory, &defaultSub, nil, nil)
	ctx := context.Background()
	calls := map[string]func() error{
		"Get": func() error {
			_, err := service.Get(ctx, nil, &empty, false, false, false, false, false, "")
			return err
		},
		"ListMembers":  func() error { _, err := service.ListMembers(ctx, nil, &empty); return err },
		"ListActivity": func() error { _, err := service.ListActivity(ctx, nil, &empty, 50, nil); return err },
	}
	for name, call := range calls {
		t.Run(name, func(t *testing.T) {
			var validation *sdkerrors.ValidationError
			if err := call(); !stderrors.As(err, &validation) {
				t.Errorf("got %v, want validation error for missing required subaccount", err)
			}
			if len(captures) != 0 {
				<-captures
				t.Error("empty required subaccount reached transport")
			}
		})
	}
	scoped := ScopedSubAccount{DefaultSubAccountID: &defaultSub}
	for _, override := range []any{"", &empty} {
		sub, err := scoped.ResolveSubAccountID(override, nil)
		if err != nil || sub != nil {
			t.Fatalf("explicit empty: sub=%v err=%v", sub, err)
		}
		if _, err := scoped.ResolveSubAccountID(override, "main"); err == nil {
			t.Fatal("empty override with account selector must be refused")
		}
	}
	type namedID string
	for _, override := range []any{namedID(""), namedID("  ")} {
		sub, err := scoped.ResolveSubAccountID(override, nil)
		if err != nil || sub != nil {
			t.Fatalf("named empty %q: sub=%v err=%v, want it omitted, not the default", override, sub, err)
		}
	}
	var omitted *string
	sub, err := scoped.ResolveSubAccountID(omitted, nil)
	if err != nil || sub == nil || *sub != defaultSub {
		t.Fatalf("typed nil must use default: sub=%v err=%v", sub, err)
	}
}

func scopeOrder(sub *string) models.CreateOrderRequest {
	price := models.PriceFromDecimal("100")
	return models.CreateOrderRequest{Symbol: new("BTC-USDT"), Side: "buy", OrderType: "limit", Price: &price, Qty: models.QtyFromDecimal("0.01"), SubAccountID: sub}
}

func TestOrderScopeConflicts(t *testing.T) {
	for _, account := range []string{"main", "active"} {
		for _, override := range []string{"", codecs.FormatID(44)} {
			for _, preview := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%q/preview=%t", account, override, preview), func(t *testing.T) {
					factory, captures := scopeCaptureFactory(t)
					defaultSub := codecs.FormatID(42)
					orders := NewOrdersService(factory, auditCatalog(t), &defaultSub, nil, nil, nil, nil)
					req := scopeOrder(&override)
					var err error
					if preview {
						_, err = orders.Preview(context.Background(), req, account)
					} else {
						_, err = orders.Create(context.Background(), req, account)
					}
					var validation *sdkerrors.ValidationError
					if !stderrors.As(err, &validation) {
						t.Errorf("got %v, want scope conflict validation", err)
					}
					if len(captures) != 0 {
						t.Error("conflicting override reached transport")
					}
				})
			}
		}
	}
}

func TestBatchCreateRefusesItemScope(t *testing.T) {
	for _, override := range []string{"", codecs.FormatID(42), codecs.FormatID(44)} {
		for _, top := range []*string{nil, new(""), new(codecs.FormatID(44))} {
			t.Run(fmt.Sprintf("%q/top=%v", override, top), func(t *testing.T) {
				factory, captures := scopeCaptureFactory(t)
				defaultSub := codecs.FormatID(42)
				orders := NewOrdersService(factory, auditCatalog(t), &defaultSub, nil, nil, nil, nil)
				_, err := orders.BatchCreate(context.Background(), nil, []models.CreateOrderRequest{scopeOrder(nil), scopeOrder(&override)}, top, nil, nil, false)
				var validation *sdkerrors.ValidationError
				if !stderrors.As(err, &validation) {
					t.Errorf("got %v, want item scope validation", err)
				}
				if len(captures) != 0 {
					t.Error("item override reached transport")
				}
			})
		}
	}
}

func TestStopAllTradingScopeOnEveryRequest(t *testing.T) {
	ptr := func(s string) *string { return &s }
	defaultSub := codecs.FormatID(42)
	for _, control := range []struct {
		name    string
		account AccountScope
		sub     *string
		want    uint64
	}{
		{name: "empty", sub: ptr("")},
		{name: "omitted", want: 42},
		{name: "main", account: "main", want: 42},
		{name: "explicit", sub: ptr(codecs.FormatID(44)), want: 44},
		{name: "selector_id", account: codecs.FormatID(44), want: 44},
	} {
		t.Run(control.name, func(t *testing.T) {
			type captured struct {
				method string
				sub    *uint64
			}
			var seen []captured
			mux := http.NewServeMux()
			mux.HandleFunc(triggersv1connect.TriggersServiceListTriggersProcedure, func(w http.ResponseWriter, r *http.Request) {
				var req triggersv1.ListTriggersRequest
				readScopeRequest(t, r, &req)
				seen = append(seen, captured{"ListTriggers", req.SubaccountId})
				writeScopeResponse(t, w, &triggersv1.ListTriggersResponse{Triggers: []*triggersv1.Trigger{{TriggerId: 9}}})
			})
			mux.HandleFunc(triggersv1connect.TriggersServiceCancelTriggerProcedure, func(w http.ResponseWriter, r *http.Request) {
				var req triggersv1.CancelTriggerRequest
				readScopeRequest(t, r, &req)
				seen = append(seen, captured{"CancelTrigger", req.SubaccountId})
				writeScopeResponse(t, w, &triggersv1.CancelTriggerResponse{TriggerId: 9})
			})
			mux.HandleFunc(ordersv1connect.OrdersServiceCancelAllOrdersProcedure, func(w http.ResponseWriter, r *http.Request) {
				var req orderv1.CancelAllOrdersRequest
				readScopeRequest(t, r, &req)
				seen = append(seen, captured{"CancelAllOrders", req.SubaccountId})
				writeScopeResponse(t, w, &orderv1.CancelAllOrdersResponse{})
			})
			server := httptest.NewServer(mux)
			t.Cleanup(server.Close)
			creds, err := auth.LoadCredentials("ak_local", auth.GenerateEd25519Keypair().SecretKeyHex.Reveal(), false)
			if err != nil {
				t.Fatal(err)
			}
			cfg := transport.DefaultConfig()
			cfg.APIURL = server.URL
			factory := transport.NewFactory(cfg, creds, server.Client())
			cats := auditCatalog(t)
			triggers := NewTriggersService(factory, cats, &defaultSub, nil, nil, nil, nil)
			orders := NewOrdersService(factory, cats, &defaultSub, nil, nil, nil, nil)
			_, _ = StopAllTrading(context.Background(), triggers, orders, control.account, StopAllTradingScope{SubAccountID: control.sub})
			if len(seen) != 3 {
				t.Fatalf("captured %+v, want list, cancel and cancel-all", seen)
			}
			for _, call := range seen {
				got := uint64(0)
				if call.sub != nil {
					got = *call.sub
				}
				if got != control.want || (control.want == 0 && call.sub != nil) {
					t.Errorf("%s subaccount_id=%v, want %d", call.method, call.sub, control.want)
				}
			}
		})
	}
}

func readScopeRequest(t *testing.T, r *http.Request, msg proto.Message) {
	t.Helper()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		t.Error(err)
		return
	}
	if err := proto.Unmarshal(body, msg); err != nil {
		t.Error(err)
	}
}

func writeScopeResponse(t *testing.T, w http.ResponseWriter, msg proto.Message) {
	t.Helper()
	body, err := proto.Marshal(msg)
	if err != nil {
		t.Error(err)
		return
	}
	w.Header().Set("Content-Type", "application/proto")
	if _, err := w.Write(body); err != nil {
		t.Error(err)
	}
}

func TestBatchSignRefusesActionScope(t *testing.T) {
	for _, key := range []string{"sub_account_id", "subaccount_id", "subaccountId", "account"} {
		for _, top := range []*string{nil, new(""), new(codecs.FormatID(44))} {
			t.Run(fmt.Sprintf("%s/top=%v", key, top), func(t *testing.T) {
				factory, captures := scopeCaptureFactory(t)
				defaultSub := codecs.FormatID(42)
				guard := NewGuardSignerService(factory, &defaultSub)
				actions := []map[string]any{{"action": "withdraw", key: codecs.FormatID(44)}}
				_, err := guard.BatchSignProtectedActions(context.Background(), actions, nil, top)
				var validation *sdkerrors.ValidationError
				if !stderrors.As(err, &validation) {
					t.Errorf("got %v, want action scope validation", err)
				}
				if len(captures) != 0 {
					t.Error("action scope reached transport")
				}
			})
		}
	}
}
