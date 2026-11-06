package realtime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Fabric-Labs/polyester-sdk-go/auth"
	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	authv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/auth/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/hardening"
	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/proto"
)

const testResumeSub uint64 = 2305843009213693953

var fastResubscribeTiming = resubscribeTiming{
	maxAttempts:    3,
	initialBackoff: 10 * time.Millisecond,
	stableAfter:    time.Hour,
	attemptSpacing: time.Millisecond,
	readInterval:   time.Millisecond,
}

func resumeChannel(sub uint64) string {
	return "private:ledger:balances:" + codecs.FormatID(sub) + ":proto"
}

// resumeServer is a real Centrifugo-shaped socket and token/list API. Counters
// only count the operation they name.
type resumeServer struct {
	t         *testing.T
	api       *httptest.Server
	ws        *httptest.Server
	mu        sync.Mutex
	conns     map[string][]*websocket.Conn
	listed    atomic.Pointer[func() (int, string)]
	token     atomic.Pointer[func(channel string) (int, string)]
	onSubbed  atomic.Pointer[func(channel string, conn *websocket.Conn)]
	listCalls atomic.Int64
	subTokens atomic.Int64
	connToks  atomic.Int64
	subscribe atomic.Int64
	subbed    chan string
}

func newResumeServer(t *testing.T) *resumeServer {
	t.Helper()
	s := &resumeServer{t: t, conns: map[string][]*websocket.Conn{}, subbed: make(chan string, 256)}
	s.listSubaccounts(testResumeSub)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/rt/token", func(w http.ResponseWriter, _ *http.Request) {
		s.connToks.Add(1)
		_ = json.NewEncoder(w).Encode(map[string]string{"token": "connection"})
	})
	mux.HandleFunc("GET /v1/rt/subscribe", func(w http.ResponseWriter, r *http.Request) {
		s.subTokens.Add(1)
		if decide := s.token.Load(); decide != nil {
			if status, body := (*decide)(r.URL.Query().Get("channel")); status != http.StatusOK {
				w.WriteHeader(status)
				_, _ = w.Write([]byte(body))
				return
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"token": "subscribe"})
	})
	mux.HandleFunc("GET /v1/auth/subaccounts", func(w http.ResponseWriter, _ *http.Request) {
		s.listCalls.Add(1)
		status, body := (*s.listed.Load())()
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
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
		s.serve(conn)
	}))
	t.Cleanup(s.ws.Close)
	return s
}

// serve answers connect, then subscribe, then keeps the socket open.
func (s *resumeServer) serve(conn *websocket.Conn) {
	channel := ""
	for replies := uint32(1); replies <= 2; replies++ {
		_, raw, err := conn.ReadMessage()
		if err != nil {
			return
		}
		if replies == 2 {
			channel = s.channelIn(raw)
			s.subscribe.Add(1)
		}
		if err := conn.WriteMessage(websocket.BinaryMessage, hardening.CentrifugoOKReply(replies)); err != nil {
			return
		}
	}
	s.mu.Lock()
	s.conns[channel] = append(s.conns[channel], conn)
	s.mu.Unlock()
	s.subbed <- channel
	if hook := s.onSubbed.Load(); hook != nil {
		(*hook)(channel, conn)
	}
	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			return
		}
	}
}

// channelIn reads the channel of a length-delimited subscribe command frame.
func (s *resumeServer) channelIn(raw []byte) string {
	start := bytes.Index(raw, []byte("private:"))
	if public := bytes.Index(raw, []byte("public:")); start < 0 || (public >= 0 && public < start) {
		start = public
	}
	if start < 0 {
		s.t.Errorf("subscribe command names no channel: %x", raw)
		return ""
	}
	// The byte before the channel is its length.
	return string(raw[start : start+int(raw[start-1])])
}

