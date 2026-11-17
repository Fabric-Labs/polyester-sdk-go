package polyester

import (
	"context"
	"encoding/base64"
	stderrors "errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"

	"github.com/Fabric-Labs/polyester-sdk-go/auth"
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	"github.com/Fabric-Labs/polyester-sdk-go/hardening"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
	"github.com/Fabric-Labs/polyester-sdk-go/services"
)

// sentLimit is the limit field of one request the capture server received.
type sentLimit struct {
	procedure string
	present   bool
	value     uint64
	msg       proto.Message
}

// limitCapture decodes every request it receives as its method's input
// message and records the limit field, then answers unimplemented.
type limitCapture struct {
	mu   sync.Mutex
	sent []sentLimit
}

func (c *limitCapture) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	procedure := strings.TrimPrefix(r.URL.Path, "/")
	service, method, _ := strings.Cut(procedure, "/")
	desc, err := protoregistry.GlobalFiles.FindDescriptorByName(protoreflect.FullName(service))
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	input := desc.(protoreflect.ServiceDescriptor).Methods().ByName(protoreflect.Name(method)).Input()
	msgType, err := protoregistry.GlobalTypes.FindMessageByName(input.FullName())
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	body, _ := io.ReadAll(r.Body)
	contentType := r.Header.Get("Content-Type")
	if r.Method == http.MethodGet {
		query := r.URL.Query()
		body = []byte(query.Get("message"))
		if query.Get("base64") == "1" {
			body, _ = base64.RawURLEncoding.DecodeString(query.Get("message"))
		}
		contentType = query.Get("encoding")
	}
	msg := msgType.New().Interface()
	if strings.Contains(contentType, "json") {
		err = protojson.Unmarshal(body, msg)
	} else {
		err = proto.Unmarshal(body, msg)
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	got := sentLimit{procedure: procedure, msg: msg}
	if field := input.Fields().ByName("limit"); field != nil && msg.ProtoReflect().Has(field) {
		got.present, got.value = true, msg.ProtoReflect().Get(field).Uint()
	}
	c.mu.Lock()
	c.sent = append(c.sent, got)
	c.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusNotImplemented)
	_, _ = io.WriteString(w, `{"code":"unimplemented","message":"capture only"}`)
}

func (c *limitCapture) take() []sentLimit {
	c.mu.Lock()
	defer c.mu.Unlock()
	sent := c.sent
	c.sent = nil
	return sent
}

