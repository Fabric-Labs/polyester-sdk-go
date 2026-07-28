package codecs

import (
	"github.com/Fabric-Labs/polyester-sdk-go/errors"
	orderv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/orders/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

func requireOrderKey(key models.OrderKey, op string) error {
	if !key.IsSet() {
		return &errors.ValidationError{Msg: op + " requires an OrderKey (OrderKeyByID or OrderKeyByClientID)"}
	}
	return nil
}

// ApplyOrderKeyToGet sets GetOrderRequest.Key from a typed OrderKey.
func ApplyOrderKeyToGet(req *orderv1.GetOrderRequest, key models.OrderKey) error {
	if err := requireOrderKey(key, "get"); err != nil {
		return err
	}
	if orderID, ok := key.OrderID(); ok {
		id, err := IDToInt(orderID, "order_id")
		if err != nil {
			return err
		}
		req.Key = &orderv1.GetOrderRequest_OrderId{OrderId: id}
		return nil
	}
	clientOrderID, _ := key.ClientOrderID()
	validated, err := ValidateClientOrderID(clientOrderID)
	if err != nil {
		return err
	}
	req.Key = &orderv1.GetOrderRequest_ClientOrderId{ClientOrderId: validated}
	return nil
}

// ApplyOrderKeyToCancel sets CancelOrderRequest.Key from a typed OrderKey.
func ApplyOrderKeyToCancel(req *orderv1.CancelOrderRequest, key models.OrderKey) error {
	if err := requireOrderKey(key, "cancel"); err != nil {
		return err
	}
	if orderID, ok := key.OrderID(); ok {
		id, err := IDToInt(orderID, "order_id")
		if err != nil {
			return err
		}
		req.Key = &orderv1.CancelOrderRequest_OrderId{OrderId: id}
		return nil
	}
	clientOrderID, _ := key.ClientOrderID()
	validated, err := ValidateClientOrderID(clientOrderID)
	if err != nil {
		return err
	}
	req.Key = &orderv1.CancelOrderRequest_ClientOrderId{ClientOrderId: validated}
	return nil
}

// ApplyOrderKeyToModify sets ModifyOrderRequest.Key from a typed OrderKey.
func ApplyOrderKeyToModify(req *orderv1.ModifyOrderRequest, key models.OrderKey) error {
	if err := requireOrderKey(key, "modify"); err != nil {
		return err
	}
	if orderID, ok := key.OrderID(); ok {
		id, err := IDToInt(orderID, "order_id")
		if err != nil {
			return err
		}
		req.Key = &orderv1.ModifyOrderRequest_OrderId{OrderId: id}
		return nil
	}
	clientOrderID, _ := key.ClientOrderID()
	validated, err := requiredClientID(clientOrderID, "client_order_id")
	if err != nil {
		return err
	}
	req.Key = &orderv1.ModifyOrderRequest_ClientOrderId{ClientOrderId: validated}
	return nil
}
