package services

import (
	"context"
	"errors"
	"math"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Fabric-Labs/polyester-sdk-go/auth"
	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	privatewsv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/privatews/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/hardening"
	"github.com/Fabric-Labs/polyester-sdk-go/realtime"
	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/proto"
)

// Integer account IDs must address the account they name. 42 is the public ID
// "j"; read as text, "42" is the public ID of account 175.

type accountIDNamedInt int64

var (
	refusedAccountIDs = map[string]any{
		"zero":            0,
		"uint64 zero":     uint64(0),
		"negative":        -1,
		"negative int64":  int64(math.MinInt64),
		"bool":            true,
		"false":           false,
		"big over uint64": new(big.Int).Lsh(big.NewInt(1), 64),
		"big zero":        new(big.Int),
		"float":           42.0,
	}
	maxUint64PublicID = codecs.FormatID(math.MaxUint64)
)

// acceptedAccountIDs maps each accepted account ID input to the public ID it
// must address.
func acceptedAccountIDs() map[string]struct {
	value any
	want  string
} {
	big42 := big.NewInt(42)
	publicID := codecs.FormatID(42)
	return map[string]struct {
		value any
		want  string
	}{
		"int 42":           {42, codecs.FormatID(42)},
		"uint32 42":        {uint32(42), codecs.FormatID(42)},
		"named int 42":     {accountIDNamedInt(42), codecs.FormatID(42)},
		"pointer int 42":   {func() *int { v := 42; return &v }(), codecs.FormatID(42)},
		"big 42":           {big42, codecs.FormatID(42)},
		"uint64 max":       {uint64(math.MaxUint64), maxUint64PublicID},
		"public ID string": {publicID, publicID},
		"public ID ptr":    {&publicID, publicID},
		"digit string":     {"42", "42"},
	}
}

func TestResolveAccountIDEncodesIntegersAsPublicIDs(t *testing.T) {
	for name, tc := range acceptedAccountIDs() {
		got, err := ResolveAccountID(tc.value, nil)
		if err != nil || got != tc.want {
			t.Errorf("%s: ResolveAccountID(%v) = %q, %v; want %q", name, tc.value, got, err, tc.want)
		}
	}
	for name, value := range refusedAccountIDs {
		def := codecs.FormatID(7)
		got, err := ResolveAccountID(value, &def)
		var validation *sdkerrors.ValidationError
		if !errors.As(err, &validation) {
			t.Errorf("%s: ResolveAccountID(%v) = %q, %v; want ValidationError", name, value, got, err)
		}
	}
	def := 42
	if got, err := ResolveAccountID(nil, def); err != nil || got != codecs.FormatID(42) {
		t.Errorf("integer default = %q, %v", got, err)
	}
}

func TestScopedSubAccountEncodesIntegerSubAccountIDs(t *testing.T) {
	scoped := ScopedSubAccount{}
	for name, tc := range acceptedAccountIDs() {
		got, err := scoped.ResolveSubAccountID(tc.value, nil)
		if err != nil || got == nil || *got != tc.want {
			t.Errorf("%s: ResolveSubAccountID(%v) = %v, %v; want %q", name, tc.value, got, err, tc.want)
			continue
		}
		if tc.want != codecs.FormatID(42) {
			continue
		}
		// The request field carries the account the caller named.
		field, err := codecs.ParseOptionalSubaccountID(got)
		if err != nil || field == nil || *field != 42 {
			t.Errorf("%s: subaccount field = %v, %v; want 42", name, field, err)
		}
	}
	for name, value := range refusedAccountIDs {
		got, err := scoped.ResolveSubAccountID(value, nil)
		var validation *sdkerrors.ValidationError
		if !errors.As(err, &validation) {
			t.Errorf("%s: ResolveSubAccountID(%v) = %v, %v; want ValidationError", name, value, got, err)
		}
	}
}

// fakeCentrifugo answers the private-channel handshake and records the
// channel each subscription token was requested for.
type fakeCentrifugo struct {
	wsURL    string
	apiURL   string
	mu       sync.Mutex
	channels []string
	requests int
}

