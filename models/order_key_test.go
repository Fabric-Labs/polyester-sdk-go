package models_test

import (
	"testing"

	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

func TestOrderKeyExclusiveAccessors(t *testing.T) {
	byID := models.OrderKeyByID("42")
	if id, ok := byID.OrderID(); !ok || id != "42" {
		t.Fatalf("OrderID: got %q ok=%v", id, ok)
	}
	if _, ok := byID.ClientOrderID(); ok {
		t.Fatal("ClientOrderID should be unset for OrderKeyByID")
	}
	if !byID.IsSet() {
		t.Fatal("expected IsSet")
	}

	byClient := models.OrderKeyByClientID("cid-1")
	if cid, ok := byClient.ClientOrderID(); !ok || cid != "cid-1" {
		t.Fatalf("ClientOrderID: got %q ok=%v", cid, ok)
	}
	if _, ok := byClient.OrderID(); ok {
		t.Fatal("OrderID should be unset for OrderKeyByClientID")
	}

	if (models.OrderKey{}).IsSet() {
		t.Fatal("zero OrderKey must not be set")
	}
	if models.OrderKeyByID("").IsSet() {
		t.Fatal("empty OrderKeyByID must not be set")
	}
}
