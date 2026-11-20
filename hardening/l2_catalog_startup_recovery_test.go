package hardening_test

// Catalog startup through an HTTPS proxy that drops or stalls a
// read.
//
// A local HTTPS Connect API (HTTP/2, a test certificate) sits behind a local
// HTTP CONNECT proxy, and the client's transport trusts the test certificate
// and uses the proxy, so every request crosses a real TLS tunnel. The API can
// close every tunnel when a request arrives, which the client sees as the
// connection dropping, or hold a request until the client gives up.

import (
	"bufio"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"connectrpc.com/connect"
	polyester "github.com/Fabric-Labs/polyester-sdk-go"
	"github.com/Fabric-Labs/polyester-sdk-go/auth"
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	zipperv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/chain/zipper/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/gen/chain/zipper/v1/chainzipperv1connect"
	marketdatav1 "github.com/Fabric-Labs/polyester-sdk-go/gen/marketdata/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/gen/marketdata/v1/marketdatav1connect"
)

// Synthetic proxy credentials: they must never reach an error message.
const (
	startupProxyUser   = "sdk-test-user"
	startupProxySecret = "proxy-secret-value"
	startupSymbol      = "BTC-USDT"
)

type startupAction int

const (
	startupOK startupAction = iota
	// startupDrop closes every proxy tunnel, as a proxy dropping the
	// connection does, and never answers.
	startupDrop
	// startupStall never answers.
	startupStall
)

// connectProxy is an HTTP CONNECT proxy that pipes each tunnel to target.
type connectProxy struct {
	listener net.Listener
	target   string

	mu    sync.Mutex
	conns []net.Conn
}

func newConnectProxy(t *testing.T, target string) *connectProxy {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	p := &connectProxy{listener: listener, target: target}
	go p.serve()
	t.Cleanup(func() {
		_ = listener.Close()
		p.closeTunnels()
	})
	return p
}

func (p *connectProxy) url() *url.URL {
	return &url.URL{
		Scheme: "http",
		User:   url.UserPassword(startupProxyUser, startupProxySecret),
		Host:   p.listener.Addr().String(),
	}
}

func (p *connectProxy) serve() {
	for {
		conn, err := p.listener.Accept()
		if err != nil {
			return
		}
		go p.tunnel(conn)
	}
}

func (p *connectProxy) tunnel(client net.Conn) {
	reader := bufio.NewReader(client)
	req, err := http.ReadRequest(reader)
	if err != nil || req.Method != http.MethodConnect {
		_ = client.Close()
		return
	}
	upstream, err := net.Dial("tcp", p.target)
	if err != nil {
		_ = client.Close()
		return
	}
	p.mu.Lock()
	p.conns = append(p.conns, client, upstream)
	p.mu.Unlock()
	_, _ = client.Write([]byte("HTTP/1.1 200 Connection established\r\n\r\n"))
	go func() {
		_, _ = io.Copy(upstream, reader)
		_ = upstream.Close()
		_ = client.Close()
	}()
	_, _ = io.Copy(client, upstream)
	_ = client.Close()
	_ = upstream.Close()
}

// closeTunnels closes every open tunnel without a TLS close_notify.
func (p *connectProxy) closeTunnels() {
	p.mu.Lock()
	conns := p.conns
	p.conns = nil
	p.mu.Unlock()
	for _, conn := range conns {
		_ = conn.Close()
	}
}

// startupAPI answers the two catalog reads by plan and records other calls.
type startupAPI struct {
	marketdatav1connect.UnimplementedMarketDataServiceHandler
	chainzipperv1connect.UnimplementedZipperServiceHandler

	plan  func(operation string, seen int) startupAction
	proxy *connectProxy

	mu    sync.Mutex
	calls map[string]int
	// conns holds, for each operation, the connection each call arrived on,
	// in order.
	conns map[string][]int64
	// drops counts the drops done: the tunnels were closed.
	drops atomic.Int32
	// spotRead is closed when the first spot config read arrives.
	spotRead     chan struct{}
	spotReadOnce sync.Once
}

func newStartupAPI(plan func(operation string, seen int) startupAction) *startupAPI {
	return &startupAPI{plan: plan, calls: map[string]int{}, conns: map[string][]int64{}, spotRead: make(chan struct{})}
}

// startupConnKey keys the number the API gives each connection it accepts:
// every tunnel the proxy opens is a connection of its own.
type startupConnKey struct{}

