package hardening

import (
	"sync/atomic"

	"github.com/gorilla/websocket"
)

// CentrifugoUnsubscribe encodes a server unsubscribe push for channel.
// Wire shape: Reply.push { channel = 2, unsubscribe = 7 { code = 2, reason = 3 } }.
func CentrifugoUnsubscribe(channel string, code uint32, reason string) []byte {
	unsubscribe := appendVarint(nil, 2<<3)
	unsubscribe = appendVarint(unsubscribe, uint64(code))
	unsubscribe = appendBytesField(unsubscribe, 3, []byte(reason))
	push := appendBytesField(nil, 2, []byte(channel))
	push = appendBytesField(push, 7, unsubscribe)
	return lengthDelimit(appendBytesField(nil, 4, push))
}

// SpawnCentrifugoFrames completes each connect and subscribe, then writes raw
// reply frames from frames to whichever connection receives them.
func SpawnCentrifugoFrames(active *atomic.Int64, frames <-chan []byte) *MockWSServer {
	return spawnWS(active, func(conn *websocket.Conn, active *atomic.Int64, connects *atomic.Int64) {
		connects.Add(1)
		active.Add(1)
		defer active.Add(-1)
		for replies := 0; replies < 2; {
			messageType, _, err := conn.ReadMessage()
			if err != nil {
				return
			}
			if messageType == websocket.BinaryMessage {
				replies++
				if err := conn.WriteMessage(websocket.BinaryMessage, CentrifugoOKReply(uint32(replies))); err != nil {
					return
				}
			}
		}
		done := make(chan struct{})
		go func() {
			defer close(done)
			for {
				if _, _, err := conn.ReadMessage(); err != nil {
					return
				}
			}
		}()
		for {
			select {
			case <-done:
				return
			case frame, ok := <-frames:
				if !ok {
					return
				}
				if err := conn.WriteMessage(websocket.BinaryMessage, frame); err != nil {
					return
				}
			}
		}
	})
}
