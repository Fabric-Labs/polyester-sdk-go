package codecs

import (
	"testing"

	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

// A supplied sub-account ID on a batch or a trigger modify is parsed
// whatever it holds: a blank one is refused, never the main account. Only a
// nil sub-account is the main account.
func TestBatchesAndTriggerModifyRefuseABlankSubAccount(t *testing.T) {
	symbol, symbolID := "BTC-USDT", uint32(1)
	price, qty := models.PriceFromDecimal("100"), models.QtyFromDecimal("1")
	create := []models.CreateOrderRequest{{Symbol: &symbol, SymbolID: &symbolID, Side: "buy", OrderType: "limit", Qty: qty, Price: &price}}
	cancel := []models.BatchCancelItem{{Key: models.OrderKeyByID(FormatID(100)), SymbolID: &symbolID}}
	replace := []models.BatchReplaceItem{{Key: models.OrderKeyByID(FormatID(100)), NewPrice: &price}}
	trailing := int64(5)
	calls := map[string]func(sub *string) error{
		"BatchCreateOrdersToProto": func(sub *string) error {
			_, err := BatchCreateOrdersToProto(create, sub, ptr("b-1"), false, 8, 2)
			return err
		},
		"BatchCancelOrdersToProto": func(sub *string) error {
			_, err := BatchCancelOrdersToProto(cancel, sub, ptr("b-1"))
			return err
		},
		"BatchReplaceOrdersToProto": func(sub *string) error {
			_, err := BatchReplaceOrdersToProto(replace, symbolID, sub, ptr("b-1"), 8, symbol)
			return err
		},
		"ModifyTriggerToProto": func(sub *string) error {
			_, err := ModifyTriggerToProto(FormatID(2), 0, "", sub, ModifyTriggerOptions{TrailingDistanceTicks: &trailing})
			return err
		},
	}
	for name, call := range calls {
		for _, id := range []string{"", " ", "\t", " ", " 23", "23 "} {
			id := id
			if err := call(&id); err == nil {
				t.Errorf("%s(sub_account_id %q): encoded, want a ValidationError", name, id)
			} else {
				refusedNaming(t, "sub_account_id", err)
			}
		}
		if err := call(nil); err != nil {
			t.Errorf("%s(nil sub-account): %v", name, err)
		}
	}
}