func (a *startupAPI) count(operation string) int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.calls[operation]
}

// connsOf returns the connection each call of operation arrived on, in order.
func (a *startupAPI) connsOf(operation string) []int64 {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]int64(nil), a.conns[operation]...)
}

// callsOn counts the calls of any operation that arrived on conn.
func (a *startupAPI) callsOn(conn int64) int {
	a.mu.Lock()
	defer a.mu.Unlock()
	n := 0
	for _, conns := range a.conns {
		for _, c := range conns {
			if c == conn {
				n++
			}
		}
	}
	return n
}

func (a *startupAPI) arrive(ctx context.Context, operation string) error {
	conn, _ := ctx.Value(startupConnKey{}).(int64)
	a.mu.Lock()
	a.calls[operation]++
	seen := a.calls[operation]
	a.conns[operation] = append(a.conns[operation], conn)
	a.mu.Unlock()
	if operation == "GetSpotConfig" {
		a.spotReadOnce.Do(func() { close(a.spotRead) })
	}
	switch a.plan(operation, seen) {
	case startupDrop:
		a.proxy.closeTunnels()
		a.drops.Add(1)
		<-ctx.Done()
		return ctx.Err()
	case startupStall:
		<-ctx.Done()
		return ctx.Err()
	}
	return nil
}

func (a *startupAPI) GetSpotConfig(ctx context.Context, _ *connect.Request[marketdatav1.GetSpotConfigRequest]) (*connect.Response[marketdatav1.GetSpotConfigResponse], error) {
	if err := a.arrive(ctx, "GetSpotConfig"); err != nil {
		return nil, err
	}
	return connect.NewResponse(&marketdatav1.GetSpotConfigResponse{
		Assets: []*marketdatav1.AssetConfig{
			{Asset: "BTC", QuantityScale: 8, MarketDataVolumeScale: 8},
			{Asset: "USDT", QuantityScale: 6, MarketDataVolumeScale: 6},
		},
		Pairs: []*marketdatav1.PairConfig{{
			Symbol: startupSymbol, SymbolId: 1, BaseAsset: "BTC", QuoteAsset: "USDT",
			BaseQuantityScale: 8, QuoteQuantityScale: 6, ReferencePriceScale: 9,
		}},
	}), nil
}

func (a *startupAPI) GetDepositWithdrawConfig(ctx context.Context, _ *connect.Request[zipperv1.GetDepositWithdrawConfigRequest]) (*connect.Response[zipperv1.GetDepositWithdrawConfigResponse], error) {
	if err := a.arrive(ctx, "GetDepositWithdrawConfig"); err != nil {
		return nil, err
	}
	return connect.NewResponse(&zipperv1.GetDepositWithdrawConfigResponse{
		Assets: []*zipperv1.AssetConfig{{Asset: "USDT", LedgerId: 2, QuantityScale: 6}},
	}), nil
}

// other refuses every other call once its plan lets it answer.
func (a *startupAPI) other(w http.ResponseWriter, r *http.Request) {
	operation := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
	if err := a.arrive(r.Context(), operation); err != nil {
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)
	_, _ = w.Write([]byte(`{"code":"failed_precondition","message":"stub"}`))
}

type startupHarness struct {
	api   *startupAPI
	proxy *connectProxy
	srv   *httptest.Server
}

func newStartupHarness(t *testing.T, plan func(string, int) startupAction) *startupHarness {
	t.Helper()
	api := newStartupAPI(plan)
	mux := http.NewServeMux()
	path, handler := marketdatav1connect.NewMarketDataServiceHandler(api)
	mux.Handle(path+"GetSpotConfig", handler)
	zpath, zhandler := chainzipperv1connect.NewZipperServiceHandler(api)
	mux.Handle(zpath+"GetDepositWithdrawConfig", zhandler)
	mux.HandleFunc("/", api.other)
	srv := httptest.NewUnstartedServer(mux)
	var conns atomic.Int64
	srv.Config.ConnContext = func(ctx context.Context, _ net.Conn) context.Context {
		return context.WithValue(ctx, startupConnKey{}, conns.Add(1))
	}
	srv.EnableHTTP2 = true
	srv.StartTLS()
	t.Cleanup(srv.Close)
	api.proxy = newConnectProxy(t, srv.Listener.Addr().String())
	return &startupHarness{api: api, proxy: api.proxy, srv: srv}
}

