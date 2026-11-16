package marketstream

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	ms "github.com/Fabric-Labs/polyester-sdk-go/internal/sbe/marketstream"
)

// Frames captured from the server, one WebSocket message per file, listed by
// each capture's frames.tsv. POLYESTER_MARKETSTREAM_GOLDEN names the directory
// of captures; the tests skip without it.

type goldenFrame struct {
	file, direction, message string
	data                     []byte
}

func goldenCaptures(t *testing.T) map[string][]goldenFrame {
	t.Helper()
	root := os.Getenv("POLYESTER_MARKETSTREAM_GOLDEN")
	if root == "" {
		t.Skip("POLYESTER_MARKETSTREAM_GOLDEN is not set")
	}
	dirs, err := filepath.Glob(filepath.Join(root, "capture-*"))
	if err != nil || len(dirs) == 0 {
		t.Fatalf("no captures under %s", root)
	}
	out := map[string][]goldenFrame{}
	for _, dir := range dirs {
		index, err := os.ReadFile(filepath.Join(dir, "frames.tsv"))
		if err != nil {
			t.Fatal(err)
		}
		for i, line := range strings.Split(strings.TrimSpace(string(index)), "\n") {
			cols := strings.Split(line, "\t")
			if i == 0 || len(cols) < 3 {
				continue
			}
			data, err := os.ReadFile(filepath.Join(dir, cols[0]))
			if err != nil {
				t.Fatal(err)
			}
			out[filepath.Base(dir)] = append(out[filepath.Base(dir)], goldenFrame{file: cols[0], direction: cols[1], message: cols[2], data: data})
		}
	}
	return out
}

var goldenTypes = map[string]reflect.Type{
	"Trades":            reflect.TypeFor[*tradesMsg](),
	"TradesSkipped":     reflect.TypeFor[*tradesSkippedMsg](),
	"TradesThrough":     reflect.TypeFor[*tradesThroughMsg](),
	"BestBidOffer":      reflect.TypeFor[*bboMsg](),
	"DepthSnapshot":     reflect.TypeFor[*depthSnapshotMsg](),
	"DepthUpdate":       reflect.TypeFor[*depthUpdateMsg](),
	"MarketStatus":      reflect.TypeFor[*marketStatusMsg](),
	"SubscribeResponse": reflect.TypeFor[*subscribeResponseMsg](),
	"Pong":              reflect.TypeFor[*pongMsg](),
}

// Every server frame decodes to its message, and every client frame is what
// this client encodes for the same request.
func TestGoldenFramesDecodeAndEncode(t *testing.T) {
	seen := map[string]int{}
	for capture, frames := range goldenCaptures(t) {
		for _, f := range frames {
			name := capture + "/" + f.file
			if f.direction == "client" {
				var h ms.MessageHeader
				h.Wrap(f.data, 0, 0, uint64(len(f.data)))
				var again []byte
				switch f.message {
				case "Subscribe":
					var m ms.Subscribe
					m.WrapForDecode(f.data, uint64(headerLength), uint64(h.BlockLength()), uint64(h.Version()), uint64(len(f.data)))
					var topics []Topic
					items := m.Items()
					for items.HasNext() {
						it := items.Next()
						topics = append(topics, Topic{SymbolID: it.SymbolId(), Stream: Stream(it.Stream())})
					}
					again = encodeSubscribe(m.RequestId(), topics)
				case "Ping":
					var m ms.Ping
					m.WrapForDecode(f.data, uint64(headerLength), uint64(h.BlockLength()), uint64(h.Version()), uint64(len(f.data)))
					again = encodePing(m.RequestId())
				default:
					t.Fatalf("%s: client message %s", name, f.message)
				}
				if !bytes.Equal(again, f.data) {
					t.Fatalf("%s: this client encodes\n%x\nthe capture holds\n%x", name, again, f.data)
				}
				seen["client "+f.message]++
				continue
			}
			got, err := decodeFrame(f.data)
			if err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			if want := goldenTypes[f.message]; want == nil || reflect.TypeOf(got) != want {
				t.Fatalf("%s: %s decoded to %T", name, f.message, got)
			}
			seen[f.message]++
		}
	}
	for _, m := range []string{"client Subscribe", "client Ping", "SubscribeResponse", "Pong", "Trades", "TradesThrough", "BestBidOffer", "DepthSnapshot", "DepthUpdate", "MarketStatus"} {
		if seen[m] == 0 {
			t.Errorf("no captured %s", m)
		}
	}
}

