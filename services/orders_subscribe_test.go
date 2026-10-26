package services

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/Fabric-Labs/polyester-sdk-go/auth"
	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	orderv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/orders/v1"
	privatewsv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/privatews/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/realtime"
	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/proto"
)

// TestOrdersSubscribeUsesPrivateWebSocket checks that Orders.Subscribe goes
// through /v1/ws/private with a base-channel subscribe token for the default
// account, and decodes orders to models.Order.
func TestOrdersSubscribeUsesPrivateWebSocket(t *testing.T) {
	account := codecs.FormatID(42)
	var mu sync.Mutex
	var channels []string
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/rt/token", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"token":"connect"}`))
	})
	mux.HandleFunc("/v1/rt/subscribe", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		channels = append(channels, r.URL.Query().Get("channel"))
		mu.Unlock()
		_, _ = w.Write([]byte(`{"token":"sub"}`))
	})
	upgrader := websocket.Upgrader{Subprotocols: []string{"polyester.v1.proto"}}
	serverDone := make(chan struct{})
	mux.HandleFunc("/v1/ws/private", func(w http.ResponseWriter, r *http.Request) {
		ws, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer close(serverDone)
		defer func() { _ = ws.Close() }()
		read := func() *privatewsv1.ClientMessage {
			_, data, err := ws.ReadMessage()
			if err != nil {
				return nil
			}
			msg := &privatewsv1.ClientMessage{}
			if proto.Unmarshal(data, msg) != nil {
				return nil
			}
			return msg
		}
		write := func(msg *privatewsv1.ServerMessage) {
			data, _ := proto.Marshal(msg)
			_ = ws.WriteMessage(websocket.BinaryMessage, data)
		}
		if read().GetAuth().GetToken() != "connect" {
			return
		}
		request := read().GetSubscribe()
		if request.GetAccount() != 42 || request.GetToken() != "sub" {
			return
		}
		write(&privatewsv1.ServerMessage{Sub: "s1", Frame: &privatewsv1.ServerMessage_Subscribed{
			Subscribed: &privatewsv1.Subscribed{Id: request.GetId(), Account: 42},
		}})
		write(&privatewsv1.ServerMessage{Sub: "s1", Seq: 1, Frame: &privatewsv1.ServerMessage_SnapshotBegin{SnapshotBegin: &privatewsv1.SnapshotBegin{}}})
		write(&privatewsv1.ServerMessage{Sub: "s1", Seq: 2, Frame: &privatewsv1.ServerMessage_SnapshotPage{SnapshotPage: &privatewsv1.SnapshotPage{
			Orders: []*orderv1.Order{{OrderId: 7, SymbolId: 1, Version: 1, Status: orderv1.OrderStatus_WORKING}},
		}}})
		write(&privatewsv1.ServerMessage{Sub: "s1", Seq: 3, Frame: &privatewsv1.ServerMessage_SnapshotEnd{SnapshotEnd: &privatewsv1.SnapshotEnd{Cursor: "c1", Count: 1}}})
		read() // until the client leaves
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	kp := auth.GenerateEd25519Keypair()
	creds, err := auth.LoadCredentials("test-key", kp.SecretKeyHex, false)
	if err != nil {
		t.Fatal(err)
	}
	rt := realtime.NewClient("ws://127.0.0.1:1", srv.URL, creds, nil, 16)
	defer rt.Close()
	orders := NewOrdersService(nil, nil, nil, rt, &account, nil, nil)

	sub, err := orders.Subscribe(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	next := func() realtime.OrderStreamEvent {
		select {
		case event, ok := <-sub.Messages():
			if !ok {
				t.Fatalf("stream ended: %v", sub.Err())
			}
			return event
		case <-time.After(5 * time.Second):
			t.Fatal("no event")
			return nil
		}
	}
	if _, ok := next().(realtime.OrderStreamConnected); !ok {
		t.Fatal("want OrderStreamConnected first")
	}
	snapshot, ok := next().(realtime.OrderSnapshot)
	if !ok || len(snapshot.Orders) != 1 || snapshot.Orders[0].OrderID != codecs.FormatID(7) || snapshot.Orders[0].Status != "working" {
		t.Fatalf("snapshot %#v", snapshot)
	}
	sub.Close()
	<-serverDone
	mu.Lock()
	defer mu.Unlock()
	if len(channels) != 1 || channels[0] != "private:spot:orders:"+account {
		t.Fatalf("subscribe token channels %v", channels)
	}
}

func TestOrdersSubscribeRequiresRealtimeAndAccount(t *testing.T) {
	var realtimeErr *sdkerrors.RealtimeError
	if _, err := NewOrdersService(nil, nil, nil, nil, nil, nil, nil).Subscribe(context.Background(), "abc"); !errors.As(err, &realtimeErr) {
		t.Fatalf("want RealtimeError without a realtime client, got %v", err)
	}
	rt := realtime.NewClient("ws://127.0.0.1:1", "http://127.0.0.1:1", nil, nil, 1)
	var validationErr *sdkerrors.ValidationError
	if _, err := NewOrdersService(nil, nil, nil, rt, nil, nil, nil).Subscribe(context.Background(), nil); !errors.As(err, &validationErr) {
		t.Fatalf("want ValidationError without an account, got %v", err)
	}
}
