package codecs

import (
	"errors"
	"math"
	"testing"

	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

func TestMaxProtocolScaleConstant(t *testing.T) {
	if MaxProtocolScale != 36 {
		t.Fatalf("MaxProtocolScale=%d want 36", MaxProtocolScale)
	}
}

func TestFormatQtyScaledAcceptsBoundaryScales(t *testing.T) {
	for _, scale := range []int{0, 18, 36} {
		got, err := FormatQtyScaled(1, scale)
		if err != nil {
			t.Fatalf("scale %d: %v", scale, err)
		}
		if got == "" {
			t.Fatalf("scale %d: empty result", scale)
		}
	}
}

func TestFormatQtyScaledRejectsAboveMax(t *testing.T) {
	for _, scale := range []int{37, 65534, 65535, 65536, math.MaxInt32} {
		_, err := FormatQtyScaled(1, scale)
		var ve *sdkerrors.ValidationError
		if !errors.As(err, &ve) {
			t.Fatalf("scale %d: want ValidationError, got %v", scale, err)
		}
	}
}

func TestFormatLedgerU128RejectsAboveMax(t *testing.T) {
	for _, scale := range []int{37, 65535, math.MaxUint16} {
		_, err := FormatLedgerU128("1", scale)
		var ve *sdkerrors.ValidationError
		if !errors.As(err, &ve) {
			t.Fatalf("scale %d: want ValidationError, got %v", scale, err)
		}
	}
	got, err := FormatLedgerU128("1500000000000000000", 18)
	if err != nil || got != "1.5" {
		t.Fatalf("got %q err=%v", got, err)
	}
}

func TestParseQtyScaledRejectsAboveMax(t *testing.T) {
	_, err := ParseQtyScaled("1", 65535, "qty")
	var ve *sdkerrors.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("want ValidationError, got %v", err)
	}
}

func TestQtyScaledFormatRejectsAboveMax(t *testing.T) {
	scale := 65535
	q := models.MustQtyScaled(1).WithScale(scale)
	_, err := q.Format()
	var ve *sdkerrors.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("want ValidationError, got %v", err)
	}
}
