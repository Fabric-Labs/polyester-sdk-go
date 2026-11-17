package codecs

import (
	"errors"
	"strings"
	"testing"

	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

// A zero new price or quantity is refused, never sent: every form of it.
func TestModifyAndBatchReplaceRefuseZeroPriceAndQuantity(t *testing.T) {
	prices := map[string]models.PriceInput{
		"decimal 0":    models.PriceFromDecimal("0"),
		"decimal 0.00": models.PriceFromDecimal("0.00"),
		"ticks 0":      models.PriceFromTicksInt(0),
	}
	quantities := map[string]models.QtyInput{
		"decimal 0":   models.QtyFromDecimal("0"),
		"decimal 0.0": models.QtyFromDecimal("0.0"),
		"scaled 0":    models.QtyFromScaledInt(0),
	}
	expectRefused := func(t *testing.T, field string, err error) {
		t.Helper()
		var validation *sdkerrors.ValidationError
		if !errors.As(err, &validation) || !strings.Contains(err.Error(), field) || !strings.Contains(err.Error(), "positive") {
			t.Fatalf("err=%v, want a %s must be positive *errors.ValidationError", err, field)
		}
	}
	for name, price := range prices {
		t.Run("modify price "+name, func(t *testing.T) {
			_, err := ModifyOrderToProto("SOL-USDT", 3, models.OrderKeyByID(FormatID(100)), nil, ptr("mod-1"), &price, nil, nil, false, nil, nil, 6)
			expectRefused(t, "new_price", err)
		})
		t.Run("batch replace price "+name, func(t *testing.T) {
			_, err := BatchReplaceOrdersToProto([]models.BatchReplaceItem{{Key: models.OrderKeyByID(FormatID(100)), NewPrice: &price}}, 3, nil, ptr("br-1"), 6, "SOL-USDT")
			expectRefused(t, "new_price", err)
		})
	}
	for name, qty := range quantities {
		t.Run("modify qty "+name, func(t *testing.T) {
			_, err := ModifyOrderToProto("SOL-USDT", 3, models.OrderKeyByID(FormatID(100)), nil, ptr("mod-1"), nil, &qty, nil, false, nil, nil, 6)
			expectRefused(t, "new_qty", err)
		})
		t.Run("batch replace qty "+name, func(t *testing.T) {
			_, err := BatchReplaceOrdersToProto([]models.BatchReplaceItem{{Key: models.OrderKeyByID(FormatID(100)), NewQty: &qty}}, 3, nil, ptr("br-1"), 6, "SOL-USDT")
			expectRefused(t, "new_qty", err)
		})
	}
	// Positive values still encode.
	price, qty := models.PriceFromDecimal("0.000000001"), models.QtyFromDecimal("0.000001")
	req, err := ModifyOrderToProto("SOL-USDT", 3, models.OrderKeyByID(FormatID(100)), nil, ptr("mod-1"), &price, &qty, nil, false, nil, nil, 6)
	if err != nil || req.GetNewPriceTicks() != 1 || req.GetNewQtyScaled() != 1 {
		t.Fatalf("smallest positive modify: req=%v err=%v", req, err)
	}
	batch, err := BatchReplaceOrdersToProto([]models.BatchReplaceItem{{Key: models.OrderKeyByID(FormatID(100)), NewPrice: &price, NewQty: &qty}}, 3, nil, ptr("br-1"), 6, "SOL-USDT")
	if err != nil || batch.Items[0].GetNewPriceTicks() != 1 || batch.Items[0].GetNewQtyScaled() != 1 {
		t.Fatalf("smallest positive batch replace: req=%v err=%v", batch, err)
	}
}
