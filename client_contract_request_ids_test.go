package polyester

import (
	"context"
	stderrors "errors"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/Fabric-Labs/polyester-sdk-go/auth"
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	"github.com/Fabric-Labs/polyester-sdk-go/internal/hardening"
)

// newRecordingClient returns a client whose server records the method of
// every request and refuses it, so a test sees what was sent.
func newRecordingClient(t *testing.T) (*Client, func() []string) {
	t.Helper()
	var mu sync.Mutex
	var sent []string
	srv := hardening.SpawnHTTP(func(req hardening.ParsedRequest) hardening.HttpResponse {
		mu.Lock()
		sent = append(sent, req.Path[strings.LastIndex(req.Path, "/")+1:])
		mu.Unlock()
		return hardening.Json(http.StatusBadRequest, []byte(`{"code":"failed_precondition","message":"stub"}`))
	})
	t.Cleanup(srv.Close)
	keypair := auth.GenerateEd25519Keypair()
	accountID := "2"
	client, err := New(Config{APIURL: srv.BaseURL(), APIKeyID: "ak_test", APIPrivateKey: keypair.SecretKeyHex,
		DefaultAccountID: &accountID, SkipCatalogHydration: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	return client, func() []string {
		mu.Lock()
		defer mu.Unlock()
		out := sent
		sent = nil
		return out
	}
}

func requireValidationNaming(t *testing.T, name string, err error, field string) {
	t.Helper()
	var validation *sdkerrors.ValidationError
	if !stderrors.As(err, &validation) || !strings.Contains(validation.Msg, field) {
		t.Fatalf("%s: err = %T %v, want a *errors.ValidationError naming %s", name, err, err, field)
	}
}

// Contract asset_info.proto, ListAssetInfoRequest: `repeated uint32
// asset_ids = 1` with `max_items: 1000` and `items: { uint32: { gt: 0, lte:
// 2147483647 } }`. An ID of 0 or above 2147483647, or more than 1000 IDs, is
// refused before sending; an empty list (all assets) and the bounds are sent.
func TestListAssetInfoRefusesIDsOutsideTheContractRange(t *testing.T) {
	client, sent := newRecordingClient(t)
	ctx := context.Background()
	many := make([]uint32, 1001)
	for i := range many {
		many[i] = uint32(i + 1)
	}
	for name, ids := range map[string][]uint32{
		"zero":           {0},
		"above int32":    {2147483648},
		"zero and above": {0, 2147483648},
		"max uint32":     {1, 4294967295},
		"1001 ids":       many,
	} {
		_, err := client.MarketData.ListAssetInfo(ctx, ids...)
		requireValidationNaming(t, name, err, "asset_ids")
		if got := sent(); len(got) != 0 {
			t.Fatalf("%s: sent %v, want nothing", name, got)
		}
	}
	for name, ids := range map[string][]uint32{
		"bounds":   {1, 2147483647},
		"none":     nil,
		"1000 ids": many[:1000],
	} {
		_, _ = client.MarketData.ListAssetInfo(ctx, ids...)
		if got := sent(); len(got) != 1 || got[0] != "ListAssetInfo" {
			t.Fatalf("%s: sent %v, want one ListAssetInfo", name, got)
		}
	}
}

// Contract chain/deposit, CreateDepositAddressRequest: `uint32 chain_id = 2
// [(buf.validate.field).uint32.gt = 0]` with no IGNORE_IF_ZERO_VALUE, so a
// create for chain 0 is refused before sending. ListDepositAddressesRequest's
// chain_id ignores zero (all chains), so listing with 0 is still sent.
func TestCreateDepositAddressRefusesChainZero(t *testing.T) {
	client, sent := newRecordingClient(t)
	ctx := context.Background()
	_, err := client.Deposit.CreateAddress(ctx, 0, nil, nil)
	requireValidationNaming(t, "create chain 0", err, "chain_id")
	if got := sent(); len(got) != 0 {
		t.Fatalf("create chain 0 sent %v, want nothing", got)
	}
	_, _ = client.Deposit.CreateAddress(ctx, 1, nil, nil)
	if got := sent(); len(got) != 1 || got[0] != "CreateDepositAddress" {
		t.Fatalf("create chain 1 sent %v, want one CreateDepositAddress", got)
	}
	_, _ = client.Deposit.ListAddresses(ctx, 0, nil, nil)
	if got := sent(); len(got) != 1 || got[0] != "ListDepositAddresses" {
		t.Fatalf("list chain 0 sent %v, want one ListDepositAddresses", got)
	}
}
