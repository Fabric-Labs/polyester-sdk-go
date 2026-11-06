package services

import (
	"connectrpc.com/connect"
	"context"
	authv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/auth/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/gen/auth/v1/authv1connect"
	orderv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/orders/v1"
	triggersv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/triggers/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/realtime"
	"google.golang.org/protobuf/proto"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Fabric-Labs/polyester-sdk-go/auth"
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
	"github.com/Fabric-Labs/polyester-sdk-go/transport"

	"github.com/Fabric-Labs/polyester-sdk-go/hardening"
)

func TestOrderCreateNeverResendsUnknownOutcome(t *testing.T) {
	for _, clientOrderID := range []*string{nil, new("reconcile-first")} {
		for _, tc := range []struct {
			status            int
			contentType, body string
		}{
			{504, "application/problem+json", `{"code":"ORDER_CREATE_OUTCOME_UNKNOWN"}`},
			{502, "text/plain", ""},
			{503, "text/html", "<html>Unavailable</html>"},
			{504, "application/json", `{"code":"deadline_exceeded"}`},
			{503, "application/json", `{"code":"unavailable"}`},
			{504, "text/html", "<html>Gateway Timeout</html>"},
			{503, "application/json", `{"code":"unavailable","details":[{"type":"orders.v1.ErrorDetail","value":"CK8C"}]}`},
			{504, "application/json", `{"code":"deadline_exceeded","details":[{"type":"orders.v1.ErrorDetail","value":"CK8C"}]}`},
		} {
			t.Run(tc.contentType, func(t *testing.T) {
				var calls atomic.Int32
				srv := hardening.NewTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.URL.Path != "/orders.v1.OrdersService/CreateOrder" {
						t.Errorf("unexpected path %s", r.URL.Path)
					}
					calls.Add(1)
					w.Header().Set("Content-Type", tc.contentType)
					w.WriteHeader(tc.status)
					_, _ = w.Write([]byte(tc.body))
				}))
				defer srv.Close()
				creds, err := auth.LoadCredentials("ak_test", auth.GenerateEd25519Keypair().SecretKeyHex, false)
				if err != nil {
					t.Fatal(err)
				}
				cfg := transport.DefaultConfig()
				cfg.APIURL = srv.URL
				service := NewOrdersService(transport.NewFactory(cfg, creds, nil), auditCatalog(t), nil, nil, nil, nil, nil)
				symbol := "BTC-USDT"
				_, err = service.Create(context.Background(), models.CreateOrderRequest{
					ClientOrderID: clientOrderID, Symbol: &symbol, Side: "buy", OrderType: "market", Qty: models.QtyFromDecimal("0.001"),
				}, nil)
				if sdkerrors.IsRetryable(err) {
					t.Fatalf("must reconcile first: %v", err)
				}
				if strings.Contains(tc.body, "OUTCOME_UNKNOWN") || strings.Contains(tc.body, "details") {
					if _, ok := err.(*sdkerrors.APIError); !ok || sdkerrors.IsRetryable(err) {
						t.Fatalf("class=%T retryable=%v", err, sdkerrors.IsRetryable(err))
					}
				}
				if !sdkerrors.MutationOutcomeUnknown(err) || calls.Load() != 1 {
					t.Fatalf("calls=%d error=%T %v", calls.Load(), err, err)
				}
			})
		}
	}
}

