package services

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"connectrpc.com/connect"
	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
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

func (h *readPreservationHandler) ModifyOrder(_ context.Context, r *connect.Request[orders.ModifyOrderRequest]) (*connect.Response[orders.ModifyOrderResponse], error) {
	h.record(r.Msg)
	return connect.NewResponse(&orders.ModifyOrderResponse{OldOrderId: 42, FinalOrderId: 42, ActionTaken: 1}), nil
}
func (h *readPreservationHandler) BatchReplaceOrders(_ context.Context, r *connect.Request[orders.BatchReplaceOrdersRequest]) (*connect.Response[orders.BatchReplaceOrdersResponse], error) {
	h.record(r.Msg)
	return connect.NewResponse(&orders.BatchReplaceOrdersResponse{BatchRequestId: 42, Status: 1}), nil
}
func TestOrderWritesValidatePairIdentity(t *testing.T) {
	for _, operation := range []string{"create_id", "create_name", "batch_create", "modify", "batch_replace"} {
		for _, tag := range []string{"id", "symbol", "matching"} {
			t.Run(operation+"/"+tag, func(t *testing.T) {
				h := &readPreservationHandler{}
				factory, cats := newReadPreservationServices(t, h, "binary")
				service := NewOrdersService(factory, cats, nil, nil, nil, nil, nil)
				qty := models.MustQtyScaled(16000).WithSymbolID(2)
				if tag == "symbol" {
					qty = models.MustQtyScaled(16000).WithSymbol("ETH-USDT")
				}
				if tag == "matching" {
					qty = models.MustQtyScaled(16000).WithSymbolID(1).WithSymbol("BTC-USDT")
				}
				input := models.QtyFromScaled(qty)
				price := models.PriceFromDecimal("100")
				id := uint32(1)
				symbol := "BTC-USDT"
				req := models.CreateOrderRequest{SymbolID: &id, Side: "buy", OrderType: "limit", Qty: input, Price: &price}
				var err error
				switch operation {
				case "create_id":
					_, err = service.Create(context.Background(), req, nil)
				case "create_name":
					req.SymbolID = nil
					req.Symbol = &symbol
					_, err = service.Create(context.Background(), req, nil)
				case "batch_create":
					_, err = service.BatchCreate(context.Background(), nil, []models.CreateOrderRequest{req}, nil, nil, nil, false)
				case "modify":
					_, err = service.ModifyWith(context.Background(), nil, ModifyParams{Symbol: symbol, Key: models.OrderKeyByID("j"), NewQty: &input})
				case "batch_replace":
					_, err = service.BatchReplace(context.Background(), nil, []models.BatchReplaceItem{{Key: models.OrderKeyByID("j"), NewQty: &input}}, symbol, nil, nil)
				}
				if tag == "matching" {
					if err != nil {
						t.Fatal(err)
					}
					if len(h.calls) != 1 {
						t.Fatal("missing matching write")
					}
				} else {
					if err == nil || !strings.Contains(err.Error(), "mismatch") {
						t.Fatalf("expected pair mismatch, got %v", err)
					}
					if len(h.calls) != 0 {
						t.Fatal("mismatched pair sent")
					}
				}
			})
		}
	}
}
func TestCreateRefusesConflictingPairSelectors(t *testing.T) {
	for _, batch := range []bool{false, true} {
		h := &readPreservationHandler{}
		factory, cats := newReadPreservationServices(t, h, "binary")
		service := NewOrdersService(factory, cats, nil, nil, nil, nil, nil)
		id := uint32(1)
		symbol := "ETH-USDT"
		price := models.PriceFromDecimal("100")
		req := models.CreateOrderRequest{SymbolID: &id, Symbol: &symbol, Side: "buy", OrderType: "limit", Qty: models.QtyFromDecimal("0.00016"), Price: &price}
		var err error
		if batch {
			_, err = service.BatchCreate(context.Background(), nil, []models.CreateOrderRequest{req}, nil, nil, nil, false)
		} else {
			_, err = service.Create(context.Background(), req, nil)
		}
		if err == nil || !strings.Contains(err.Error(), "mismatch") {
			t.Fatalf("expected pair mismatch, got %v", err)
		}
		if len(h.calls) != 0 {
			t.Fatal("conflicting pair sent")
		}
	}
}

func TestTriggerQuantityIdentity(t *testing.T) {
	for _, tag := range []string{"id", "symbol", "matching"} {
		t.Run(tag, func(t *testing.T) {
			factory, requests := requestCaptureFactory(t)
			h := &readPreservationHandler{}
			_, cats := newReadPreservationServices(t, h, "binary")
			service := NewTriggersService(factory, cats, nil, nil, nil, nil, nil)
			qty := models.MustQtyScaled(16000).WithSymbolID(2)
			if tag == "symbol" {
				qty = models.MustQtyScaled(16000).WithSymbol("ETH-USDT")
			}
			if tag == "matching" {
				qty = models.MustQtyScaled(16000).WithSymbolID(1).WithSymbol("BTC-USDT")
			}
			price := models.PriceFromDecimal("100")
			_, err := service.Create(context.Background(), nil, "BTC-USDT", "stop_loss", &price, "sell", models.QtyFromScaled(qty), "market", nil, "last", "", nil, nil, false, codecs.CreateTriggerOptions{})
			if tag == "matching" {
				if err != nil {
					t.Fatal(err)
				}
				if len(requests) != 1 {
					t.Fatal("missing matching write")
				}
			} else {
				if err == nil || !strings.Contains(err.Error(), "mismatch") {
					t.Fatalf("expected pair mismatch, got %v", err)
				}
				if len(requests) != 0 {
					t.Fatal("mismatched trigger sent")
				}
			}
		})
	}
}
func TestQuoteBudgetIdentity(t *testing.T) {
	for _, tag := range []string{"id", "symbol", "matching"} {
		for _, batch := range []bool{false, true} {
			h := &readPreservationHandler{}
			factory, cats := newReadPreservationServices(t, h, "binary")
			service := NewOrdersService(factory, cats, nil, nil, nil, nil, nil)
			budget := models.MustQtyQuoteScaled(16000, 6).WithSymbolID(2)
			if tag == "symbol" {
				budget = models.MustQtyQuoteScaled(16000, 6).WithSymbol("ETH-USDT")
			}
			if tag == "matching" {
				budget = models.MustQtyQuoteScaled(16000, 6).WithSymbolID(1).WithSymbol("BTC-USDT")
			}
			id := uint32(1)
			req := models.CreateOrderRequest{SymbolID: &id, Side: "buy", OrderType: "market", MaxQuoteDebitScaled: models.QtyFromScaled(budget)}
			var err error
			if batch {
				_, err = service.BatchCreate(context.Background(), nil, []models.CreateOrderRequest{req}, nil, nil, nil, false)
			} else {
				_, err = service.Create(context.Background(), req, nil)
			}
			if tag == "matching" {
				if err != nil {
					t.Fatal(err)
				}
				if len(h.calls) != 1 {
					t.Fatal("missing matching write")
				}
			} else {
				if err == nil || !strings.Contains(err.Error(), "mismatch") {
					t.Fatalf("expected pair mismatch, got %v", err)
				}
				if len(h.calls) != 0 {
					t.Fatal("mismatched quote budget sent")
				}
			}
		}
	}
}
