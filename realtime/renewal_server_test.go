package realtime

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Fabric-Labs/polyester-sdk-go/auth"
	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/encoding/protowire"
)

// renewalServer is a Centrifugo-shaped socket with its token API. It issues
// connection and subscription tokens that expire ttl units after they are
// used, like Centrifugo it lets a token run grace past its TTL, and then it
// ends the subscription with 2501 or closes the connection with 3005. Every
// token it mints is single use.
type renewalServer struct {
	t     *testing.T
	api   *httptest.Server
	ws    *httptest.Server
	unit  time.Duration
	grace time.Duration
	// connTTL and subTTL are in units; 0 issues a token that does not expire.
	connTTL uint32
	subTTL  uint32

	// tokenHook, when set, runs before each token response; a status other
	// than 200 refuses the token.
	tokenHook atomic.Pointer[func(kind string) (status int)]
	// onCommand, when set, sees every renewal command before it is answered
	// and decides how the server answers it.
	onCommand atomic.Pointer[func(kind string) renewalAction]

	mu        sync.Mutex
	minted    map[string]bool // token -> used
	conns     []*websocket.Conn
	connToks  atomic.Int64
	subToks   atomic.Int64
	accepted  atomic.Int64
	subscribe atomic.Int64
	refresh   atomic.Int64
	subRenew  atomic.Int64
	subExpiry atomic.Int64
	conExpiry atomic.Int64
	reused    atomic.Int64
	// stale counts renewals carrying a subscription token minted before the
	// connection's own subscribe: work left over from an earlier connection.
	stale     atomic.Int64
	published atomic.Int64
	// publication is sent every publishEvery once subscribed, when set.
	publication  []byte
	publishEvery time.Duration
}

type renewalServerConfig struct {
	unit time.Duration
	// grace defaults to three tenths of a unit.
	grace           time.Duration
	connTTL, subTTL uint32
	publication     []byte
	publishEvery    time.Duration
}

func newRenewalServer(t *testing.T, cfg renewalServerConfig) *renewalServer {
	t.Helper()
	if cfg.grace == 0 {
		cfg.grace = cfg.unit * 3 / 10
	}
	s := &renewalServer{
		t:            t,
		unit:         cfg.unit,
		grace:        cfg.grace,
		connTTL:      cfg.connTTL,
		subTTL:       cfg.subTTL,
		publication:  cfg.publication,
		publishEvery: cfg.publishEvery,
		minted:       map[string]bool{},
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/rt/token", func(w http.ResponseWriter, _ *http.Request) {
		s.serveToken(w, "conn", s.connToks.Add(1))
	})
	mux.HandleFunc("GET /v1/rt/subscribe", func(w http.ResponseWriter, _ *http.Request) {
		s.serveToken(w, "sub", s.subToks.Add(1))
	})
	s.api = httptest.NewServer(mux)
	t.Cleanup(s.api.Close)
	upgrader := websocket.Upgrader{Subprotocols: []string{centrifugoProtobufSubprotocol}}
	s.ws = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		s.accepted.Add(1)
		s.mu.Lock()
		s.conns = append(s.conns, conn)
		s.mu.Unlock()
		s.serve(conn)
	}))
	t.Cleanup(s.ws.Close)
	return s
}

func (s *renewalServer) serveToken(w http.ResponseWriter, kind string, n int64) {
	if hook := s.tokenHook.Load(); hook != nil {
		if status := (*hook)(kind); status != http.StatusOK {
			w.WriteHeader(status)
			_, _ = w.Write([]byte(`{"code":"permission_denied","message":"token refused"}`))
			return
		}
	}
	token := fmt.Sprintf("%s-%d", kind, n)
	s.mu.Lock()
	s.minted[token] = false
	s.mu.Unlock()
	_ = json.NewEncoder(w).Encode(map[string]string{"token": token})
}

// useToken reports whether token was minted by this server and not used before.
func (s *renewalServer) useToken(token string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	used, ok := s.minted[token]
	if !ok || used {
		s.reused.Add(1)
		return false
	}
	s.minted[token] = true
	return true
}

func (s *renewalServer) client(t *testing.T) *Client {
	t.Helper()
	creds, err := auth.LoadCredentials("ak_test", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", false)
	if err != nil {
		t.Fatal(err)
	}
	c := NewClient("ws"+strings.TrimPrefix(s.ws.URL, "http"), s.api.URL, creds, &http.Client{Timeout: 5 * time.Second}, 1000)
	t.Cleanup(c.Close)
	return c
}

// dropAll closes every open connection from the server side and reports
// how many it closed.
func (s *renewalServer) dropAll() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := len(s.conns)
	for _, conn := range s.conns {
		_ = conn.Close()
	}
	s.conns = nil
	return n
}

