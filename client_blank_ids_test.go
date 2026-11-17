package polyester

import (
	"context"
	"testing"
)

// A supplied trigger ID filter on open orders and order history is parsed
// whatever it holds: a blank one is refused and nothing is sent, where it
// was dropped and every order was listed. A nil filter lists every order.
func TestOrderListsRefuseABlankTriggerFilter(t *testing.T) {
	ctx := context.Background()
	client, capture := newLimitCaptureClient(t)
	calls := map[string]func(triggerID *string) error{
		"orders.ListOpen": func(triggerID *string) error {
			_, err := client.Orders.ListOpen(ctx, nil, nil, nil, nil, false, false, triggerID)
			return err
		},
		"orders.ListHistory": func(triggerID *string) error {
			_, err := client.Orders.ListHistory(ctx, nil, nil, nil, nil, nil, 0, false, false, triggerID)
			return err
		},
	}
	for name, call := range calls {
		for _, id := range []string{"", " ", "\t", " ", " 23", "23 "} {
			id := id
			if err := call(&id); !isValidationNaming(err, "trigger_id") {
				t.Errorf("%s(trigger_id %q): err = %T %v, want a trigger_id ValidationError", name, id, err, err)
			}
			if sent := capture.take(); len(sent) != 0 {
				t.Errorf("%s(trigger_id %q): sent %+v", name, id, sent)
			}
		}
		_ = call(nil)
		sent := capture.take()
		if len(sent) == 0 {
			t.Fatalf("%s(nil): nothing sent", name)
		}
		type triggerFiltered interface{ GetTriggerId() uint64 }
		if req, ok := sent[0].msg.(triggerFiltered); !ok || req.GetTriggerId() != 0 {
			t.Errorf("%s(nil): sent %v, want no trigger filter", name, sent[0].msg)
		}
	}
}
