package hardening_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"connectrpc.com/connect"
	polyester "github.com/Fabric-Labs/polyester-sdk-go"
	"github.com/Fabric-Labs/polyester-sdk-go/auth"
	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	"github.com/Fabric-Labs/polyester-sdk-go/codecs/decode"
	ledgerrdv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/ledger/read/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/gen/ledger/read/v1/ledgerrdv1connect"
	typev1 "github.com/Fabric-Labs/polyester-sdk-go/gen/polyester/type/v1"
)

// 1.5 * 10^18 as ledger scaled integer (already scaled; must not be multiplied again).
const onePointFiveE18Lo = uint64(1_500_000_000_000_000_000)

type balancesFixture struct {
	ledgerrdv1connect.UnimplementedLedgerReadServiceHandler
}

func (balancesFixture) GetBalances(context.Context, *connect.Request[ledgerrdv1.GetBalancesRequest]) (*connect.Response[ledgerrdv1.GetBalancesResponse], error) {
	return connect.NewResponse(&ledgerrdv1.GetBalancesResponse{
		Balances: []*ledgerrdv1.AssetBalance{{
			AssetId: 1,
			Trading: &typev1.U128{Lo: onePointFiveE18Lo},
			Funding: &typev1.U128{Lo: onePointFiveE18Lo},
		}},
	}), nil
}

func TestM5NamedBalanceDecodeNoDoubleScale(t *testing.T) {
	// L1: decode vector
	msg := &ledgerrdv1.AssetBalance{
		AssetId: 1,
		Trading: &typev1.U128{Lo: onePointFiveE18Lo},
		Funding: &typev1.U128{Lo: onePointFiveE18Lo},
	}
	row := decode.AssetBalanceFromProto(msg)
	if row.Trading != "1500000000000000000" {
		t.Fatalf("trading wire=%q want raw scaled integer string", row.Trading)
	}
	formatted, err := codecs.FormatLedgerU128(row.Trading, codecs.LedgerScale)
	if err != nil {
		t.Fatal(err)
	}
	if formatted != "1.5" {
		t.Fatalf("format=%q want 1.5 (double-scale would be tiny or huge)", formatted)
	}

	// L2: public Balances.List through Connect fixture
	mux := http.NewServeMux()
	path, h := ledgerrdv1connect.NewLedgerReadServiceHandler(balancesFixture{})
	mux.Handle(path, h)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	kp := auth.GenerateEd25519Keypair()
	client, err := polyester.New(polyester.Config{
		APIURL:          srv.URL,
		APIKeyID:        "ak_test",
		APIPrivateKey:   kp.SecretKeyHex,
		HydrateCatalogs: false,
		Timeout:         5 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })

	list, err := client.Balances.List(context.Background(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(list.Balances) != 1 {
		t.Fatalf("balances=%+v", list.Balances)
	}
	got := list.Balances[0]
	if got.Trading != "1500000000000000000" || got.Funding != "1500000000000000000" {
		t.Fatalf("decoded trading/funding=%q/%q (double-scale bug)", got.Trading, got.Funding)
	}
	formatted, err = codecs.FormatLedgerU128(got.Trading, 18)
	if err != nil || formatted != "1.5" {
		t.Fatalf("format=%q err=%v", formatted, err)
	}
}
