package decode_test

import (
	"errors"
	"testing"

	"github.com/Fabric-Labs/polyester-sdk-go/codecs/decode"
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	chaindepositv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/chain/deposit/v1"
)

func TestCreateDepositAddressRejectsMissingRequiredEntity(t *testing.T) {
	_, err := decode.CreateDepositAddressFromProto(
		&chaindepositv1.CreateDepositAddressResponse{},
	)
	var transportErr *sdkerrors.TransportError
	if !errors.As(err, &transportErr) {
		t.Fatalf("expected TransportError, got %T: %v", err, err)
	}
}
