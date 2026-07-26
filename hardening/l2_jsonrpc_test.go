package hardening_test

import (
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Fabric-Labs/polyester-sdk-go/chain"
	"github.com/Fabric-Labs/polyester-sdk-go/hardening"
)

func TestL2JSONRPCHeadersThenStalledBodyTimesOut(t *testing.T) {
	stall := 30 * time.Second
	timeout := 400 * time.Millisecond
	httpSrv := hardening.SpawnHTTP(func(hardening.ParsedRequest) hardening.HttpResponse {
		return hardening.HeadersThenStall(200, [][2]string{
			{"Content-Type", "application/json"},
			{"Transfer-Encoding", "chunked"},
		}, stall)
	})
	t.Cleanup(httpSrv.Close)

	rpc := chain.NewJSONRPCClient(httpSrv.BaseURL(), timeout)
	started := time.Now()
	_, err := rpc.Request("eth_chainId", []any{})
	elapsed := time.Since(started)
	if err == nil {
		t.Fatal("stalled JSON-RPC body must timeout")
	}
	if elapsed >= timeout+800*time.Millisecond {
		t.Fatalf("elapsed %s exceeded deadline+slack", elapsed)
	}
	if !isTimeoutErr(err) {
		t.Fatalf("unexpected error: %v", err)
	}
	// Confirm http.Client.Timeout is the e2e deadline (request+body), not a
	// headers-only budget that would let a slow body hang past timeout.
	if elapsed < timeout/2 {
		t.Fatalf("elapsed %s too short for Client.Timeout=%s", elapsed, timeout)
	}
}

