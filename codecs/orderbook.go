package codecs

import "fmt"

func DepthToConnectEnum(depth int) string {
	switch {
	case depth <= 0:
		return "DEPTH_5"
	case depth == 1:
		return "DEPTH_1"
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
	case depth <= 200:
		return "DEPTH_200"
	case depth <= 500:
		return "DEPTH_500"
	default:
		return "DEPTH_1000"
	}
}

func DepthProtoValue(depth int, values map[string]int32) (int32, error) {
	name := DepthToConnectEnum(depth)
	if v, ok := values[name]; ok {
		return v, nil
	}
	return 0, fmt.Errorf("unsupported depth %d", depth)
}
