package codecs

import (
	"errors"
	"testing"

	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

func TestCorrelationIDValidation(t *testing.T) {
	valid := " A.B_c:1/2-3 "
	got, err := optionalClientID(&valid, "client_order_id")
	if err != nil || got == nil || *got != "A.B_c:1/2-3" {
		t.Fatalf("got=%v err=%v", got, err)
	}

	for _, value := range []string{"bad id", "id!", string(make([]byte, clientIDMaxLen+1))} {
		_, err := optionalClientID(&value, "client_order_id")
		var validationErr *sdkerrors.ValidationError
		if !errors.As(err, &validationErr) {
			t.Fatalf("value %q: expected ValidationError, got %T: %v", value, err, err)
		}
	}

	tooLong := string(make([]byte, requestIDMaxLen+1))
	if _, err := coalesceRequestID(&tooLong, "test"); err == nil {
		t.Fatal("expected oversized request_id rejection")
	}
}

func TestOrderEncodersRejectInvalidCorrelationIDs(t *testing.T) {
	symbol := "BTC-USDT"
	sid := uint32(1)
	clientOrderID := "bad id"
	req := models.CreateOrderRequest{
		Symbol:        &symbol,
		SymbolID:      &sid,
		Side:          "buy",
		OrderType:     "market",
		Qty:           models.QtyFromScaledInt(1),
		ClientOrderID: &clientOrderID,
	}
	if _, err := CreateOrderToProto(req, 8, 0); err == nil {
		t.Fatal("expected invalid client_order_id rejection")
	}

	requestID := "bad request"
	if _, err := CancelAllOrdersToProto(nil, nil, nil, false, &requestID); err == nil {
		t.Fatal("expected invalid request_id rejection")
	}
}
