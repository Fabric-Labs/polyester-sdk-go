package services

import (
	"context"
	stderrors "errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/Fabric-Labs/polyester-sdk-go/auth"
	"github.com/Fabric-Labs/polyester-sdk-go/catalogs"
	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	withdrawv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/chain/withdraw/v1"
	ledgerv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/ledger/read/v1"
	transferv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/transfer/v1"
	triggersv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/triggers/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/hardening"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
	"github.com/Fabric-Labs/polyester-sdk-go/transport"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// Every request is captured after serialization on a loopback-only HTTP server.
func requestCaptureFactory(t *testing.T) (*transport.Factory, chan proto.Message) {
	t.Helper()
	requests := make(chan proto.Message, 10)
	srv := hardening.NewTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req, resp proto.Message
		switch {
		case strings.HasSuffix(r.URL.Path, "/CreateTrigger"):
			req = &triggersv1.CreateTriggerRequest{}
			resp = &triggersv1.CreateTriggerResponse{TriggerId: 5}
		case strings.HasSuffix(r.URL.Path, "/ModifyTrigger"):
			req = &triggersv1.ModifyTriggerRequest{}
			resp = &triggersv1.ModifyTriggerResponse{TriggerId: 5}
		case strings.HasSuffix(r.URL.Path, "/CreateInternalTransfer"):
			req = &transferv1.CreateInternalTransferRequest{}
			resp = &transferv1.CreateInternalTransferResponse{RequestId: "local-request", TransferId: "local-transfer"}
		case strings.HasSuffix(r.URL.Path, "/CreateTradingWithdraw"):
			req = &withdrawv1.CreateTradingWithdrawRequest{}
			resp = &withdrawv1.CreateTradingWithdrawResponse{IntentId: "local-intent"}
		case strings.HasSuffix(r.URL.Path, "/CreateWalletTradingWithdraw"):
			req = &withdrawv1.CreateWalletTradingWithdrawRequest{}
			resp = &withdrawv1.CreateWalletTradingWithdrawResponse{IntentId: "local-intent"}
		case strings.HasSuffix(r.URL.Path, "/ListTransfers"):
			req = &ledgerv1.ListTransfersRequest{}
			resp = &ledgerv1.ListTransfersResponse{}
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
			return
		}
		if err := proto.Unmarshal(body, req); err != nil {
			t.Error(err)
			return
		}
		requests <- req
		body, err = proto.Marshal(resp)
		if err != nil {
			t.Error(err)
			return
		}
		w.Header().Set("Content-Type", "application/proto")
		if _, err := w.Write(body); err != nil {
			t.Error(err)
		}
	}))
	t.Cleanup(srv.Close)
	creds, err := auth.LoadCredentials("ak_local", auth.GenerateEd25519Keypair().SecretKeyHex.Reveal(), false)
	if err != nil {
		t.Fatal(err)
	}
	cfg := transport.DefaultConfig()
	cfg.APIURL = srv.URL
	return transport.NewFactory(cfg, creds, srv.Client()), requests
}

func capturedRequest(t *testing.T, requests chan proto.Message) proto.Message {
	t.Helper()
	if len(requests) != 1 {
		t.Fatalf("captured %d requests, want exactly one", len(requests))
	}
	return <-requests
}

func requestBuilderAmount() models.AssetAmountInput {
	return models.AssetAmountFromScaled(models.MustAssetAmountScaled(1).
		WithScale(0).WithDomain(models.QuantityDomainLedgerE18).WithAssetID(7))
}

