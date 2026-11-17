package codecs

import (
	stderrors "errors"
	"testing"

	"github.com/Fabric-Labs/polyester-sdk-go/errors"
	orderv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/orders/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
	"google.golang.org/protobuf/proto"
)

// isClearedRisk reports whether risk is the empty RiskPolicy the API reads as
// "remove the order's attached risk".
func isClearedRisk(risk *orderv1.RiskPolicy) bool {
	return risk != nil && proto.Equal(risk, &orderv1.RiskPolicy{})
}

// Clearing alone, or with a price or quantity change, sends an empty
// new_attached_risk, as the TypeScript SDK's clearRisk does.
func TestModifyClearAttachedRiskSendsEmptyRiskPolicy(t *testing.T) {
	price := models.PriceFromDecimal("185.25")
	for name, newPrice := range map[string]*models.PriceInput{"alone": nil, "with a new price": &price} {
		req, err := ModifyOrderToProto("SOL-USDT", 3, models.OrderKeyByID(FormatID(100)), nil, ptr("mod-1"),
			newPrice, nil, nil, true, nil, nil, 6)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if !isClearedRisk(req.NewAttachedRisk) {
			t.Fatalf("%s: new_attached_risk = %v, want an empty RiskPolicy", name, req.NewAttachedRisk)
		}
		if (newPrice != nil) != (req.NewPriceTicks != nil) {
			t.Fatalf("%s: new_price_ticks = %v", name, req.NewPriceTicks)
		}
	}
}

func TestBatchReplaceClearAttachedRiskSendsEmptyRiskPolicy(t *testing.T) {
	price := models.PriceFromDecimal("83589")
	req, err := BatchReplaceOrdersToProto([]models.BatchReplaceItem{
		{Key: models.OrderKeyByClientID("cid-1"), ClearAttachedRisk: true},
		{Key: models.OrderKeyByClientID("cid-2"), NewPrice: &price, ClearAttachedRisk: true},
	}, 1, nil, ptr("batch-1"), 8)
	if err != nil {
		t.Fatal(err)
	}
	for i, item := range req.GetItems() {
		if !isClearedRisk(item.NewAttachedRisk) {
			t.Fatalf("item %d: new_attached_risk = %v, want an empty RiskPolicy", i, item.NewAttachedRisk)
		}
	}
}

// Clearing and setting new attached risk together is refused before sending.
func TestClearAttachedRiskWithNewRiskIsRefused(t *testing.T) {
	var validation *errors.ValidationError
	_, err := ModifyOrderToProto("SOL-USDT", 3, models.OrderKeyByID(FormatID(100)), nil, nil,
		nil, nil, attachedOCO("185.25", "140.5"), true, nil, nil, 6)
	if !stderrors.As(err, &validation) {
		t.Fatalf("modify: err = %v, want a validation error", err)
	}
	_, err = BatchReplaceOrdersToProto([]models.BatchReplaceItem{{
		Key: models.OrderKeyByClientID("cid-1"), NewAttachedRisk: attachedOCO("185.25", "140.5"), ClearAttachedRisk: true,
	}}, 1, nil, nil, 8)
	if !stderrors.As(err, &validation) {
		t.Fatalf("batch replace: err = %v, want a validation error", err)
	}
}

// An empty AttachedRiskInput is still refused: it never silently becomes a
// clear or a no-op.
func TestModifyEmptyAttachedRiskIsRefused(t *testing.T) {
	var validation *errors.ValidationError
	_, err := ModifyOrderToProto("SOL-USDT", 3, models.OrderKeyByID(FormatID(100)), nil, nil,
		nil, nil, &models.AttachedRiskInput{}, false, nil, nil, 6)
	if !stderrors.As(err, &validation) {
		t.Fatalf("err = %v, want a validation error", err)
	}
}
