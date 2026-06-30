package codecs

import (
	"math/big"
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
func FormatLedgerU128(raw string, scale int) string {
	if scale <= 0 {
		scale = LedgerScale
	}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		raw = "0"
	}
	value := new(big.Rat)
	if _, ok := value.SetString(raw); !ok {
		return "0"
	}
	divisor := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(scale)), nil)
	value.Quo(value, new(big.Rat).SetInt(divisor))
	if value.Sign() == 0 {
		return "0"
	}
	out := value.FloatString(int(scale))
	out = strings.TrimRight(strings.TrimRight(out, "0"), ".")
	if out == "" || out == "-0" {
		return "0"
	}
	return out
}
