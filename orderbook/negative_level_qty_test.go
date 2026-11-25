package orderbook_test

import (
	stderrors "errors"
	"testing"

	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	orderbookv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/orderbook/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/orderbook"
)

// A snapshot level with a negative quantity is a contract violation naming
// the field; an omitted (0) quantity stays an invalid-or-missing level.
func TestLevelsFromProtoLevelsRefusesANegativeQuantity(t *testing.T) {
	levels := []*orderbookv1.PriceLevel{{PriceTicks: 100, QtyScaled: 5}, {PriceTicks: 101, QtyScaled: -2}}
	_, err := orderbook.LevelsFromProtoLevels(levels)
	var contract *sdkerrors.ResponseContractError
	if !stderrors.As(err, &contract) || contract.Operation != "PriceLevel.qty_scaled" || contract.Msg != "negative quantity -2" {
		t.Fatalf("err = %v, want the PriceLevel.qty_scaled contract error", err)
	}

	_, err = orderbook.LevelsFromProtoLevels([]*orderbookv1.PriceLevel{{PriceTicks: 100}})
	var validation *sdkerrors.ValidationError
	if !stderrors.As(err, &validation) {
		t.Fatalf("omitted quantity: err = %v, want *ValidationError", err)
	}
}
