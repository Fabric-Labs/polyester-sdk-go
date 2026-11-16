package orderbook

import (
	"errors"
	"strings"
	"testing"

	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
)

// A whitespace-only bucket is not empty: like a whitespace-only price, it is
// refused. Whitespace around a valid price is ignored, as for every price.
var invalidBuckets = []string{"abc", "-1", "0", "0.000", " ", "\t \n", "0.0000000001", "1e3"}

func TestParseBucketTicks(t *testing.T) {
	for bucket, want := range map[string]int{"": 0, "0.01": 10_000_000, " 0.01\t": 10_000_000, "1": 1_000_000_000, "0.000000001": 1} {
		got, err := ParseBucketTicks(bucket)
		if err != nil || got != want {
			t.Fatalf("ParseBucketTicks(%q)=%d, %v; want %d", bucket, got, err, want)
		}
	}
	for _, bucket := range invalidBuckets {
		_, err := ParseBucketTicks(bucket)
		var validation *sdkerrors.ValidationError
		if !errors.As(err, &validation) {
			t.Fatalf("ParseBucketTicks(%q) err=%v, want *errors.ValidationError", bucket, err)
		}
		if !strings.Contains(err.Error(), "positive price") || !strings.Contains(err.Error(), "empty for no bucketing") {
			t.Fatalf("ParseBucketTicks(%q) error %q does not say what is valid", bucket, err)
		}
	}
}

func TestSetBucketRefusesInvalidAndKeepsCurrent(t *testing.T) {
	bucket := -1
	renders := 0
	sub := NewSubscription(nil, func() { renders++ }, func(ticks int) { bucket = ticks })
	if err := sub.SetBucket("0.01"); err != nil || bucket != 10_000_000 {
		t.Fatalf("SetBucket(0.01) err=%v bucket=%d", err, bucket)
	}
	before := renders
	for _, invalid := range invalidBuckets {
		var validation *sdkerrors.ValidationError
		if err := sub.SetBucket(invalid); !errors.As(err, &validation) {
			t.Fatalf("SetBucket(%q) err=%v, want *errors.ValidationError", invalid, err)
		}
		if bucket != 10_000_000 {
			t.Fatalf("SetBucket(%q) changed the bucket to %d", invalid, bucket)
		}
	}
	if renders != before {
		t.Fatalf("an invalid bucket rendered %d views", renders-before)
	}
	if err := sub.SetBucket(""); err != nil || bucket != 0 {
		t.Fatalf("SetBucket(\"\") err=%v bucket=%d, want no bucketing", err, bucket)
	}
}
