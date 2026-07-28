package models

// OrderKey identifies an order by exactly one of exchange order id or client order id.
//
// Construct with OrderKeyByID or OrderKeyByClientID. The zero value is unset and
// rejected by Get/Cancel/Modify and batch item encoders.
type OrderKey struct {
	orderID       string
	clientOrderID string
	byClient      bool
	set           bool
}

// OrderKeyByID builds a key from an exchange-assigned order id.
func OrderKeyByID(orderID string) OrderKey {
	return OrderKey{orderID: orderID, set: true}
}

// OrderKeyByClientID builds a key from a client order id.
func OrderKeyByClientID(clientOrderID string) OrderKey {
	return OrderKey{clientOrderID: clientOrderID, byClient: true, set: true}
}

// OrderID returns the exchange order id when this key was built with OrderKeyByID.
func (k OrderKey) OrderID() (string, bool) {
	if !k.set || k.byClient || k.orderID == "" {
		return "", false
	}
	return k.orderID, true
}

// ClientOrderID returns the client order id when this key was built with OrderKeyByClientID.
func (k OrderKey) ClientOrderID() (string, bool) {
	if !k.set || !k.byClient || k.clientOrderID == "" {
		return "", false
	}
	return k.clientOrderID, true
}

// IsSet reports whether a non-empty key variant was provided.
func (k OrderKey) IsSet() bool {
	if !k.set {
		return false
	}
	if k.byClient {
		return k.clientOrderID != ""
	}
	return k.orderID != ""
}
