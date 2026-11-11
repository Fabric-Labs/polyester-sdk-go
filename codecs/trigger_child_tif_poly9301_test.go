package codecs

import "testing"

// POLY-9301: a market child (stop_loss, take_profit or twap market) and a
// trailing stop always execute IOC, so an explicit gtc or fok is refused
// instead of being sent as IOC. Unset and ioc still work.
func TestPoly9301TriggerIOCChildRefusesGTCAndFOK(t *testing.T) {
	for _, tc := range []struct{ kind, orderType string }{
		{"stop_loss", "market"}, {"take_profit", "market"}, {"twap", "market"},
		{"trailing_stop", "market"}, {"trailing_stop", ""},
	} {
		for _, tif := range []string{"gtc", "fok", "GTC"} {
			_, err := createTrigger(tc.kind, tc.orderType, tif, CreateTriggerOptions{})
			if err == nil {
				t.Fatalf("%s %s tif=%s: accepted, want refusal", tc.kind, tc.orderType, tif)
			}
			wantValidation(t, err, "applies only to limit children")
		}
		for _, tif := range []string{"", "ioc"} {
			if _, err := createTrigger(tc.kind, tc.orderType, tif, CreateTriggerOptions{}); err != nil {
				t.Fatalf("%s %s tif=%q: %v", tc.kind, tc.orderType, tif, err)
			}
		}
	}
}

// Limit children keep their time in force: stop and take-profit limit
// children take gtc, ioc or fok; ladder and twap limit slices take gtc.
func TestPoly9301TriggerLimitChildrenKeepTheirTIF(t *testing.T) {
	for _, tc := range []struct{ kind, orderType, tif string }{
		{"stop_loss", "limit", "gtc"}, {"stop_loss", "limit", "ioc"}, {"stop_loss", "limit", "fok"},
		{"take_profit", "limit", "fok"}, {"twap", "limit", "gtc"}, {"ladder", "", "gtc"},
	} {
		if _, err := createTrigger(tc.kind, tc.orderType, tc.tif, CreateTriggerOptions{}); err != nil {
			t.Fatalf("%s %s tif=%s: %v", tc.kind, tc.orderType, tc.tif, err)
		}
	}
}
