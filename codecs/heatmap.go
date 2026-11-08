package codecs

import (
	"fmt"
	"strings"

	"github.com/Fabric-Labs/polyester-sdk-go/errors"
	heatmapv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/marketdata/v1"
)

var IntervalAliases = map[string]string{"1s": "INTERVAL_1S", "1m": "INTERVAL_1M", "5m": "INTERVAL_5M", "1h": "INTERVAL_1H"}
var QtyModeAliases = map[string]string{"close": "CLOSE", "peak": "PEAK"}

// heatmapIntervalChannelLabels are the interval segments the server publishes
// live heatmap buckets on, by interval enum name.
var heatmapIntervalChannelLabels = map[string]string{
	"INTERVAL_1S": "1s",
	"INTERVAL_1M": "1m",
	"INTERVAL_5M": "5m",
	"INTERVAL_1H": "1h",
}

// ResolveHeatmapInterval returns the HeatmapInterval value of interval, an
// alias ("1s", "1m", "5m", "1h") or an interval enum name ("INTERVAL_1S").
// Any other value, INTERVAL_UNSPECIFIED and quantity modes included, is a
// ValidationError.
func ResolveHeatmapInterval(interval string) (heatmapv1.HeatmapInterval, error) {
	name := interval
	if alias, ok := IntervalAliases[interval]; ok {
		name = alias
	}
	if _, ok := heatmapIntervalChannelLabels[name]; !ok {
		return 0, &errors.ValidationError{Msg: fmt.Sprintf("unknown heatmap interval %q; use 1s, 1m, 5m or 1h", interval)}
	}
	return heatmapv1.HeatmapInterval(heatmapv1.HeatmapInterval_value[name]), nil
}

// HeatmapIntervalChannelLabel returns the interval segment of the live heatmap
// channel for interval, an alias ("1s", "1m", "5m", "1h") or an interval enum
// name ("INTERVAL_1S"), as Heatmap.Get accepts it. Any other value is a
// ValidationError: the server publishes on no other channel.
func HeatmapIntervalChannelLabel(interval string) (string, error) {
	name := interval
	if alias, ok := IntervalAliases[interval]; ok {
		name = alias
	}
	if label, ok := heatmapIntervalChannelLabels[name]; ok {
		return label, nil
	}
	return "", &errors.ValidationError{Msg: fmt.Sprintf("unknown heatmap interval %q; use 1s, 1m, 5m or 1h", interval)}
}

// HeatmapDepth returns the depth Heatmap.Get requests for depth, by the rule of
// OrderbookSnapshotDepth: depth itself when it is a supported depth (1, 5, 10,
// 20, 50, 100, 200, 500, 1000), else the smallest one covering it. 0 is the
// default 50. A depth below 0 or above 1000 is a ValidationError.
func HeatmapDepth(depth int) (int, error) {
	return coveringSnapshotDepth(depth, "heatmap depth")
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
