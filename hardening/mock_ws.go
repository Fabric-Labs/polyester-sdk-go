package hardening

import (
	"net"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

// MockWSServer is a local Centrifugo-like protobuf WebSocket server.
type MockWSServer struct {
	addr     string
	ln       net.Listener
	active   *atomic.Int64
	connects *atomic.Int64
	done     chan struct{}
}

var upgrader = websocket.Upgrader{
	CheckOrigin:  func(r *http.Request) bool { return true },
	Subprotocols: []string{"centrifuge-protobuf"},
}

// SpawnHangAfterAccept accepts WS upgrades and stays open without replying.
func SpawnHangAfterAccept() *MockWSServer {
	active := &atomic.Int64{}
	return SpawnHangAfterAcceptCounted(active)
}

// SpawnHangAfterAcceptCounted is like SpawnHangAfterAccept but tracks active conns.
func SpawnHangAfterAcceptCounted(active *atomic.Int64) *MockWSServer {
	if active == nil {
		active = &atomic.Int64{}
	}
	return spawnWS(active, func(conn *websocket.Conn, active *atomic.Int64, connects *atomic.Int64) {
		connects.Add(1)
		active.Add(1)
		defer active.Add(-1)
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	})
}

// SpawnCentrifugoPublic accepts WS upgrades, tracks active connections, and
// replies OK to the first two binary frames (connect + subscribe).
func SpawnCentrifugoPublic(active *atomic.Int64) *MockWSServer {
	if active == nil {
		active = &atomic.Int64{}
	}
	return spawnWS(active, func(conn *websocket.Conn, active *atomic.Int64, connects *atomic.Int64) {
		connects.Add(1)
		active.Add(1)
		defer active.Add(-1)
		replies := 0
		for {
			messageType, _, err := conn.ReadMessage()
			if err != nil {
				return
			}
			if messageType == websocket.BinaryMessage && replies < 2 {
				replies++
				id := uint32(replies)
				if err := conn.WriteMessage(websocket.BinaryMessage, CentrifugoOKReply(id)); err != nil {
					return
				}
			}
		}
	})
}

// SpawnCentrifugoDisconnectAfterHandshake replies to connect+subscribe then
// closes the socket so the client reconnects.
func SpawnCentrifugoDisconnectAfterHandshake(active *atomic.Int64) *MockWSServer {
	if active == nil {
		active = &atomic.Int64{}
	}
	return spawnWS(active, func(conn *websocket.Conn, active *atomic.Int64, connects *atomic.Int64) {
		connects.Add(1)
		active.Add(1)
		defer active.Add(-1)
		replies := 0
		for {
			messageType, _, err := conn.ReadMessage()
			if err != nil {
				return
			}
			if messageType == websocket.BinaryMessage && replies < 2 {
				replies++
				if err := conn.WriteMessage(websocket.BinaryMessage, CentrifugoOKReply(uint32(replies))); err != nil {
					return
				}
				if replies == 2 {
					_ = conn.Close()
					return
				}
			}
		}
	})
}

// SpawnCentrifugoDisconnectThenPublish disconnects the first session after
// handshake. Subsequent sessions stay up; after handshake they forward payloads
// from pubCh as Centrifugo publication frames (and drain reads).
func SpawnCentrifugoDisconnectThenPublish(active *atomic.Int64, pubCh <-chan []byte) *MockWSServer {
	if active == nil {
		active = &atomic.Int64{}
	}
	return spawnWS(active, func(conn *websocket.Conn, active *atomic.Int64, connects *atomic.Int64) {
		n := connects.Add(1)
		active.Add(1)
		defer active.Add(-1)
		replies := 0
		for replies < 2 {
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
		if n == 1 {
			_ = conn.Close()
			return
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
			case payload, ok := <-pubCh:
				if !ok {
					return
				}
				if err := conn.WriteMessage(websocket.BinaryMessage, CentrifugoPublication(payload)); err != nil {
					return
				}
			}
		}
	})
}

func spawnWS(active *atomic.Int64, handle func(*websocket.Conn, *atomic.Int64, *atomic.Int64)) *MockWSServer {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		panic("hardening: bind ws: " + err.Error())
	}
	if active == nil {
		active = &atomic.Int64{}
	}
	connects := &atomic.Int64{}
	s := &MockWSServer{
		addr:     ln.Addr().String(),
		ln:       ln,
		active:   active,
		connects: connects,
		done:     make(chan struct{}),
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/connection/websocket", func(w http.ResponseWriter, r *http.Request) {
		proto := r.Header.Get("Sec-WebSocket-Protocol")
		var respHeader http.Header
		if strings.Contains(proto, "centrifuge-protobuf") {
			respHeader = http.Header{}
			respHeader.Set("Sec-WebSocket-Protocol", "centrifuge-protobuf")
		}
		conn, err := upgrader.Upgrade(w, r, respHeader)
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		handle(conn, s.active, s.connects)
	})
	srv := &http.Server{Handler: mux}
	go func() {
		defer close(s.done)
		_ = srv.Serve(ln)
	}()
	return s
}

// WSURL returns ws://host/connection/websocket.
func (s *MockWSServer) WSURL() string {
	return "ws://" + s.addr + "/connection/websocket"
}

// ActiveConns returns the current active connection count.
func (s *MockWSServer) ActiveConns() int64 {
	return s.active.Load()
}

// Connects returns how many websocket handshakes completed.
func (s *MockWSServer) Connects() int64 {
	return s.connects.Load()
}

// Close stops the listener.
func (s *MockWSServer) Close() {
	_ = s.ln.Close()
	select {
	case <-s.done:
	case <-time.After(2 * time.Second):
	}
}