func newFakeCentrifugo(t *testing.T) *fakeCentrifugo {
	t.Helper()
	f := &fakeCentrifugo{}
	upgrader := websocket.Upgrader{Subprotocols: []string{"centrifuge-protobuf"}}
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/rt/token", func(w http.ResponseWriter, _ *http.Request) {
		f.count()
		_, _ = w.Write([]byte(`{"token":"connect"}`))
	})
	mux.HandleFunc("/v1/rt/subscribe", func(w http.ResponseWriter, r *http.Request) {
		f.count()
		f.mu.Lock()
		f.channels = append(f.channels, r.URL.Query().Get("channel"))
		f.mu.Unlock()
		_, _ = w.Write([]byte(`{"token":"sub"}`))
	})
	mux.HandleFunc("/connection/websocket", func(w http.ResponseWriter, r *http.Request) {
		f.count()
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		// Connect reply id 1, then subscribe reply id 2.
		for _, ack := range [][]byte{{2, 8, 1}, {2, 8, 2}} {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
			if err := conn.WriteMessage(websocket.BinaryMessage, ack); err != nil {
				return
			}
		}
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	})
	srv := hardening.NewTestServer(mux)
	f.apiURL = srv.URL
	f.wsURL = "ws" + strings.TrimPrefix(srv.URL, "http") + "/connection/websocket"
	t.Cleanup(srv.Close)
	return f
}

func (f *fakeCentrifugo) count() {
	f.mu.Lock()
	f.requests++
	f.mu.Unlock()
}

func (f *fakeCentrifugo) take() ([]string, int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	channels, requests := f.channels, f.requests
	f.channels, f.requests = nil, 0
	return channels, requests
}

func accountIDTestCredentials(t *testing.T) *auth.Credentials {
	t.Helper()
	kp := auth.GenerateEd25519Keypair()
	creds, err := auth.LoadCredentials("test-key", kp.SecretKeyHex.Reveal(), false)
	if err != nil {
		t.Fatal(err)
	}
	return creds
}

func opened[T any](sub *realtime.Subscription[T], err error) (func(), error) {
	if err != nil {
		return nil, err
	}
	return sub.Close, nil
}

type accountSubscribe struct {
	name     string
	template string
	call     func(context.Context, any) (func(), error)
}

func accountSubscribes(rt RealtimeClient, def *string) []accountSubscribe {
	apiKeys := NewApiKeysService(nil, nil, rt, def)
	book := NewAddressBookService(nil, nil, rt, def)
	balances := NewBalancesService(nil, nil, nil, rt, def)
	lifecycle := NewLifecycleService(nil, rt)
	policies := NewPoliciesService(rt, def)
	subAccounts := NewSubAccountsService(nil, nil, rt, def)
	trades := NewTradesService(nil, nil, nil, rt, def)
	transfers := NewTransfersService(nil, nil, rt, def)
	triggers := NewTriggersService(nil, nil, nil, rt, def, nil, nil)
	return []accountSubscribe{
		{"ApiKeys.Subscribe", "private:auth:api-keys:%s:proto", func(ctx context.Context, id any) (func(), error) {
			return opened(apiKeys.Subscribe(ctx, id))
		}},
		{"AddressBook.Subscribe", "private:auth:address-books:%s:proto", func(ctx context.Context, id any) (func(), error) {
			return opened(book.Subscribe(ctx, id))
		}},
		{"AddressBook.SubscribeViewInvalidations", "private:auth:address-books:%s:proto", func(ctx context.Context, id any) (func(), error) {
			return opened(book.SubscribeViewInvalidations(ctx, id))
		}},
		{"Balances.Subscribe", "private:ledger:balances:%s:proto", func(ctx context.Context, id any) (func(), error) {
			return opened(balances.Subscribe(ctx, id))
		}},
		{"Lifecycle.SubscribeOpenFlows", "private:chain:lifecycle:flows:%s:proto", func(ctx context.Context, id any) (func(), error) {
			return opened(lifecycle.SubscribeOpenFlows(ctx, id))
		}},
		{"Policies.Subscribe", "private:auth:subaccount-policies:%s:proto", func(ctx context.Context, id any) (func(), error) {
			return opened(policies.Subscribe(ctx, id))
		}},
		{"Policies.SubscribeSubaccountPolicies", "private:auth:subaccount-policies:%s:proto", func(ctx context.Context, id any) (func(), error) {
			return opened(policies.SubscribeSubaccountPolicies(ctx, id))
		}},
		{"Policies.SubscribeAPIPolicies", "private:auth:api-policies:%s:proto", func(ctx context.Context, id any) (func(), error) {
			return opened(policies.SubscribeAPIPolicies(ctx, id))
		}},
		{"SubAccounts.Subscribe", "private:auth:subaccounts:%s:proto", func(ctx context.Context, id any) (func(), error) {
			return opened(subAccounts.Subscribe(ctx, id))
		}},
		{"SubAccounts.SubscribeAPIKeys", "private:auth:api-keys:%s:proto", func(ctx context.Context, id any) (func(), error) {
			return opened(subAccounts.SubscribeAPIKeys(ctx, id))
		}},
		{"Trades.Subscribe", "private:spot:trades:%s:proto", func(ctx context.Context, id any) (func(), error) {
			return opened(trades.Subscribe(ctx, id))
		}},
		{"Transfers.Subscribe", "private:ledger:transfers:%s:proto", func(ctx context.Context, id any) (func(), error) {
			return opened(transfers.Subscribe(ctx, id))
		}},
		{"Triggers.Subscribe", "private:spot:triggers:%s:proto", func(ctx context.Context, id any) (func(), error) {
			return opened(triggers.Subscribe(ctx, id))
		}},
		{"Triggers.SubscribeEvents", "private:spot:triggers:events:%s:proto", func(ctx context.Context, id any) (func(), error) {
			return opened(triggers.SubscribeEvents(ctx, id))
		}},
	}
}

