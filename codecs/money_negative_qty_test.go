package codecs_test

import (
	stderrors "errors"
	"testing"

	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

func TestDecodeQtyScaledReadsZeroAndRefusesNegative(t *testing.T) {
	sid := uint32(3)
	zero, err := codecs.DecodeQtyScaled(0, 8, "BTC-USDT", &sid, "T.qty_scaled")
	if err != nil {
		t.Fatal(err)
	}
	if zero.Scaled() != 0 || zero.Domain() != models.QuantityDomainOrderBase || *zero.Scale() != 8 || *zero.SymbolID() != 3 {
		t.Fatalf("zero = %+v", zero)
	}

	for _, scaled := range []int64{-1, -9_223_372_036_854_775_808} {
		q, err := codecs.DecodeQtyScaled(scaled, 8, "BTC-USDT", &sid, "T.qty_scaled")
		var contract *sdkerrors.ResponseContractError
		if !stderrors.As(err, &contract) {
			t.Fatalf("%d: err = %v, want *ResponseContractError", scaled, err)
		}
		if contract.Operation != "T.qty_scaled" || q != (models.QtyScaled{}) {
			t.Fatalf("%d: err = %+v, qty = %+v", scaled, contract, q)
		}
		if !stderrors.Is(err, sdkerrors.ErrPolyester) {
			t.Fatalf("%d: %v is not ErrPolyester", scaled, err)
		}
	}
}
