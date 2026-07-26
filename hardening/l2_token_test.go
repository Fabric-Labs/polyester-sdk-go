package hardening_test

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	"github.com/Fabric-Labs/polyester-sdk-go/hardening"
	"github.com/Fabric-Labs/polyester-sdk-go/realtime"
)

func TestL2TokenHeadersThenStalledBodyTimesOutViaSubscribeProto(t *testing.T) {
	stall := 30 * time.Second
	timeout := 400 * time.Millisecond
	httpSrv := hardening.SpawnHTTP(func(req hardening.ParsedRequest) hardening.HttpResponse {
		if strings.HasPrefix(req.Path, "/v1/rt/") {
			return hardening.HeadersThenStall(200, [][2]string{
				{"Transfer-Encoding", "chunked"},
			}, stall)
		}
		return hardening.NotFound()
	})
	t.Cleanup(httpSrv.Close)
	ws := hardening.SpawnHangAfterAccept()
	t.Cleanup(ws.Close)

	rt := newRT(ws.WSURL(), httpSrv.BaseURL(), testCreds(t), timeout)
	ctx := context.Background()
	started := time.Now()
	_, err := realtime.SubscribeProto(ctx, rt, "private:spot:orders:acct:proto", identityDecode)
	elapsed := time.Since(started)
	if err == nil {
		t.Fatal("stalled token body must fail")
	}
	if elapsed >= timeout+800*time.Millisecond {
		t.Fatalf("elapsed %s exceeded deadline+slack; body likely outside timeout", elapsed)
	}
	if !isTimeoutErr(err) {
		t.Fatalf("unexpected error: %v", err)
	}
	hardening.WaitUntil(t, func() bool {
		return httpSrv.InFlight() == 0 && ws.ActiveConns() == 0
	}, 750*time.Millisecond)
}

func TestL2TokenNoHeadersTimesOutViaSubscribeProto(t *testing.T) {
	timeout := 400 * time.Millisecond
	httpSrv := hardening.SpawnHTTP(func(req hardening.ParsedRequest) hardening.HttpResponse {
		if strings.HasPrefix(req.Path, "/v1/rt/") {
			return hardening.NeverRespond()
		}
		return hardening.NotFound()
	})
	t.Cleanup(httpSrv.Close)
	ws := hardening.SpawnHangAfterAccept()
	t.Cleanup(ws.Close)

	rt := newRT(ws.WSURL(), httpSrv.BaseURL(), testCreds(t), timeout)
	started := time.Now()
	_, err := realtime.SubscribeProto(context.Background(), rt, "private:spot:orders:acct:proto", identityDecode)
	elapsed := time.Since(started)
	if err == nil {
		t.Fatal("no-headers token fetch must fail")
	}
	if elapsed >= timeout+800*time.Millisecond {
		t.Fatalf("elapsed %s exceeded deadline+slack", elapsed)
	}
	if !isTimeoutErr(err) {
		t.Fatalf("unexpected error: %v", err)
	}
	hardening.WaitUntil(t, func() bool {
		return httpSrv.InFlight() == 0 && ws.ActiveConns() == 0
	}, 750*time.Millisecond)
}

func TestL2TokenSlowDripExceedsTotalDeadlineViaSubscribeProto(t *testing.T) {
	// Each byte arrives well before any reasonable per-read timeout, but the
	// full body drip exceeds http.Client.Timeout (one e2e deadline).
	timeout := 400 * time.Millisecond
	body := []byte(`{"token":"abcdefghijklmnopqrstuvwxyz0123456789"}`)
	httpSrv := hardening.SpawnHTTP(func(req hardening.ParsedRequest) hardening.HttpResponse {
		if strings.HasPrefix(req.Path, "/v1/rt/") {
			return hardening.SlowDrip(200, [][2]string{
				{"Content-Type", "application/json"},
			}, body, 1, 80*time.Millisecond)
		}
		return hardening.NotFound()
	})
	t.Cleanup(httpSrv.Close)
	ws := hardening.SpawnHangAfterAccept()
	t.Cleanup(ws.Close)

	rt := newRT(ws.WSURL(), httpSrv.BaseURL(), testCreds(t), timeout)
	started := time.Now()
	_, err := realtime.SubscribeProto(context.Background(), rt, "private:spot:orders:acct:proto", identityDecode)
	elapsed := time.Since(started)
	if err == nil {
		t.Fatal("slow-drip token body must fail via total deadline")
	}
	if elapsed >= timeout+800*time.Millisecond {
		t.Fatalf("elapsed %s exceeded deadline+slack; body likely outside Client.Timeout", elapsed)
	}
	if !isTimeoutErr(err) {
		t.Fatalf("unexpected error: %v", err)
	}
	hardening.WaitUntil(t, func() bool {
		return httpSrv.InFlight() == 0 && ws.ActiveConns() == 0
	}, 750*time.Millisecond)
}

