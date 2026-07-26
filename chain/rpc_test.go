package chain

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestJSONRPCValidResult(t *testing.T) {
	var gotID int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		_ = json.NewDecoder(r.Body).Decode(&req)
		gotID = int64(req["id"].(float64))
		_ = json.NewEncoder(w).Encode(map[string]any{
			"jsonrpc": "2.0",
			"id":      gotID,
			"result":  "0x1",
		})
	}))
	defer srv.Close()

	client := NewJSONRPCClient(srv.URL, time.Second)
	raw, err := client.Request("eth_chainId", nil)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != `"0x1"` {
		t.Fatalf("result=%s", raw)
	}
}

func TestJSONRPCRejectsWrongVersion(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"jsonrpc":"1.0","id":1,"result":1}`))
	}))
	defer srv.Close()
	_, err := NewJSONRPCClient(srv.URL, time.Second).Request("eth_chainId", nil)
	if err == nil || !strings.Contains(err.Error(), "version") {
		t.Fatalf("want version error, got %v", err)
	}
}

func TestJSONRPCRejectsIDMismatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":999,"result":1}`))
	}))
	defer srv.Close()
	_, err := NewJSONRPCClient(srv.URL, time.Second).Request("eth_chainId", nil)
	if err == nil || !strings.Contains(err.Error(), "id mismatch") {
		t.Fatalf("want id mismatch, got %v", err)
	}
}

func TestJSONRPCRejectsMissingResultAndError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1}`))
	}))
	defer srv.Close()
	_, err := NewJSONRPCClient(srv.URL, time.Second).Request("eth_chainId", nil)
	if err == nil || !strings.Contains(err.Error(), "exactly one") {
		t.Fatalf("want envelope error, got %v", err)
	}
}

func TestJSONRPCRejectsBothResultAndError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":1,"error":{"code":1,"message":"x"}}`))
	}))
	defer srv.Close()
	_, err := NewJSONRPCClient(srv.URL, time.Second).Request("eth_chainId", nil)
	if err == nil || !strings.Contains(err.Error(), "exactly one") {
		t.Fatalf("want envelope error, got %v", err)
	}
}

func TestJSONRPCRejectsOversizedBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":"`))
		_, _ = w.Write(make([]byte, MaxJSONRPCResponseBytes+16))
		_, _ = w.Write([]byte(`"}`))
	}))
	defer srv.Close()
	_, err := NewJSONRPCClient(srv.URL, 5*time.Second).Request("eth_chainId", nil)
	if err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("want size error, got %v", err)
	}
}

func TestJSONRPCTimeoutCoversBodyStall(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		time.Sleep(2 * time.Second)
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":1}`))
	}))
	defer srv.Close()

	start := time.Now()
	_, err := NewJSONRPCClient(srv.URL, 200*time.Millisecond).Request("eth_chainId", nil)
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("expected timeout")
	}
	if elapsed > time.Second {
		t.Fatalf("timeout took too long: %s", elapsed)
	}
}

func TestJSONRPCNoHeadersTimesOut(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer func() { _ = c.Close() }()
				buf := make([]byte, 4096)
				_, _ = c.Read(buf)
				time.Sleep(5 * time.Second)
			}(conn)
		}
	}()

	start := time.Now()
	_, err = NewJSONRPCClient("http://"+ln.Addr().String(), 200*time.Millisecond).Request("eth_chainId", nil)
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("expected timeout when server never sends headers")
	}
	if elapsed > time.Second {
		t.Fatalf("timeout took too long: %s", elapsed)
	}
}

func TestJSONRPCConcurrentReorderedOK(t *testing.T) {
	var next atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ID int64 `json:"id"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		// Intentionally delay early ids so responses reorder.
		if req.ID <= 5 {
			time.Sleep(20 * time.Millisecond)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"jsonrpc": "2.0",
			"id":      req.ID,
			"result":  fmt.Sprintf("ok-%d", req.ID),
		})
		next.Add(1)
	}))
	defer srv.Close()

	client := NewJSONRPCClient(srv.URL, 5*time.Second)
	errs := make(chan error, 25)
	for i := 0; i < 25; i++ {
		go func() {
			_, err := client.Request("eth_chainId", nil)
			errs <- err
		}()
	}
	for i := 0; i < 25; i++ {
		if err := <-errs; err != nil {
			t.Fatalf("request %d: %v", i, err)
		}
	}
}