type renewalAction int

const (
	answerRenewal renewalAction = iota
	// refuseRenewal answers with Centrifugo error 109, token expired.
	refuseRenewal
	// dropOnRenewal closes the connection without answering.
	dropOnRenewal
)

// tokenSerial is the mint order of a token this server issued.
func tokenSerial(token string) int {
	_, serial, _ := strings.Cut(token, "-")
	n, _ := strconv.Atoi(serial)
	return n
}

type renewalCommand struct {
	id      uint32
	kind    string // connect, subscribe, refresh, sub_refresh, pong
	channel string
	token   string
}

func (s *renewalServer) deadline(ttl uint32) <-chan time.Time {
	if ttl == 0 {
		return nil
	}
	return time.After(time.Duration(ttl)*s.unit + s.grace)
}

func (s *renewalServer) serve(conn *websocket.Conn) {
	commands := make(chan renewalCommand, 64)
	go func() {
		defer close(commands)
		for {
			_, raw, err := conn.ReadMessage()
			if err != nil {
				return
			}
			for _, cmd := range parseRenewalCommands(s.t, raw) {
				commands <- cmd
			}
		}
	}()
	write := func(frame []byte) bool {
		return conn.WriteMessage(websocket.BinaryMessage, frame) == nil
	}
	next := func() (renewalCommand, bool) {
		for cmd := range commands {
			if cmd.kind != "pong" {
				return cmd, true
			}
		}
		return renewalCommand{}, false
	}

	cmd, ok := next()
	if !ok || cmd.kind != "connect" || !s.useToken(cmd.token) {
		return
	}
	if !write(replyFrame(cmd.id, 5, expiryResult(3, 4, s.connTTL))) {
		return
	}
	connExpires := s.deadline(s.connTTL)
	cmd, ok = next()
	if !ok || cmd.kind != "subscribe" || !s.useToken(cmd.token) {
		return
	}
	s.subscribe.Add(1)
	channel := cmd.channel
	subscribedWith := tokenSerial(cmd.token)
	if !write(replyFrame(cmd.id, 6, expiryResult(1, 2, s.subTTL))) {
		return
	}
	subExpires := s.deadline(s.subTTL)
	var publish <-chan time.Time
	if s.publishEvery > 0 {
		ticker := time.NewTicker(s.publishEvery)
		defer ticker.Stop()
		publish = ticker.C
	}
	for {
		select {
		case cmd, ok := <-commands:
			if !ok {
				return
			}
			if cmd.kind == "pong" {
				continue
			}
			action := answerRenewal
			if hook := s.onCommand.Load(); hook != nil {
				action = (*hook)(cmd.kind)
			}
			switch action {
			case dropOnRenewal:
				return
			case refuseRenewal:
				if !write(errorFrame(cmd.id, 109, "token expired")) {
					return
				}
				continue
			}
			switch cmd.kind {
			case "sub_refresh":
				if tokenSerial(cmd.token) <= subscribedWith {
					s.stale.Add(1)
				}
				if cmd.channel != channel || !s.useToken(cmd.token) {
					if !write(errorFrame(cmd.id, 109, "token expired")) {
						return
					}
					continue
				}
				s.subRenew.Add(1)
				subExpires = s.deadline(s.subTTL)
				if !write(replyFrame(cmd.id, 15, expiryResult(1, 2, s.subTTL))) {
					return
				}
			case "refresh":
				if !s.useToken(cmd.token) {
					if !write(errorFrame(cmd.id, 109, "token expired")) {
						return
					}
					continue
				}
				s.refresh.Add(1)
				connExpires = s.deadline(s.connTTL)
				if !write(replyFrame(cmd.id, 14, expiryResult(3, 4, s.connTTL))) {
					return
				}
			default:
				s.t.Errorf("unexpected %s command after subscribe", cmd.kind)
				return
			}
		case <-subExpires:
			s.subExpiry.Add(1)
			subExpires = nil
			_ = write(unsubscribePush(channel, 2501, "subscription expired"))
		case <-connExpires:
			s.conExpiry.Add(1)
			_ = conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(3005, "expired"), time.Now().Add(time.Second))
			return
		case <-publish:
			if s.publication != nil && write(publicationFrame(s.publication)) {
				s.published.Add(1)
			}
		}
	}
}

