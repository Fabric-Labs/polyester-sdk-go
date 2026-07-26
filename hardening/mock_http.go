package hardening

import (
	"fmt"
	"io"
	"net"
	"strings"
	"sync/atomic"
	"time"
)

// ParsedRequest is a minimal HTTP request line parse for mock handlers.
type ParsedRequest struct {
	Method string
	Path   string
}

// HttpScript controls how the mock HTTP server answers a request.
type HttpScript int

const (
	// ScriptNotFound returns HTTP 404 with an empty body.
	ScriptNotFound HttpScript = iota
	// ScriptJson returns status + JSON body (Content-Type application/json).
	ScriptJson
	// ScriptRaw returns status + custom headers + body.
	ScriptRaw
	// ScriptHeadersThenStall writes status/headers then waits without a body.
	ScriptHeadersThenStall
	// ScriptNeverRespond accepts the TCP connection and never writes a response.
	ScriptNeverRespond
	// ScriptChunkedBody writes a chunked body of TotalBytes.
	ScriptChunkedBody
	// ScriptSlowDrip writes headers then drips body bytes with InterChunkDelay.
	ScriptSlowDrip
)

// HttpResponse is the payload for mock HTTP scripts.
type HttpResponse struct {
	Script          HttpScript
	Status          int
	Headers         [][2]string
	Body            []byte
	Stall           time.Duration
	TotalBytes      int
	ChunkSize       int
	InterChunkDelay time.Duration
}

// MockHTTPServer is a raw TCP HTTP server for timeout/body-stall L2 tests.
type MockHTTPServer struct {
	baseURL  string
	ln       net.Listener
	reqs     atomic.Int64
	inFlight atomic.Int64
	done     chan struct{}
}

// SpawnHTTP starts a mock HTTP server on 127.0.0.1:0.
func SpawnHTTP(handler func(ParsedRequest) HttpResponse) *MockHTTPServer {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		panic("hardening: bind http: " + err.Error())
	}
	s := &MockHTTPServer{
		baseURL: "http://" + ln.Addr().String(),
		ln:      ln,
		done:    make(chan struct{}),
	}
	go func() {
		defer close(s.done)
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go s.serve(conn, handler)
		}
	}()
	return s
}

// BaseURL returns the http://127.0.0.1:port base URL.
func (s *MockHTTPServer) BaseURL() string { return s.baseURL }

// RequestCount returns how many HTTP requests have been accepted.
func (s *MockHTTPServer) RequestCount() int64 { return s.reqs.Load() }

// InFlight returns how many HTTP connections are currently being served.
func (s *MockHTTPServer) InFlight() int64 { return s.inFlight.Load() }

// Close stops the listener.
func (s *MockHTTPServer) Close() {
	_ = s.ln.Close()
	<-s.done
}

