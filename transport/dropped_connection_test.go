package transport

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"connectrpc.com/connect"
	orderv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/orders/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/gen/orders/v1/ordersv1connect"
)

// droppingListener keeps every raw connection it accepts, so a test can drop
// them all at once without a TLS close_notify, as a proxy closing its tunnels
// does.
type droppingListener struct {
	net.Listener
	mu    sync.Mutex
	conns []net.Conn
}

func (l *droppingListener) Accept() (net.Conn, error) {
	conn, err := l.Listener.Accept()
	if err == nil {
		l.mu.Lock()
		l.conns = append(l.conns, conn)
		l.mu.Unlock()
	}
	return conn, err
}

func (l *droppingListener) dropAll() {
	l.mu.Lock()
	conns := l.conns
	l.conns = nil
	l.mu.Unlock()
	for _, conn := range conns {
		_ = conn.Close()
	}
}

// closeNotifyingConn reports when the client closes it.
type closeNotifyingConn struct {
	net.Conn
	once   sync.Once
	closed chan struct{}
}

func (c *closeNotifyingConn) Close() error {
	c.once.Do(func() { close(c.closed) })
	return c.Conn.Close()
}

type countingOrders struct {
	ordersv1connect.UnimplementedOrdersServiceHandler
	creates atomic.Int32
}

func (h *countingOrders) CreateOrder(context.Context, *connect.Request[orderv1.CreateOrderRequest]) (*connect.Response[orderv1.CreateOrderResponse], error) {
	h.creates.Add(1)
	return connect.NewResponse(&orderv1.CreateOrderResponse{}), nil
}

// await waits for ch; the timeout only guards against a hang.
func await[T any](t *testing.T, ch <-chan T, what string) T {
	t.Helper()
	select {
	case v := <-ch:
		return v
	case <-time.After(30 * time.Second):
		t.Fatalf("timed out waiting for %s", what)
		var zero T
		return zero
	}
}

// An HTTP/2 connection that drops before it carries any request stays in
// the pool, and net/http hands it to the next request, which fails with
// "http2: client conn could not be established" before any of it is
// written. The SDK sends that request again on a fresh connection: a create
// then succeeds and reaches the server once.
func TestACreateAfterAnUnusedConnectionDroppedIsSentOnce(t *testing.T) {
	orders := &countingOrders{}
	mux := http.NewServeMux()
	path, handler := ordersv1connect.NewOrdersServiceHandler(orders)
	mux.Handle(path, handler)
	srv := httptest.NewUnstartedServer(mux)
	listener := &droppingListener{Listener: srv.Listener}
	srv.Listener = listener
	// An HTTP/2 server marks a connection idle once it has read the
	// client's preface.
	prefaced := make(chan struct{}, 8)
	srv.Config.ConnState = func(_ net.Conn, state http.ConnState) {
		if state == http.StateIdle {
			prefaced <- struct{}{}
		}
	}
	srv.EnableHTTP2 = true
	srv.StartTLS()
	t.Cleanup(srv.Close)

	pooled := srv.Client().Transport.(*http.Transport).Clone()
	dialing, release := make(chan struct{}), make(chan struct{})
	clientConns := make(chan *closeNotifyingConn, 8)
	var dials atomic.Int32
	pooled.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		if dials.Add(1) == 1 {
			close(dialing)
			<-release
		}
		conn, err := (&net.Dialer{}).DialContext(ctx, network, addr)
		if err != nil {
			return nil, err
		}
		notifying := &closeNotifyingConn{Conn: conn, closed: make(chan struct{})}
		clientConns <- notifying
		return notifying, nil
	}
	factory := NewFactory(Config{APIURL: srv.URL}, nil, &http.Client{Transport: pooled})
	client := ordersv1connect.NewOrdersServiceClient(factory.HTTP, srv.URL, factory.ConnectOptions(false)...)
	create := connect.NewRequest(&orderv1.CreateOrderRequest{})

	// A create whose caller gives up while its connection is dialing leaves
	// that connection to the pool, never used.
	ctx, cancel := context.WithCancel(context.Background())
	go func() { <-dialing; cancel() }()
	if _, err := client.CreateOrder(ctx, create); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled create: %v", err)
	}
	close(release)
	spare := await(t, clientConns, "the spare connection")
	await(t, prefaced, "the server to read the spare connection's preface")

	// It drops before it carries any request; the client notices.
	listener.dropAll()
	await(t, spare.closed, "the client to close the dropped connection")

	// The next create is sent once, on a fresh connection.
	if _, err := client.CreateOrder(context.Background(), connect.NewRequest(&orderv1.CreateOrderRequest{})); err != nil {
		t.Fatalf("create after the drop: %v", err)
	}
	if n := orders.creates.Load(); n != 1 {
		t.Fatalf("server saw %d creates, want 1", n)
	}
}
