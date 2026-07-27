package codecs

import "testing"

func TestDepthToConnectEnumPreservesProtocolBoundaries(t *testing.T) {
	tests := map[int]string{
		1:    "DEPTH_1",
		5:    "DEPTH_5",
		500:  "DEPTH_500",
		1000: "DEPTH_1000",
	}
	for depth, want := range tests {
		if got := DepthToConnectEnum(depth); got != want {
			t.Fatalf("depth %d: got %s want %s", depth, got, want)
		}
	}
}