// TestAccountSubscribesAddressTheIntegerAccount captures the channel each
// private Subscribe asks a token for, and checks refused IDs never connect.
func TestAccountSubscribesAddressTheIntegerAccount(t *testing.T) {
	fake := newFakeCentrifugo(t)
	rt := realtime.NewClient(fake.wsURL, fake.apiURL, accountIDTestCredentials(t), nil, 16)
	t.Cleanup(rt.Close)
	def := codecs.FormatID(7)
	cases := map[string]struct {
		value any
		want  string
	}{
		"int 42":           {42, codecs.FormatID(42)},
		"int64 42":         {int64(42), codecs.FormatID(42)},
		"uint64 max":       {uint64(math.MaxUint64), maxUint64PublicID},
		"public ID string": {codecs.FormatID(42), codecs.FormatID(42)},
	}
	for _, sub := range accountSubscribes(rt, &def) {
		for name, tc := range cases {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			closeSub, err := sub.call(ctx, tc.value)
			cancel()
			channels, _ := fake.take()
			if err != nil {
				t.Errorf("%s(%s): %v", sub.name, name, err)
				continue
			}
			closeSub()
			want := strings.Replace(sub.template, "%s", tc.want, 1)
			if len(channels) != 1 || channels[0] != want {
				t.Errorf("%s(%s) channels %v; want %q", sub.name, name, channels, want)
			}
		}
		for name, value := range refusedAccountIDs {
			closeSub, err := sub.call(context.Background(), value)
			if closeSub != nil {
				closeSub()
			}
			_, requests := fake.take()
			var validation *sdkerrors.ValidationError
			if !errors.As(err, &validation) || requests != 0 {
				t.Errorf("%s(%s) = %v after %d requests; want ValidationError before connecting", sub.name, name, err, requests)
			}
		}
	}
}