func (s *MockHTTPServer) serve(conn net.Conn, handler func(ParsedRequest) HttpResponse) {
	s.inFlight.Add(1)
	defer func() {
		s.inFlight.Add(-1)
		_ = conn.Close()
	}()
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	buf := make([]byte, 16*1024)
	n, err := conn.Read(buf)
	if err != nil || n == 0 {
		return
	}
	s.reqs.Add(1)
	raw := string(buf[:n])
	line, _, _ := strings.Cut(raw, "\r\n")
	parts := strings.Fields(line)
	method, path := "GET", "/"
	if len(parts) >= 1 {
		method = parts[0]
	}
	if len(parts) >= 2 {
		path = parts[1]
		if i := strings.IndexByte(path, '?'); i >= 0 {
			path = path[:i]
		}
	}
	resp := handler(ParsedRequest{Method: method, Path: path})
	if resp.Status == 0 {
		resp.Status = 200
	}
	switch resp.Script {
	case ScriptNeverRespond:
		waitOrPeerClose(conn, 30*time.Second)
	case ScriptNotFound:
		_, _ = io.WriteString(conn, "HTTP/1.1 404 Not Found\r\nContent-Length: 0\r\nConnection: close\r\n\r\n")
	case ScriptJson:
		head := fmt.Sprintf(
			"HTTP/1.1 %d OK\r\nContent-Type: application/json\r\nContent-Length: %d\r\nConnection: close\r\n\r\n",
			resp.Status, len(resp.Body),
		)
		_, _ = io.WriteString(conn, head)
		_, _ = conn.Write(resp.Body)
	case ScriptRaw:
		var b strings.Builder
		fmt.Fprintf(&b, "HTTP/1.1 %d OK\r\n", resp.Status)
		for _, hv := range resp.Headers {
			fmt.Fprintf(&b, "%s: %s\r\n", hv[0], hv[1])
		}
		b.WriteString("Connection: close\r\n\r\n")
		_, _ = io.WriteString(conn, b.String())
		_, _ = conn.Write(resp.Body)
	case ScriptHeadersThenStall:
		var b strings.Builder
		fmt.Fprintf(&b, "HTTP/1.1 %d OK\r\n", resp.Status)
		for _, hv := range resp.Headers {
			fmt.Fprintf(&b, "%s: %s\r\n", hv[0], hv[1])
		}
		b.WriteString("\r\n")
		_, _ = io.WriteString(conn, b.String())
		stall := resp.Stall
		if stall <= 0 {
			stall = 30 * time.Second
		}
		waitOrPeerClose(conn, stall)
	case ScriptChunkedBody:
		total := resp.TotalBytes
		if total <= 0 {
			total = 70_000
		}
		chunk := resp.ChunkSize
		if chunk <= 0 {
			chunk = 4096
		}
		head := fmt.Sprintf("HTTP/1.1 %d OK\r\nTransfer-Encoding: chunked\r\nConnection: close\r\n\r\n", resp.Status)
		if _, err := io.WriteString(conn, head); err != nil {
			return
		}
		sent := 0
		for sent < total {
			n := chunk
			if rem := total - sent; rem < n {
				n = rem
			}
			payload := bytesRepeat('x', n)
			if _, err := fmt.Fprintf(conn, "%x\r\n", n); err != nil {
				return
			}
			if _, err := conn.Write(payload); err != nil {
				return
			}
			if _, err := io.WriteString(conn, "\r\n"); err != nil {
				return
			}
			sent += n
		}
		_, _ = io.WriteString(conn, "0\r\n\r\n")
	case ScriptSlowDrip:
		body := resp.Body
		if len(body) == 0 {
			body = []byte(`{"token":"slow-drip-token-value"}`)
		}
		delay := resp.InterChunkDelay
		if delay <= 0 {
			delay = 150 * time.Millisecond
		}
		chunk := resp.ChunkSize
		if chunk <= 0 {
			chunk = 1
		}
		var b strings.Builder
		fmt.Fprintf(&b, "HTTP/1.1 %d OK\r\n", resp.Status)
		hasCL := false
		for _, hv := range resp.Headers {
			fmt.Fprintf(&b, "%s: %s\r\n", hv[0], hv[1])
			if strings.EqualFold(hv[0], "Content-Length") {
				hasCL = true
			}
		}
		if !hasCL {
			fmt.Fprintf(&b, "Content-Length: %d\r\n", len(body))
		}
		b.WriteString("Connection: close\r\n\r\n")
		if _, err := io.WriteString(conn, b.String()); err != nil {
			return
		}
		for off := 0; off < len(body); off += chunk {
			end := off + chunk
			if end > len(body) {
				end = len(body)
			}
			if _, err := conn.Write(body[off:end]); err != nil {
				return
			}
			if end < len(body) {
				if !sleepOrPeerClose(conn, delay) {
					return
				}
			}
		}
	}
}

func waitOrPeerClose(conn net.Conn, stall time.Duration) {
	_ = conn.SetReadDeadline(time.Now().Add(stall))
	buf := make([]byte, 1)
	_, _ = conn.Read(buf) // returns on peer close or deadline
}

func sleepOrPeerClose(conn net.Conn, d time.Duration) bool {
	_ = conn.SetReadDeadline(time.Now().Add(d))
	buf := make([]byte, 1)
	_, err := conn.Read(buf)
	if err == nil {
		return true
	}
	if ne, ok := err.(net.Error); ok && ne.Timeout() {
		return true
	}
	return false
}

func bytesRepeat(b byte, n int) []byte {
	out := make([]byte, n)
	for i := range out {
		out[i] = b
	}
	return out
}

// Json is a convenience constructor for ScriptJson.
func Json(status int, body []byte) HttpResponse {
	return HttpResponse{Script: ScriptJson, Status: status, Body: body}
}

// Raw is a convenience constructor for ScriptRaw.
func Raw(status int, headers [][2]string, body []byte) HttpResponse {
	return HttpResponse{Script: ScriptRaw, Status: status, Headers: headers, Body: body}
}

// HeadersThenStall is a convenience constructor for ScriptHeadersThenStall.
func HeadersThenStall(status int, headers [][2]string, stall time.Duration) HttpResponse {
	return HttpResponse{Script: ScriptHeadersThenStall, Status: status, Headers: headers, Stall: stall}
}

// NeverRespond is a convenience constructor for ScriptNeverRespond.
func NeverRespond() HttpResponse {
	return HttpResponse{Script: ScriptNeverRespond}
}

// NotFound is a convenience constructor for ScriptNotFound.
func NotFound() HttpResponse {
	return HttpResponse{Script: ScriptNotFound, Status: 404}
}

// ChunkedBody returns a chunked response that streams TotalBytes.
func ChunkedBody(status, totalBytes, chunkSize int) HttpResponse {
	return HttpResponse{
		Script:     ScriptChunkedBody,
		Status:     status,
		TotalBytes: totalBytes,
		ChunkSize:  chunkSize,
	}
}

// SlowDrip returns headers then drips Body with InterChunkDelay between chunks.
func SlowDrip(status int, headers [][2]string, body []byte, chunkSize int, delay time.Duration) HttpResponse {
	return HttpResponse{
		Script:          ScriptSlowDrip,
		Status:          status,
		Headers:         headers,
		Body:            body,
		ChunkSize:       chunkSize,
		InterChunkDelay: delay,
	}
}
