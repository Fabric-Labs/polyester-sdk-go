package public_smoke_test

import (
	"context"
	"testing"
	"time"

	polyester "github.com/Fabric-Labs/polyester-sdk-go"
	"github.com/Fabric-Labs/polyester-sdk-go/chain"
	"github.com/Fabric-Labs/polyester-sdk-go/hardening"
)

// Public smoke: construct an unauthenticated client and exercise catalog wait
// disabled + JSON-RPC against a local mock (no POLYESTER_API_KEY_* required).
func TestPublicSmokeClientConstructAndJSONRPC(t *testing.T) {
	httpSrv := hardening.SpawnHTTP(func(hardening.ParsedRequest) hardening.HttpResponse {
		return hardening.Json(200, []byte(`{"jsonrpc":"2.0","id":1,"result":"0x1"}`))
	})
	t.Cleanup(httpSrv.Close)

	client, err := polyester.New(polyester.Config{
		APIURL:          "http://127.0.0.1:1", // unused for this smoke
		HydrateCatalogs: false,
		Timeout:         time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := client.WaitForCatalogs(ctx); err != nil {
		t.Fatalf("WaitForCatalogs with hydrate disabled: %v", err)
	}

	rpc := chain.NewJSONRPCClient(httpSrv.BaseURL(), time.Second)
	raw, err := rpc.Request("eth_chainId", []any{})
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != `"0x1"` {
		t.Fatalf("result=%s", raw)
	}
}
