package marketstream

import (
	"errors"
	"reflect"
	"testing"

	ms "github.com/Fabric-Labs/polyester-sdk-go/internal/sbe/marketstream"
)

func TestDecodeEveryServerTemplate(t *testing.T) {
	cases := []struct {
		name  string
		frame []byte
		want  any
	}{
		{"trades", tradesFrame(7, 1, 42, fill(1), fill(2)), &tradesMsg{symbolID: 7, epoch: 1, updateID: 42, transactNs: 1_700_000_000_000_000_000, sendNs: 1_700_000_000_000_001_000, fills: []fillMsg{fill(1), fill(2)}}},
		{"trades through", tradesThroughFrame(7, 2, 1<<40), &tradesThroughMsg{symbolID: 7, epoch: 2, last: 1 << 40, sendNs: 1}},
		{"trades skipped", tradesSkippedFrame(7, 1, 5, 0), &tradesSkippedMsg{symbolID: 7, epoch: 1, first: 5, sendNs: 1}},
		{"bbo", bboFrame(7, 1, 9, 100, 2, noPrice, 0), &bboMsg{symbolID: 7, epoch: 1, updateID: 9, transactNs: 1, sendNs: 2, bidPrice: 100, bidQty: 2, askPrice: noPrice}},
		{"depth20 snapshot", depthSnapshotFrame(7, StreamDepth20, 1, 9, []levelMsg{lv(100, 1)}, []levelMsg{lv(101, 2), lv(102, 3)}),
			&depthSnapshotMsg{symbolID: 7, stream: uint8(StreamDepth20), epoch: 1, updateID: 9, transactNs: 1, sendNs: 2, levelsPerSide: 20, bids: []levelMsg{lv(100, 1)}, asks: []levelMsg{lv(101, 2), lv(102, 3)}}},
		{"depth snapshot", depthSnapshotFrame(7, StreamDepth, 1, 9, nil, nil), &depthSnapshotMsg{symbolID: 7, stream: uint8(StreamDepth), epoch: 1, updateID: 9, transactNs: 1, sendNs: 2, levelsPerSide: 1000, bids: []levelMsg{}, asks: []levelMsg{}}},
		{"depth update", depthUpdateFrame(7, 1, 10, 12, []levelMsg{lv(100, 0)}, nil), &depthUpdateMsg{symbolID: 7, epoch: 1, first: 10, last: 12, transactNs: 1, sendNs: 2, bids: []levelMsg{lv(100, 0)}, asks: []levelMsg{}}},
		{"market status", marketStatusFrame(7, FeedStale), &marketStatusMsg{symbolID: 7, feed: uint8(FeedStale), sendNs: 3}},
		{"subscribe response", subscribeResponseFrame(5, resultMsg{7, 1, 1}, resultMsg{8, 3, 2}), &subscribeResponseMsg{requestID: 5, results: []resultMsg{{7, 1, 1}, {8, 3, 2}}}},
		{"pong", pongFrame(11), &pongMsg{requestID: 11, sendNs: 4}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := decodeFrame(tc.frame)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("decoded %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestUnknownTemplateIsSkipped(t *testing.T) {
	// A template a newer server adds, with a body this client cannot know.
	frame := withHeader(append(pongFrame(1), 1, 2, 3), 19, 99, SchemaID, SchemaVersion+1)
	got, err := decodeFrame(frame)
	if got != nil || err != nil {
		t.Fatalf("unknown template decoded to %v, %v; want skipped", got, err)
	}
	// Client templates are not server messages either.
	got, err = decodeFrame(encodePing(3))
	if got != nil || err != nil {
		t.Fatalf("ping decoded to %v, %v; want skipped", got, err)
	}
}

func TestAnotherSchemaIsRefused(t *testing.T) {
	frame := withHeader(pongFrame(1), ms.PongSbeBlockLength, ms.PongSbeTemplateID, 7, 0)
	_, err := decodeFrame(frame)
	var schema *SchemaError
	if !errors.As(err, &schema) || schema.SchemaID != 7 {
		t.Fatalf("err %v, want SchemaError for schema 7", err)
	}
}

func TestNewerVersionIsReadByItsBlockLengths(t *testing.T) {
	cases := []struct {
		name   string
		frame  []byte
		groups int
	}{
		{"trades", tradesFrame(7, 1, 42, fill(1), fill(2)), 1},
		{"bbo", bboFrame(7, 1, 9, 100, 2, 101, 3), 0},
		{"depth snapshot", depthSnapshotFrame(7, StreamDepth, 1, 9, []levelMsg{lv(100, 1), lv(99, 4)}, []levelMsg{lv(101, 2)}), 2},
		{"depth update", depthUpdateFrame(7, 1, 10, 12, []levelMsg{lv(100, 0)}, []levelMsg{lv(101, 5)}), 2},
		{"subscribe response", subscribeResponseFrame(5, resultMsg{7, 1, 1}, resultMsg{8, 3, 2}), 1},
		{"trades through", tradesThroughFrame(7, 2, 1<<40), 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			want, err := decodeFrame(tc.frame)
			if err != nil {
				t.Fatal(err)
			}
			got, err := decodeFrame(newerVersion(tc.frame, 13, 5, tc.groups))
			if err != nil {
				t.Fatalf("newer version refused: %v", err)
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("newer version decoded %+v, want %+v", got, want)
			}
		})
	}
}

func TestLayoutsThisVersionCannotReadAreRefused(t *testing.T) {
	trades := tradesFrame(7, 1, 42, fill(1), fill(2))
	shortBlock := withHeader(trades, ms.TradesSbeBlockLength-1, ms.TradesSbeTemplateID, SchemaID, SchemaVersion+1)
	shortEntry := append([]byte(nil), trades...)
	shortEntry[headerLength+int(ms.TradesSbeBlockLength)] = 24 // entries of 24 bytes, this version's are 25
	trailing := append(append([]byte(nil), trades...), 0)
	for name, frame := range map[string][]byte{
		"newer version with a shorter block": shortBlock,
		"group entries shorter than known":   shortEntry,
		"bytes after the last group":         trailing,
		"header only":                        trades[:headerLength],
		"block past the end":                 trades[:headerLength+10],
	} {
		t.Run(name, func(t *testing.T) {
			_, err := decodeFrame(frame)
			var frameErr *FrameError
			if !errors.As(err, &frameErr) {
				t.Fatalf("err %v, want FrameError", err)
			}
		})
	}
}

func TestEveryTruncationIsRefusedWithoutPanic(t *testing.T) {
	frames := [][]byte{
		tradesFrame(7, 1, 42, fill(1), fill(2)),
		depthSnapshotFrame(7, StreamDepth, 1, 9, []levelMsg{lv(100, 1)}, []levelMsg{lv(101, 2)}),
		depthUpdateFrame(7, 1, 10, 12, []levelMsg{lv(100, 0)}, []levelMsg{lv(101, 5)}),
		subscribeResponseFrame(5, resultMsg{7, 1, 1}),
		bboFrame(7, 1, 9, 100, 2, 101, 3),
	}
	for _, frame := range frames {
		for n := range len(frame) {
			if _, err := decodeFrame(frame[:n]); err == nil {
				t.Fatalf("a %d-byte prefix of a %d-byte frame decoded", n, len(frame))
			}
		}
	}
}

func FuzzDecodeFrame(f *testing.F) {
	f.Add(tradesFrame(7, 1, 42, fill(1)))
	f.Add(depthSnapshotFrame(7, StreamDepth20, 1, 9, []levelMsg{lv(100, 1)}, nil))
	f.Add(newerVersion(depthUpdateFrame(7, 1, 10, 12, []levelMsg{lv(100, 0)}, nil), 3, 2, 2))
	f.Fuzz(func(t *testing.T, frame []byte) {
		_, _ = decodeFrame(frame) // must never panic
	})
}

func TestClientMessagesEncodeTheSchema(t *testing.T) {
	topics := []Topic{{SymbolID: 7, Stream: StreamTrades}, {SymbolID: 8, Stream: StreamDepth}}
	frame := encodeSubscribe(99, topics)
	var h ms.MessageHeader
	h.Wrap(frame, 0, 0, uint64(len(frame)))
	if h.TemplateId() != ms.SubscribeSbeTemplateID || h.SchemaId() != SchemaID || h.Version() != SchemaVersion || h.BlockLength() != ms.SubscribeSbeBlockLength {
		t.Fatalf("subscribe header %d/%d/%d/%d", h.TemplateId(), h.SchemaId(), h.Version(), h.BlockLength())
	}
	var m ms.Subscribe
	m.WrapForDecode(frame, uint64(headerLength), uint64(h.BlockLength()), uint64(h.Version()), uint64(len(frame)))
	if m.RequestId() != 99 {
		t.Fatalf("request ID %d", m.RequestId())
	}
	items := m.Items()
	var got []Topic
	for items.HasNext() {
		it := items.Next()
		got = append(got, Topic{SymbolID: it.SymbolId(), Stream: Stream(it.Stream())})
	}
	if !reflect.DeepEqual(got, topics) || m.SbePosition() != uint64(len(frame)) {
		t.Fatalf("items %v, position %d of %d", got, m.SbePosition(), len(frame))
	}

	unsub := encodeUnsubscribe(98, topics[:1])
	h.Wrap(unsub, 0, 0, uint64(len(unsub)))
	var u ms.Unsubscribe
	u.WrapForDecode(unsub, uint64(headerLength), uint64(h.BlockLength()), 0, uint64(len(unsub)))
	uitems := u.Items()
	if h.TemplateId() != ms.UnsubscribeSbeTemplateID || u.RequestId() != 98 || uitems.Count() != 1 {
		t.Fatalf("unsubscribe %x", unsub)
	}
	if it := uitems.Next(); it.SymbolId() != 7 || Stream(it.Stream()) != StreamTrades || u.SbePosition() != uint64(len(unsub)) {
		t.Fatalf("unsubscribe item %x", unsub)
	}

	ping := encodePing(5)
	h.Wrap(ping, 0, 0, uint64(len(ping)))
	var p ms.Ping
	p.WrapForDecode(ping, uint64(headerLength), uint64(h.BlockLength()), 0, uint64(len(ping)))
	if h.TemplateId() != ms.PingSbeTemplateID || p.RequestId() != 5 || len(ping) != headerLength+int(ms.PingSbeBlockLength) {
		t.Fatalf("ping %x", ping)
	}
}

func TestURL(t *testing.T) {
	for base, want := range map[string]string{
		"wss://api.testnet.polyester.com":                      "wss://api.testnet.polyester.com/ws/market.sbe",
		"wss://api.testnet.polyester.com/":                     "wss://api.testnet.polyester.com/ws/market.sbe",
		"wss://api.testnet.polyester.com/connection/websocket": "wss://api.testnet.polyester.com/ws/market.sbe",
		"wss://api.testnet.polyester.com/ws/market.sbe":        "wss://api.testnet.polyester.com/ws/market.sbe",
		"ws://127.0.0.1:8080/edge":                             "ws://127.0.0.1:8080/edge/ws/market.sbe",
	} {
		got, err := URL(base)
		if err != nil || got != want {
			t.Errorf("URL(%q) = %q, %v; want %q", base, got, err, want)
		}
	}
	for _, bad := range []string{"https://api.polyester.com", "wss://h/?x=1", "wss://h/#f", "wss://", ":"} {
		if _, err := URL(bad); err == nil {
			t.Errorf("URL(%q) accepted", bad)
		}
	}
}
