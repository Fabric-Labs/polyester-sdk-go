package services

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"connectrpc.com/connect"
	orders "github.com/Fabric-Labs/polyester-sdk-go/gen/orders/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

func (h *readPreservationHandler) CreateOrder(_ context.Context, r *connect.Request[orders.CreateOrderRequest]) (*connect.Response[orders.CreateOrderResponse], error) {
	h.record(r.Msg)
	qty := r.Msg.Order.GetBaseQtyScaled()
	return connect.NewResponse(&orders.CreateOrderResponse{OrderId: 42, ResolvedBaseQtyScaled: qty}), nil
}

func (h *readPreservationHandler) BatchCreateOrders(_ context.Context, r *connect.Request[orders.BatchCreateOrdersRequest]) (*connect.Response[orders.BatchCreateOrdersResponse], error) {
	h.record(r.Msg)
	return connect.NewResponse(&orders.BatchCreateOrdersResponse{}), nil
}

func TestCreateQuantityCatalogScale(t *testing.T) {
	for _, wire := range []string{"binary", "json"} {
		for _, scale := range []int{-1, 8, 6} {
			t.Run(fmt.Sprintf("%s/scale=%d", wire, scale), func(t *testing.T) {
				h := &readPreservationHandler{}
				factory, cats := newReadPreservationServices(t, h, wire)
				service := NewOrdersService(factory, cats, nil, nil, nil, nil, nil)
				qty := models.MustQtyScaled(16000)
				if scale >= 0 {
					qty = qty.WithScale(scale)
				}
				id := uint32(1)
				price := models.PriceFromDecimal("100")
				result, err := service.Create(context.Background(), models.CreateOrderRequest{
					SymbolID: &id, Side: "buy", OrderType: "limit", Price: &price,
					Qty: models.QtyFromScaled(qty),
				}, nil)
				if scale == 6 {
					if err == nil || !strings.Contains(err.Error(), "scale mismatch") {
						t.Fatalf("expected scale mismatch: %v", err)
					}
					if len(h.calls) != 0 {
						t.Fatal("mismatched quantity sent")
					}
					return
				}
				if err != nil {
					t.Fatal(err)
				}
				req := h.calls[0].(*orders.CreateOrderRequest)
				if req.Order.GetBaseQtyScaled() != 16000 || req.Order.SymbolId != 1 || req.Order.GetLimitGtc().PriceTicks != 100_000_000_000 {
					t.Fatal(req)
				}
				formatted, err := result.ResolvedBaseQty.Format()
				if err != nil || formatted != "0.00016" {
					t.Fatalf("resolved qty %s: %v", formatted, err)
				}
			})
		}
	}
}

func TestBatchCreateQuantityCatalogScalePerItem(t *testing.T) {
	for _, typed := range []bool{false, true} {
		h := &readPreservationHandler{}
		factory, cats := newReadPreservationServices(t, h, "binary")
		if err := cats.HydrateSpotConfig(map[string]any{"pairs": []any{
			map[string]any{"symbol": "BTC-USDT", "symbol_id": float64(1), "base_quantity_scale": float64(8)},
			map[string]any{"symbol": "ETH-USDT", "symbol_id": float64(2), "base_quantity_scale": float64(6)},
		}}); err != nil {
			t.Fatal(err)
		}
		service := NewOrdersService(factory, cats, nil, nil, nil, nil, nil)
		price := models.PriceFromDecimal("100")
		items := make([]models.CreateOrderRequest, 2)
		for i, scale := range []int{8, 6} {
			id := uint32(i + 1)
			qty := models.QtyFromDecimal("0.00016")
			if typed {
				qty = models.QtyFromScaled(models.MustQtyScaled(16000).WithScale(scale))
			}
			items[i] = models.CreateOrderRequest{SymbolID: &id, Side: "buy", OrderType: "limit", Price: &price, Qty: qty}
		}
		// A batch default must not override an item's explicit destination.
		symbol := "BTC-USDT"
		if _, err := service.BatchCreate(context.Background(), nil, items, nil, &symbol, nil, false); err != nil {
			t.Fatal(err)
		}
		req := h.calls[0].(*orders.BatchCreateOrdersRequest)
		expected := int64(160)
		if typed {
			expected = 16000
		}
		if req.Items[0].GetBaseQtyScaled() != 16000 || req.Items[1].GetBaseQtyScaled() != expected {
			t.Fatal(req)
		}
	}
}

func TestBatchCreateRejectsMismatchedOrUnknownScale(t *testing.T) {
	for _, id := range []uint32{1, 99} {
		h := &readPreservationHandler{}
		factory, cats := newReadPreservationServices(t, h, "binary")
		service := NewOrdersService(factory, cats, nil, nil, nil, nil, nil)
		price := models.PriceFromDecimal("100")
		_, err := service.BatchCreate(context.Background(), nil, []models.CreateOrderRequest{{
			SymbolID: &id, Side: "buy", OrderType: "limit", Price: &price,
			Qty: models.QtyFromScaled(models.MustQtyScaled(16000).WithScale(6)),
		}}, nil, nil, nil, false)
		if err == nil {
			t.Fatal("expected catalog scale validation")
		}
		if len(h.calls) != 0 {
			t.Fatal("invalid batch was sent")
		}
	}
}

func TestBatchCreateQuoteCatalogScalePerItem(t *testing.T) {
	h := &readPreservationHandler{}
	factory, cats := newReadPreservationServices(t, h, "binary")
	if err := cats.HydrateSpotConfig(map[string]any{"pairs": []any{
		map[string]any{"symbol": "BTC-USDT", "symbol_id": float64(1), "base_quantity_scale": float64(8), "quote_quantity_scale": float64(6)},
		map[string]any{"symbol": "ETH-USDT", "symbol_id": float64(2), "base_quantity_scale": float64(6), "quote_quantity_scale": float64(4)},
	}}); err != nil {
		t.Fatal(err)
	}
	service := NewOrdersService(factory, cats, nil, nil, nil, nil, nil)
	items := make([]models.CreateOrderRequest, 2)
	for i, scale := range []int{6, 4} {
		id := uint32(i + 1)
		items[i] = models.CreateOrderRequest{SymbolID: &id, Side: "buy", OrderType: "market", MaxQuoteDebitScaled: models.QtyFromQuoteScaled(16000, scale)}
	}
	if _, err := service.BatchCreate(context.Background(), nil, items, nil, nil, nil, false); err != nil {
		t.Fatal(err)
	}
	req := h.calls[0].(*orders.BatchCreateOrdersRequest)
	for _, item := range req.Items {
		if item.GetMaxQuoteDebitScaled() != 16000 {
			t.Fatal(req)
		}
	}
}
