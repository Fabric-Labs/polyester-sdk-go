package chain

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync/atomic"
	"time"
)

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
	url    string
	client *http.Client
	nextID atomic.Int64
}

// NewJSONRPCClient creates a client for url with the given timeout.
func NewJSONRPCClient(url string, timeout time.Duration) *JSONRPCClient {
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	return &JSONRPCClient{
		url: url,
		client: &http.Client{
			Timeout: timeout,
		},
	}
}

type jsonRPCRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int64  `json:"id"`
	Method  string `json:"method"`
	Params  any    `json:"params"`
}

type jsonRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  json.RawMessage `json:"result"`
	Error   *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    any    `json:"data"`
	} `json:"error"`
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
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read json-rpc response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("json-rpc http %d: %s", resp.StatusCode, string(body))
	}
	var decoded jsonRPCResponse
	if err := json.Unmarshal(body, &decoded); err != nil {
		return nil, fmt.Errorf("decode json-rpc response: %w", err)
	}
	if decoded.Error != nil {
		code := decoded.Error.Code
		return nil, &JSONRPCError{
			Message: decoded.Error.Message,
			Code:    &code,
			Data:    decoded.Error.Data,
		}
	}
	return decoded.Result, nil
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
