package realtime

import (
	"context"
	"encoding/hex"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/Fabric-Labs/polyester-sdk-go/hardening"
)

func ticketPublication(data string) []byte {
	pub := appendBytesField(nil, 4, []byte(data))
	push := appendBytesField(nil, 4, pub)
	reply := appendBytesField(nil, 4, push)
	return append(appendVarint(nil, uint64(len(reply))), reply...)
}

func TestTicket8475SetupPreservesPublicationOrder(t *testing.T) {
	ack := []byte{2, 8, 2}
	first := ticketPublication("first")
	fixture, err := hex.DecodeString("0208020b2209220722056669727374")
	if err != nil {
		t.Fatal(err)
	}
	for name, frames := range map[string][][]byte{
		"fixture": {fixture}, "before": {append(append([]byte{}, first...), ack...)},
		"before_separate": {first, ack}, "split_control": {ack, first},
		"multiple": {append(append(append([]byte{}, first...), ack...), ticketPublication("second")...)},
	} {
		t.Run(name, func(t *testing.T) {
			serverErr := make(chan error, 1)
			upgrader := websocket.Upgrader{Subprotocols: []string{centrifugoProtobufSubprotocol}}
			srv := hardening.NewTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				conn, err := upgrader.Upgrade(w, r, nil)
				if err != nil {
					serverErr <- err
					return
				}
				defer func() { _ = conn.Close() }()
				for _, frame := range [][]byte{{2, 8, 1}, nil} {
					if _, _, err = conn.ReadMessage(); err != nil {
						serverErr <- err
						return
					}
					if frame != nil {
						if err = conn.WriteMessage(websocket.BinaryMessage, frame); err != nil {
							serverErr <- err
							return
						}
					}
				}
				for _, frame := range append(frames, ticketPublication("control")) {
					if err = conn.WriteMessage(websocket.BinaryMessage, frame); err != nil {
						serverErr <- err
						return
					}
				}
				serverErr <- nil
				for {
					if _, _, err = conn.ReadMessage(); err != nil {
						return
					}
				}
			}))
			defer srv.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			client := NewClient("ws"+strings.TrimPrefix(srv.URL, "http"), "", nil, nil, 200)
			sub, err := SubscribeProto(ctx, client, "public:test:proto", func(data []byte) (string, error) { return string(data), nil })
			if err != nil {
				t.Fatal(err)
			}
			defer sub.Close()
			var got []string
			for {
				select {
				case item, ok := <-sub.Messages():
					if !ok {
						t.Fatalf("closed: %v", sub.Err())
					}
					got = append(got, item)
					if item == "control" {
						goto complete
					}
				case <-ctx.Done():
					t.Fatal("publications never arrived")
				}
			}
		complete:
			want := []string{"first", "control"}
			if name == "multiple" {
				want = []string{"first", "second", "control"}
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("publications=%q want=%q", got, want)
			}
			if err := <-serverErr; err != nil {
				t.Fatal(err)
			}
		})
	}
}
