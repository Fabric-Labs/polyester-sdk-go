package services

import (
	"reflect"
	"testing"
)

// A flow names its account only by its smart account address: the contract
// removed owner_account_id, so the helper neither takes nor emits it.
func TestLifecycleAccountFieldsNamesOnlyTheSmartAccount(t *testing.T) {
	address := "0x5aaeb6053f3e94c9b9a09f33669435e7ef1beaed"
	if got := LifecycleAccountFields(address); !reflect.DeepEqual(got, map[string]string{"smart_account_address": address}) {
		t.Fatalf("fields = %v", got)
	}
	if got := LifecycleAccountFields(""); len(got) != 0 {
		t.Fatalf("fields without an address = %v", got)
	}
	if n := reflect.TypeOf(LifecycleAccountFields).NumIn(); n != 1 {
		t.Fatalf("LifecycleAccountFields takes %d arguments; want the address alone", n)
	}
}
