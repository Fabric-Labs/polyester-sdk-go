package services

import (
	"context"
	"encoding/hex"
	"fmt"
	"net/http"
	"reflect"
	"strconv"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/Fabric-Labs/polyester-sdk-go/auth"
	"github.com/Fabric-Labs/polyester-sdk-go/codecs/decode"
	authv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/auth/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/gen/auth/v1/authv1connect"
	orders "github.com/Fabric-Labs/polyester-sdk-go/gen/orders/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/gen/orders/v1/ordersv1connect"
	"github.com/Fabric-Labs/polyester-sdk-go/hardening"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
	"github.com/Fabric-Labs/polyester-sdk-go/transport"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Typed reads keep contract fields the server sends: an API key's policy,
// subaccount, IP allowlist and expiry (POLY-9894), an order's batch replace
// ID (POLY-9896), and each address-book view entry's own revision
// (POLY-9893). Every read goes through the public service over both wire
// formats; the streams decode through the same decoders.

var retentionWires = []string{"binary", "json"}

type retentionHandler struct {
	ordersv1connect.UnimplementedOrdersReadServiceHandler
	authv1connect.UnimplementedApiKeyServiceHandler
	authv1connect.UnimplementedAddressBookServiceHandler
	key   *authv1.ApiKey
	order *orders.Order
}

func (h retentionHandler) GetApiKey(context.Context, *connect.Request[authv1.GetApiKeyRequest]) (*connect.Response[authv1.GetApiKeyResponse], error) {
	return connect.NewResponse(&authv1.GetApiKeyResponse{ApiKey: h.key}), nil
}

func (h retentionHandler) ListApiKeys(context.Context, *connect.Request[authv1.ListApiKeysRequest]) (*connect.Response[authv1.ListApiKeysResponse], error) {
	return connect.NewResponse(&authv1.ListApiKeysResponse{ApiKeys: []*authv1.ApiKey{h.key, absentKey()}}), nil
}

func (h retentionHandler) GetOrder(context.Context, *connect.Request[orders.GetOrderRequest]) (*connect.Response[orders.GetOrderResponse], error) {
	return connect.NewResponse(&orders.GetOrderResponse{Order: h.order}), nil
}

func (h retentionHandler) GetOpenOrders(context.Context, *connect.Request[orders.GetOpenOrdersRequest]) (*connect.Response[orders.GetOpenOrdersResponse], error) {
	return connect.NewResponse(&orders.GetOpenOrdersResponse{Orders: []*orders.Order{h.order}}), nil
}

func (h retentionHandler) GetOrderHistory(context.Context, *connect.Request[orders.GetOrderHistoryRequest]) (*connect.Response[orders.GetOrderHistoryResponse], error) {
	return connect.NewResponse(&orders.GetOrderHistoryResponse{Orders: []*orders.Order{h.order}}), nil
}

func (retentionHandler) GetAddressBookView(context.Context, *connect.Request[authv1.GetAddressBookViewRequest]) (*connect.Response[authv1.GetAddressBookViewResponse], error) {
	return connect.NewResponse(addressBookViewFixture()), nil
}

func newRetentionFactory(t *testing.T, wire string, h retentionHandler) *transport.Factory {
	t.Helper()
	mux := http.NewServeMux()
	path, handler := ordersv1connect.NewOrdersReadServiceHandler(h)
	mux.Handle(path, handler)
	path, handler = authv1connect.NewApiKeyServiceHandler(h)
	mux.Handle(path, handler)
	path, handler = authv1connect.NewAddressBookServiceHandler(h)
	mux.Handle(path, handler)
	server := hardening.NewTestServer(mux)
	t.Cleanup(server.Close)
	creds, err := auth.LoadCredentials("ak_test", auth.GenerateEd25519Keypair().SecretKeyHex.Reveal(), false)
	if err != nil {
		t.Fatal(err)
	}
	cfg := transport.DefaultConfig()
	cfg.APIURL = server.URL
	cfg.WireFormat = transport.WireFormatFromString(wire)
	return transport.NewFactory(cfg, creds, nil)
}

// --- POLY-9894: API-key policy, subaccount, IP allowlist and expiry --------

var retentionPublicKey = func() []byte {
	b := make([]byte, 32)
	for i := range b {
		b[i] = byte(i)
	}
	return b
}()

func populatedKey(policyID uint64, expires time.Time) *authv1.ApiKey {
	subaccount := uint64(42)
	return &authv1.ApiKey{
		KeyId:            "ak_11111111111111111111111111111111",
		Label:            "bot",
		Status:           authv1.ApiKeyStatus_ACTIVE,
		SubaccountId:     &subaccount,
		PolicyId:         &policyID,
		IpWhitelist:      []string{"192.0.2.1", "198.51.100.0/24"},
		PublicKeyEd25519: retentionPublicKey,
		ExpiresAt:        timestamppb.New(expires),
		Revision:         3,
	}
}

func absentKey() *authv1.ApiKey {
	return &authv1.ApiKey{KeyId: "ak_22222222222222222222222222222222", Label: "plain", Revision: 1}
}

type keyCase struct {
	policyID uint64
	policy   string // public ID, as ApiPolicy.PolicyID
	expires  time.Time
}

var keyCases = []keyCase{
	{7, "8", time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC)},
	{19, "L", time.Date(2100, 12, 31, 23, 59, 59, 123_456_789, time.UTC)},
}