func TestL2TokenContentLength65537RejectedViaSubscribeProto(t *testing.T) {
	body := []byte(`{"token":"x"}`)
	httpSrv := hardening.SpawnHTTP(func(req hardening.ParsedRequest) hardening.HttpResponse {
		if strings.HasPrefix(req.Path, "/v1/rt/") {
			return hardening.Raw(200, [][2]string{
				{"Content-Type", "application/json"},
				{"Content-Length", "65537"},
			}, body)
		}
		return hardening.NotFound()
	})
	t.Cleanup(httpSrv.Close)
	ws := hardening.SpawnHangAfterAccept()
	t.Cleanup(ws.Close)

	rt := newRT(ws.WSURL(), httpSrv.BaseURL(), testCreds(t), 2*time.Second)
	_, err := realtime.SubscribeProto(context.Background(), rt, "private:spot:orders:acct:proto", identityDecode)
	if err == nil {
		t.Fatal("oversized Content-Length must be rejected")
	}
	if !strings.Contains(errText(err), "exceeds") {
		t.Fatalf("want exceeds error, got %v", err)
	}
	hardening.WaitUntil(t, func() bool {
		return httpSrv.InFlight() == 0 && ws.ActiveConns() == 0
	}, 750*time.Millisecond)
}

func TestL2TokenChunkedOversizedRejectedViaSubscribeProto(t *testing.T) {
	httpSrv := hardening.SpawnHTTP(func(req hardening.ParsedRequest) hardening.HttpResponse {
		if strings.HasPrefix(req.Path, "/v1/rt/") {
			return hardening.ChunkedBody(200, 70_000, 4096)
		}
		return hardening.NotFound()
	})
	t.Cleanup(httpSrv.Close)
	ws := hardening.SpawnHangAfterAccept()
	t.Cleanup(ws.Close)

	rt := newRT(ws.WSURL(), httpSrv.BaseURL(), testCreds(t), 2*time.Second)
	_, err := realtime.SubscribeProto(context.Background(), rt, "private:spot:orders:acct:proto", identityDecode)
	if err == nil {
		t.Fatal("chunked >64KiB must be rejected")
	}
	if !strings.Contains(errText(err), "exceeds") && !strings.Contains(errText(err), "64") {
		t.Fatalf("want exceeds/64KiB error, got %v", err)
	}
	hardening.WaitUntil(t, func() bool {
		return httpSrv.InFlight() == 0 && ws.ActiveConns() == 0
	}, 750*time.Millisecond)
}

func TestL2TokenEmptyTokenRejectedViaSubscribeProto(t *testing.T) {
	httpSrv := hardening.SpawnHTTP(func(req hardening.ParsedRequest) hardening.HttpResponse {
		if strings.HasPrefix(req.Path, "/v1/rt/") {
			return hardening.Json(200, []byte(`{"token":""}`))
		}
		return hardening.NotFound()
	})
	t.Cleanup(httpSrv.Close)
	ws := hardening.SpawnHangAfterAccept()
	t.Cleanup(ws.Close)

	rt := newRT(ws.WSURL(), httpSrv.BaseURL(), testCreds(t), 2*time.Second)
	_, err := realtime.SubscribeProto(context.Background(), rt, "private:spot:orders:acct:proto", identityDecode)
	if err == nil {
		t.Fatal("empty token must be rejected")
	}
	if !strings.Contains(errText(err), "token") {
		t.Fatalf("want token error, got %v", err)
	}
	hardening.WaitUntil(t, func() bool {
		return httpSrv.InFlight() == 0 && ws.ActiveConns() == 0
	}, 750*time.Millisecond)
}

