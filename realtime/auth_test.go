package realtime

import (
	"net/http"
	"testing"
)

func TestContentLengthExceedsLimit(t *testing.T) {
	header := http.Header{}
	header.Set("Content-Length", "65537")
	if !contentLengthExceedsLimit(header, maxTokenResponseBytes) {
		t.Fatal("expected oversized content-length to exceed limit")
	}
	header.Set("Content-Length", "1024")
	if contentLengthExceedsLimit(header, maxTokenResponseBytes) {
		t.Fatal("expected small content-length to pass")
	}
}