func parseRenewalCommands(t *testing.T, frame []byte) []renewalCommand {
	var out []renewalCommand
	for len(frame) > 0 {
		length, n := protowire.ConsumeVarint(frame)
		if n < 0 || uint64(len(frame)-n) < length {
			t.Errorf("malformed command frame %x", frame)
			return out
		}
		record := frame[n : n+int(length)]
		frame = frame[n+int(length):]
		if len(record) == 0 {
			out = append(out, renewalCommand{kind: "pong"})
			continue
		}
		cmd := renewalCommand{}
		for len(record) > 0 {
			num, typ, n := protowire.ConsumeTag(record)
			record = record[n:]
			switch {
			case num == 1 && typ == protowire.VarintType:
				v, n := protowire.ConsumeVarint(record)
				cmd.id = uint32(v)
				record = record[n:]
			case typ == protowire.BytesType:
				body, n := protowire.ConsumeBytes(record)
				record = record[n:]
				kinds := map[protowire.Number]string{4: "connect", 5: "subscribe", 14: "refresh", 15: "sub_refresh"}
				kind, known := kinds[num]
				if !known {
					t.Errorf("unexpected command field %d", num)
					continue
				}
				cmd.kind = kind
				cmd.channel, cmd.token = requestStrings(kind, body)
			default:
				n := protowire.ConsumeFieldValue(num, typ, record)
				record = record[n:]
			}
		}
		out = append(out, cmd)
	}
	return out
}

// requestStrings reads the channel and token of a request. Connect and
// refresh carry token = 1; subscribe and sub_refresh carry channel = 1 and
// token = 2.
func requestStrings(kind string, body []byte) (channel, token string) {
	for len(body) > 0 {
		num, typ, n := protowire.ConsumeTag(body)
		body = body[n:]
		if typ != protowire.BytesType {
			body = body[protowire.ConsumeFieldValue(num, typ, body):]
			continue
		}
		value, n := protowire.ConsumeBytes(body)
		body = body[n:]
		switch {
		case (kind == "connect" || kind == "refresh") && num == 1:
			token = string(value)
		case num == 1:
			channel = string(value)
		case num == 2:
			token = string(value)
		}
	}
	return channel, token
}

func expiryResult(expiresField, ttlField protowire.Number, ttl uint32) []byte {
	if ttl == 0 {
		return nil
	}
	out := protowire.AppendTag(nil, expiresField, protowire.VarintType)
	out = protowire.AppendVarint(out, 1)
	out = protowire.AppendTag(out, ttlField, protowire.VarintType)
	return protowire.AppendVarint(out, uint64(ttl))
}

func lengthDelimitedReply(reply []byte) []byte {
	return append(protowire.AppendVarint(nil, uint64(len(reply))), reply...)
}

func replyFrame(id uint32, resultField protowire.Number, result []byte) []byte {
	reply := protowire.AppendTag(nil, 1, protowire.VarintType)
	reply = protowire.AppendVarint(reply, uint64(id))
	reply = protowire.AppendTag(reply, resultField, protowire.BytesType)
	reply = protowire.AppendBytes(reply, result)
	return lengthDelimitedReply(reply)
}

func errorFrame(id uint32, code uint32, message string) []byte {
	errMsg := protowire.AppendTag(nil, 1, protowire.VarintType)
	errMsg = protowire.AppendVarint(errMsg, uint64(code))
	errMsg = protowire.AppendTag(errMsg, 2, protowire.BytesType)
	errMsg = protowire.AppendString(errMsg, message)
	reply := protowire.AppendTag(nil, 1, protowire.VarintType)
	reply = protowire.AppendVarint(reply, uint64(id))
	reply = protowire.AppendTag(reply, 2, protowire.BytesType)
	reply = protowire.AppendBytes(reply, errMsg)
	return lengthDelimitedReply(reply)
}

func unsubscribePush(channel string, code uint32, reason string) []byte {
	unsub := protowire.AppendTag(nil, 2, protowire.VarintType)
	unsub = protowire.AppendVarint(unsub, uint64(code))
	unsub = protowire.AppendTag(unsub, 3, protowire.BytesType)
	unsub = protowire.AppendString(unsub, reason)
	push := protowire.AppendTag(nil, 2, protowire.BytesType)
	push = protowire.AppendString(push, channel)
	push = protowire.AppendTag(push, 7, protowire.BytesType)
	push = protowire.AppendBytes(push, unsub)
	reply := protowire.AppendTag(nil, 4, protowire.BytesType)
	reply = protowire.AppendBytes(reply, push)
	return lengthDelimitedReply(reply)
}

func publicationFrame(data []byte) []byte {
	pub := protowire.AppendTag(nil, 4, protowire.BytesType)
	pub = protowire.AppendBytes(pub, data)
	push := protowire.AppendTag(nil, 4, protowire.BytesType)
	push = protowire.AppendBytes(push, pub)
	reply := protowire.AppendTag(nil, 4, protowire.BytesType)
	reply = protowire.AppendBytes(reply, push)
	return lengthDelimitedReply(reply)
}
