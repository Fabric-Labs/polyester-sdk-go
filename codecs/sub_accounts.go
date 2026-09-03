package codecs

import (
	"strings"

	"github.com/Fabric-Labs/polyester-sdk-go/errors"
	authv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/auth/v1"
)

// InviteDirectionFromLabel maps a public incoming/outgoing filter onto the
// generated SubaccountInviteDirection enum. Empty means "both" (UNSPECIFIED).
func InviteDirectionFromLabel(direction string) (authv1.SubaccountInviteDirection, error) {
	switch strings.ToLower(strings.TrimSpace(direction)) {
	case "", "unspecified":
		return authv1.SubaccountInviteDirection_DIRECTION_UNSPECIFIED, nil
	case "incoming":
		return authv1.SubaccountInviteDirection_INCOMING, nil
	case "outgoing":
		return authv1.SubaccountInviteDirection_OUTGOING, nil
	default:
		return authv1.SubaccountInviteDirection_DIRECTION_UNSPECIFIED, &errors.ValidationError{
			Msg: "invites_direction must be incoming, outgoing, or empty",
		}
	}
}