func (s *resumeServer) client() *Client {
	creds, err := auth.LoadCredentials("ak_test", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", false)
	if err != nil {
		s.t.Fatal(err)
	}
	c := NewClient("ws"+strings.TrimPrefix(s.ws.URL, "http"), s.api.URL, creds, &http.Client{Timeout: 5 * time.Second}, 100)
	c.access = newSubaccountAccess(c, fastResubscribeTiming)
	s.t.Cleanup(c.Close)
	return c
}

func (s *resumeServer) listSubaccounts(ids ...uint64) {
	entries := []map[string]any{}
	for _, id := range ids {
		entries = append(entries, map[string]any{"id": codecs.FormatUint64ID(id), "role": "VIEWER"})
	}
	body, _ := json.Marshal(map[string]any{"subaccounts": entries})
	respond := func() (int, string) { return http.StatusOK, string(body) }
	s.listed.Store(&respond)
}

func (s *resumeServer) respondList(status int, body string) {
	respond := func() (int, string) { return status, body }
	s.listed.Store(&respond)
}

func (s *resumeServer) refuseTokens(status int, body string) {
	decide := func(string) (int, string) { return status, body }
	s.token.Store(&decide)
}

func (s *resumeServer) awaitSubscribed(channel string) {
	s.t.Helper()
	deadline := time.After(10 * time.Second)
	for {
		select {
		case got := <-s.subbed:
			if got == channel {
				return
			}
		case <-deadline:
			s.t.Fatalf("%s was not subscribed", channel)
		}
	}
}

func (s *resumeServer) push(channel string, frame []byte) {
	s.t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	conns := s.conns[channel]
	if len(conns) == 0 {
		s.t.Fatalf("no connection for %s", channel)
	}
	if err := conns[len(conns)-1].WriteMessage(websocket.BinaryMessage, frame); err != nil {
		s.t.Fatal(err)
	}
}

func (s *resumeServer) endSubscription(channel string) {
	s.t.Helper()
	s.push(channel, hardening.CentrifugoUnsubscribe(channel, 2000, "server unsubscribe"))
}

func awaitEnded[T any](t *testing.T, sub *Subscription[T], within time.Duration) *sdkerrors.SubscriptionEndedError {
	t.Helper()
	deadline := time.After(within)
	for {
		select {
		case _, ok := <-sub.Messages():
			if ok {
				continue
			}
			var ended *sdkerrors.SubscriptionEndedError
			if err := sub.Err(); !errors.As(err, &ended) {
				t.Fatalf("subscription ended with %v, want SubscriptionEndedError", err)
			}
			return ended
		case <-deadline:
			t.Fatal("subscription stayed open after the server ended it")
		}
	}
}

func identity(data []byte) ([]byte, error) { return data, nil }

// Re-grant plus a stale unsubscribe: the server still lists the sub-account
// with a role, so the subscription comes back with a fresh token and live data.
func TestResumeAfterStaleUnsubscribeWhileListed(t *testing.T) {
	s := newResumeServer(t)
	channel := resumeChannel(testResumeSub)
	sub, err := SubscribeProto(context.Background(), s.client(), channel, identity)
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close()
	s.awaitSubscribed(channel)
	if got := s.subTokens.Load(); got != 1 {
		t.Fatalf("initial subscribe tokens = %d", got)
	}
	if s.listCalls.Load() != 0 {
		t.Fatal("a healthy subscribe must not read the list")
	}

	s.endSubscription(channel)
	s.awaitSubscribed(channel)
	if got := s.subTokens.Load(); got != 2 {
		t.Fatalf("subscribe tokens = %d, want a fresh one", got)
	}
	if got := s.listCalls.Load(); got != 1 {
		t.Fatalf("list reads = %d, want 1", got)
	}
	s.push(channel, hardening.CentrifugoPublication([]byte("live again")))
	select {
	case got, ok := <-sub.Messages():
		if !ok || string(got) != "live again" {
			t.Fatalf("got %q open=%v err=%v", got, ok, sub.Err())
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no live data after resuming")
	}
	if !sub.TakeResubscribed() || sub.Resubscribes() != 1 {
		t.Fatal("resuming must raise the resubscribed gap signal")
	}
	if sub.Err() != nil {
		t.Fatalf("resumed subscription has error %v", sub.Err())
	}
}

// Removal: the list no longer has the sub-account, so nothing resubscribes.
func TestNoResumeWhenNotListed(t *testing.T) {
	s := newResumeServer(t)
	channel := resumeChannel(testResumeSub)
	sub, err := SubscribeProto(context.Background(), s.client(), channel, identity)
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close()
	s.awaitSubscribed(channel)
	s.listSubaccounts(testResumeSub + 1)

	s.endSubscription(channel)
	ended := awaitEnded(t, sub, 5*time.Second)
	if ended.Reason != sdkerrors.SubscriptionEndNotListed || ended.Channel != channel || ended.Code != 2000 || ended.ServerReason != "server unsubscribe" {
		t.Fatalf("ended = %+v", ended)
	}
	if got := s.listCalls.Load(); got != 1 {
		t.Fatalf("list reads = %d, want the decision to read once", got)
	}
	if got := s.subTokens.Load(); got != 1 {
		t.Fatalf("subscribe tokens = %d, want no new one", got)
	}
}

// A role-less entry is not access, even if listed.
func TestNoResumeWhenListedWithoutRole(t *testing.T) {
	s := newResumeServer(t)
	channel := resumeChannel(testResumeSub)
	sub, err := SubscribeProto(context.Background(), s.client(), channel, identity)
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close()
	s.awaitSubscribed(channel)
	s.respondList(http.StatusOK, `{"subaccounts":[{"id":"`+codecs.FormatUint64ID(testResumeSub)+`","role":"SUBACCOUNT_ROLE_UNSPECIFIED"}]}`)
	s.endSubscription(channel)
	if ended := awaitEnded(t, sub, 5*time.Second); ended.Reason != sdkerrors.SubscriptionEndNotListed {
		t.Fatalf("ended = %+v", ended)
	}
}

// A removal seen on the caller's own stream stops without any request; the
// matching later update clears it again.
func TestSeenRemovalStopsWithoutCalls(t *testing.T) {
	s := newResumeServer(t)
	client := s.client()
	updates := subaccountsChannelPrefix + "root:proto"
	stream, err := SubscribeProto(context.Background(), client, updates, identity)
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	s.awaitSubscribed(updates)
	channel := resumeChannel(testResumeSub)
	sub, err := SubscribeProto(context.Background(), client, channel, identity)
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close()
	s.awaitSubscribed(channel)
	tokensBefore := s.subTokens.Load()

	publish := func(update *authv1.Subaccount) {
		t.Helper()
		payload, err := proto.Marshal(update)
		if err != nil {
			t.Fatal(err)
		}
		s.push(updates, hardening.CentrifugoPublication(payload))
		select {
		case <-stream.Messages():
		case <-time.After(5 * time.Second):
			t.Fatal("update not delivered")
		}
	}
	publish(&authv1.Subaccount{Id: testResumeSub, Removed: true})
	s.endSubscription(channel)
	ended := awaitEnded(t, sub, 5*time.Second)
	if ended.Reason != sdkerrors.SubscriptionEndAccessRemoved {
		t.Fatalf("ended = %+v", ended)
	}
	if got := s.listCalls.Load(); got != 0 {
		t.Fatalf("list reads = %d, want none", got)
	}
	if got := s.subTokens.Load(); got != tokensBefore {
		t.Fatalf("subscribe tokens = %d, want %d", got, tokensBefore)
	}

	// Re-granted: the same client resumes again.
	publish(&authv1.Subaccount{Id: testResumeSub, Role: authv1.SubaccountRole_VIEWER})
	again, err := SubscribeProto(context.Background(), client, channel, identity)
	if err != nil {
		t.Fatal(err)
	}
	defer again.Close()
	s.awaitSubscribed(channel)
	s.endSubscription(channel)
	s.awaitSubscribed(channel)
	if got := s.listCalls.Load(); got != 1 {
		t.Fatalf("list reads after re-grant = %d, want 1", got)
	}
}

// A refused token stops with the refusal, and no more requests follow.
func TestRefusedTokenStops(t *testing.T) {
	s := newResumeServer(t)
	channel := resumeChannel(testResumeSub)
	sub, err := SubscribeProto(context.Background(), s.client(), channel, identity)
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close()
	s.awaitSubscribed(channel)
	s.refuseTokens(http.StatusForbidden, `{"code":"SUBACCOUNT_READ_FORBIDDEN","message":"no access"}`)

	s.endSubscription(channel)
	ended := awaitEnded(t, sub, 5*time.Second)
	if ended.Reason != sdkerrors.SubscriptionEndTokenRefused {
		t.Fatalf("ended = %+v", ended)
	}
	var refusal *sdkerrors.AuthError
	if !errors.As(sub.Err(), &refusal) || refusal.Status != http.StatusForbidden {
		t.Fatalf("cause = %v, want the 403 refusal", sub.Err())
	}
	if got := s.subTokens.Load(); got != 2 {
		t.Fatalf("subscribe tokens = %d, want exactly one refused", got)
	}
}

// A refused list says the subscription ended and why, for every refusal the
// server can give a key: both statuses keep the refusal as the cause.
func TestRefusedListStopsWithTypedCause(t *testing.T) {
	for _, status := range []int{http.StatusUnauthorized, http.StatusForbidden} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			s := newResumeServer(t)
			channel := resumeChannel(testResumeSub)
			sub, err := SubscribeProto(context.Background(), s.client(), channel, identity)
			if err != nil {
				t.Fatal(err)
			}
			defer sub.Close()
			s.awaitSubscribed(channel)
			s.respondList(status, `{"code":"AUTH_SUBACCOUNT_ACCESS_DENIED","message":"forbidden"}`)

			s.endSubscription(channel)
			ended := awaitEnded(t, sub, 5*time.Second)
			if ended.Reason != sdkerrors.SubscriptionEndListRefused {
				t.Fatalf("ended = %+v", ended)
			}
			var refusal *sdkerrors.AuthError
			if !errors.As(sub.Err(), &refusal) || refusal.Status != status || refusal.Code != "AUTH_SUBACCOUNT_ACCESS_DENIED" {
				t.Fatalf("cause = %v", sub.Err())
			}
			msg := sub.Err().Error()
			if !strings.Contains(msg, "server ended realtime subscription") || !strings.Contains(msg, string(sdkerrors.SubscriptionEndListRefused)) {
				t.Fatalf("message does not say what ended and why: %s", msg)
			}
			if got := s.listCalls.Load(); got != 1 {
				t.Fatalf("list reads = %d, want 1", got)
			}
			if got := s.subTokens.Load(); got != 1 {
				t.Fatalf("subscribe tokens = %d, want no new one", got)
			}
		})
	}
}

