package hardening_test

// A client built from a zero Config loads its catalogs, as the Python and
// Rust SDKs do, so market reads name their symbols. Only
// Config.SkipCatalogHydration leaves them unloaded.

import (
	"context"
	"net/http"
	"testing"
	"time"

	"connectrpc.com/connect"
	polyester "github.com/Fabric-Labs/polyester-sdk-go"
	"github.com/Fabric-Labs/polyester-sdk-go/gen/chain/zipper/v1/chainzipperv1connect"
	"github.com/Fabric-Labs/polyester-sdk-go/gen/marketdata/v1/marketdatav1connect"
	marketoverviewv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/marketoverview/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/gen/marketoverview/v1/marketoverviewv1connect"
	"github.com/Fabric-Labs/polyester-sdk-go/hardening"
)

type overviewStub struct {
	marketoverviewv1connect.UnimplementedMarketOverviewServiceHandler
}

func (overviewStub) ListMarketOverview(context.Context, *connect.Request[marketoverviewv1.ListMarketOverviewRequest]) (*connect.Response[marketoverviewv1.ListMarketOverviewResponse], error) {
	return connect.NewResponse(&marketoverviewv1.ListMarketOverviewResponse{
		Markets: []*marketoverviewv1.MarketOverview{{SymbolId: hydrationSymbolID, LastPriceTicks: 100_000_000}},
	}), nil
}

func TestZeroConfigHydratesCatalogsForMarketReads(t *testing.T) {
	stub := newHydrationStub()
	stub.release(false)
	mux := http.NewServeMux()
	path, handler := marketdatav1connect.NewMarketDataServiceHandler(stub)
	mux.Handle(path+"GetSpotConfig", handler)
	zpath, zhandler := chainzipperv1connect.NewZipperServiceHandler(stub)
	mux.Handle(zpath+"GetDepositWithdrawConfig", zhandler)
	mux.Handle(marketoverviewv1connect.NewMarketOverviewServiceHandler(overviewStub{}))
	srv := hardening.NewTestServer(mux)
	t.Cleanup(srv.Close)

	client, err := polyester.New(polyester.Config{APIURL: srv.URL, WSURL: "ws://127.0.0.1:1/unused", Timeout: 5 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	list, err := client.MarketOverview.List(ctx, nil, 10, "", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(list.Markets) != 1 || list.Markets[0].Symbol != hydrationSymbol {
		t.Fatalf("market overview rows = %+v, want one row named %s", list.Markets, hydrationSymbol)
	}
	if err := client.WaitForCatalogs(ctx); err != nil {
		t.Fatalf("WaitForCatalogs: %v", err)
	}
}
