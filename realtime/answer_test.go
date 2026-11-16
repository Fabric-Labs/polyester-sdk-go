package realtime

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	"github.com/Fabric-Labs/polyester-sdk-go/hardening"
)

// Every error from a realtime token or subscription answer carries the
// answer's status and Retry-After: a temporary failure, an auth or
// permission refusal and an edge's ban page alike.
func TestRealtimeTokenAnswerCarriesItsMetadata(t *testing.T) {
	for _, tc := range []struct {
		status int
		body   string
	}{
		{503, `{"code":"AUTH_TEMPORARILY_UNAVAILABLE","status":503}`},
		{401, `{"code":"unauthenticated","message":"bad signature"}`},
		{403, `{"code":"permission_denied","message":"missing permission"}`},
		{403, `error code: 1010`},
	} {
		srv := hardening.NewTestServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Retry-After", "30")
			w.WriteHeader(tc.status)
			_, _ = w.Write([]byte(tc.body))
		}))
		_, err := fetchRTToken(context.Background(), &http.Client{Timeout: time.Second}, testCreds(t), srv.URL+"/v1/rt/token", "realtime connection token")
		srv.Close()
		if err == nil {
			t.Fatalf("%d %s: no error", tc.status, tc.body)
		}
		if got := sdkerrors.AnswerStatus(err); got != tc.status {
			t.Errorf("%d %s: %T status %d", tc.status, tc.body, err, got)
		}
		if got := sdkerrors.RetryAfter(err); got == nil || *got != 30 {
			t.Errorf("%d %s: %T retry after %v, want 30", tc.status, tc.body, err, got)
		}
	}
}

// A refused WebSocket upgrade is an answer too: its status and delay ride on
// the dial's error. A refused connection has no answer and carries none.
func TestRefusedUpgradeCarriesItsAnswer(t *testing.T) {
	srv := hardening.NewTestServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Retry-After", "30")
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()
	dialer := websocket.Dialer{HandshakeTimeout: time.Second}
	_, resp, err := dialer.DialContext(context.Background(), "ws"+srv.URL[len("http"):], nil)
	mapped := upgradeAnswerError(&sdkerrors.RealtimeError{Msg: err.Error()}, resp)
	if sdkerrors.AnswerStatus(mapped) != http.StatusServiceUnavailable {
		t.Fatalf("status %d", sdkerrors.AnswerStatus(mapped))
	}
	if got := sdkerrors.RetryAfter(mapped); got == nil || *got != 30 {
		t.Fatalf("retry after %v", got)
	}
	_, resp, err = dialer.DialContext(context.Background(), "ws://127.0.0.1:1/", nil)
	mapped = upgradeAnswerError(&sdkerrors.RealtimeError{Msg: err.Error()}, resp)
	if sdkerrors.AnswerStatus(mapped) != 0 || sdkerrors.RetryAfter(mapped) != nil {
		t.Fatalf("refused connection carries an answer: %+v", mapped)
	}
}
