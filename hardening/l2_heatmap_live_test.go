package hardening_test

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	heatmapv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/marketdata/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/hardening"
	"github.com/Fabric-Labs/polyester-sdk-go/services"
	"github.com/Fabric-Labs/polyester-sdk-go/transport"
	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/proto"
)

// heatmapPublisher behaves like the public heatmap stream: it acknowledges a
// subscribe to any channel, but publishes live buckets only on
// public:spot:market:heatmap:<1s|1m|5m|1h>:<symbol_id>:proto.
type heatmapPublisher struct {
	t          *testing.T
	mu         sync.Mutex
	subscribed []string
	published  map[string]heatmapv1.HeatmapInterval
}

func newHeatmapPublisher(t *testing.T) *heatmapPublisher {
	return &heatmapPublisher{t: t, published: map[string]heatmapv1.HeatmapInterval{
		"public:spot:market:heatmap:1s:1:proto": heatmapv1.HeatmapInterval_INTERVAL_1S,
		"public:spot:market:heatmap:1m:1:proto": heatmapv1.HeatmapInterval_INTERVAL_1M,
		"public:spot:market:heatmap:5m:1:proto": heatmapv1.HeatmapInterval_INTERVAL_5M,
		"public:spot:market:heatmap:1h:1:proto": heatmapv1.HeatmapInterval_INTERVAL_1H,
	}}
}

func (p *heatmapPublisher) serveWS(w http.ResponseWriter, r *http.Request) {
	upgrader := websocket.Upgrader{Subprotocols: []string{"centrifuge-protobuf"}}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer func() { _ = conn.Close() }()
	if _, _, err = conn.ReadMessage(); err != nil {
		return
	}
	if err = conn.WriteMessage(websocket.BinaryMessage, hardening.CentrifugoOKReply(1)); err != nil {
		return
	}
	_, command, err := conn.ReadMessage()
	if err != nil {
		return
	}
	start := strings.Index(string(command), "public:")
	if start < 1 {
		return
	}
	channel := string(command[start : start+int(command[start-1])])
	p.mu.Lock()
	p.subscribed = append(p.subscribed, channel)
	p.mu.Unlock()
	if err = conn.WriteMessage(websocket.BinaryMessage, hardening.CentrifugoOKReply(2)); err != nil {
		return
	}
	if interval, ok := p.published[channel]; ok {
		payload, err := proto.Marshal(&heatmapv1.HeatmapLiveBucket{SymbolId: 1, Interval: interval, TsSec: 1_700_000_000, UpdatesInBucket: 3})
		if err != nil {
			p.t.Error(err)
			return
		}
		if err = conn.WriteMessage(websocket.BinaryMessage, hardening.CentrifugoPublication(payload)); err != nil {
			return
		}
	}
	for {
		if _, _, err = conn.ReadMessage(); err != nil {
			return
		}
	}
}

// Every heatmap interval, by alias and by enum name, subscribes to the
// channel the server publishes on and receives its live buckets.
func TestHeatmapSubscribeLiveReceivesPublishedBucketsForEveryInterval(t *testing.T) {
	cases := []struct {
		interval string
		channel  string
		want     string
	}{
		{"1s", "public:spot:market:heatmap:1s:1:proto", "INTERVAL_1S"},
		{"1m", "public:spot:market:heatmap:1m:1:proto", "INTERVAL_1M"},
		{"5m", "public:spot:market:heatmap:5m:1:proto", "INTERVAL_5M"},
		{"1h", "public:spot:market:heatmap:1h:1:proto", "INTERVAL_1H"},
		{"INTERVAL_1S", "public:spot:market:heatmap:1s:1:proto", "INTERVAL_1S"},
		{"INTERVAL_5M", "public:spot:market:heatmap:5m:1:proto", "INTERVAL_5M"},
	}
	for _, tc := range cases {
		t.Run(tc.interval, func(t *testing.T) {
			publisher := newHeatmapPublisher(t)
			ws := hardening.NewTestServer(http.HandlerFunc(publisher.serveWS))
			t.Cleanup(ws.Close)
			rt := newRT("ws"+strings.TrimPrefix(ws.URL, "http"), ws.URL, nil, 5*time.Second)
			t.Cleanup(rt.Close)
			service := services.NewHeatmapService(transport.NewFactory(transport.DefaultConfig(), nil, nil), nil, rt)
			id := uint32(1)
			sub, err := service.SubscribeLive(context.Background(), nil, &id, tc.interval)
			if err != nil {
				t.Fatal(err)
			}
			defer sub.Close()
			select {
			case bucket := <-sub.Messages():
				if got := bucket.Raw["interval"]; got != tc.want {
					t.Fatalf("bucket interval = %v, want %s", got, tc.want)
				}
			case <-time.After(3 * time.Second):
				publisher.mu.Lock()
				defer publisher.mu.Unlock()
				t.Fatalf("no live bucket within 3s; subscribed %q, published on %q", publisher.subscribed, tc.channel)
			}
		})
	}
}

// An interval the server does not publish, or a quantity mode in its place,
// is refused before connecting instead of subscribing to a silent channel.
func TestHeatmapSubscribeLiveRefusesUnpublishedIntervals(t *testing.T) {
	service := services.NewHeatmapService(transport.NewFactory(transport.DefaultConfig(), nil, nil), nil, nil)
	id := uint32(1)
	for _, interval := range []string{"", "2m", "PEAK", "INTERVAL_UNSPECIFIED", "1S"} {
		if _, err := service.SubscribeLive(context.Background(), nil, &id, interval); err == nil || !strings.Contains(err.Error(), "heatmap interval") {
			t.Fatalf("interval %q: err = %v, want a heatmap interval refusal", interval, err)
		}
	}
}
