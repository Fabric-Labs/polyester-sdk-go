package hardening_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"connectrpc.com/connect"
	polyester "github.com/Fabric-Labs/polyester-sdk-go"
	zipperv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/chain/zipper/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/gen/chain/zipper/v1/chainzipperv1connect"
	marketdatav1 "github.com/Fabric-Labs/polyester-sdk-go/gen/marketdata/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/gen/marketdata/v1/marketdatav1connect"
	"github.com/Fabric-Labs/polyester-sdk-go/hardening"
	"github.com/Fabric-Labs/polyester-sdk-go/transport"
)

type spotHandler struct {
	marketdatav1connect.UnimplementedMarketDataServiceHandler
	fn func(context.Context, *connect.Request[marketdatav1.GetSpotConfigRequest]) (*connect.Response[marketdatav1.GetSpotConfigResponse], error)
}

func (h *spotHandler) GetSpotConfig(ctx context.Context, req *connect.Request[marketdatav1.GetSpotConfigRequest]) (*connect.Response[marketdatav1.GetSpotConfigResponse], error) {
	return h.fn(ctx, req)
}

type zipperHandler struct {
	chainzipperv1connect.UnimplementedZipperServiceHandler
	fn func(context.Context, *connect.Request[zipperv1.GetDepositWithdrawConfigRequest]) (*connect.Response[zipperv1.GetDepositWithdrawConfigResponse], error)
}

func (h *zipperHandler) GetDepositWithdrawConfig(ctx context.Context, req *connect.Request[zipperv1.GetDepositWithdrawConfigRequest]) (*connect.Response[zipperv1.GetDepositWithdrawConfigResponse], error) {
	return h.fn(ctx, req)
}

func catalogServer(t *testing.T, spot spotHandler, zipper zipperHandler) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	path, h := marketdatav1connect.NewMarketDataServiceHandler(&spot)
	mux.Handle(path, h)
	zpath, zh := chainzipperv1connect.NewZipperServiceHandler(&zipper)
	mux.Handle(zpath, zh)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func emptyZipper() zipperHandler {
	return zipperHandler{fn: func(context.Context, *connect.Request[zipperv1.GetDepositWithdrawConfigRequest]) (*connect.Response[zipperv1.GetDepositWithdrawConfigResponse], error) {
		return connect.NewResponse(&zipperv1.GetDepositWithdrawConfigResponse{}), nil
	}}
}

func TestL2WaitForCatalogsFailClosedOnHTTP500(t *testing.T) {
	var spotCalls atomic.Int64
	srv := catalogServer(t, spotHandler{fn: func(context.Context, *connect.Request[marketdatav1.GetSpotConfigRequest]) (*connect.Response[marketdatav1.GetSpotConfigResponse], error) {
		spotCalls.Add(1)
		return nil, connect.NewError(connect.CodeInternal, errors.New("nope"))
	}}, emptyZipper())

	client, err := polyester.New(polyester.Config{
		APIURL:          srv.URL,
		HydrateCatalogs: true,
		Timeout:         500 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })

	err = client.WaitForCatalogs(context.Background())
	if err == nil {
		t.Fatal("HTTP 500 must fail closed")
	}
	if !strings.Contains(err.Error(), "catalog") {
		t.Fatalf("want catalog hydration error, got %v", err)
	}
	if client.CatalogsLastError() == nil {
		t.Fatal("CatalogsLastError should be set")
	}
	if scale, ok := client.Catalogs.BaseQuantityScaleForSymbol("BTC-USDT"); ok {
		t.Fatalf("must not expose scale after failed hydrate, got %d", scale)
	}
}

func TestL2WaitForCatalogsFailClosedOnEmptyOrMalformed(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		srv := catalogServer(t, spotHandler{fn: func(context.Context, *connect.Request[marketdatav1.GetSpotConfigRequest]) (*connect.Response[marketdatav1.GetSpotConfigResponse], error) {
			return connect.NewResponse(&marketdatav1.GetSpotConfigResponse{}), nil
		}}, emptyZipper())

		client, err := polyester.New(polyester.Config{
			APIURL:          srv.URL,
			HydrateCatalogs: true,
			Timeout:         time.Second,
		})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = client.Close() })

		err = client.WaitForCatalogs(context.Background())
		if err == nil {
			t.Fatal("empty spot config must fail closed")
		}
		if !strings.Contains(err.Error(), "empty") && !strings.Contains(err.Error(), "catalog") {
			t.Fatalf("want empty/catalog error, got %v", err)
		}
	})

	t.Run("malformed_scale", func(t *testing.T) {
		srv := catalogServer(t, spotHandler{fn: func(context.Context, *connect.Request[marketdatav1.GetSpotConfigRequest]) (*connect.Response[marketdatav1.GetSpotConfigResponse], error) {
			return connect.NewResponse(&marketdatav1.GetSpotConfigResponse{
				Pairs: []*marketdatav1.PairConfig{{
					Symbol:            "BTC-USDT",
					SymbolId:          1,
					BaseQuantityScale: 65535,
				}},
			}), nil
		}}, emptyZipper())

		client, err := polyester.New(polyester.Config{
			APIURL:          srv.URL,
			HydrateCatalogs: true,
			Timeout:         time.Second,
		})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = client.Close() })

		err = client.WaitForCatalogs(context.Background())
		if err == nil {
			t.Fatal("malformed scale must fail closed")
		}
		if !strings.Contains(strings.ToLower(err.Error()), "scale") {
			t.Fatalf("want scale error, got %v", err)
		}
		if scale, ok := client.Catalogs.BaseQuantityScaleForSymbol("BTC-USDT"); ok {
			t.Fatalf("must not store rejected scale, got %d", scale)
		}
	})
}