func requirePopulatedKey(t *testing.T, label string, got *models.ApiKeySummary, c keyCase) {
	t.Helper()
	if got == nil {
		t.Fatalf("%s: key missing", label)
	}
	if got.KeyID != "ak_11111111111111111111111111111111" || got.Label != "bot" || got.Revision != 3 {
		t.Fatalf("%s: identity %+v", label, got)
	}
	if got.PolicyID != c.policy {
		t.Errorf("%s: PolicyID = %q, want %q", label, got.PolicyID, c.policy)
	}
	if got.SubaccountID != "j" {
		t.Errorf("%s: SubaccountID = %q, want %q", label, got.SubaccountID, "j")
	}
	if !reflect.DeepEqual(got.IPWhitelist, []string{"192.0.2.1", "198.51.100.0/24"}) {
		t.Errorf("%s: IPWhitelist = %#v", label, got.IPWhitelist)
	}
	if got.ExpiresAt == nil || !got.ExpiresAt.Equal(c.expires) {
		t.Errorf("%s: ExpiresAt = %v, want %v", label, got.ExpiresAt, c.expires)
	}
	if got.PublicKeyEd25519 != hex.EncodeToString(retentionPublicKey) {
		t.Errorf("%s: PublicKeyEd25519 = %q", label, got.PublicKeyEd25519)
	}
}

func requireAbsentKey(t *testing.T, label string, got models.ApiKeySummary) {
	t.Helper()
	if got.KeyID != "ak_22222222222222222222222222222222" {
		t.Fatalf("%s: identity %+v", label, got)
	}
	if got.PolicyID != "" || got.SubaccountID != "" || got.IPWhitelist != nil || got.ExpiresAt != nil || got.PublicKeyEd25519 != "" {
		t.Errorf("%s: absent fields read %+v", label, got)
	}
}

func TestApiKeyReadsKeepPolicySubaccountAllowlistAndExpiry(t *testing.T) {
	for _, wire := range retentionWires {
		for _, c := range keyCases {
			t.Run(fmt.Sprintf("%s/policy-%d", wire, c.policyID), func(t *testing.T) {
				factory := newRetentionFactory(t, wire, retentionHandler{key: populatedKey(c.policyID, c.expires)})
				svc := NewApiKeysService(factory, nil, nil, nil)
				ctx := context.Background()

				got, err := svc.Get(ctx, "ak_11111111111111111111111111111111")
				if err != nil {
					t.Fatal(err)
				}
				requirePopulatedKey(t, "Get", got, c)

				listed, err := svc.List(ctx, nil, nil)
				if err != nil {
					t.Fatal(err)
				}
				if len(listed.Keys) != 2 {
					t.Fatalf("List keys = %d", len(listed.Keys))
				}
				requirePopulatedKey(t, "List[0]", &listed.Keys[0], c)
				requireAbsentKey(t, "List[1]", listed.Keys[1])
			})
		}
	}
}

func TestApiKeyStreamKeepsPolicySubaccountAllowlistAndExpiry(t *testing.T) {
	for _, c := range keyCases {
		payload, err := proto.Marshal(populatedKey(c.policyID, c.expires))
		if err != nil {
			t.Fatal(err)
		}
		got, err := decode.ApiKeyFromBytes(payload)
		if err != nil {
			t.Fatal(err)
		}
		requirePopulatedKey(t, "stream", &got, c)
	}
	payload, err := proto.Marshal(absentKey())
	if err != nil {
		t.Fatal(err)
	}
	got, err := decode.ApiKeyFromBytes(payload)
	if err != nil {
		t.Fatal(err)
	}
	requireAbsentKey(t, "stream", got)
}

// --- POLY-9896: order batch_request_id -------------------------------------

type batchCase struct {
	id   uint64
	want string
}

var batchCases = []batchCase{{7, "8"}, {18_446_744_073_709_551_614, "jpXCZedGfVP"}, {0, ""}}

