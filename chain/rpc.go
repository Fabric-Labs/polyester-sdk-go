package chain

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	"github.com/Fabric-Labs/polyester-sdk-go/useragent"
)

// MaxJSONRPCResponseBytes is the default response body size cap (1 MiB).
const MaxJSONRPCResponseBytes = 1 << 20

// JSONRPCError is a JSON-RPC error object returned by a node / bundler / paymaster.
type JSONRPCError struct {
	Message string
	Code    *int
	Data    any
}

func (e *JSONRPCError) Error() string {
	if e == nil {
		return "json-rpc error"
	}
	return e.Message
}

// JSONRPCClient is a minimal JSON-RPC 2.0 POST helper.
type JSONRPCClient struct {
	url     string
	client  *http.Client
	nextID  atomic.Int64
	maxBody int64
}

// NewJSONRPCClient creates a client for url with the given timeout.
// Client.Timeout covers the full request including body read (e2e deadline).
func NewJSONRPCClient(url string, timeout time.Duration) *JSONRPCClient {
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	return &JSONRPCClient{
		url: url,
		client: useragent.WrapClient(&http.Client{
			Timeout: timeout,
		}),
		maxBody: MaxJSONRPCResponseBytes,
	}
}

type jsonRPCRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int64  `json:"id"`
	Method  string `json:"method"`
	Params  any    `json:"params"`
}

// Request posts a JSON-RPC method and returns the decoded result.
func (c *JSONRPCClient) Request(method string, params any) (json.RawMessage, error) {
	if params == nil {
		params = []any{}
	}
	id := c.nextID.Add(1)
	payload, err := json.Marshal(jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      id,
		Method:  method,
		Params:  params,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal json-rpc request: %w", err)
	}
	resp, err := c.client.Post(c.url, "application/json", bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("json-rpc post: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	maxBody := c.maxBody
	if maxBody <= 0 {
		maxBody = MaxJSONRPCResponseBytes
	}
	limited := io.LimitReader(resp.Body, maxBody+1)
	body, err := io.ReadAll(limited)
	if err != nil {
		return nil, fmt.Errorf("read json-rpc response: %w", err)
	}
	if int64(len(body)) > maxBody {
		return nil, fmt.Errorf("json-rpc response exceeds %d bytes", maxBody)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("json-rpc http %d: %s", resp.StatusCode, truncateBody(body, 256))
	}

	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, fmt.Errorf("decode json-rpc response: %w", err)
	}
	return parseJSONRPCSuccess(envelope, id)
}

func parseJSONRPCSuccess(envelope map[string]json.RawMessage, expectedID int64) (json.RawMessage, error) {
	versionRaw, ok := envelope["jsonrpc"]
	if !ok {
		return nil, fmt.Errorf("json-rpc invalid version: want 2.0")
	}
	var version string
	if err := json.Unmarshal(versionRaw, &version); err != nil || version != "2.0" {
		return nil, fmt.Errorf("json-rpc invalid version: want 2.0")
	}

	idRaw, ok := envelope["id"]
	if !ok || len(idRaw) == 0 || string(idRaw) == "null" {
		return nil, fmt.Errorf("json-rpc missing id")
	}
	gotID, err := parseJSONRPCID(idRaw)
	if err != nil {
		return nil, fmt.Errorf("json-rpc invalid id: %w", err)
	}
	if gotID != expectedID {
		return nil, fmt.Errorf("json-rpc id mismatch: got %d want %d", gotID, expectedID)
	}

	_, hasResult := envelope["result"]
	errorRaw, hasError := envelope["error"]
	if hasResult == hasError {
		if !hasResult {
			return nil, fmt.Errorf("json-rpc response must contain exactly one of result or error")
		}
		return nil, fmt.Errorf("json-rpc response must contain exactly one of result or error")
	}
	if hasError {
		if len(errorRaw) == 0 || string(errorRaw) == "null" {
			return nil, fmt.Errorf("json-rpc malformed error object")
		}
		var rpcErr struct {
			Code    *int   `json:"code"`
			Message string `json:"message"`
			Data    any    `json:"data"`
		}
		if err := json.Unmarshal(errorRaw, &rpcErr); err != nil {
			return nil, fmt.Errorf("json-rpc malformed error object: %w", err)
		}
		if rpcErr.Message == "" {
			return nil, fmt.Errorf("json-rpc malformed error object: missing message")
		}
		return nil, &JSONRPCError{
			Message: rpcErr.Message,
			Code:    rpcErr.Code,
			Data:    rpcErr.Data,
		}
	}
	return envelope["result"], nil
}

func parseJSONRPCID(raw json.RawMessage) (int64, error) {
	var asInt int64
	if err := json.Unmarshal(raw, &asInt); err == nil {
		return asInt, nil
	}
	var asString string
	if err := json.Unmarshal(raw, &asString); err == nil {
		return strconv.ParseInt(asString, 10, 64)
	}
	return 0, fmt.Errorf("id must be number or string")
}

func truncateBody(body []byte, max int) string {
	if len(body) <= max {
		return string(body)
	}
	return string(body[:max]) + "…"
}

// RequestDecode posts a JSON-RPC method and unmarshals the result into out.
func (c *JSONRPCClient) RequestDecode(method string, params any, out any) error {
	raw, err := c.Request(method, params)
	if err != nil {
		return err
	}
	if out == nil || len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("decode json-rpc result: %w", err)
	}
	return nil
}

// ethCallWord decodes the one 32-byte ABI word that an eth_call of a function
// returning a single static value answers: 0x and exactly 64 hex digits,
// surrounding whitespace ignored. Anything else, such as non-ASCII, short,
// odd-length or non-hex text from the RPC endpoint, is an
// *errors.TransportError; the text is not echoed.
func ethCallWord(raw, kind string) ([32]byte, error) {
	var word [32]byte
	text := strings.TrimSpace(raw)
	digits, ok := strings.CutPrefix(text, "0x")
	if ok && len(digits) == 2*len(word) {
		if _, err := hex.Decode(word[:], []byte(digits)); err == nil {
			return word, nil
		}
	}
	return [32]byte{}, &sdkerrors.TransportError{Msg: fmt.Sprintf(
		"eth_call %s result is not a 0x-prefixed 32-byte hex word (%d bytes of text)", kind, len(raw),
	)}
}

// ethCallAddress decodes an eth_call result that is one ABI-encoded address:
// a 32-byte word whose upper 12 bytes are zero.
func ethCallAddress(raw, kind string) (string, error) {
	word, err := ethCallWord(raw, kind)
	if err != nil {
		return "", err
	}
	for _, b := range word[:12] {
		if b != 0 {
			return "", &sdkerrors.TransportError{Msg: "eth_call " + kind + " result is not an ABI-encoded address"}
		}
	}
	return "0x" + hex.EncodeToString(word[12:]), nil
}

// ethCallUint decodes an eth_call result that is one ABI-encoded unsigned
// integer of at most bits bits.
func ethCallUint(raw, kind string, bits int) (*big.Int, error) {
	word, err := ethCallWord(raw, kind)
	if err != nil {
		return nil, err
	}
	n := new(big.Int).SetBytes(word[:])
	if n.BitLen() > bits {
		return nil, &sdkerrors.TransportError{Msg: fmt.Sprintf("eth_call %s result exceeds uint%d", kind, bits)}
	}
	return n, nil
}
