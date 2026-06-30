package codecs

import (
	"strings"

	"github.com/Fabric-Labs/polyester-sdk-go/errors"
	analyticsv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/chain/analytics/v1"
)

var analyticsRangeToProto = map[string]analyticsv1.ChainAnalyticsRange{
	"1d":   analyticsv1.ChainAnalyticsRange_DAY_1,
	"7d":   analyticsv1.ChainAnalyticsRange_DAY_7,
	"30d":  analyticsv1.ChainAnalyticsRange_DAY_30,
	"90d":  analyticsv1.ChainAnalyticsRange_DAY_90,
	"180d": analyticsv1.ChainAnalyticsRange_DAY_180,
	"365d": analyticsv1.ChainAnalyticsRange_DAY_365,
}

// ResolveAnalyticsRange maps SDK range strings to chain analytics proto enums.
func ResolveAnalyticsRange(rangeKey string) (analyticsv1.ChainAnalyticsRange, error) {
	key := strings.ToLower(strings.TrimSpace(rangeKey))
	v, ok := analyticsRangeToProto[key]
	if !ok {
		return 0, &errors.ValidationError{Msg: "range must be one of '1d', '7d', '30d', '90d', '180d', or '365d'"}
	}
	return v, nil
}