type anyPairs struct{}

func (anyPairs) BaseQuantityScaleForSymbolID(uint32) (int, bool) { return 8, true }
func (anyPairs) SymbolForSymbolID(uint32) *string                { return nil }

// Each capture replays through the client's handlers without a protocol
// error: books follow the depth stream's exact-base chain across the
// replacement snapshot, and trades follow their frontier.
func TestGoldenCapturesReplay(t *testing.T) {
	for capture, frames := range goldenCaptures(t) {
		t.Run(capture, func(t *testing.T) {
			c := newClient(Config{Pairs: anyPairs{}, QueueSize: 1 << 16})
			defer c.cancel()
			var lastDepth uint64
			// A capture may keep only some frames of a template: trades of
			// a pair whose baseline it did not keep are not replayed.
			baseline := map[uint32]bool{}
			for _, f := range frames {
				if f.direction == "client" {
					// The captured client's requests, as this client records
					// what it sends: responses are matched to them by ID.
					if f.message == "Subscribe" {
						var h ms.MessageHeader
						h.Wrap(f.data, 0, 0, uint64(len(f.data)))
						var m ms.Subscribe
						m.WrapForDecode(f.data, uint64(headerLength), uint64(h.BlockLength()), uint64(h.Version()), uint64(len(f.data)))
						var topics []Topic
						items := m.Items()
						for items.HasNext() {
							it := items.Next()
							topics = append(topics, Topic{SymbolID: it.SymbolId(), Stream: Stream(it.Stream())})
						}
						c.recordSent(m.RequestId(), false, topics)
					}
					continue
				}
				decoded, _ := decodeFrame(f.data)
				switch m := decoded.(type) {
				case *tradesThroughMsg:
					baseline[m.symbolID] = true
				case *tradesMsg:
					if !baseline[m.symbolID] {
						continue
					}
				case *tradesSkippedMsg:
					if !baseline[m.symbolID] {
						continue
					}
				}
				if err := c.handle(f.data); err != nil {
					t.Fatalf("%s: %v", f.file, err)
				}
				switch m := decoded.(type) {
				case *depthUpdateMsg:
					lastDepth = m.last
				case *depthSnapshotMsg:
					if Stream(m.stream) == StreamDepth {
						lastDepth = m.updateID
					}
				}
			}
			if c.Err() != nil {
				t.Fatal(c.Err())
			}
			for topic, b := range c.books {
				// A capture that ends stale leaves the book waiting for its
				// replacement snapshot.
				stale := c.feeds[topic.SymbolID] == FeedStale
				if topic.Stream == StreamDepth && b.recovering {
					// A capture of the server's old order (LIVE before the
					// replacement snapshot) leaves the stream recovering; the
					// capture has no answer to its requests.
					t.Logf("%s leaves pair %d's depth stream recovering", capture, topic.SymbolID)
					continue
				}
				if topic.Stream == StreamDepth && (b.synced == stale || b.updateID != lastDepth) {
					t.Fatalf("depth book of %d at %d (synced %t, feed stale %t), the stream ended at %d", topic.SymbolID, b.updateID, b.synced, stale, lastDepth)
				}
			}
			gaps := 0
			for len(c.events) > 0 {
				if _, ok := (<-c.events).(*TradesGap); ok {
					gaps++
				}
			}
			if gaps != 0 {
				t.Fatalf("%d trade gaps in a capture without lost trades", gaps)
			}
		})
	}
}