// TestOrdersSubscribeAddressesTheIntegerAccount checks the private order
// stream subscribes account 42 for int 42, and refuses invalid IDs before
// connecting.
func TestOrdersSubscribeAddressesTheIntegerAccount(t *testing.T) {
	type captured struct {
		channel string
		account uint64
	}
	var mu sync.Mutex
	var requests int
	var channel string
	got := make(chan captured, 4)
	upgrader := websocket.Upgrader{Subprotocols: []string{"polyester.v1.proto"}}
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/rt/token", func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		requests++
		mu.Unlock()
		_, _ = w.Write([]byte(`{"token":"connect"}`))
	})
	mux.HandleFunc("/v1/rt/subscribe", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requests++
		channel = r.URL.Query().Get("channel")
		mu.Unlock()
		_, _ = w.Write([]byte(`{"token":"sub"}`))
	})
	mux.HandleFunc("/v1/ws/private", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requests++
		mu.Unlock()
		ws, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer func() { _ = ws.Close() }()
		for {
			_, data, err := ws.ReadMessage()
			if err != nil {
				return
			}
			msg := &privatewsv1.ClientMessage{}
			if proto.Unmarshal(data, msg) != nil {
				return
			}
			if request := msg.GetSubscribe(); request != nil {
				mu.Lock()
				select {
				case got <- captured{channel: channel, account: request.GetAccount()}:
				default:
				}
				mu.Unlock()
			}
		}
	})
	srv := hardening.NewTestServer(mux)
	t.Cleanup(srv.Close)
	rt := realtime.NewClient("ws://127.0.0.1:1", srv.URL, accountIDTestCredentials(t), nil, 16)
	t.Cleanup(rt.Close)
	def := codecs.FormatID(7)
	orders := NewOrdersService(nil, nil, nil, rt, &def, nil, nil)

	for name, tc := range map[string]struct {
		value   any
		account uint64
	}{
		"int 42":           {42, 42},
		"uint64 max":       {uint64(math.MaxUint64), math.MaxUint64},
		"public ID string": {codecs.FormatID(42), 42},
	} {
		sub, err := orders.Subscribe(context.Background(), tc.value)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		select {
		case c := <-got:
			if c.account != tc.account || c.channel != "private:spot:orders:"+codecs.FormatID(tc.account) {
				t.Errorf("%s: subscribed %+v; want account %d", name, c, tc.account)
			}
		case <-time.After(5 * time.Second):
			t.Errorf("%s: no subscribe frame", name)
		}
		sub.Close()
	}

	// A fresh client and server, so no earlier stream can add requests.
	var refusedRequests int
	refusedSrv := hardening.NewTestServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		mu.Lock()
		refusedRequests++
		mu.Unlock()
	}))
	t.Cleanup(refusedSrv.Close)
	refusedRT := realtime.NewClient("ws://127.0.0.1:1", refusedSrv.URL, accountIDTestCredentials(t), nil, 16)
	t.Cleanup(refusedRT.Close)
	refusedOrders := NewOrdersService(nil, nil, nil, refusedRT, &def, nil, nil)
	for name, value := range refusedAccountIDs {
		sub, err := refusedOrders.Subscribe(context.Background(), value)
		var validation *sdkerrors.ValidationError
		if !errors.As(err, &validation) {
			t.Errorf("%s: Orders.Subscribe(%v) = %v; want ValidationError", name, value, err)
			if sub != nil {
				sub.Close()
			}
		}
	}
	time.Sleep(100 * time.Millisecond)
	mu.Lock()
	defer mu.Unlock()
	if refusedRequests != 0 {
		t.Errorf("refused IDs made %d requests", refusedRequests)
	}
}

// TestLifecycleSubscribeOpenFlowsUnsetStaysPublic keeps an unset account on
// the public open-flows channel.
func TestLifecycleSubscribeOpenFlowsUnsetStaysPublic(t *testing.T) {
	fake := newFakeCentrifugo(t)
	rt := realtime.NewClient(fake.wsURL, fake.apiURL, accountIDTestCredentials(t), nil, 16)
	t.Cleanup(rt.Close)
	lifecycle := NewLifecycleService(nil, rt)
	var empty *string
	for name, unset := range map[string]any{"nil": nil, "empty string": "", "nil pointer": empty} {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		sub, err := lifecycle.SubscribeOpenFlows(ctx, unset)
		cancel()
		channels, requests := fake.take()
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		sub.Close()
		// A public channel needs no tokens: one WebSocket, no token requests.
		if len(channels) != 0 || requests != 1 {
			t.Errorf("%s: token channels %v, %d requests; want the public channel", name, channels, requests)
		}
	}
}