// config is a client configuration whose requests cross the proxy.
func (h *startupHarness) config(timeout time.Duration) polyester.Config {
	transport := h.srv.Client().Transport.(*http.Transport).Clone()
	transport.Proxy = http.ProxyURL(h.proxy.url())
	kp := auth.GenerateEd25519Keypair()
	accountID := "1"
	return polyester.Config{
		APIURL:           h.srv.URL,
		WSURL:            "ws://127.0.0.1:1/unused",
		APIKeyID:         "ak_test",
		APIPrivateKey:    kp.SecretKeyHex,
		DefaultAccountID: &accountID,
		Timeout:          timeout,
		HTTPClient:       &http.Client{Transport: transport},
	}
}

// fundingAfterSpot holds the funding config's first read until the spot
// config's read has reached the API. Over HTTP/2 the funding read then finds
// the spot read's connection in the client's pool and shares it, as it does
// whenever it starts late.
func (h *startupHarness) fundingAfterSpot(cfg polyester.Config) {
	transport := cfg.HTTPClient.Transport.(*http.Transport)
	proxy := transport.Proxy
	var first sync.Once
	transport.Proxy = func(req *http.Request) (*url.URL, error) {
		if strings.HasSuffix(req.URL.Path, "/GetDepositWithdrawConfig") {
			first.Do(func() { <-h.api.spotRead })
		}
		return proxy(req)
	}
}

func (h *startupHarness) client(t *testing.T, cfg polyester.Config) *polyester.Client {
	t.Helper()
	client, err := polyester.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	return client
}

func dropOnce(target string) func(string, int) startupAction {
	return func(operation string, seen int) startupAction {
		if operation == target && seen == 1 {
			return startupDrop
		}
		return startupOK
	}
}

func stallAlways(target string) func(string, int) startupAction {
	return func(operation string, _ int) startupAction {
		if operation == target {
			return startupStall
		}
		return startupOK
	}
}

// assertSpotOrderReachesServer checks an order was not refused for its
// catalogs: the stub server answered it.
func assertSpotOrderReachesServer(t *testing.T, client *polyester.Client, api *startupAPI) {
	t.Helper()
	before := api.count("CreateOrder")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := client.Orders.Create(ctx, limitOrder(startupSymbol), nil)
	var hydrationErr *sdkerrors.CatalogHydrationError
	if errors.As(err, &hydrationErr) {
		t.Fatalf("spot order refused for its catalogs: %v", err)
	}
	if api.count("CreateOrder") != before+1 {
		t.Fatalf("spot order did not reach the server: %v", err)
	}
}

func TestL2FundingReadDroppedOnceThroughProxyRecovers(t *testing.T) {
	h := newStartupHarness(t, dropOnce("GetDepositWithdrawConfig"))
	client := h.client(t, h.config(5*time.Second))
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := client.WaitForCatalogs(ctx); err != nil {
		t.Fatalf("WaitForCatalogs: %v", err)
	}
	if id := client.Catalogs.SymbolIDForSymbol(startupSymbol); id == nil || *id != 1 {
		t.Fatalf("spot catalog not loaded: %v", id)
	}
	// The drop closes every tunnel: send the order once it has happened, so
	// the order's own connection is not the one it closes.
	dropped := time.Now().Add(5 * time.Second)
	for h.api.drops.Load() == 0 {
		if time.Now().After(dropped) {
			t.Fatal("the funding read was never dropped")
		}
		time.Sleep(time.Millisecond)
	}
	assertSpotOrderReachesServer(t, client, h.api)
	// The funding config loads too, on a second read.
	deadline := time.Now().Add(5 * time.Second)
	for client.Catalogs.LedgerIDForAsset("USDT") == nil {
		if time.Now().After(deadline) {
			t.Fatalf("funding config never loaded; %d reads", h.api.count("GetDepositWithdrawConfig"))
		}
		time.Sleep(10 * time.Millisecond)
	}
	if got := h.api.count("GetDepositWithdrawConfig"); got != 2 {
		t.Fatalf("funding reads = %d, want 2", got)
	}
}

func TestL2SpotReadDroppedOnceThroughProxyRecovers(t *testing.T) {
	h := newStartupHarness(t, dropOnce("GetSpotConfig"))
	client := h.client(t, h.config(5*time.Second))
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := client.WaitForCatalogs(ctx); err != nil {
		t.Fatalf("WaitForCatalogs: %v", err)
	}
	assertSpotOrderReachesServer(t, client, h.api)
}

