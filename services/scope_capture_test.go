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
	creds, err := auth.LoadCredentials("ak_local", auth.GenerateEd25519Keypair().SecretKeyHex, false)
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
		}{
			{"empty", nil, ptr(""), 0}, {"omitted", nil, nil, defaultID},
			{"main", "main", nil, defaultID}, {"active", "active", nil, defaultID},
			{"explicit", nil, ptr(codecs.FormatID(44)), 44},
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
				calls := map[string]func(){
					"balances.List":    func() { _, _ = balances.List(ctx, a, s) },
					"balances.History": func() { _, _ = balances.GetBalanceHistory(ctx, a, "1d", s, 0, nil) },
					"balances.Equity":  func() { _, _ = balances.GetEquityHistory(ctx, a, "1d", s, nil, "account") },
					"orders.Create":    func() { _, _ = orders.Create(ctx, intent, a) },
					"orders.Preview":   func() { _, _ = orders.Preview(ctx, intent, a) },
					"orders.BatchCreate": func() {
						_, _ = orders.BatchCreate(ctx, a, []models.CreateOrderRequest{scopeOrder(nil)}, s, nil, nil, false)
					},
					"orders.BatchCancel": func() { _, _ = orders.BatchCancel(ctx, a, []models.BatchCancelItem{{Key: key}}, s, nil) },
					"orders.BatchReplace": func() {
						_, _ = orders.BatchReplace(ctx, a, []models.BatchReplaceItem{{Key: key, NewQty: &qty}}, "BTC-USDT", s, nil)
					},
					"orders.Modify": func() { _, _ = orders.Modify(ctx, a, "BTC-USDT", key, s, nil, &price, nil, nil, nil) },
					"orders.ModifyWith": func() {
						_, _ = orders.ModifyWith(ctx, a, ModifyParams{Symbol: "BTC-USDT", Key: key, SubAccountID: s, NewPrice: &price})
					},
					"triggers.Create": func() {
						_, _ = triggers.Create(ctx, a, "BTC-USDT", "trailing_stop", nil, "sell", qty, "market", nil, "", "gtc", s, nil, false, codecs.CreateTriggerOptions{TrailingDistanceBps: new(int32(100))})
					},
					"triggers.Modify": func() {
						_, _ = triggers.Modify(ctx, a, codecs.FormatID(9), s, nil, nil, codecs.ModifyTriggerOptions{TrailingDistanceBps: new(int32(100))})
					},
					"orders.ListOpen":    func() { _, _ = orders.ListOpen(ctx, a, s, nil, nil, false, false, nil) },
					"orders.ListHistory": func() { _, _ = orders.ListHistory(ctx, a, s, nil, nil, nil, 50, false, false, nil) },
					"orders.Cancel":      func() { _, _ = orders.Cancel(ctx, a, models.OrderKeyByID(codecs.FormatID(9)), nil, nil, s) },
					"orders.CancelAll":   func() { _, _ = orders.CancelAll(ctx, a, s, nil, nil, false, nil) },
					"withdraw.CreateWalletTradingWithdraw": func() {
						deadline := uint64(1000)
						_, _ = NewWithdrawService(factory, &defaultSub).CreateWalletTradingWithdraw(ctx, "to_funding", 7, requestBuilderAmount(), "local", []byte("local"), "0xlocal", a, s, 0, &deadline, "42", "", 0)
					},
					"triggers.Cancel":              func() { _, _ = triggers.Cancel(ctx, a, codecs.FormatID(9), s) },
					"triggers.Pause":               func() { _, _ = triggers.Pause(ctx, a, codecs.FormatID(9), s) },
					"triggers.Resume":              func() { _, _ = triggers.Resume(ctx, a, codecs.FormatID(9), s, nil, nil) },
					"triggers.ListEvents":          func() { _, _ = triggers.ListEvents(ctx, a, codecs.FormatID(9), s, 50, nil, nil) },
					"orders.CancelAllAfter":        func() { _, _ = orders.CancelAllAfter(ctx, a, 60, s, nil, nil, nil) },
					"orders.GetBatchReplaceStatus": func() { _, _ = orders.GetBatchReplaceStatus(ctx, a, codecs.FormatID(9), s) },
					"deposit.CreateAddress":        func() { _, _ = deposit.CreateAddress(ctx, 1, a, s) },
					"deposit.GetAddress":           func() { _, _ = deposit.ListAddresses(ctx, 1, a, s) },
					"guard.CreateWallet":           func() { _, _ = guard.CreateWallet(ctx, a, s) },
					"guard.GetStatus":              func() { _, _ = guard.GetStatus(ctx, a, s) },
					"guard.RotateWallet":           func() { _, _ = guard.RotateWallet(ctx, a, s) },
					"guard.ExportWallet":           func() { _, _ = guard.ExportWallet(ctx, a, s) },
					"guard.BatchSign":              func() { _, _ = guard.BatchSignProtectedActions(ctx, nil, a, s) },
					"book.ListEntries":             func() { _, _ = book.ListEntries(ctx, a, s, nil, 50, nil) },
					"book.ListCounterparties":      func() { _, _ = book.ListTransferCounterparties(ctx, a, s, nil, nil, 50) },
					"book.ListDestinations":        func() { _, _ = book.ListTransferDestinations(ctx, a, s, "", 50, nil) },
					"book.ListWhitelist":           func() { _, _ = book.ListInternalTransferWhitelistEntries(ctx, a, s, 50, nil) },
					"book.GetWhitelist":            func() { _, _ = book.GetWithdrawWhitelistView(ctx, a, s) },
					"book.GetView":                 func() { _, _ = book.GetView(ctx, a, s, 50, 0) },
					"book.CreateTag":               func() { _, _ = book.CreateTag(ctx, a, s, "local", "blue") },
					"apiKeys.List":                 func() { _, _ = NewApiKeysService(factory, &defaultSub, nil, nil).List(ctx, a, s) },
					"fees.Get":                     func() { _, _ = NewFeeService(factory, cats, &defaultSub).GetSpotFeeRates(ctx, a, s, nil) },
					"limits.Get":                   func() { _, _ = NewRateLimitService(factory, &defaultSub).GetTradingRateLimits(ctx, a, s) },
					"transfers.List":               func() { _, _ = NewTransfersService(factory, &defaultSub, nil, nil).List(ctx, a, s, 50, false, nil) },
					"triggers.List":                func() { _, _ = triggers.List(ctx, a, s, nil, nil, 50, nil) },
					"triggers.Get":                 func() { _, _ = triggers.Get(ctx, a, codecs.FormatID(9), s) },
					"internalTransfers.Create": func() {
						_, _ = NewInternalTransfersService(factory, cats, &defaultSub).Create(ctx, 7, requestBuilderAmount(), "local", a, s, ptr(codecs.FormatID(8)), nil, nil, nil)
					},
				}

				for name, call := range calls {
					t.Run(name, func(t *testing.T) {
						call()
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
