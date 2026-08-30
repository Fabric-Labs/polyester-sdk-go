package connectx

// WireFormat selects Connect unary encoding.
type WireFormat string

const (
	WireBinary WireFormat = "binary"
	WireJSON   WireFormat = "json"

	ProtocolVersion  = "1"
	JSONContentType  = "application/json"
	// ProtoContentType is the Connect unary binary media type.
	// Streaming envelopes use application/connect+proto instead.
	ProtoContentType = "application/proto"
)

// ContentType returns the Connect Content-Type header value.
func ContentType(wire WireFormat) string {
	if wire == WireJSON {
		return JSONContentType
	}
	return ProtoContentType
}

// Headers returns standard Connect client headers.
func Headers(wire WireFormat) map[string]string {
	return map[string]string{
		"Content-Type":             ContentType(wire),
		"Connect-Protocol-Version": ProtocolVersion,
	}
}