func TestL2PersistentFundingStallLeavesSpotTradingReady(t *testing.T) {
	h := newStartupHarness(t, stallAlways("GetDepositWithdrawConfig"))
	started := time.Now()
	client := h.client(t, h.config(500*time.Millisecond))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := client.WaitForCatalogs(ctx); err != nil {
		t.Fatalf("WaitForCatalogs: %v", err)
	}
	if elapsed := time.Since(started); elapsed > 450*time.Millisecond {
		t.Fatalf("spot readiness waited %s for the funding config", elapsed)
	}
	assertSpotOrderReachesServer(t, client, h.api)
}

func TestL2DroppedOrderIsNeverRetried(t *testing.T) {
	h := newStartupHarness(t, func(operation string, _ int) startupAction {
		if operation == "CreateOrder" {
			return startupDrop
		}
		return startupOK
	})
	client := h.client(t, h.config(5*time.Second))
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := client.WaitForCatalogs(ctx); err != nil {
		t.Fatalf("WaitForCatalogs: %v", err)
	}
	if _, err := client.Orders.Create(ctx, limitOrder(startupSymbol), nil); err == nil {
		t.Fatal("a dropped order succeeded")
	}
	if got := h.api.count("CreateOrder"); got != 1 {
		t.Fatalf("CreateOrder sent %d times, want 1", got)
	}
}

// cutFirstAnswer answers the first request of each operation in ops with a
// 200 whose body stops short of its Content-Length, as a connection dropped
// mid-answer does, and passes every other request to next.
func cutFirstAnswer(next http.Handler, ops ...string) (http.Handler, *sync.Map) {
	cut := &sync.Map{}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		operation := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
		for _, op := range ops {
			if op != operation {
				continue
			}
			if _, done := cut.LoadOrStore(op, true); done {
				break
			}
			w.Header().Set("Content-Type", r.Header.Get("Content-Type"))
			w.Header().Set("Content-Length", "64")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte{0x0a, 0x10})
			// Returning short of the declared length makes the server close
			// the connection: the client reads an unexpected EOF.
			return
		}
		next.ServeHTTP(w, r)
	}), cut
}

// A catalog read changes nothing on the server, so one whose connection
// closed mid-answer is read again within the startup retry, as one answered
// unavailable is, instead of failing after one attempt.
func TestL2CatalogReadCutMidAnswerIsReadAgain(t *testing.T) {
	api := newStartupAPI(func(string, int) startupAction { return startupOK })
	mux := http.NewServeMux()
	path, handler := marketdatav1connect.NewMarketDataServiceHandler(api)
	mux.Handle(path+"GetSpotConfig", handler)
	zpath, zhandler := chainzipperv1connect.NewZipperServiceHandler(api)
	mux.Handle(zpath+"GetDepositWithdrawConfig", zhandler)
	mux.HandleFunc("/", api.other)
	cutting, cut := cutFirstAnswer(mux, "GetSpotConfig", "GetDepositWithdrawConfig")
	srv := httptest.NewServer(cutting)
	t.Cleanup(srv.Close)
	kp := auth.GenerateEd25519Keypair()
	client, err := polyester.New(polyester.Config{
		APIURL:        srv.URL,
		WSURL:         "ws://127.0.0.1:1/unused",
		APIKeyID:      "ak_test",
		APIPrivateKey: kp.SecretKeyHex,
		Timeout:       5 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := client.WaitForCatalogs(ctx); err != nil {
		t.Fatalf("WaitForCatalogs: %v", err)
	}
	if id := client.Catalogs.SymbolIDForSymbol(startupSymbol); id == nil || *id != 1 {
		t.Fatalf("spot catalog not loaded: %v", id)
	}
	deadline := time.Now().Add(5 * time.Second)
	for client.Catalogs.LedgerIDForAsset("USDT") == nil {
		if time.Now().After(deadline) {
			t.Fatalf("funding config never loaded; %d reads after the cut", api.count("GetDepositWithdrawConfig"))
		}
		time.Sleep(10 * time.Millisecond)
	}
	for _, op := range []string{"GetSpotConfig", "GetDepositWithdrawConfig"} {
		if _, wasCut := cut.Load(op); !wasCut {
			t.Fatalf("%s was never cut", op)
		}
		if got := api.count(op); got != 1 {
			t.Fatalf("%s answered %d times after its cut, want 1", op, got)
		}
	}
}