// A temporary list failure retries within the limit and can still recover.
func TestTemporaryListFailureRetriesThenResumes(t *testing.T) {
	s := newResumeServer(t)
	channel := resumeChannel(testResumeSub)
	sub, err := SubscribeProto(context.Background(), s.client(), channel, identity)
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close()
	s.awaitSubscribed(channel)
	var calls atomic.Int64
	ok, _ := json.Marshal(map[string]any{"subaccounts": []map[string]any{{"id": codecs.FormatUint64ID(testResumeSub), "role": "VIEWER"}}})
	respond := func() (int, string) {
		if calls.Add(1) == 1 {
			return http.StatusServiceUnavailable, `{}`
		}
		return http.StatusOK, string(ok)
	}
	s.listed.Store(&respond)
	s.endSubscription(channel)
	s.awaitSubscribed(channel)
	if calls.Load() != 2 {
		t.Fatalf("list reads = %d, want 2", calls.Load())
	}
	if sub.Err() != nil {
		t.Fatalf("recovered subscription has error %v", sub.Err())
	}
}

// Hard limit: a list that never answers cannot retry forever.
func TestTemporaryListFailureHasAHardLimit(t *testing.T) {
	s := newResumeServer(t)
	channel := resumeChannel(testResumeSub)
	sub, err := SubscribeProto(context.Background(), s.client(), channel, identity)
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close()
	s.awaitSubscribed(channel)
	s.respondList(http.StatusServiceUnavailable, `{}`)
	s.endSubscription(channel)
	ended := awaitEnded(t, sub, 10*time.Second)
	if ended.Reason != sdkerrors.SubscriptionEndRetriesExhausted || ended.Cause == nil {
		t.Fatalf("ended = %+v", ended)
	}
	if got := s.listCalls.Load(); got != int64(fastResubscribeTiming.maxAttempts) {
		t.Fatalf("list reads = %d, want the limit %d", got, fastResubscribeTiming.maxAttempts)
	}
}

