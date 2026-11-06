package transport

import (
	"errors"
	"fmt"
	"testing"

	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
)

func TestRESTOrderUnavailableClassification(t *testing.T) {
	for _, tc := range []struct {
		code   string
		status int
	}{
		{"RATE_LIMIT_POLICY_UNAVAILABLE", 503},
		{"POLICY_UNAVAILABLE", 503},
		{"FEE_SCHEDULE_UNAVAILABLE", 503},
		{"UNAVAILABLE", 503},
		{"PAIR_BACKLOGGED", 503},
		{"ORDER_MODIFY_IN_PROGRESS", 503},
		{"ORDER_PLACEMENT_PENDING", 503},
		{"SETTLEMENT_BACKLOGGED", 503},
		{"TRIGGER_CONTROLS_BUSY", 503},
		{"TRIGGER_CONTROL_IN_PROGRESS", 503},
		{"ACCOUNT_ROUTING_INVALID", 503},
		{"UPSTREAM_ERROR", 502},
		{"SERVICE_UNAVAILABLE", 503},
		{"ACCOUNT_SHARD_UNAVAILABLE", 503},
		{"CAPITAL_VIEW_UNAVAILABLE", 503},
		{"CHAIN_METADATA_UNAVAILABLE", 503},
		{"DESTINATION_VALIDATION_UNAVAILABLE", 503},
		{"FEE_UNAVAILABLE", 503},
		{"STEP_UP_UNAVAILABLE", 503},
		{"SUPPLY_UNAVAILABLE", 503},
	} {
		code := tc.code
		err := callMeAgainst(t, tc.status, "application/problem+json",
			fmt.Sprintf("{\"code\":%q,\"status\":%d,\"detail\":\"retry later\"}", code, tc.status))
		var server *sdkerrors.ServerError
		var auth *sdkerrors.AuthError
		if !errors.As(err, &server) || errors.As(err, &auth) {
			t.Fatalf("%s mapped=%T retryable=%v: %v", code, err, sdkerrors.IsRetryable(err), err)
		}
		known := map[string]bool{"UNAVAILABLE": true, "FEE_SCHEDULE_UNAVAILABLE": true, "POLICY_UNAVAILABLE": true, "RATE_LIMIT_POLICY_UNAVAILABLE": true, "PAIR_BACKLOGGED": true, "SETTLEMENT_BACKLOGGED": true, "ORDER_MODIFY_IN_PROGRESS": true, "ORDER_PLACEMENT_PENDING": true, "TRIGGER_CONTROLS_BUSY": true, "TRIGGER_CONTROL_IN_PROGRESS": true}[code]
		if sdkerrors.IsRetryable(err) != known || sdkerrors.MutationOutcomeUnknown(err) == known {
			t.Fatalf("%s retryable=%v unknown=%v", code, sdkerrors.IsRetryable(err), sdkerrors.MutationOutcomeUnknown(err))
		}
		if server.Code != code || server.Status != tc.status {
			t.Fatalf("%s server code=%q status=%d", code, server.Code, server.Status)
		}
	}
}
