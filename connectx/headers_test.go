package connectx

import (
	"strings"
	"testing"
)

func TestContentTypeUsesUnaryMediaTypes(t *testing.T) {
	if got := ContentType(WireBinary); got != "application/proto" {
		t.Fatalf("binary content type: got %q", got)
	}
	if got := ContentType(WireJSON); got != "application/json" {
		t.Fatalf("json content type: got %q", got)
	}
	if strings.Contains(ContentType(WireBinary), "connect+") {
		t.Fatal("unary binary must not use the streaming connect+ proto media type")
	}
}

func TestHeadersSetConnectProtocolVersion(t *testing.T) {
	headers := Headers(WireBinary)
	if headers["Content-Type"] != ProtoContentType {
		t.Fatalf("content type: %q", headers["Content-Type"])
	}
	if headers["Connect-Protocol-Version"] != ProtocolVersion {
		t.Fatalf("protocol version: %q", headers["Connect-Protocol-Version"])
	}
}
