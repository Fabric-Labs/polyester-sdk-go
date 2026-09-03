package hardening_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"connectrpc.com/connect"
	polyester "github.com/Fabric-Labs/polyester-sdk-go"
	"github.com/Fabric-Labs/polyester-sdk-go/auth"
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	chaindepositv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/chain/deposit/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/gen/chain/deposit/v1/chaindepositv1connect"
	lifecyclev1 "github.com/Fabric-Labs/polyester-sdk-go/gen/chain/lifecycle/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/gen/chain/lifecycle/v1/chainlifecyclev1connect"
	zipperv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/chain/zipper/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/gen/chain/zipper/v1/chainzipperv1connect"
	marketdatav1 "github.com/Fabric-Labs/polyester-sdk-go/gen/marketdata/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/gen/marketdata/v1/marketdatav1connect"
	orderv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/orders/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/gen/orders/v1/ordersv1connect"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

type inconsistentBatchCancelHandler struct {
	ordersv1connect.UnimplementedOrdersServiceHandler
}

func (*inconsistentBatchCancelHandler) BatchCancelOrders(context.Context, *connect.Request[orderv1.BatchCancelOrdersRequest]) (*connect.Response[orderv1.BatchCancelOrdersResponse], error) {
	return connect.NewResponse(&orderv1.BatchCancelOrdersResponse{
		Results:       []*orderv1.BatchCancelResultItem{{Status: orderv1.BatchCancelResultItem_ACCEPTED, OrderId: 9}},
		AcceptedCount: 0,
		RejectedCount: 1,
	}), nil
}

func (*inconsistentBatchCancelHandler) BatchReplaceOrders(context.Context, *connect.Request[orderv1.BatchReplaceOrdersRequest]) (*connect.Response[orderv1.BatchReplaceOrdersResponse], error) {
	return connect.NewResponse(&orderv1.BatchReplaceOrdersResponse{
		BatchRequestId: 11,
		Status:         orderv1.BatchReplaceAdmissionStatus_BATCH_REPLACE_ADMISSION_STATUS_PARTIALLY_ADMITTED,
		Results: []*orderv1.BatchReplaceAdmissionItem{{
			ItemIndex:          0,
			Status:             orderv1.BatchReplaceItemAdmissionStatus_BATCH_REPLACE_ITEM_ADMISSION_STATUS_ADMITTED,
			OldOrderId:         9,
			ReplacementOrderId: 10,
		}},
		RejectedCount: 1,
	}), nil
}

func TestL2BatchCancelRejectsInconsistentCountsThroughPublicService(t *testing.T) {
	path, handler := ordersv1connect.NewOrdersServiceHandler(&inconsistentBatchCancelHandler{})
	mux := http.NewServeMux()
	mux.Handle(path, handler)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	kp := auth.GenerateEd25519Keypair()
	client, err := polyester.New(polyester.Config{
		APIURL:          srv.URL,
		APIKeyID:        "ak_test",
		APIPrivateKey:   kp.SecretKeyHex,
		HydrateCatalogs: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })

	orderID := "9"
	_, err = client.Orders.BatchCancel(
		context.Background(),
		nil,
		[]models.BatchCancelItem{{Key: models.OrderKeyByID(orderID)}},
		nil,
		nil,
	)
	var contractErr *sdkerrors.ResponseContractError
	if !errors.As(err, &contractErr) {
		t.Fatalf("expected ResponseContractError, got %T: %v", err, err)
	}
}

type misalignedCandlesHandler struct {
	marketdatav1connect.UnimplementedMarketDataServiceHandler
}

func (*misalignedCandlesHandler) GetSpotConfig(context.Context, *connect.Request[marketdatav1.GetSpotConfigRequest]) (*connect.Response[marketdatav1.GetSpotConfigResponse], error) {
	return connect.NewResponse(&marketdatav1.GetSpotConfigResponse{
		Pairs: []*marketdatav1.PairConfig{{
			Symbol:            "BTC-USDT",
			SymbolId:          1,
			BaseQuantityScale: 8,
		}},
	}), nil
}

func (*misalignedCandlesHandler) GetCandlesColumns(context.Context, *connect.Request[marketdatav1.GetCandlesColumnsRequest]) (*connect.Response[marketdatav1.GetCandlesColumnsResponse], error) {
	return connect.NewResponse(&marketdatav1.GetCandlesColumnsResponse{
		SymbolId:  1,
		Timeframe: marketdatav1.Timeframe_MIN_1,
		TsSec:     []uint64{10, 20},
		Open:      []int64{1, 2},
		High:      []int64{1},
		Low:       []int64{1, 2},
		Close:     []int64{1, 2},
		Volume:    []int64{1, 2},
	}), nil
}

type validZipperCatalogHandler struct {
	chainzipperv1connect.UnimplementedZipperServiceHandler
}

func (*validZipperCatalogHandler) GetDepositWithdrawConfig(context.Context, *connect.Request[zipperv1.GetDepositWithdrawConfigRequest]) (*connect.Response[zipperv1.GetDepositWithdrawConfigResponse], error) {
	return connect.NewResponse(&zipperv1.GetDepositWithdrawConfigResponse{
		Assets: []*zipperv1.AssetConfig{{
			Asset:         "USDT",
			LedgerId:      99,
			QuantityScale: 6,
		}},
	}), nil
}

