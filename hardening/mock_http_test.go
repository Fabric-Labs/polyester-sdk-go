package hardening_test

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/Fabric-Labs/polyester-sdk-go/hardening"
)

func sendRaw(t *testing.T, address, requestLine string) string {
	t.Helper()
	conn, err := net.Dial("tcp", address)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	if _, err := fmt.Fprintf(conn, "%s\r\nHost: test\r\nConnection: close\r\n\r\n", requestLine); err != nil {
		t.Fatal(err)
	}
	response, err := io.ReadAll(conn)
	if err != nil {
		t.Fatal(err)
	}
	return string(response)
}

func TestMockHTTPHidesOnlyAPortProbe(t *testing.T) {
	var mu sync.Mutex
	var seen []string
	srv := hardening.SpawnHTTP(func(req hardening.ParsedRequest) hardening.HttpResponse {
		mu.Lock()
		defer mu.Unlock()
		seen = append(seen, req.Method+" "+req.Path)
		return hardening.NotFound()
	})
	t.Cleanup(srv.Close)
	address := strings.TrimPrefix(srv.BaseURL(), "http://")
	for _, line := range []string{"HEAD / HTTP/1.1", "HEAD /v1/rt/token HTTP/1.1", "GET / HTTP/1.1"} {
		if response := sendRaw(t, address, line); !strings.HasPrefix(response, "HTTP/1.1 404") {
			t.Fatalf("%s: %q", line, response)
		}
	}
	if got := srv.RequestCount(); got != 2 {
		t.Fatalf("RequestCount=%d want 2", got)
	}
	mu.Lock()
	defer mu.Unlock()
	if want := []string{"HEAD /v1/rt/token", "GET /"}; !slices.Equal(seen, want) {
		t.Fatalf("handler saw %v want %v", seen, want)
	}
}

func TestNewTestServerHidesOnlyAPortProbe(t *testing.T) {
	var mu sync.Mutex
	var seen []string
	srv := hardening.NewTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		seen = append(seen, r.Method+" "+r.URL.Path)
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(srv.Close)
	address := strings.TrimPrefix(srv.URL, "http://")
	if response := sendRaw(t, address, "HEAD / HTTP/1.1"); !strings.HasPrefix(response, "HTTP/1.1 404") {
		t.Fatalf("probe: %q", response)
	}
	for _, line := range []string{"HEAD /v1/rt/token HTTP/1.1", "GET / HTTP/1.1"} {
		if response := sendRaw(t, address, line); !strings.HasPrefix(response, "HTTP/1.1 204") {
			t.Fatalf("%s: %q", line, response)
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if want := []string{"HEAD /v1/rt/token", "GET /"}; !slices.Equal(seen, want) {
		t.Fatalf("handler saw %v want %v", seen, want)
	}
}
