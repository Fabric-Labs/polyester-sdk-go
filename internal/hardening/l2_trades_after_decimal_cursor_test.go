package hardening_test

import (
	"context"
	"testing"
)

// The cursor is a decimal match ID, as GetTradesAfter returns it in MatchID
// and NextAfterTradeID. A short decimal that also spells a valid base58 ID
// (such as "42", base58 for 175) must still be sent as its decimal value.
func TestL2GetTradesAfterSendsShortDecimalCursorExactly(t *testing.T) {
	handler := &tradesAfterHandler{}
	client := newTradesAfterClient(t, handler)
	symbol := "ETH-USDT"
	cases := map[string]uint64{
		"42": 42, "5": 5, "999": 999, "3871524": 3871524, "2345678912": 2345678912,
		"175": 175, "100": 100, "19791209306181": 19791209306181,
		"18446744073709551615": 18446744073709551615,
	}
	for cursor, want := range cases {
		handler.mu.Lock()
		handler.requests = nil
		handler.mu.Unlock()
		if _, err := client.MarketData.GetTradesAfter(context.Background(), &symbol, nil, cursor, 10); err != nil {
			t.Fatalf("cursor %q: %v", cursor, err)
		}
		handler.mu.Lock()
		requests := handler.requests
		handler.mu.Unlock()
		if len(requests) != 1 || requests[0].AfterTradeId == nil {
			t.Fatalf("cursor %q: requests %+v", cursor, requests)
		}
		if got := requests[0].GetAfterTradeId(); got != want {
			t.Errorf("cursor %q sent as after_trade_id=%d, want %d", cursor, got, want)
		}
	}
	for _, bad := range []string{"18446744073709551616", "4fr", "0x2a", "+42", "4 2"} {
		if _, err := client.MarketData.GetTradesAfter(context.Background(), &symbol, nil, bad, 10); err == nil {
			t.Errorf("cursor %q: want a validation error", bad)
		}
	}
}