func TestL2WaitForCatalogsRejectsBadWireResponse(t *testing.T) {
	tests := []struct {
		name string
		body []byte
	}{
		{name: "malformed_protobuf", body: []byte{0x0f}},
		{name: "oversized_protobuf", body: make([]byte, transport.MaxConnectResponseBytes+1)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.Contains(r.URL.Path, "GetSpotConfig") {
					w.Header().Set("Content-Type", "application/proto")
					w.Header().Set("Content-Length", fmt.Sprint(len(tt.body)))
					_, _ = w.Write(tt.body)
					return
				}
				http.NotFound(w, r)
			}))
			t.Cleanup(srv.Close)

			client, err := polyester.New(polyester.Config{
				APIURL:          srv.URL,
				HydrateCatalogs: true,
				Timeout:         2 * time.Second,
			})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = client.Close() })

			if err := client.WaitForCatalogs(context.Background()); err == nil {
				t.Fatalf("%s must fail closed", tt.name)
			}
			if client.CatalogsLastError() == nil {
				t.Fatal("CatalogsLastError should be set")
			}
			if _, ok := client.Catalogs.BaseQuantityScaleForSymbol("BTC-USDT"); ok {
				t.Fatal("catalogs must not expose data after bad wire response")
			}
		})
	}
}

func TestL2ConcurrentWaitForCatalogsShareOneAttempt(t *testing.T) {
	var spotCalls atomic.Int64
	started := make(chan struct{})
	release := make(chan struct{})
	srv := catalogServer(t, spotHandler{fn: func(context.Context, *connect.Request[marketdatav1.GetSpotConfigRequest]) (*connect.Response[marketdatav1.GetSpotConfigResponse], error) {
		n := spotCalls.Add(1)
		if n == 1 {
			close(started)
			<-release
		}
		return connect.NewResponse(&marketdatav1.GetSpotConfigResponse{
			Pairs: []*marketdatav1.PairConfig{{
				Symbol:            "ETH-USDT",
				SymbolId:          2,
				BaseQuantityScale: 6,
			}},
		}), nil
	}}, emptyZipper())

	client, err := polyester.New(polyester.Config{
		APIURL:          srv.URL,
		HydrateCatalogs: true,
		Timeout:         5 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })

	hardening.WaitUntil(t, func() bool {
		select {
		case <-started:
			return true
		default:
			return false
		}
	}, 2*time.Second)

	errCh := make(chan error, 8)
	for i := 0; i < 8; i++ {
		go func() {
			errCh <- client.WaitForCatalogs(context.Background())
		}()
	}
	time.Sleep(50 * time.Millisecond)
	close(release)

	for i := 0; i < 8; i++ {
		if err := <-errCh; err != nil {
			t.Fatalf("waiter %d: %v", i, err)
		}
	}
	if got := spotCalls.Load(); got != 1 {
		t.Fatalf("spot GetSpotConfig calls=%d want 1 (shared hydration attempt)", got)
	}
}

func TestL2ScaleFormatAndCatalogRejectPanicBoundary(t *testing.T) {
	// Covered jointly with codecs/catalogs unit tests; L2 path: hydrate rejects then format.
	srv := catalogServer(t, spotHandler{fn: func(context.Context, *connect.Request[marketdatav1.GetSpotConfigRequest]) (*connect.Response[marketdatav1.GetSpotConfigResponse], error) {
		return connect.NewResponse(&marketdatav1.GetSpotConfigResponse{
			Pairs: []*marketdatav1.PairConfig{{
				Symbol:            "BTC-USDT",
				SymbolId:          1,
				BaseQuantityScale: 65535,
			}},
		}), nil
	}}, emptyZipper())

	client, err := polyester.New(polyester.Config{
		APIURL:          srv.URL,
		HydrateCatalogs: true,
		Timeout:         time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })

	err = client.WaitForCatalogs(context.Background())
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "scale") {
		t.Fatalf("catalog must reject panic-boundary scale, got %v", err)
	}
	if scale, ok := client.Catalogs.BaseQuantityScaleForSymbol("BTC-USDT"); ok {
		t.Fatalf("must not store scale, got %d", scale)
	}
}