// Hard limit: a server that ends every resumed subscription cannot keep the
// client looping; the budget only comes back after staying up.
func TestRepeatedStaleUnsubscribesHaveAHardLimit(t *testing.T) {
	s := newResumeServer(t)
	channel := resumeChannel(testResumeSub)
	frame := hardening.CentrifugoUnsubscribe(channel, 2000, "server unsubscribe")
	var unsubs atomic.Int64
	hook := func(_ string, conn *websocket.Conn) {
		unsubs.Add(1)
		_ = conn.WriteMessage(websocket.BinaryMessage, frame)
	}
	s.onSubbed.Store(&hook)
	sub, err := SubscribeProto(context.Background(), s.client(), channel, identity)
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close()
	ended := awaitEnded(t, sub, 10*time.Second)
	if ended.Reason != sdkerrors.SubscriptionEndRetriesExhausted {
		t.Fatalf("ended = %+v", ended)
	}
	limit := int64(fastResubscribeTiming.maxAttempts)
	if got := s.subTokens.Load(); got != limit+1 {
		t.Fatalf("subscribe tokens = %d, want initial plus %d", got, limit)
	}
	if got := unsubs.Load(); got != limit+1 {
		t.Fatalf("unsubscribes = %d, want %d: the scenario must repeat to the limit", got, limit+1)
	}
}