func TestL2ColumnarCandlesRejectMisalignedColumnsThroughPublicService(t *testing.T) {
	mux := http.NewServeMux()
	path, handler := marketdatav1connect.NewMarketDataServiceHandler(&misalignedCandlesHandler{})
	mux.Handle(path, handler)
	zpath, zhandler := chainzipperv1connect.NewZipperServiceHandler(&validZipperCatalogHandler{})
	mux.Handle(zpath, zhandler)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	client, err := polyester.New(polyester.Config{
		APIURL:          srv.URL,
		HydrateCatalogs: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	if err := client.WaitForCatalogs(context.Background()); err != nil {
		t.Fatal(err)
	}

	symbol := "BTC-USDT"
	_, err = client.MarketData.GetCandlesColumns(
		context.Background(), &symbol, nil, "1m", 100, nil, nil, false, nil,
	)
	var transportErr *sdkerrors.TransportError
	if !errors.As(err, &transportErr) {
		t.Fatalf("expected TransportError, got %T: %v", err, err)
	}
}

func TestL2BatchReplaceRejectsInconsistentCountsThroughPublicService(t *testing.T) {
	mux := http.NewServeMux()
	path, handler := marketdatav1connect.NewMarketDataServiceHandler(&misalignedCandlesHandler{})
	mux.Handle(path, handler)
	zpath, zhandler := chainzipperv1connect.NewZipperServiceHandler(&validZipperCatalogHandler{})
	mux.Handle(zpath, zhandler)
	opath, ohandler := ordersv1connect.NewOrdersServiceHandler(&inconsistentBatchCancelHandler{})
	mux.Handle(opath, ohandler)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	kp := auth.GenerateEd25519Keypair()
	client, err := polyester.New(polyester.Config{
		APIURL:          srv.URL,
		APIKeyID:        "ak_test",
		APIPrivateKey:   kp.SecretKeyHex,
		HydrateCatalogs: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	if err := client.WaitForCatalogs(context.Background()); err != nil {
		t.Fatal(err)
	}

	orderID := "9"
	price := models.PriceFromDecimal("1")
	_, err = client.Orders.BatchReplace(
		context.Background(),
		nil,
		[]models.BatchReplaceItem{{Key: models.OrderKeyByID(orderID), NewPrice: &price}},
		"BTC-USDT",
		nil,
		nil,
	)
	var contractErr *sdkerrors.ResponseContractError
	if !errors.As(err, &contractErr) {
		t.Fatalf("expected ResponseContractError, got %T: %v", err, err)
	}
}

type missingDepositAddressHandler struct {
	chaindepositv1connect.UnimplementedDepositAddressServiceHandler
}

func (*missingDepositAddressHandler) CreateDepositAddress(context.Context, *connect.Request[chaindepositv1.CreateDepositAddressRequest]) (*connect.Response[chaindepositv1.CreateDepositAddressResponse], error) {
	return connect.NewResponse(&chaindepositv1.CreateDepositAddressResponse{}), nil
}

func TestL2CreateDepositAddressRejectsMissingEntityThroughPublicService(t *testing.T) {
	path, handler := chaindepositv1connect.NewDepositAddressServiceHandler(&missingDepositAddressHandler{})
	mux := http.NewServeMux()
	mux.Handle(path, handler)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	kp := auth.GenerateEd25519Keypair()
	client, err := polyester.New(polyester.Config{
		APIURL:          srv.URL,
		APIKeyID:        "ak_test",
		APIPrivateKey:   kp.SecretKeyHex,
		HydrateCatalogs: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })

	_, err = client.Deposit.CreateAddress(context.Background(), 1, nil, nil)
	var transportErr *sdkerrors.TransportError
	if !errors.As(err, &transportErr) {
		t.Fatalf("expected TransportError, got %T: %v", err, err)
	}
}

type lifecycleFlowsHandler struct {
	chainlifecyclev1connect.UnimplementedLifecycleReadServiceHandler
	lastLimit uint32
}

func (h *lifecycleFlowsHandler) ListFlowsByTx(_ context.Context, req *connect.Request[lifecyclev1.ListFlowsByTxRequest]) (*connect.Response[lifecyclev1.ListFlowsByTxResponse], error) {
	h.lastLimit = req.Msg.GetLimit()
	return connect.NewResponse(&lifecyclev1.ListFlowsByTxResponse{
		Matches: []*lifecyclev1.FlowTxMatchView{
			{FlowId: "flow-a"},
			{FlowId: "flow-b"},
		},
	}), nil
}

func TestL2GetFlowByTxReturnsAllMatchesThroughPublicService(t *testing.T) {
	service := &lifecycleFlowsHandler{}
	path, handler := chainlifecyclev1connect.NewLifecycleReadServiceHandler(service)
	mux := http.NewServeMux()
	mux.Handle(path, handler)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	client, err := polyester.New(polyester.Config{
		APIURL:          srv.URL,
		HydrateCatalogs: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })

	result, err := client.Lifecycle.GetFlowByTx(context.Background(), "0x01", "any", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Flows) != 2 || result.Flows[0].IntentID != "flow-a" || result.Flows[1].IntentID != "flow-b" {
		t.Fatalf("result=%+v", result)
	}
	if service.lastLimit != 50 {
		t.Fatalf("default limit=%d, want 50", service.lastLimit)
	}
}
