package codecs_test

import (
	"errors"
	"testing"

	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	authv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/auth/v1"
)

func TestInviteDirectionFromLabel(t *testing.T) {
	cases := []struct {
		in   string
		want authv1.SubaccountInviteDirection
	}{
		{"", authv1.SubaccountInviteDirection_DIRECTION_UNSPECIFIED},
		{"unspecified", authv1.SubaccountInviteDirection_DIRECTION_UNSPECIFIED},
		{"incoming", authv1.SubaccountInviteDirection_INCOMING},
		{"OUTGOING", authv1.SubaccountInviteDirection_OUTGOING},
	}
	for _, tc := range cases {
		got, err := codecs.InviteDirectionFromLabel(tc.in)
		if err != nil {
			t.Fatalf("direction %q: %v", tc.in, err)
		}
		if got != tc.want {
			t.Fatalf("direction %q: got %v want %v", tc.in, got, tc.want)
		}
	}
	_, err := codecs.InviteDirectionFromLabel("sideways")
	var validation *sdkerrors.ValidationError
	if !errors.As(err, &validation) {
		t.Fatalf("expected ValidationError, got %T: %v", err, err)
	}
}