// A burst of ended subscriptions on one client shares list reads and paces
// token requests, and every subscription still comes back.
func TestRevocationBurstCannotFloodRequests(t *testing.T) {
	s := newResumeServer(t)
	client := s.client()
	timing := resubscribeTiming{
		maxAttempts:    6,
		initialBackoff: 5 * time.Millisecond,
		stableAfter:    time.Hour,
		attemptSpacing: 40 * time.Millisecond,
		readInterval:   200 * time.Millisecond,
	}
	client.access = newSubaccountAccess(client, timing)
	const burst = 25
	ids := make([]uint64, burst)
	subs := make([]*Subscription[[]byte], burst)
	for i := range ids {
		ids[i] = testResumeSub + uint64(i)
	}
	s.listSubaccounts(ids...)
	for i, id := range ids {
		sub, err := SubscribeProto(context.Background(), client, resumeChannel(id), identity)
		if err != nil {
			t.Fatal(err)
		}
		defer sub.Close()
		subs[i] = sub
		s.awaitSubscribed(resumeChannel(id))
	}
	tokensBefore := s.connToks.Load() + s.subTokens.Load()
	started := time.Now()
	for _, id := range ids {
		s.endSubscription(resumeChannel(id))
	}
	for i := 0; i < burst; i++ {
		select {
		case <-s.subbed:
		case <-time.After(30 * time.Second):
			t.Fatalf("only %d of %d subscriptions came back", i, burst)
		}
	}
	elapsed := time.Since(started)
	for i, sub := range subs {
		if sub.Err() != nil {
			t.Fatalf("subscription %d failed: %v", i, sub.Err())
		}
	}
	maxReads := int64(elapsed/timing.readInterval) + 2
	if got := s.listCalls.Load(); got > maxReads || got >= burst {
		t.Fatalf("list reads = %d in %s, want at most %d and fewer than the burst", got, elapsed, maxReads)
	}
	// Spacing means the burst cannot finish faster than its slots.
	if minElapsed := time.Duration(burst-1) * timing.attemptSpacing; elapsed < minElapsed {
		t.Fatalf("%d attempts took %s, faster than the %s pacing allows", burst, elapsed, minElapsed)
	}
	if got := s.connToks.Load() + s.subTokens.Load() - tokensBefore; got != 2*burst {
		t.Fatalf("token requests = %d, want one connection and one subscribe token each", got)
	}
}