func TestL2JSONRPCNoHeadersTimesOut(t *testing.T) {
	timeout := 400 * time.Millisecond
	httpSrv := hardening.SpawnHTTP(func(hardening.ParsedRequest) hardening.HttpResponse {
		return hardening.NeverRespond()
	})
	t.Cleanup(httpSrv.Close)

	rpc := chain.NewJSONRPCClient(httpSrv.BaseURL(), timeout)
	started := time.Now()
	_, err := rpc.Request("eth_chainId", []any{})
	elapsed := time.Since(started)
	if err == nil {
		t.Fatal("no-headers JSON-RPC must timeout")
	}
	if elapsed >= timeout+800*time.Millisecond {
		t.Fatalf("elapsed %s exceeded deadline+slack", elapsed)
	}
	if !isTimeoutErr(err) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestL2JSONRPCSlowDripExceedsTotalDeadline(t *testing.T) {
	timeout := 400 * time.Millisecond
	body := []byte(`{"jsonrpc":"2.0","id":1,"result":"0x1"}`)
	httpSrv := hardening.SpawnHTTP(func(hardening.ParsedRequest) hardening.HttpResponse {
		return hardening.SlowDrip(200, [][2]string{
			{"Content-Type", "application/json"},
		}, body, 1, 80*time.Millisecond)
	})
	t.Cleanup(httpSrv.Close)

	rpc := chain.NewJSONRPCClient(httpSrv.BaseURL(), timeout)
	started := time.Now()
	_, err := rpc.Request("eth_chainId", []any{})
	elapsed := time.Since(started)
	if err == nil {
		t.Fatal("slow-drip JSON-RPC must timeout via Client.Timeout")
	}
	if elapsed >= timeout+800*time.Millisecond {
		t.Fatalf("elapsed %s exceeded deadline+slack; body likely outside Client.Timeout", elapsed)
	}
	if !isTimeoutErr(err) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestL2JSONRPCChunkedOversizedRejected(t *testing.T) {
	httpSrv := hardening.SpawnHTTP(func(hardening.ParsedRequest) hardening.HttpResponse {
		return hardening.ChunkedBody(200, 2*1024*1024, 64*1024)
	})
	t.Cleanup(httpSrv.Close)

	rpc := chain.NewJSONRPCClient(httpSrv.BaseURL(), 2*time.Second)
	_, err := rpc.Request("eth_call", []any{})
	if err == nil || (!strings.Contains(err.Error(), "exceeds") && !strings.Contains(err.Error(), "read")) {
		t.Fatalf("want oversized rejection, got %v", err)
	}
}

func TestL2JSONRPCRejectsOversizedAndBadEnvelope(t *testing.T) {
	httpSrv := hardening.SpawnHTTP(func(req hardening.ParsedRequest) hardening.HttpResponse {
		switch {
		case strings.Contains(req.Path, "big"):
			body := make([]byte, 2*1024*1024)
			for i := range body {
				body[i] = 'x'
			}
			return hardening.Raw(200, [][2]string{
				{"Content-Type", "application/json"},
				{"Content-Length", "2097152"},
			}, body)
		case strings.Contains(req.Path, "malformed"):
			return hardening.Json(200, []byte(`{"jsonrpc":"2.0","id":1,"result":`))
		case strings.Contains(req.Path, "baderr"):
			return hardening.Json(200, []byte(`{"jsonrpc":"2.0","id":1,"error":"not-an-object"}`))
		case strings.Contains(req.Path, "ver"):
			return hardening.Json(200, []byte(`{"jsonrpc":"1.0","id":1,"result":1}`))
		case strings.Contains(req.Path, "nover"):
			return hardening.Json(200, []byte(`{"id":1,"result":1}`))
		case strings.Contains(req.Path, "noid"):
			return hardening.Json(200, []byte(`{"jsonrpc":"2.0","result":1}`))
		case strings.Contains(req.Path, "nullid"):
			return hardening.Json(200, []byte(`{"jsonrpc":"2.0","id":null,"result":1}`))
		case strings.Contains(req.Path, "wrongid"):
			return hardening.Json(200, []byte(`{"jsonrpc":"2.0","id":999,"result":1}`))
		case strings.Contains(req.Path, "neither"):
			return hardening.Json(200, []byte(`{"jsonrpc":"2.0","id":1}`))
		case strings.Contains(req.Path, "nullok"):
			return hardening.Json(200, []byte(`{"jsonrpc":"2.0","id":1,"result":null}`))
		default:
			return hardening.Json(200, []byte(`{"jsonrpc":"2.0","id":1,"result":1,"error":{"code":-1,"message":"x"}}`))
		}
	})
	t.Cleanup(httpSrv.Close)

	mustErrContains := func(path, substr string) {
		t.Helper()
		rpc := chain.NewJSONRPCClient(httpSrv.BaseURL()+path, 2*time.Second)
		_, err := rpc.Request("eth_call", []any{})
		if err == nil || !strings.Contains(strings.ToLower(err.Error()), substr) {
			t.Fatalf("%s: want %q in error, got %v", path, substr, err)
		}
	}

	mustErrContains("/ok", "exactly one")
	mustErrContains("/big", "exceed")
	mustErrContains("/malformed", "decode")
	mustErrContains("/baderr", "malformed")
	mustErrContains("/ver", "version")
	mustErrContains("/nover", "version")
	mustErrContains("/noid", "id")
	mustErrContains("/nullid", "id")
	mustErrContains("/wrongid", "id")
	mustErrContains("/neither", "exactly one")

	// null result is a valid success envelope (result present, error absent).
	rpc := chain.NewJSONRPCClient(httpSrv.BaseURL()+"/nullok", 2*time.Second)
	raw, err := rpc.Request("eth_call", []any{})
	if err != nil {
		t.Fatalf("null result must succeed: %v", err)
	}
	if string(raw) != "null" {
		t.Fatalf("result=%s want null", raw)
	}
}

func TestL2JSONRPC25ConcurrentReorderedResponsesSucceed(t *testing.T) {
	type pending struct {
		conn net.Conn
		id   int64
	}
	var mu sync.Mutex
	var batch []pending

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				buf := make([]byte, 64*1024)
				n, err := c.Read(buf)
				if err != nil || n == 0 {
					_ = c.Close()
					return
				}
				raw := string(buf[:n])
				body := raw
				if i := strings.Index(raw, "\r\n\r\n"); i >= 0 {
					body = raw[i+4:]
				}
				var req struct {
					ID int64 `json:"id"`
				}
				_ = json.Unmarshal([]byte(body), &req)

				var flush []pending
				mu.Lock()
				batch = append(batch, pending{conn: c, id: req.ID})
				if len(batch) == 25 {
					flush = batch
					batch = nil
				}
				mu.Unlock()
				if flush == nil {
					return
				}
				// Reply in reverse id order (reorder-safe client).
				for i := 0; i < len(flush); i++ {
					for j := i + 1; j < len(flush); j++ {
						if flush[j].id > flush[i].id {
							flush[i], flush[j] = flush[j], flush[i]
						}
					}
				}
				for _, p := range flush {
					resp := fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"result":"ok-%d"}`, p.id, p.id)
					head := fmt.Sprintf(
						"HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nContent-Length: %d\r\nConnection: close\r\n\r\n",
						len(resp),
					)
					_, _ = p.conn.Write([]byte(head + resp))
					_ = p.conn.Close()
				}
			}(conn)
		}
	}()

	rpc := chain.NewJSONRPCClient("http://"+ln.Addr().String(), 5*time.Second)
	type outcome struct {
		result string
		err    error
	}
	out := make(chan outcome, 25)
	for i := 0; i < 25; i++ {
		go func() {
			raw, err := rpc.Request("eth_chainId", []any{})
			if err != nil {
				out <- outcome{err: err}
				return
			}
			var s string
			if err := json.Unmarshal(raw, &s); err != nil {
				out <- outcome{err: err}
				return
			}
			out <- outcome{result: s}
		}()
	}
	for i := 0; i < 25; i++ {
		o := <-out
		if o.err != nil {
			t.Fatalf("request %d: %v", i, o.err)
		}
		if !strings.HasPrefix(o.result, "ok-") {
			t.Fatalf("result=%q", o.result)
		}
	}
}

func TestL2JSONRPCSuccessPathStillWorks(t *testing.T) {
	httpSrv := hardening.SpawnHTTP(func(hardening.ParsedRequest) hardening.HttpResponse {
		return hardening.Json(200, []byte(`{"jsonrpc":"2.0","id":1,"result":"0x1"}`))
	})
	t.Cleanup(httpSrv.Close)

	rpc := chain.NewJSONRPCClient(httpSrv.BaseURL(), 2*time.Second)
	raw, err := rpc.Request("eth_chainId", []any{})
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != `"0x1"` {
		t.Fatalf("result=%s", raw)
	}
}
