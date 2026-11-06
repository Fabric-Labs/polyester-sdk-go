package services

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Fabric-Labs/polyester-sdk-go/catalogs"
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	overviewpb "github.com/Fabric-Labs/polyester-sdk-go/gen/marketoverview/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
	"github.com/Fabric-Labs/polyester-sdk-go/realtime"
	"github.com/Fabric-Labs/polyester-sdk-go/transport"
	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/proto"
)

type overviewTicketSend struct {
	data []byte
	sent chan error
}

func overviewTicketService(t *testing.T) (*MarketOverviewService, func([]byte), <-chan struct{}, *atomic.Int32) {
	t.Helper()
	frames := make(chan overviewTicketSend)
	stop := make(chan struct{})
	released := make(chan struct{})
	conns := new(atomic.Int32)
	upgrader := websocket.Upgrader{Subprotocols: []string{"centrifuge-protobuf"}}
	ws := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		index := conns.Add(1)
		defer func() {
			_ = conn.Close()
			if index == 1 {
				close(released)
			}
		}()
		for _, ack := range [][]byte{{2, 8, 1}, {2, 8, 2}} {
			if _, _, err = conn.ReadMessage(); err != nil {
				return
			}
			if err = conn.WriteMessage(websocket.BinaryMessage, ack); err != nil {
				return
			}
		}
		peerClosed := make(chan struct{})
		go func() {
			defer close(peerClosed)
			for {
				if _, _, err := conn.ReadMessage(); err != nil {
					return
				}
			}
		}()
		for {
			select {
			case frame := <-frames:
				frame.sent <- conn.WriteMessage(websocket.BinaryMessage, frame.data)
			case <-peerClosed:
				return
			case <-stop:
				return
			}
		}
	}))
	httpSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/marketoverview.v1.MarketOverviewService/ListMarketOverview" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/proto")
		body, _ := proto.Marshal(&overviewpb.ListMarketOverviewResponse{Markets: []*overviewpb.MarketOverview{{SymbolId: 1, LastPriceTicks: 1}}})
		_, _ = w.Write(body)
	}))
	t.Cleanup(func() { close(stop); ws.Close(); httpSrv.Close() })
	rt := realtime.NewClient("ws"+strings.TrimPrefix(ws.URL, "http"), httpSrv.URL, nil, nil, 200)
	t.Cleanup(rt.Close)
	factory := transport.NewFactory(transport.Config{APIURL: httpSrv.URL, Timeout: 10 * time.Second}, nil, nil)
	send := func(data []byte) {
		t.Helper()
		sent := make(chan error, 1)
		select {
		case frames <- overviewTicketSend{data, sent}:
		case <-time.After(10 * time.Second):
			t.Fatal("frame never dispatched")
		}
		select {
		case err := <-sent:
			if err != nil {
				t.Fatal(err)
			}
		case <-time.After(10 * time.Second):
			t.Fatal("frame never sent")
		}
	}
	return NewMarketOverviewService(factory, catalogs.NewManager(), rt), send, released, conns
}

func overviewTicketFrame(price int64) []byte {
	data, _ := proto.Marshal(&overviewpb.MarketOverviewBatch{Markets: []*overviewpb.MarketOverview{{SymbolId: 1, LastPriceTicks: price}}})
	return append([]byte{byte(len(data) + 6), 34, byte(len(data) + 4), 34, byte(len(data) + 2), 34, byte(len(data))}, data...)
}

func overviewTicketNext(t *testing.T, updates <-chan []models.MarketOverviewEntry) ([]models.MarketOverviewEntry, bool) {
	t.Helper()
	select {
	case rows, ok := <-updates:
		return rows, ok
	case <-time.After(10 * time.Second):
		t.Fatal("overview consumer remained pending")
		return nil, false
	}
}

func TestTicket8511MarketOverviewTerminalLifecycle(t *testing.T) {
	for _, mode := range []string{"bad_frame", "overflow", "close"} {
		t.Run(mode, func(t *testing.T) {
			svc, send, released, conns := overviewTicketService(t)
			events := make(chan []models.MarketOverviewEntry, 1)
			reported := make(chan error, 10)
			sub, err := svc.CreateSubscription(context.Background(), MarketOverviewCreateSubscriptionOptions{
				OnEvent: func(rows []models.MarketOverviewEntry) { events <- rows }, OnError: func(err error) { reported <- err },
			})
			if err != nil {
				t.Fatal(err)
			}
			expectPrice := func(rows []models.MarketOverviewEntry, price int64) {
				t.Helper()
				if len(rows) != 1 || rows[0].LastPrice.Ticks() != price {
					t.Fatalf("expected price %d: %+v", price, rows)
				}
			}
			row, _ := overviewTicketNext(t, events)
			expectPrice(row, 1)
			if mode != "overflow" {
				row, ok := overviewTicketNext(t, sub.Updates())
				if !ok {
					t.Fatal("healthy snapshot missing")
				}
				expectPrice(row, 1)
			}
			last := int64(3)
			if mode == "close" {
				last = 1
			}
			if mode == "overflow" {
				last = 51
			}
			for price := int64(2); price <= last; price++ {
				send(overviewTicketFrame(price))
				row, _ := overviewTicketNext(t, events)
				expectPrice(row, price)
			}
			if mode == "bad_frame" {
				send([]byte{0xff})
			}
			if mode != "close" {
				select {
				case err = <-reported:
				case <-time.After(10 * time.Second):
					t.Fatal("terminal error never observed")
				}
				if sub.Err() == nil || err.Error() != sub.Err().Error() {
					t.Fatalf("terminal error lost: %v / %v", err, sub.Err())
				}
				if mode == "overflow" {
					var typed *sdkerrors.QueueOverflowError
					if !errors.As(err, &typed) || err.Error() != "market overview subscription queue full; consumer too slow" {
						t.Fatalf("wrong overflow: %v", err)
					}
				} else {
					var typed *sdkerrors.RealtimeError
					if !errors.As(err, &typed) || !strings.Contains(err.Error(), "invalid realtime protobuf frame") {
						t.Fatalf("wrong terminal type: %T %v", err, err)
					}
				}
			}
			// Explicit close is a control and must also return after a terminal overflow.
			closeDone := make(chan struct{})
			if mode != "bad_frame" {
				go func() { sub.Close(); close(closeDone) }()
				select {
				case <-closeDone:
				case <-time.After(10 * time.Second):
					t.Fatal("overview Close self-waited")
				}
			}
			first, wantCount := int64(2), 2
			if mode == "close" {
				wantCount = 0
			}
			if mode == "overflow" {
				first, wantCount = 1, 50
			}
			count := 0
			for {
				rows, ok := overviewTicketNext(t, sub.Updates())
				if !ok {
					break
				}
				expectPrice(rows, first+int64(count))
				count++
			}
			if count != wantCount {
				t.Fatalf("queued rows lost: %d, want %d", count, wantCount)
			}
			if mode == "bad_frame" {
				sub.Close()
			}
			if mode == "close" && sub.Err() != nil {
				t.Fatal(sub.Err())
			}
			select {
			case err := <-reported:
				t.Fatalf("extra error callback: %v", err)
			default:
			}
			select {
			case <-released:
			case <-time.After(10 * time.Second):
				t.Fatal("socket was not released")
			}
			if conns.Load() != 1 {
				t.Fatalf("unexpected reconnects: %d", conns.Load())
			}
		})
	}
}