func newLimitCaptureClient(t *testing.T) (*Client, *limitCapture) {
	t.Helper()
	capture := &limitCapture{}
	srv := hardening.NewTestServer(capture)
	t.Cleanup(srv.Close)
	keypair := auth.GenerateEd25519Keypair()
	client, err := New(Config{APIURL: srv.URL, APIKeyID: "ak_limits", APIPrivateKey: keypair.SecretKeyHex, SkipCatalogHydration: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	return client, capture
}

// pagedCall makes one list call with the given page limit. Errors other than
// a local refusal are the capture server's unimplemented answer.
type pagedCall struct {
	name string
	call func(ctx context.Context, c *Client, limit int) error
}

func orderListCalls() []pagedCall {
	return []pagedCall{
		{"orders.ListHistory", func(ctx context.Context, c *Client, limit int) error {
			_, err := c.Orders.ListHistory(ctx, nil, nil, nil, nil, nil, limit, false, false, nil)
			return err
		}},
		{"orders.ListOpen", func(ctx context.Context, c *Client, limit int) error {
			_, err := c.Orders.ListOpen(ctx, nil, nil, nil, &limit, false, false, nil)
			return err
		}},
		{"orders.Get", func(ctx context.Context, c *Client, limit int) error {
			value := uint32(limit)
			_, err := c.Orders.Get(ctx, nil, models.OrderKeyByID("A"), nil, false, false, models.GetOrderOptions{Limit: &value})
			return err
		}},
		{"trades.List", func(ctx context.Context, c *Client, limit int) error {
			_, err := c.Trades.List(ctx, nil, nil, nil, nil, limit, nil, nil)
			return err
		}},
		{"transfers.List", func(ctx context.Context, c *Client, limit int) error {
			_, err := c.Transfers.List(ctx, nil, nil, limit, false, nil)
			return err
		}},
		{"triggers.List", func(ctx context.Context, c *Client, limit int) error {
			_, err := c.Triggers.List(ctx, nil, nil, nil, nil, limit, nil)
			return err
		}},
		{"triggers.ListEvents", func(ctx context.Context, c *Client, limit int) error {
			_, err := c.Triggers.ListEvents(ctx, nil, "2", nil, limit, nil, nil)
			return err
		}},
	}
}

func isLimitRefusal(err error) bool {
	var validation *sdkerrors.ValidationError
	return stderrors.As(err, &validation) && strings.Contains(validation.Msg, "limit")
}

// A page limit above the contract's maximum is refused before sending on
// every order, trade, transfer and trigger list; 1 and 1000 are sent as
// given.
func TestOrderTradeTransferTriggerListsRefuseLimitsAboveTheContractMaximum(t *testing.T) {
	ctx := context.Background()
	client, capture := newLimitCaptureClient(t)
	for _, call := range orderListCalls() {
		for _, limit := range []int{1001, 5000} {
			if err := call.call(ctx, client, limit); !isLimitRefusal(err) {
				t.Errorf("%s limit %d: err = %T %v, want a limit ValidationError", call.name, limit, err, err)
			}
			if sent := capture.take(); len(sent) != 0 {
				t.Errorf("%s limit %d: sent %+v", call.name, limit, sent)
			}
		}
		for _, limit := range []int{1, 1000} {
			if err := call.call(ctx, client, limit); isLimitRefusal(err) {
				t.Errorf("%s limit %d refused: %v", call.name, limit, err)
			}
			sent := capture.take()
			if len(sent) == 0 || !sent[0].present || sent[0].value != uint64(limit) {
				t.Errorf("%s limit %d: sent %+v", call.name, limit, sent)
			}
		}
	}
}

// A zero limit on order history and user trades is the documented "unset":
// it is left out so the server's default applies, never sent as limit=0,
// which the contract's gte 1 refuses. An explicit zero where the call takes
// a pointer is refused.
func TestOrderHistoryAndTradesLeaveAZeroLimitOut(t *testing.T) {
	ctx := context.Background()
	client, capture := newLimitCaptureClient(t)
	for _, call := range orderListCalls() {
		switch call.name {
		case "orders.ListHistory", "trades.List":
			if err := call.call(ctx, client, 0); isLimitRefusal(err) {
				t.Errorf("%s limit 0 refused: %v", call.name, err)
			}
			sent := capture.take()
			if len(sent) == 0 || sent[0].present {
				t.Errorf("%s limit 0: sent %+v, want limit left out", call.name, sent)
			}
		case "orders.ListOpen", "orders.Get":
			if err := call.call(ctx, client, 0); !isLimitRefusal(err) {
				t.Errorf("%s explicit limit 0: err = %T %v, want a limit ValidationError", call.name, err, err)
			}
			if sent := capture.take(); len(sent) != 0 {
				t.Errorf("%s explicit limit 0: sent %+v", call.name, sent)
			}
		}
	}
}

// A heatmap limit has no presence and the contract takes 1 to 20000: 0 is on
// the wire as 0, which the contract refuses, so it is refused before sending.
func TestHeatmapRefusesAZeroLimit(t *testing.T) {
	client, capture := newLimitCaptureClient(t)
	symbolID := uint32(1)
	_, err := client.Heatmap.Get(context.Background(), nil, &symbolID, "1m", 50, 0, "close", nil, nil, nil)
	if !isLimitRefusal(err) {
		t.Fatalf("limit 0: err = %T %v, want a limit ValidationError", err, err)
	}
	if sent := capture.take(); len(sent) != 0 {
		t.Fatalf("limit 0: sent %+v", sent)
	}
}

// Every other list call refuses a page limit above its own request's
// contract maximum before sending, and sends the maximum itself.
func TestEveryListCallRefusesALimitAboveItsContractMaximum(t *testing.T) {
	ctx := context.Background()
	client, capture := newLimitCaptureClient(t)
	err := client.Catalogs.HydrateSpotConfig(map[string]any{
		"assets": []any{map[string]any{"asset": "BTC", "market_data_volume_scale": float64(8)}},
		"pairs": []any{map[string]any{
			"symbol": "BTC-USDT", "symbol_id": float64(1), "base_asset": "BTC",
			"base_quantity_scale": float64(6), "quote_quantity_scale": float64(2),
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	symbolID := uint32(1)
	for _, tc := range []struct {
		name string
		max  int
		call func(limit int) error
	}{
		{"marketData.GetTrades", 1000, func(limit int) error {
			_, err := client.MarketData.GetTrades(ctx, nil, &symbolID, limit, nil)
			return err
		}},
		{"marketData.GetTradesAfter", 1000, func(limit int) error {
			_, err := client.MarketData.GetTradesAfter(ctx, nil, &symbolID, "5", limit)
			return err
		}},
		{"marketData.GetCandles", 10000, func(limit int) error {
			_, err := client.MarketData.GetCandles(ctx, nil, &symbolID, "1m", limit, nil, nil, false)
			return err
		}},
		{"marketData.GetCandlesColumns", 10000, func(limit int) error {
			_, err := client.MarketData.GetCandlesColumns(ctx, nil, &symbolID, "1m", limit, nil, nil, false, nil)
			return err
		}},
		{"marketOverview.List", 2000, func(limit int) error {
			_, err := client.MarketOverview.List(ctx, nil, limit, "", false)
			return err
		}},
		{"lifecycle.ListFlows", 500, func(limit int) error {
			_, err := client.Lifecycle.ListFlows(ctx, limit, nil, nil, nil, false)
			return err
		}},
		{"lifecycle.ListFlowsByTx", 500, func(limit int) error {
			_, err := client.Lifecycle.ListFlowsByTx(ctx, "0x"+strings.Repeat("ab", 32), "", limit, nil)
			return err
		}},
		{"addressBook.ListEntries", 500, func(limit int) error {
			_, err := client.AddressBook.ListEntries(ctx, nil, nil, nil, limit, nil)
			return err
		}},
		{"addressBook.ListTransferCounterparties", 200, func(limit int) error {
			_, err := client.AddressBook.ListTransferCounterparties(ctx, nil, nil, nil, nil, limit)
			return err
		}},
		{"addressBook.ListInternalTransferWhitelistEntries", 500, func(limit int) error {
			_, err := client.AddressBook.ListInternalTransferWhitelistEntries(ctx, nil, nil, limit, nil)
			return err
		}},
		{"addressBook.GetView", 200, func(limit int) error {
			_, err := client.AddressBook.GetView(ctx, nil, nil, limit, 0)
			return err
		}},
		{"subAccounts.ListActivity", 200, func(limit int) error {
			sub := "2"
			_, err := client.SubAccounts.ListActivity(ctx, nil, &sub, limit, nil)
			return err
		}},
		{"changelog.ListPosts", 100, func(limit int) error {
			_, err := client.Changelog.ListPosts(ctx, models.ListChangelogPostsOptions{Limit: limit})
			return err
		}},
		{"changelog.ListChanges", 500, func(limit int) error {
			_, err := client.Changelog.ListChanges(ctx, models.ListChangelogChangesOptions{Limit: limit})
			return err
		}},
		{"changelog.ListDeploymentGroups", 100, func(limit int) error {
			_, err := client.Changelog.ListDeploymentGroups(ctx, models.ListChangelogDeploymentGroupsOptions{Limit: limit})
			return err
		}},
		{"system.ListStatusEvents", 200, func(limit int) error {
			_, err := client.ListStatusEvents(ctx, models.ListStatusEventsOptions{Limit: limit})
			return err
		}},
		{"heatmap.Get", 20000, func(limit int) error {
			_, err := client.Heatmap.Get(ctx, nil, &symbolID, "1m", 50, limit, "close", nil, nil, nil)
			return err
		}},
		{"dustConversions.List", 100, func(limit int) error {
			_, err := client.DustConversions.List(ctx, services.ListDustConversionsOptions{Limit: limit})
			return err
		}},
	} {
		if err := tc.call(tc.max + 1); !isLimitRefusal(err) {
			t.Errorf("%s limit %d: err = %T %v, want a limit ValidationError", tc.name, tc.max+1, err, err)
		}
		if sent := capture.take(); len(sent) != 0 {
			t.Errorf("%s limit %d: sent %+v", tc.name, tc.max+1, sent)
		}
		if err := tc.call(tc.max); isLimitRefusal(err) {
			t.Errorf("%s limit %d refused: %v", tc.name, tc.max, err)
		}
		if sent := capture.take(); len(sent) == 0 || sent[0].value != uint64(tc.max) {
			t.Errorf("%s limit %d: sent %+v", tc.name, tc.max, sent)
		}
	}
}

func isValidationNaming(err error, field string) bool {
	var validation *sdkerrors.ValidationError
	return stderrors.As(err, &validation) && strings.Contains(validation.Msg, field)
}
