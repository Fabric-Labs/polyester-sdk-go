package wire

import (
	"strings"

	authv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/auth/v1"
	withdrawv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/chain/withdraw/v1"
	ledgerrdv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/ledger/read/v1"
	orderv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/orders/v1"
	transferv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/transfer/v1"
)

// restOnlyCodes are contract REST codes that no generated error enum names.
var restOnlyCodes = map[string]struct{}{
	"API_KEY_ACTION_DENY":                  {},
	"API_KEY_POLICY_NOT_FOUND":             {},
	"INVALID_BEFORE_TS_NS":                 {},
	"INVALID_DEPTH":                        {},
	"INVALID_END_TIME":                     {},
	"INVALID_LIMIT":                        {},
	"INVALID_OFFSET":                       {},
	"INVALID_ORDER_STATUS":                 {},
	"INVALID_PAGE_TOKEN":                   {},
	"INVALID_START_TIME":                   {},
	"INVALID_SYMBOL":                       {},
	"INVALID_TRIGGER_ID":                   {},
	"INVALID_TRIGGER_STATUS":               {},
	"ORDER_ID_REQUIRED":                    {},
	"REQUEST_TOO_LARGE":                    {},
	"ROLE_ACTION_DENY":                     {},
	"ROLE_SPOT_TRADE_DENY":                 {},
	"SERVER_ERROR":                         {},
	"SUBACCOUNT_POLICY_DENY":               {},
	"TRAILING_STOP_ORDER_TYPE_UNSUPPORTED": {},
	"TRIGGER_ID_REQUIRED":                  {},
}

// IsRecognisedRESTCode reports whether the contract defines this REST problem
// code. A mutation's unknown outcome is recognised by its suffix, including one
// added to the contract after this SDK was generated.
func IsRecognisedRESTCode(code string) bool {
	if code == "" {
		return false
	}
	if _, ok := restOnlyCodes[code]; ok || strings.HasSuffix(code, "_OUTCOME_UNKNOWN") {
		return true
	}
	for _, name := range []string{code, "ERROR_CODE_" + code} {
		for _, enum := range []map[string]int32{orderv1.ErrorCode_value, withdrawv1.ErrorCode_value, transferv1.ErrorCode_value, ledgerrdv1.ErrorCode_value} {
			if _, ok := enum[name]; ok {
				return true
			}
		}
	}
	for _, name := range []string{code, "AUTH_" + code} {
		if _, ok := authv1.AuthErrorCode_value[name]; ok {
			return true
		}
	}
	return false
}

// IsRouteNotFoundMessage reports whether message is a router's own answer for a
// path it doesn't know.
func IsRouteNotFoundMessage(message string) bool {
	_, ok := routeNotFoundMessages[strings.ToLower(strings.TrimSpace(message))]
	return ok
}
