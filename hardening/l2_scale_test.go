package hardening_test

import (
	"context"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"connectrpc.com/connect"
	polyester "github.com/Fabric-Labs/polyester-sdk-go"
	"github.com/Fabric-Labs/polyester-sdk-go/auth"
	"github.com/Fabric-Labs/polyester-sdk-go/catalogs"
	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	zipperv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/chain/zipper/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/gen/chain/zipper/v1/chainzipperv1connect"
	marketdatav1 "github.com/Fabric-Labs/polyester-sdk-go/gen/marketdata/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/gen/marketdata/v1/marketdatav1connect"
	orderv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/orders/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/gen/orders/v1/ordersv1connect"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

func TestL2ScaleFormatPublicAPIsRejectPanicBoundary(t *testing.T) {
	if _, err := codecs.FormatQtyScaled(1, 18); err != nil {
		t.Fatal(err)
	}
	if _, err := codecs.FormatQtyScaled(1, codecs.MaxProtocolScale); err != nil {
		t.Fatal(err)
	}
	for _, scale := range []int{37, 65534, 65535, 65536, math.MaxInt32} {
		if _, err := codecs.FormatQtyScaled(1, scale); err == nil {
			t.Fatalf("FormatQtyScaled(%d) must err", scale)
		}
		if _, err := codecs.FormatLedgerU128("1", scale); err == nil {
			t.Fatalf("FormatLedgerU128(%d) must err", scale)
		}
		q := models.MustQtyScaled(1).WithScale(8)
		q = q.WithScale(scale)
		if _, err := q.Format(); err == nil {
			t.Fatalf("QtyScaled.Format(%d) must err", scale)
		}
	}

	m := catalogs.NewManager()
	err := m.HydrateSpotConfig(map[string]any{
		"pairs": []any{
			map[string]any{
				"symbol":              "BTC-USDT",
				"symbol_id":           float64(1),
				"base_quantity_scale": float64(65535),
			},
		},
	})
	if err == nil {
		t.Fatal("catalog must reject panic-boundary scale")
	}
	if scale, ok := m.BaseQuantityScaleForSymbol("BTC-USDT"); ok {
		t.Fatalf("must not store scale, got %d", scale)
	}
}

func TestL2ScaleHydrateWaitForCatalogsThenFormatAndOrderPath(t *testing.T) {
	var createCalls atomic.Int64
	mux := http.NewServeMux()
	spotPath, spotH := marketdatav1connect.NewMarketDataServiceHandler(&spotHandler{fn: func(context.Context, *connect.Request[marketdatav1.GetSpotConfigRequest]) (*connect.Response[marketdatav1.GetSpotConfigResponse], error) {
		return connect.NewResponse(&marketdatav1.GetSpotConfigResponse{
			Pairs: []*marketdatav1.PairConfig{{
				Symbol:            "ETH-USDT",
				SymbolId:          2,
				BaseQuantityScale: 8,
			}},
		}), nil
	}})
	mux.Handle(spotPath, spotH)
	zPath, zH := chainzipperv1connect.NewZipperServiceHandler(&zipperHandler{fn: func(context.Context, *connect.Request[zipperv1.GetDepositWithdrawConfigRequest]) (*connect.Response[zipperv1.GetDepositWithdrawConfigResponse], error) {
		return connect.NewResponse(&zipperv1.GetDepositWithdrawConfigResponse{}), nil
	}})
	mux.Handle(zPath, zH)
	writePath, writeH := ordersv1connect.NewOrdersServiceHandler(&ordersWriteCapture{calls: &createCalls})
	mux.Handle(writePath, writeH)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	kp := auth.GenerateEd25519Keypair()
	client, err := polyester.New(polyester.Config{
		APIURL:          srv.URL,
		APIKeyID:        "ak_test",
		APIPrivateKey:   kp.SecretKeyHex,
		HydrateCatalogs: true,
		Timeout:         5 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })

	if err := client.WaitForCatalogs(context.Background()); err != nil {
		t.Fatalf("WaitForCatalogs: %v", err)
	}
	scale, ok := client.Catalogs.BaseQuantityScaleForSymbol("ETH-USDT")
	if !ok || scale != 8 {
		t.Fatalf("hydrated scale=%d ok=%v want 8", scale, ok)
	}
	formatted, err := codecs.FormatQtyScaled(123456789, scale)
	if err != nil {
		t.Fatal(err)
	}
	if formatted != "1.23456789" {
		t.Fatalf("format=%q", formatted)
	}

	symbol := "ETH-USDT"
	price := models.PriceFromTicksInt(1)
	_, err = client.Orders.Create(context.Background(), models.CreateOrderRequest{
		Symbol:    &symbol,
		Side:      "BUY",
		OrderType: "LIMIT",
		Qty:       models.QtyFromScaledInt(100),
		Price:     &price,
	}, nil)
	if err != nil {
		// Order path must reach scale resolution + proto encode; mock may reject
		// incomplete request fields, but must not fail on catalog/scale.
		if strings.Contains(strings.ToLower(err.Error()), "scale") ||
			strings.Contains(strings.ToLower(err.Error()), "catalog") {
			t.Fatalf("order path failed on scale/catalog: %v", err)
		}
	}
	if createCalls.Load() < 1 {
		t.Fatalf("CreateOrder must cross Connect/HTTP, calls=%d", createCalls.Load())
	}
}

type ordersWriteCapture struct {
	ordersv1connect.UnimplementedOrdersServiceHandler
	calls *atomic.Int64
}

func (h *ordersWriteCapture) CreateOrder(context.Context, *connect.Request[orderv1.CreateOrderRequest]) (*connect.Response[orderv1.CreateOrderResponse], error) {
	h.calls.Add(1)
	return connect.NewResponse(&orderv1.CreateOrderResponse{
		OrderId: 1,
	}), nil
}
