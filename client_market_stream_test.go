package polyester

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/Fabric-Labs/polyester-sdk-go/marketstream"
)

// MarketStream connects to the environment's WebSocket host at the market
// stream's path, without credentials, and Close closes it.
func TestMarketStreamUsesTheClientsWebSocketHostAndClosesWithIt(t *testing.T) {
	paths := make(chan string, 4)
	auth := make(chan string, 4)
	upgrader := websocket.Upgrader{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths <- r.URL.Path
		auth <- r.Header.Get("Authorization")
		ws, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer func() { _ = ws.Close() }()
		for {
			if _, _, err := ws.ReadMessage(); err != nil {
				return
			}
		}
	}))
	defer srv.Close()

	client, err := New(Config{APIURL: srv.URL, WSURL: "ws" + strings.TrimPrefix(srv.URL, "http"), SkipCatalogHydration: true})
	if err != nil {
		t.Fatal(err)
	}
	stream, err := client.MarketStream(context.Background(), marketstream.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if got := <-paths; got != marketstream.Path {
		t.Fatalf("connected to %q, want %q", got, marketstream.Path)
	}
	if got := <-auth; got != "" {
		t.Fatalf("sent Authorization %q to a public stream", got)
	}
	_ = client.Close()
	select {
	case <-stream.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("Close left the market stream open")
	}
	if _, err := client.MarketStream(context.Background(), marketstream.Config{}); !errors.Is(err, marketstream.ErrClosed) {
		t.Fatalf("MarketStream after Close: %v", err)
	}
}
