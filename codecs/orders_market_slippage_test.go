package codecs

import (
	"errors"
	"strings"
	"testing"

	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

func int32Ptr(v int32) *int32 { return &v }

func marketCreateRequest(t *testing.T) models.CreateOrderRequest {
	t.Helper()
	symbol := "BTC-USDT"
	sid := uint32(1)
	return models.CreateOrderRequest{
		Symbol:    &symbol,
		SymbolID:  &sid,
		Side:      "buy",
		OrderType: "market",
		Qty:       models.QtyFromScaledInt(1_000_000),
	}
}

func TestMarketIocOmitsSlippageWhenUnset(t *testing.T) {
	proto, err := CreateOrderToProto(marketCreateRequest(t), 8, 0)
	if err != nil {
		t.Fatal(err)
	}
	market := proto.GetOrder().GetMarketIoc()
	if market == nil {
		t.Fatal("market_ioc missing")
	}
	if market.GetMaxSlippage() != nil {
		t.Fatalf("max_slippage=%+v", market.GetMaxSlippage())
	}
}

func TestMarketIocSerializesMaxSlippageBps(t *testing.T) {
	req := marketCreateRequest(t)
	req.MaxSlippageBps = int32Ptr(25)
	proto, err := CreateOrderToProto(req, 8, 0)
	if err != nil {
		t.Fatal(err)
	}
	market := proto.GetOrder().GetMarketIoc()
	if market == nil || market.GetMaxSlippageBps() != 25 {
		t.Fatalf("max_slippage_bps=%d", market.GetMaxSlippageBps())
	}
	if market.GetMaxSlippageTicks() != 0 {
		t.Fatalf("max_slippage_ticks=%d", market.GetMaxSlippageTicks())
	}
}

func TestMarketIocSerializesMaxSlippageTicks(t *testing.T) {
	req := marketCreateRequest(t)
	req.MaxSlippageTicks = int32Ptr(10)
	proto, err := CreateOrderToProto(req, 8, 0)
	if err != nil {
		t.Fatal(err)
	}
	market := proto.GetOrder().GetMarketIoc()
	if market == nil || market.GetMaxSlippageTicks() != 10 {
		t.Fatalf("max_slippage_ticks=%d", market.GetMaxSlippageTicks())
	}
}

func TestMarketIocAcceptsBpsBounds(t *testing.T) {
	for _, bps := range []int32{1, 10_000} {
		req := marketCreateRequest(t)
		req.MaxSlippageBps = int32Ptr(bps)
		proto, err := CreateOrderToProto(req, 8, 0)
		if err != nil {
			t.Fatalf("bps=%d: %v", bps, err)
		}
		if proto.GetOrder().GetMarketIoc().GetMaxSlippageBps() != bps {
			t.Fatalf("bps=%d encoded=%d", bps, proto.GetOrder().GetMarketIoc().GetMaxSlippageBps())
		}
	}
}

func TestMarketIocRejectsBpsOutOfRange(t *testing.T) {
	for _, bps := range []int32{0, 10_001} {
		req := marketCreateRequest(t)
		req.MaxSlippageBps = int32Ptr(bps)
		_, err := CreateOrderToProto(req, 8, 0)
		var ve *sdkerrors.ValidationError
		if !errors.As(err, &ve) || !strings.Contains(err.Error(), "max_slippage_bps") {
			t.Fatalf("bps=%d want ValidationError, got %v", bps, err)
		}
	}
}

func TestMarketIocRejectsBothSlippageOverrides(t *testing.T) {
	req := marketCreateRequest(t)
	req.MaxSlippageBps = int32Ptr(25)
	req.MaxSlippageTicks = int32Ptr(10)
	_, err := CreateOrderToProto(req, 8, 0)
	if err == nil || !strings.Contains(err.Error(), "at most one of max_slippage_ticks or max_slippage_bps") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestMarketIocRejectsNonPositiveTicks(t *testing.T) {
	req := marketCreateRequest(t)
	req.MaxSlippageTicks = int32Ptr(0)
	_, err := CreateOrderToProto(req, 8, 0)
	if err == nil || !strings.Contains(err.Error(), "max_slippage_ticks must be positive") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLimitOrderRejectsMarketSlippage(t *testing.T) {
	symbol := "BTC-USDT"
	sid := uint32(1)
	tif := "gtc"
	price := models.PriceFromDecimal("100")
	req := models.CreateOrderRequest{
		Symbol:         &symbol,
		SymbolID:       &sid,
		Side:           "buy",
		OrderType:      "limit",
		TIF:            &tif,
		Qty:            models.QtyFromScaledInt(1_000_000),
		Price:          &price,
		MaxSlippageBps: int32Ptr(25),
	}
	_, err := CreateOrderToProto(req, 8, 0)
	if err == nil || !strings.Contains(err.Error(), "only valid for market orders") {
		t.Fatalf("unexpected error: %v", err)
	}
}
