package codecs

import (
	"testing"

	"google.golang.org/protobuf/types/known/timestamppb"
)

// An expiry in the last nanoseconds of a second reads exactly, never rounded
// into the next second.
func TestFormatExpireAtIsExactForEveryNanosecond(t *testing.T) {
	const seconds = 1794592800 // 2026-11-13T18:00:00Z
	for nanos, want := range map[int32]string{
		999999999: "2026-11-13T18:00:00.999999999Z",
		999999404: "2026-11-13T18:00:00.999999404Z",
		999999403: "2026-11-13T18:00:00.999999403Z",
		123456789: "2026-11-13T18:00:00.123456789Z",
		0:         "2026-11-13T18:00:00Z",
	} {
		if got := FormatExpireAt(&timestamppb.Timestamp{Seconds: seconds, Nanos: nanos}); got != want {
			t.Errorf("nanos %d: got %s, want %s", nanos, got, want)
		}
	}
	if got := FormatExpireAt(nil); got != "" {
		t.Errorf("unset: got %q, want empty", got)
	}
}
