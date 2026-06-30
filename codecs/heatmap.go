package codecs

import (
	"strings"

	"github.com/Fabric-Labs/polyester-sdk-go/errors"
)

var IntervalAliases = map[string]string{"1s": "INTERVAL_1S", "1m": "INTERVAL_1M", "5m": "INTERVAL_5M", "1h": "INTERVAL_1H"}
var QtyModeAliases = map[string]string{"close": "CLOSE", "peak": "PEAK"}

func DepthToProtoName(depth int) string {
	switch {
	case depth <= 5:
		return "DEPTH_5"
	case depth <= 10:
		return "DEPTH_10"
	case depth <= 20:
		return "DEPTH_20"
	case depth <= 50:
		return "DEPTH_50"
	case depth <= 100:
		return "DEPTH_100"
	default:
		return "DEPTH_200"
	}
}

func ResolveHeatmapEnum(values map[string]int32, aliases map[string]string, value, field string) (int32, error) {
	key := value
	if v, ok := aliases[key]; ok {
		key = v
	}
	if enum, ok := values[key]; ok {
		return enum, nil
	}
	if enum, ok := values[strings.ToUpper(value)]; ok {
		return enum, nil
	}
	return 0, &errors.ValidationError{Msg: "unknown " + field}
}