func TestL2TokenMalformedJSONRejectedViaSubscribeProto(t *testing.T) {
	httpSrv := hardening.SpawnHTTP(func(req hardening.ParsedRequest) hardening.HttpResponse {
		if strings.HasPrefix(req.Path, "/v1/rt/") {
			return hardening.Json(200, []byte(`{"token":`))
		}
		return hardening.NotFound()
	})
	t.Cleanup(httpSrv.Close)
	ws := hardening.SpawnHangAfterAccept()
	t.Cleanup(ws.Close)

	rt := newRT(ws.WSURL(), httpSrv.BaseURL(), testCreds(t), 2*time.Second)
	_, err := realtime.SubscribeProto(context.Background(), rt, "private:spot:orders:acct:proto", identityDecode)
	if err == nil {
		t.Fatal("malformed JSON must be rejected")
	}
	if !strings.Contains(errText(err), "invalid") && !strings.Contains(errText(err), "token") {
		t.Fatalf("want invalid token response, got %v", err)
	}
	hardening.WaitUntil(t, func() bool {
		return httpSrv.InFlight() == 0 && ws.ActiveConns() == 0
	}, 750*time.Millisecond)
}

func TestL2TokenHTTP403MapsToAuthNotRealtimeViaSubscribeProto(t *testing.T) {
	httpSrv := hardening.SpawnHTTP(func(req hardening.ParsedRequest) hardening.HttpResponse {
		if strings.HasPrefix(req.Path, "/v1/rt/") {
			return hardening.Json(403, []byte(`{"code":"permission_denied","message":"missing transfer:read"}`))
		}
		return hardening.NotFound()
	})
	t.Cleanup(httpSrv.Close)
	ws := hardening.SpawnHangAfterAccept()
	t.Cleanup(ws.Close)

	rt := newRT(ws.WSURL(), httpSrv.BaseURL(), testCreds(t), 2*time.Second)
	_, err := realtime.SubscribeProto(context.Background(), rt, "private:auth:transfers:acct:proto", identityDecode)
	if err == nil {
		t.Fatal("403 must error")
	}
	var authErr *sdkerrors.AuthError
	if !errors.As(err, &authErr) {
		t.Fatalf("expected AuthError, got %T: %v", err, err)
	}
	msg := err.Error()
	if !strings.Contains(strings.ToLower(msg), "permission") && !strings.Contains(msg, "HTTP 403") {
		t.Fatalf("want permission/403 in message, got %q", msg)
	}
	if !strings.Contains(msg, "HTTP 403") {
		t.Fatalf("want HTTP 403 in message, got %q", msg)
	}
	if !strings.Contains(msg, "transfer:read") {
		t.Fatalf("want transfer:read in body, got %q", msg)
	}
	var rtErr *sdkerrors.RealtimeError
	if errors.As(err, &rtErr) {
		t.Fatalf("must not be RealtimeError: %v", err)
	}
	hardening.WaitUntil(t, func() bool {
		return httpSrv.InFlight() == 0 && ws.ActiveConns() == 0
	}, 750*time.Millisecond)
}

func TestL2CancelDuringTokenBodyStallNoOrphan(t *testing.T) {
	stall := 30 * time.Second
	httpSrv := hardening.SpawnHTTP(func(req hardening.ParsedRequest) hardening.HttpResponse {
		if strings.HasPrefix(req.Path, "/v1/rt/") {
			return hardening.HeadersThenStall(200, [][2]string{
				{"Transfer-Encoding", "chunked"},
			}, stall)
		}
		return hardening.NotFound()
	})
	t.Cleanup(httpSrv.Close)
	var wsActive atomic.Int64
	ws := hardening.SpawnHangAfterAcceptCounted(&wsActive)
	t.Cleanup(ws.Close)

	// Long client timeout so cancel (not Timeout) is the abort path.
	rt := newRT(ws.WSURL(), httpSrv.BaseURL(), testCreds(t), 30*time.Second)
	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		_, err := realtime.SubscribeProto(ctx, rt, "private:spot:orders:acct:proto", identityDecode)
		errCh <- err
	}()

	hardening.WaitUntil(t, func() bool { return httpSrv.InFlight() >= 1 }, 2*time.Second)
	started := time.Now()
	cancel()
	select {
	case err := <-errCh:
		if err == nil {
			t.Fatal("cancel must surface an error")
		}
		if !errors.Is(err, context.Canceled) &&
			!strings.Contains(errText(err), "cancel") {
			t.Fatalf("unexpected cancel error: %v", err)
		}
	case <-time.After(750 * time.Millisecond):
		t.Fatal("cancel during token stall did not return promptly")
	}
	if time.Since(started) > 750*time.Millisecond {
		t.Fatalf("cancel lingered %s", time.Since(started))
	}
	hardening.WaitUntil(t, func() bool {
		return httpSrv.InFlight() == 0 && wsActive.Load() == 0
	}, 750*time.Millisecond)
}
