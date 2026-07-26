package codecs

import (
	"strings"

	"github.com/Fabric-Labs/polyester-sdk-go/errors"
	ledgerrdv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/ledger/read/v1"
)

// LedgerScale is the default u128 ledger decimal scale.
const LedgerScale = 18

var balanceRangeToProto = map[string]ledgerrdv1.BalanceRange{
	"1d":   ledgerrdv1.BalanceRange_DAY_1,
	"7d":   ledgerrdv1.BalanceRange_DAY_7,
	"30d":  ledgerrdv1.BalanceRange_DAY_30,
	"90d":  ledgerrdv1.BalanceRange_DAY_90,
	"180d": ledgerrdv1.BalanceRange_DAY_180,
	"365d": ledgerrdv1.BalanceRange_DAY_365,
}

var equityGroupByToProto = map[string]ledgerrdv1.EquityGroupBy{
	"account": ledgerrdv1.EquityGroupBy_GROUP_BY_ACCOUNT,
	"asset":   ledgerrdv1.EquityGroupBy_GROUP_BY_ASSET,
}

// ResolveBalanceRange maps SDK range strings to proto enums.
func ResolveBalanceRange(rangeKey string) (ledgerrdv1.BalanceRange, error) {
	key := strings.ToLower(strings.TrimSpace(rangeKey))
	v, ok := balanceRangeToProto[key]
	if !ok {
		return 0, &errors.ValidationError{Msg: "range must be one of '1d', '7d', '30d', '90d', '180d', or '365d'"}
	}
	return v, nil
}

// ResolveEquityGroupBy maps SDK group_by strings to proto enums.
func ResolveEquityGroupBy(groupBy string) (ledgerrdv1.EquityGroupBy, error) {
	key := strings.ToLower(strings.TrimSpace(groupBy))
	v, ok := equityGroupByToProto[key]
	if !ok {
		return 0, &errors.ValidationError{Msg: "group_by must be 'account' or 'asset'"}
	}
	return v, nil
}

// FormatLedgerU128 formats a ledger u128 wire decimal string.
// Scale must be in [0, MaxProtocolScale] (0 defaults to LedgerScale). Larger
// values return a ValidationError instead of allocating pathological padding.
func FormatLedgerU128(raw string, scale int) (string, error) {
	if scale <= 0 {
		scale = LedgerScale
	}
	if err := ValidateProtocolScale(scale); err != nil {
		return "", err
	}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		raw = "0"
	}
	for _, r := range raw {
		if r < '0' || r > '9' {
			return "", &errors.ValidationError{Msg: "ledger value must be an unsigned decimal integer string"}
		}
	}
	digits := strings.TrimLeft(raw, "0")
	if digits == "" {
		digits = "0"
	}
	const u128MaxDecimal = "340282366920938463463374607431768211455"
	if len(digits) > len(u128MaxDecimal) || (len(digits) == len(u128MaxDecimal) && digits > u128MaxDecimal) {
		return "", &errors.ValidationError{Msg: "ledger value exceeds u128 range"}
	}
	if scale == 0 {
		return digits, nil
	}
	width := scale + 1
	if len(digits) < width {
		digits = strings.Repeat("0", width-len(digits)) + digits
	}
	head := strings.TrimLeft(digits[:len(digits)-scale], "0")
	if head == "" {
		head = "0"
	}
	tail := strings.TrimRight(digits[len(digits)-scale:], "0")
	if tail == "" {
		return head, nil
	}
	return head + "." + tail, nil
}