func batchOrder(batchRequestID uint64) *orders.Order {
	return &orders.Order{
		OrderId: 901, SymbolId: 1, ClientOrderId: "c-901",
		Status: orders.OrderStatus_WORKING, Version: 4, BatchRequestId: batchRequestID,
	}
}

func requireBatch(t *testing.T, label string, o *models.Order, want string) {
	t.Helper()
	if o == nil {
		t.Fatalf("%s: order missing", label)
	}
	if o.ClientOrderID != "c-901" || o.Version != 4 {
		t.Fatalf("%s: identity %+v", label, o)
	}
	if o.BatchRequestID != want {
		t.Errorf("%s: BatchRequestID = %q, want %q", label, o.BatchRequestID, want)
	}
}

func TestOrderReadsKeepBatchRequestID(t *testing.T) {
	for _, wire := range retentionWires {
		for _, c := range batchCases {
			t.Run(wire+"/"+strconv.FormatUint(c.id, 10), func(t *testing.T) {
				factory := newRetentionFactory(t, wire, retentionHandler{order: batchOrder(c.id)})
				svc := NewOrdersService(factory, nil, nil, nil, nil, nil, nil)
				ctx := context.Background()

				got, err := svc.Get(ctx, nil, models.OrderKeyByID("901"), nil, false, false)
				if err != nil {
					t.Fatal(err)
				}
				requireBatch(t, "Get", got.Order, c.want)

				open, err := svc.ListOpen(ctx, nil, nil, nil, nil, false, false, nil)
				if err != nil {
					t.Fatal(err)
				}
				if len(open.Orders) != 1 {
					t.Fatalf("ListOpen orders = %d", len(open.Orders))
				}
				requireBatch(t, "ListOpen", &open.Orders[0], c.want)

				history, err := svc.ListHistory(ctx, nil, nil, nil, nil, nil, 0, false, false, nil)
				if err != nil {
					t.Fatal(err)
				}
				if len(history.Orders) != 1 {
					t.Fatalf("ListHistory orders = %d", len(history.Orders))
				}
				requireBatch(t, "ListHistory", &history.Orders[0], c.want)
			})
		}
	}
}

func TestOrderStreamKeepsBatchRequestID(t *testing.T) {
	for _, c := range batchCases {
		payload, err := proto.Marshal(batchOrder(c.id))
		if err != nil {
			t.Fatal(err)
		}
		var msg orders.Order
		if err := proto.Unmarshal(payload, &msg); err != nil {
			t.Fatal(err)
		}
		streamed := decode.OrdersFromProto([]*orders.Order{&msg})
		requireBatch(t, "stream", &streamed[0], c.want)
	}
}

// --- POLY-9893: address-book view entry revisions --------------------------

func addressBookViewFixture() *authv1.GetAddressBookViewResponse {
	return &authv1.GetAddressBookViewResponse{
		Entries: &authv1.AddressBookEntriesView{
			External: []*authv1.ExternalAddressBookEntry{
				{AddressBookEntryId: 11, Label: "ext-11", Revision: 7},
				{AddressBookEntryId: 12, Label: "ext-12"},
			},
			Internal: []*authv1.InternalAddressBookEntry{
				{AddressBookEntryId: 21, Label: "int-21", Revision: 19},
				{AddressBookEntryId: 22, Label: "int-22", Revision: 2},
			},
		},
		ViewRevision: 99,
	}
}

// The Go view exposes its entries in Raw (protobuf JSON, proto field names),
// where each entry keeps its own revision; 0 is omitted, as protobuf JSON does.
func TestAddressBookViewEntriesKeepTheirOwnRevision(t *testing.T) {
	for _, wire := range retentionWires {
		t.Run(wire, func(t *testing.T) {
			factory := newRetentionFactory(t, wire, retentionHandler{})
			svc := NewAddressBookService(factory, nil, nil, nil)
			view, err := svc.GetView(context.Background(), nil, nil, 50, 0)
			if err != nil {
				t.Fatal(err)
			}
			if view.ViewRevision != 99 {
				t.Fatalf("ViewRevision = %d", view.ViewRevision)
			}
			entries, _ := view.Raw["entries"].(map[string]any)
			want := map[string][]any{"external": {"7", nil}, "internal": {"19", "2"}}
			for kind, revisions := range want {
				rows, _ := entries[kind].([]any)
				if len(rows) != len(revisions) {
					t.Fatalf("%s entries = %#v", kind, entries[kind])
				}
				for i, revision := range revisions {
					row, _ := rows[i].(map[string]any)
					if got := row["revision"]; got != revision {
						t.Errorf("%s[%d] revision = %#v, want %#v", kind, i, got, revision)
					}
				}
			}
		})
	}
}
