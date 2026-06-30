package codecs

import (
	"strings"

	collabv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/collab/v1"
)

// BoardAudienceFromLabel maps an audience label to the proto enum.
func BoardAudienceFromLabel(value string) collabv1.BoardAudience {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "private":
		return collabv1.BoardAudience_PRIVATE
	case "public":
		return collabv1.BoardAudience_PUBLIC
	case "followers":
		return collabv1.BoardAudience_FOLLOWERS
	default:
		return collabv1.BoardAudience_AUDIENCE_UNSPECIFIED
	}
}

// BoardRoleFromLabel maps a role label to the proto enum.
func BoardRoleFromLabel(value string) collabv1.BoardRole {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "viewer":
		return collabv1.BoardRole_VIEWER
	case "editor":
		return collabv1.BoardRole_EDITOR
	case "owner":
		return collabv1.BoardRole_OWNER
	default:
		return collabv1.BoardRole_ROLE_UNSPECIFIED
	}
}