func TestPOLY8468InternalTransferDestination(t *testing.T) {
	ptr := func(s string) *string { return &s }
	for _, destination := range []string{"account", "subaccount"} {
		for _, address := range []*string{nil, ptr(""), ptr(" \t ")} {
			t.Run(destination+"/"+func() string {
				if address == nil {
					return "omitted"
				}
				return *address
			}(), func(t *testing.T) {
				factory, requests := requestCaptureFactory(t)
				service := NewInternalTransfersService(factory, nil, nil)
				var account, subaccount *string
				if destination == "account" {
					account = ptr(codecs.FormatID(42))
				} else {
					subaccount = ptr(codecs.FormatID(42))
				}
				_, err := service.Create(context.Background(), 7, requestBuilderAmount(), "local", nil, nil, account, subaccount, address, nil)
				if err != nil {
					t.Fatal(err)
				}
				req := capturedRequest(t, requests).(*transferv1.CreateInternalTransferRequest)
				want := &transferv1.CreateInternalTransferRequest{}
				if destination == "account" {
					want.Destination = &transferv1.CreateInternalTransferRequest_DestinationAccountId{DestinationAccountId: 42}
				} else {
					want.Destination = &transferv1.CreateInternalTransferRequest_DestinationSubaccountId{DestinationSubaccountId: 42}
				}
				if !proto.Equal(&transferv1.CreateInternalTransferRequest{Destination: req.Destination}, want) {
					t.Fatalf("destination=%v, want %v", req.Destination, want.Destination)
				}
			})
		}
	}
	for _, test := range []struct {
		name                  string
		account, sub, address *string
		reject                bool
	}{
		{"address only", nil, nil, ptr("0xlocal"), false},
		{"account and address", ptr(codecs.FormatID(42)), nil, ptr("0xlocal"), true},
		{"subaccount and address", nil, ptr(codecs.FormatID(42)), ptr("0xlocal"), true},
		{"account and subaccount", ptr(codecs.FormatID(42)), ptr(codecs.FormatID(43)), nil, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			factory, requests := requestCaptureFactory(t)
			_, err := NewInternalTransfersService(factory, nil, nil).Create(context.Background(), 7, requestBuilderAmount(), "local", nil, nil, test.account, test.sub, test.address, nil)
			if test.reject {
				var validation *sdkerrors.ValidationError
				if !stderrors.As(err, &validation) || len(requests) != 0 {
					t.Fatalf("err=%v calls=%d", err, len(requests))
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			req := capturedRequest(t, requests).(*transferv1.CreateInternalTransferRequest)
			if req.GetDestinationSmartAccountAddress() != "0xlocal" {
				t.Fatalf("destination=%v", req.Destination)
			}
		})
	}
}

func TestPOLY8474APIKeyWithdrawSelectors(t *testing.T) {
	ptr := func(s string) *string { return &s }
	for _, external := range []bool{false, true} {
		for _, test := range []struct {
			name    string
			account AccountScope
			sub     *string
			reject  bool
		}{
			{"omitted", nil, nil, false},
			{"account", "42", nil, true}, {"main", "main", nil, true}, {"active", "active", nil, true},
			{"dict", map[string]string{"subaccountId": "42"}, nil, true},
			{"subaccount", nil, ptr(codecs.FormatID(42)), true}, {"empty subaccount", nil, ptr(""), true},
			{"conflicting", "42", ptr(codecs.FormatID(43)), true},
		} {
			for _, defaultSub := range []*string{nil, ptr(codecs.FormatID(42))} {
				t.Run(test.name+func() string {
					if external {
						return "/external"
					}
					return "/funding"
				}()+func() string {
					if defaultSub != nil {
						return "/default"
					}
					return ""
				}(), func(t *testing.T) {
					factory, requests := requestCaptureFactory(t)
					service := NewWithdrawService(factory, defaultSub)
					deadline := uint64(1_800_000_000)
					var err error
					if external {
						_, err = service.CreateToExternalChain(context.Background(), 7, requestBuilderAmount(), []byte("local"), 42161, "0xlocal", "local", "42", &deadline, test.account, test.sub, 0)
					} else {
						_, err = service.CreateToFunding(context.Background(), 7, requestBuilderAmount(), []byte("local"), "local", "42", &deadline, test.account, test.sub, "", 0)
					}
					if test.reject {
						var validation *sdkerrors.ValidationError
						if !stderrors.As(err, &validation) || !strings.Contains(err.Error(), "API-key") || len(requests) != 0 {
							t.Fatalf("err=%v calls=%d", err, len(requests))
						}
						return
					}
					if err != nil {
						t.Fatal(err)
					}
					req := capturedRequest(t, requests).(*withdrawv1.CreateTradingWithdrawRequest)
					action := withdrawv1.TradingWithdrawAction_TO_FUNDING
					chain := uint64(0)
					if external {
						action = withdrawv1.TradingWithdrawAction_TO_EXTERNAL_CHAIN
						chain = 42161
					}
					if req.Payload.GetAction() != action || req.Payload.GetAssetId() != 7 || req.Payload.GetDestinationChainId() != chain || req.Payload.GetNonce().GetLo() != 42 || string(req.PayloadSignature) != "local" {
						t.Fatalf("request=%v", req)
					}
				})
			}
		}
	}
}

func TestPOLY8474WalletWithdrawStillRoutesScope(t *testing.T) {
	defaultSub, explicit := codecs.FormatID(42), codecs.FormatID(43)
	for _, test := range []struct {
		account AccountScope
		sub     *string
		want    uint64
	}{
		{nil, nil, 42}, {nil, &explicit, 43}, {map[string]string{"subaccountId": codecs.FormatID(44)}, nil, 44},
	} {
		factory, requests := requestCaptureFactory(t)
		deadline := uint64(1_800_000_000)
		_, err := NewWithdrawService(factory, &defaultSub).CreateWalletTradingWithdraw(context.Background(), "to_funding", 7, requestBuilderAmount(), "local", []byte("local"), "0xlocal", test.account, test.sub, 0, &deadline, "42", "", 0)
		if err != nil {
			t.Fatal(err)
		}
		req := capturedRequest(t, requests).(*withdrawv1.CreateWalletTradingWithdrawRequest)
		if req.SubaccountId != test.want {
			t.Fatalf("subaccount=%d want %d", req.SubaccountId, test.want)
		}
	}
}

func TestPOLY8471GoTransferSinceUsesMicroseconds(t *testing.T) {
	for _, since := range []*int64{nil, new(int64(0)), new(int64(1_793_922_052_911_123)), new(int64(1<<63 - 1))} {
		factory, requests := requestCaptureFactory(t)
		_, err := NewTransfersService(factory, nil, nil, nil).List(context.Background(), nil, nil, 50, false, since)
		if err != nil {
			t.Fatal(err)
		}
		req := capturedRequest(t, requests).(*ledgerv1.ListTransfersRequest)
		want := uint64(0)
		if since != nil {
			want = uint64(*since)
		}
		// ts_max_us has explicit presence: an explicit 0 would ask for an
		// empty, epoch-bounded page. The SDK sets no upper bound, so the
		// field must be absent on the wire, not zero.
		if req.TsMinUs != want || req.TsMaxUs != nil {
			t.Fatalf("request=%v want ts_min_us=%d and no ts_max_us", req, want)
		}
	}
}

func TestPOLY8471NegativeSinceRejectedBeforeTransport(t *testing.T) {
	for _, since := range []int64{-1, -1 << 63} {
		factory, requests := requestCaptureFactory(t)
		_, err := NewTransfersService(factory, nil, nil, nil).List(context.Background(), nil, nil, 50, false, &since)
		var validation *sdkerrors.ValidationError
		if !stderrors.As(err, &validation) || !strings.Contains(err.Error(), "since") || len(requests) != 0 {
			t.Errorf("since=%d err=%v calls=%d", since, err, len(requests))
		}
	}
}

func TestPOLY8513TriggerRepresentations(t *testing.T) {
	for _, modify := range []bool{false, true} {
		for _, test := range []struct {
			name          string
			distanceTicks *int64
			distanceBps   *int32
			capTicks      *int64
			capBps        *int32
			reject        bool
		}{
			{"both distances", new(int64(5)), new(int32(50)), nil, nil, true},
			{"both caps", nil, nil, new(int64(5)), new(int32(50)), true},
			{"both caps including zero", nil, nil, new(int64(5)), new(int32(0)), true},
			{"distance ticks", new(int64(5)), nil, nil, nil, false},
			{"distance bps", nil, new(int32(50)), nil, nil, false},
			{"cap ticks", nil, nil, new(int64(5)), nil, false},
			{"cap bps", nil, nil, nil, new(int32(50)), false},
			{"clear ticks", nil, nil, new(int64(0)), nil, !modify},
			{"clear bps", nil, nil, nil, new(int32(0)), !modify},
		} {
			name := test.name + "/create"
			if modify {
				name = test.name + "/modify"
			}
			t.Run(name, func(t *testing.T) {
				factory, requests := requestCaptureFactory(t)
				cats := catalogs.NewManager()
				if err := cats.HydrateSpotConfig(map[string]any{"pairs": []any{map[string]any{"symbol": "BTC-USDT", "symbol_id": float64(1), "base_quantity_scale": float64(8)}}}); err != nil {
					t.Fatal(err)
				}
				service := NewTriggersService(factory, cats, nil, nil, nil, nil, nil)
				var err error
				if modify {
					_, err = service.Modify(context.Background(), nil, codecs.FormatID(5), nil, nil, nil, codecs.ModifyTriggerOptions{TrailingDistanceTicks: test.distanceTicks, TrailingDistanceBps: test.distanceBps, MaxSlippageTicks: test.capTicks, MaxSlippageBps: test.capBps})
				} else {
					opts := codecs.CreateTriggerOptions{TrailingDistanceTicks: test.distanceTicks, TrailingDistanceBps: test.distanceBps, MaxSlippageTicks: test.capTicks, MaxSlippageBps: test.capBps}
					if opts.TrailingDistanceTicks == nil && opts.TrailingDistanceBps == nil {
						opts.TrailingDistanceBps = new(int32(100))
					}
					_, err = service.Create(context.Background(), nil, "BTC-USDT", "trailing_stop", nil, "sell", models.QtyFromDecimal("0.01"), "market", nil, "", "", nil, nil, false, opts)
				}
				if test.reject {
					var validation *sdkerrors.ValidationError
					if !stderrors.As(err, &validation) || len(requests) != 0 {
						t.Fatalf("err=%v calls=%d", err, len(requests))
					}
					return
				}
				if err != nil {
					t.Fatal(err)
				}
				var request proto.Message
				if modify {
					request = capturedRequest(t, requests)
				} else {
					request = capturedRequest(t, requests).(*triggersv1.CreateTriggerRequest).GetTrigger().GetTrailingStop()
				}
				message := request.ProtoReflect()
				for _, pair := range []struct {
					oneof, ticks, bps string
					tickValue         *int64
					bpsValue          *int32
				}{
					{"trailing_distance", "trailing_distance_ticks", "trailing_distance_bps", test.distanceTicks, test.distanceBps},
					{"max_slippage", "max_slippage_ticks", "max_slippage_bps", test.capTicks, test.capBps},
				} {
					if pair.tickValue == nil && pair.bpsValue == nil {
						continue
					}
					oneof := message.Descriptor().Oneofs().ByName(protoreflect.Name(pair.oneof))
					field := message.WhichOneof(oneof)
					wantName, wantValue := pair.ticks, int64(0)
					if pair.tickValue != nil {
						wantValue = *pair.tickValue
					} else {
						wantName = pair.bps
						wantValue = int64(*pair.bpsValue)
					}
					if field == nil || string(field.Name()) != wantName || message.Get(field).Int() != wantValue {
						t.Fatalf("request=%v want %s=%d", request, wantName, wantValue)
					}
				}
			})
		}
	}
}