func TestGateway504ReadRetriesRemainSafe(t *testing.T) {
	for _, failure := range []struct {
		status     int
		kind, body string
	}{{0, "timeout", "timeout"}, {504, "text/html", "Gateway Timeout"}, {502, "text/plain", ""}, {503, "text/html", "Service Unavailable"}, {504, "application/json", `{"code":"deadline_exceeded"}`}, {503, "application/json", `{"code":"unavailable"}`}} {
		t.Run(failure.body, func(t *testing.T) {
			for _, operation := range []string{"trigger", "batch-status", "snapshot"} {
				t.Run(operation, func(t *testing.T) {
					var calls atomic.Int32
					srv := hardening.NewTestServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
						if calls.Add(1) == 1 {
							if failure.status == 0 {
								time.Sleep(200 * time.Millisecond)
								return
							}
							w.Header().Set("Content-Type", failure.kind)
							w.WriteHeader(failure.status)
							_, _ = w.Write([]byte(failure.body))
							return
						}
						w.Header().Set("Content-Type", "application/proto")
						var message proto.Message
						switch operation {
						case "trigger":
							message = &triggersv1.GetTriggerResponse{Trigger: &triggersv1.Trigger{TriggerId: 9}}
						case "batch-status":
							message = &orderv1.GetBatchReplaceStatusResponse{BatchRequestId: 77, AcceptedCount: 1, AdmissionStatus: orderv1.BatchReplaceAdmissionStatus_BATCH_REPLACE_ADMISSION_STATUS_ADMITTED, Items: []*orderv1.BatchReplaceStatusItem{{Phase: orderv1.BatchReplacePhase_BATCH_REPLACE_PHASE_REJECTED, OldOrderId: 1, Code: "POST_ONLY_CROSS"}}}
						default:
							message = &authv1.MeResponse{}
						}
						raw, err := proto.Marshal(message)
						if err != nil {
							t.Error(err)
							return
						}
						_, _ = w.Write(raw)
					}))
					defer srv.Close()
					creds, err := auth.LoadCredentials("ak_test", auth.GenerateEd25519Keypair().SecretKeyHex, false)
					if err != nil {
						t.Fatal(err)
					}
					cfg := transport.DefaultConfig()
					cfg.APIURL = srv.URL
					factory := transport.NewFactory(cfg, creds, nil)
					if failure.status == 0 {
						factory.HTTP.Timeout = 50 * time.Millisecond
					}
					ctx, cancel := context.WithTimeout(context.Background(), time.Second)
					defer cancel()
					switch operation {
					case "trigger":
						_, err = NewTriggersService(factory, nil, nil, nil, nil, nil, nil).Get(ctx, nil, "9", nil)
					case "batch-status":
						_, err = NewOrdersService(factory, nil, nil, nil, nil, nil, nil).WaitBatchReplace(ctx, nil, "77", nil)
					default:
						client := authv1connect.NewAuthServiceClient(factory.HTTP, srv.URL, factory.ConnectOptions(false)...)
						stream := realtime.NewSnapshotThenStream[int, int](realtime.SnapshotThenStreamConfig[int, int]{
							FetchSnapshot: func(ctx context.Context) (int, error) {
								_, readErr := client.Me(ctx, connect.NewRequest(&authv1.MeRequest{}))
								return 1, transport.MapError(readErr)
							},
							ApplySnapshot: func(int, []int) {}, ApplyLivePublications: func([]int) {}, ReadPublication: func(int) []int { return nil }, Decode: func([]byte) (int, error) { return 0, nil },
						})
						defer stream.Close()
						stream.RequestSnapshotRefresh(ctx)
						for !stream.IsReady() && ctx.Err() == nil {
							time.Sleep(time.Millisecond)
						}
						if !stream.IsReady() {
							t.Fatalf("snapshot did not recover: %v", stream.Err())
						}
					}
					if err != nil || calls.Load() != 2 {
						t.Fatalf("calls=%d err=%v", calls.Load(), err)
					}
				})
			}
		})
	}
}

func TestClientTimeoutNeverReplaysCreate(t *testing.T) {
	var calls atomic.Int32
	srv := hardening.NewTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		time.Sleep(200 * time.Millisecond)
	}))
	defer srv.Close()
	creds, err := auth.LoadCredentials("ak_test", auth.GenerateEd25519Keypair().SecretKeyHex, false)
	if err != nil {
		t.Fatal(err)
	}
	cfg := transport.DefaultConfig()
	cfg.APIURL = srv.URL
	factory := transport.NewFactory(cfg, creds, nil)
	service := NewOrdersService(factory, auditCatalog(t), nil, nil, nil, nil, nil)
	symbol := "BTC-USDT"
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err = service.Create(ctx, models.CreateOrderRequest{Symbol: &symbol, Side: "buy", OrderType: "market", Qty: models.QtyFromDecimal("0.001")}, nil)
	if !sdkerrors.MutationOutcomeUnknown(err) || sdkerrors.IsRetryable(err) || calls.Load() != 1 {
		t.Fatalf("calls=%d error=%T %v", calls.Load(), err, err)
	}
}