// One read cannot answer a decision made after it started.
func TestAccessReadStartsAfterEveryJoiner(t *testing.T) {
	s := newResumeServer(t)
	client := s.client()
	client.access = newSubaccountAccess(client, resubscribeTiming{readInterval: 100 * time.Millisecond})
	var calls atomic.Int64
	release := make(chan struct{})
	body, _ := json.Marshal(map[string]any{"subaccounts": []any{}})
	respond := func() (int, string) {
		calls.Add(1)
		<-release
		return http.StatusOK, string(body)
	}
	s.listed.Store(&respond)
	first := make(chan error, 1)
	go func() {
		_, err := client.access.read(context.Background())
		first <- err
	}()
	hardening.WaitUntil(t, func() bool { return calls.Load() == 1 }, 5*time.Second)
	second := make(chan error, 1)
	go func() {
		_, err := client.access.read(context.Background())
		second <- err
	}()
	close(release)
	if err := <-first; err != nil {
		t.Fatal(err)
	}
	if err := <-second; err != nil {
		t.Fatal(err)
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("list reads = %d, want a second read for the later decision", got)
	}
}

// Channels no sub-account's access governs end with the typed error, without
// any request.
func TestUnsubscribeOutsideGovernedChannelsStops(t *testing.T) {
	s := newResumeServer(t)
	for _, channel := range []string{"private:auth:api-keys:root:proto", "public:spot:trades:1:proto"} {
		t.Run(channel, func(t *testing.T) {
			sub, err := SubscribeProto(context.Background(), s.client(), channel, identity)
			if err != nil {
				t.Fatal(err)
			}
			defer sub.Close()
			s.awaitSubscribed(channel)
			lists, tokens := s.listCalls.Load(), s.subTokens.Load()
			s.endSubscription(channel)
			ended := awaitEnded(t, sub, 5*time.Second)
			if ended.Reason != sdkerrors.SubscriptionEndNotResubscribable {
				t.Fatalf("ended = %+v", ended)
			}
			if s.listCalls.Load() != lists || s.subTokens.Load() != tokens {
				t.Fatal("a channel no access governs must not cause requests")
			}
		})
	}
}

// A code at or above 2500 asks for a new subscribe and goes through the ordinary
// reconnect, without reading the list.
func TestTemporaryUnsubscribeCodeReconnects(t *testing.T) {
	s := newResumeServer(t)
	channel := resumeChannel(testResumeSub)
	sub, err := SubscribeProto(context.Background(), s.client(), channel, identity)
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close()
	s.awaitSubscribed(channel)
	s.push(channel, hardening.CentrifugoUnsubscribe(channel, 2500, "insufficient state"))
	s.awaitSubscribed(channel)
	if s.listCalls.Load() != 0 {
		t.Fatal("a temporary unsubscribe must not read the list")
	}
	if sub.Err() != nil {
		t.Fatalf("temporary unsubscribe failed the subscription: %v", sub.Err())
	}
}

// A decimal list ID with no zero digit is also valid base58: it must still read
// as the decimal the list reports, matching the channel's public ID.
func TestListedDecimalIDWithoutZeroDigit(t *testing.T) {
	const id uint64 = 2305843111111111111
	s := newResumeServer(t)
	s.listSubaccounts(id)
	channel := resumeChannel(id)
	sub, err := SubscribeProto(context.Background(), s.client(), channel, identity)
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close()
	s.awaitSubscribed(channel)
	s.respondList(http.StatusOK, `{"subaccounts":[{"id":"2305843111111111111","role":"VIEWER"}]}`)
	s.endSubscription(channel)
	s.awaitSubscribed(channel)
	if sub.Err() != nil {
		t.Fatalf("subscription ended: %v", sub.Err())
	}
}

// awaitReservedSlot returns once the client has reserved the attempt slot after
// the one the test occupied: the resume is now in its pacing wait.
func awaitReservedSlot(t *testing.T, access *subaccountAccess, occupied time.Time) {
	t.Helper()
	hardening.WaitUntil(t, func() bool {
		access.mu.Lock()
		defer access.mu.Unlock()
		return access.nextAttemptAt.After(occupied)
	}, 5*time.Second)
}

