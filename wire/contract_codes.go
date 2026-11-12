package wire

import (
	"strings"

	"buf.build/gen/go/bufbuild/protovalidate/protocolbuffers/go/buf/validate"
	authv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/auth/v1"
	withdrawv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/chain/withdraw/v1"
	ledgerrdv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/ledger/read/v1"
	orderv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/orders/v1"
	ratelimitv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/polyester/ratelimit/v1"
	transferv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/transfer/v1"
	"google.golang.org/protobuf/proto"
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

// IsPlaceholderCode reports whether code is an enum's zero value: "no code was
// provided", never a code of its own.
func IsPlaceholderCode(code string) bool {
	switch code {
	case "UNSPECIFIED", "ERROR_CODE_UNSPECIFIED", "AUTH_UNSPECIFIED":
		return true
	}
	return false
}

// isPlaceholderDetail reports whether a decoded service detail says nothing: no
// code and no backoff. Field violations alone name what to fix, not an outcome.
func isPlaceholderDetail(detail any) bool {
	switch d := detail.(type) {
	case *authv1.AuthErrorDetail:
		return d.GetCode() == authv1.AuthErrorCode_AUTH_UNSPECIFIED
	case *orderv1.ErrorDetail:
		return d.GetCode() == orderv1.ErrorCode_ERROR_CODE_UNSPECIFIED && proto.Size(d.GetRateLimit()) == 0
	case *withdrawv1.ErrorDetail:
		return d.GetCode() == 0
	case *transferv1.ErrorDetail:
		return d.GetCode() == 0
	case *ledgerrdv1.ErrorDetail:
		return d.GetCode() == 0
	case *ratelimitv1.RateLimitDetail:
		return proto.Size(d) == 0
	case *validate.Violations:
		return len(d.GetViolations()) == 0
	}
	return false
}

// futureCodeLabel returns the label of a code this SDK's enum can't name, or ""
// for any other detail. The detail's type shows that a service sent it, not what
// the code means: it could be a future refusal or a future unknown outcome.
func futureCodeLabel(detail any) string {
	known := true
	label := ""
	switch d := detail.(type) {
	case *authv1.AuthErrorDetail:
		_, known = authv1.AuthErrorCode_name[int32(d.GetCode())]
		label = d.GetCode().String()
	case *orderv1.ErrorDetail:
		_, known = orderv1.ErrorCode_name[int32(d.GetCode())]
		label = d.GetCode().String()
	case *withdrawv1.ErrorDetail:
		_, known = withdrawv1.ErrorCode_name[int32(d.GetCode())]
		label = d.GetCode().String()
	case *transferv1.ErrorDetail:
		_, known = transferv1.ErrorCode_name[int32(d.GetCode())]
		label = d.GetCode().String()
	case *ledgerrdv1.ErrorDetail:
		_, known = ledgerrdv1.ErrorCode_name[int32(d.GetCode())]
		label = d.GetCode().String()
	}
	if known {
		return ""
	}
	return label
}

func hasFutureCode(detail any) bool { return futureCodeLabel(detail) != "" }

// CarriesContractCode reports whether an encoded error detail is a service detail
// with a code this SDK can name. One that is empty or undecodable proves nothing.
func CarriesContractCode(detailType string, value []byte) bool {
	detail := decodeServiceDetail(detailType, value)
	return detail != nil && !isPlaceholderDetail(detail) && !hasFutureCode(detail)
}

// FutureCodeLabel returns the label of an encoded detail's code when this SDK's
// enum can't name it, or "". It is kept for diagnosis only.
func FutureCodeLabel(detailType string, value []byte) string {
	detail := decodeServiceDetail(detailType, value)
	if detail == nil {
		return ""
	}
	return futureCodeLabel(detail)
}

func decodeServiceDetail(detailType string, value []byte) proto.Message {
	var detail proto.Message
	switch detailType {
	case "auth.v1.AuthErrorDetail":
		detail = &authv1.AuthErrorDetail{}
	case "orders.v1.ErrorDetail":
		detail = &orderv1.ErrorDetail{}
	case "chain.withdraw.v1.ErrorDetail":
		detail = &withdrawv1.ErrorDetail{}
	case "transfer.v1.ErrorDetail":
		detail = &transferv1.ErrorDetail{}
	case "ledger.read.v1.ErrorDetail":
		detail = &ledgerrdv1.ErrorDetail{}
	case "polyester.ratelimit.v1.RateLimitDetail":
		detail = &ratelimitv1.RateLimitDetail{}
	case "buf.validate.Violations":
		detail = &validate.Violations{}
	default:
		return nil
	}
	if proto.Unmarshal(value, detail) != nil {
		return nil
	}
	return detail
}

// IsRecognisedRESTCode reports whether the contract defines this REST problem
// code. A mutation's unknown outcome is recognised by its suffix, including one
// added to the contract after this SDK was generated.
func IsRecognisedRESTCode(code string) bool {
	if code == "" || IsPlaceholderCode(code) {
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

// OrderPermissionCodeFromREST names a REST 401 or 403 problem code as the
// orders.v1.ErrorCode its ConnectRPC detail carries, such as
// API_KEY_SPOT_TRADE_DENY as ERROR_CODE_API_KEY_SPOT_TRADE_DENY. Codes no
// order ErrorCode names, auth codes, and the generic UNAUTHENTICATED and
// PERMISSION_DENIED are returned unchanged.
func OrderPermissionCodeFromREST(code string) string {
	if code == "" || code == "UNAUTHENTICATED" || code == "PERMISSION_DENIED" || strings.HasPrefix(code, "ERROR_CODE_") {
		return code
	}
	if _, auth := authv1.AuthErrorCode_value["AUTH_"+code]; auth {
		return code
	}
	if _, ok := orderv1.ErrorCode_value["ERROR_CODE_"+code]; ok {
		return "ERROR_CODE_" + code
	}
	return code
}

// ErrorCodeFromREST names a REST problem code as its typed ConnectRPC detail
// does: SAME_SOURCE_DESTINATION reads ERROR_CODE_SAME_SOURCE_DESTINATION when
// any service's ErrorCode (orders, withdraw, transfer, ledger) names it. A
// non-order code the auth service also names, such as INVALID_REQUEST, can't
// be told apart over REST and is returned unchanged, so errors.AuthErrorCode
// still reads it. A code none of them names is returned unchanged.
func ErrorCodeFromREST(code string) string {
	prefixed := code
	if !strings.HasPrefix(code, "ERROR_CODE_") {
		prefixed = "ERROR_CODE_" + code
	}
	if _, ok := orderv1.ErrorCode_value[prefixed]; ok {
		return prefixed
	}
	if _, ok := authv1.AuthErrorCode_value["AUTH_"+code]; ok {
		return code
	}
	for _, enum := range []map[string]int32{withdrawv1.ErrorCode_value, transferv1.ErrorCode_value, ledgerrdv1.ErrorCode_value} {
		if _, ok := enum[prefixed]; ok {
			return prefixed
		}
	}
	return code
}

// IsRouteNotFoundMessage reports whether message is a router's own answer for a
// path it doesn't know.
func IsRouteNotFoundMessage(message string) bool {
	_, ok := routeNotFoundMessages[strings.ToLower(strings.TrimSpace(message))]
	return ok
}
