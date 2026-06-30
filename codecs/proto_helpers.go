package codecs

import (
	"math/big"
	"strings"

	orderv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/orders/v1"
	"google.golang.org/protobuf/reflect/protoreflect"
)

var enumPrefixes = []string{
	"SIDE_", "ORDER_TYPE_", "ORDER_STATUS_", "TIF_", "FLOW_STEP_", "FLOW_KIND_",
	"KIND_", "MODIFY_ACTION_", "API_KEY_STATUS_", "TRIGGER_TYPE_", "TRIGGER_STATUS_",
	"TRIGGER_EVENT_TYPE_", "STATUS_", "EVENT_", "TIME_IN_FORCE_", "SELF_TRADE_PREVENTION_MODE_",
	"BALANCE_RANGE_", "PROTECTED_ACTION_", "SCOPE_", "ENTRY_KIND_", "DESTINATION_",
	"INTERNAL_WHITELIST_", "TRANSFER_COUNTERPARTY_",
}

// FormatUint64ID formats a uint64 id as base58 (or "0").
func FormatUint64ID(value uint64) string {
	if value == 0 {
		return "0"
	}
	return FormatID(value)
}

// U128ToStr formats hi/lo u128 parts as decimal string.
func U128ToStr(hi, lo uint64) string {
	value := new(big.Int).SetUint64(hi)
	value.Lsh(value, 64)
	value.Add(value, new(big.Int).SetUint64(lo))
	return value.String()
}

// ProtoEnumName strips common enum prefixes.
func ProtoEnumName(enum protoreflect.Enum, value protoreflect.EnumNumber) string {
	if value == 0 {
		return ""
	}
	name := string(enum.Descriptor().Values().ByNumber(value).Name())
	lower := strings.ToLower(name)
	for _, prefix := range enumPrefixes {
		p := strings.ToLower(prefix)
		if strings.HasPrefix(lower, p) {
			return strings.TrimPrefix(lower, p)
		}
	}
	return lower
}

// OrderSideName formats order side enum.
func OrderSideName(v orderv1.Side) string {
	switch v {
	case orderv1.Side_BUY:
		return "buy"
	case orderv1.Side_SELL:
		return "sell"
	default:
		return ""
	}
}

// OrderTypeName formats order type enum.
func OrderTypeName(v orderv1.OrderType) string {
	switch v {
	case orderv1.OrderType_LIMIT:
		return "limit"
	case orderv1.OrderType_MARKET:
		return "market"
	default:
		return ""
	}
}

// TimeInForceName formats time-in-force enum.
func TimeInForceName(v orderv1.TimeInForce) string {
	switch v {
	case orderv1.TimeInForce_GTC:
		return "gtc"
	case orderv1.TimeInForce_IOC:
		return "ioc"
	case orderv1.TimeInForce_FOK:
		return "fok"
	default:
		return ""
	}
}

// TIFName is an alias for TimeInForceName.
func TIFName(v orderv1.TimeInForce) string { return TimeInForceName(v) }