// A removal that arrives while the access decision is waiting must win over an
// older answer: the list while it is in flight, the pacing wait after it. The
// removed false controls prove the same barriers still resume.
func TestRemovalWhileDecidingStopsBeforeAnyToken(t *testing.T) {
	const updates = subaccountsChannelPrefix + "root:proto"
	for _, tc := range []struct {
		name    string
		barrier string
		removed bool
	}{
		{"removed during list", "list", true},
		{"still granted during list", "list", false},
		{"removed during pacing", "pacing", true},
		{"still granted during pacing", "pacing", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := newResumeServer(t)
			client := s.client()
			timing := fastResubscribeTiming
			if tc.barrier == "pacing" {
				timing.attemptSpacing = 400 * time.Millisecond
			}
			client.access = newSubaccountAccess(client, timing)
			stream, err := SubscribeProto(context.Background(), client, updates, identity)
			if err != nil {
				t.Fatal(err)
			}
			defer stream.Close()
			s.awaitSubscribed(updates)
			channel := resumeChannel(testResumeSub)
			sub, err := SubscribeProto(context.Background(), client, channel, identity)
			if err != nil {
				t.Fatal(err)
			}
			defer sub.Close()
			s.awaitSubscribed(channel)
			tokens := s.subTokens.Load()

			release := make(chan struct{})
			var once sync.Once
			defer once.Do(func() { close(release) })
			if tc.barrier == "list" {
				body, _ := json.Marshal(map[string]any{"subaccounts": []map[string]any{{"id": codecs.FormatUint64ID(testResumeSub), "role": "VIEWER"}}})
				held := func() (int, string) {
					<-release
					return http.StatusOK, string(body)
				}
				s.listed.Store(&held)
			}
			var occupied time.Time
			if tc.barrier == "pacing" {
				// Occupy the slot so the resume must wait for the next one.
				client.access.reserveAttempt(time.Now())
				client.access.mu.Lock()
				occupied = client.access.nextAttemptAt
				client.access.mu.Unlock()
			}

			s.endSubscription(channel)
			if tc.barrier == "list" {
				hardening.WaitUntil(t, func() bool { return s.listCalls.Load() == 1 }, 5*time.Second)
			} else {
				awaitReservedSlot(t, client.access, occupied)
			}
			if got := s.subTokens.Load(); got != tokens {
				t.Fatalf("subscribe tokens = %d before the barrier opened, want %d", got, tokens)
			}

			update := &authv1.Subaccount{Id: testResumeSub, Role: authv1.SubaccountRole_VIEWER}
			if tc.removed {
				update = &authv1.Subaccount{Id: testResumeSub, Removed: true}
			}
			payload, err := proto.Marshal(update)
			if err != nil {
				t.Fatal(err)
			}
			s.push(updates, hardening.CentrifugoPublication(payload))
			select {
			case <-stream.Messages():
			case <-time.After(5 * time.Second):
				t.Fatal("update not delivered")
			}
			once.Do(func() { close(release) })

			if !tc.removed {
				s.awaitSubscribed(channel)
				if got := s.subTokens.Load(); got != tokens+1 {
					t.Fatalf("subscribe tokens = %d, want a fresh one", got)
				}
				return
			}
			ended := awaitEnded(t, sub, 5*time.Second)
			if ended.Reason != sdkerrors.SubscriptionEndAccessRemoved {
				t.Fatalf("ended = %+v", ended)
			}
			if got := s.subTokens.Load(); got != tokens {
				t.Fatalf("subscribe tokens = %d, want none after the removal", got)
			}
		})
	}
}

func TestSubaccountChannelID(t *testing.T) {
	id := codecs.FormatID(testResumeSub)
	for _, prefix := range subaccountChannelPrefixes {
		for _, suffix := range []string{"", ":proto"} {
			if prefix == "private:spot:triggers:" && suffix == "" {
				continue
			}
			got, ok := subaccountChannelID(prefix + id + suffix)
			if !ok || got != testResumeSub {
				t.Fatalf("%s%s%s => %d %v", prefix, id, suffix, got, ok)
			}
		}
	}
	for _, channel := range []string{
		"private:auth:subaccounts:" + id + ":proto",
		"private:spot:orders:" + id,
		"public:spot:trades:1:proto",
		"private:ledger:balances:",
		"private:ledger:balances:" + id + ":other",
	} {
		if _, ok := subaccountChannelID(channel); ok {
			t.Fatalf("%s must not be governed", channel)
		}
	}
}
