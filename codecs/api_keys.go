package codecs

import (
	"strings"

	"github.com/Fabric-Labs/polyester-sdk-go/errors"
	authv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/auth/v1"
)

// ApiKeyPatch carries optional API key update fields.
// Nil pointer = omit; non-nil (including empty string/slice) = set and include in the mask.
// ExpiresAt uses ClearableTime for omit/clear/set.
type ApiKeyPatch struct {
	ExpectedRevision uint64
	Label            *string
	Icon             *string
	Color            *string
	Status           *string
	IpWhitelist      *[]string
	ExpiresAt        *ClearableTime
}

// ApiKeyStatusFromLabel maps a status label to the proto enum.
func ApiKeyStatusFromLabel(status string) (authv1.ApiKeyStatus, error) {
	aliases := map[string]authv1.ApiKeyStatus{
		"active":   authv1.ApiKeyStatus_ACTIVE,
		"revoked":  authv1.ApiKeyStatus_REVOKED,
		"disabled": authv1.ApiKeyStatus_DISABLED,
	}
	key := strings.ToLower(status)
	if v, ok := aliases[key]; ok {
		return v, nil
	}
	if v, ok := authv1.ApiKeyStatus_value[strings.ToUpper(key)]; ok && v != 0 {
		return authv1.ApiKeyStatus(v), nil
	}
	return 0, &errors.ValidationError{Msg: "unknown API key status: " + status}
}

// BuildUpdateApiKeyRequest builds a durable API key update request.
func BuildUpdateApiKeyRequest(keyID string, patch ApiKeyPatch) (*authv1.UpdateApiKeyRequest, error) {
	if err := requirePositiveRevision(patch.ExpectedRevision); err != nil {
		return nil, err
	}
	spec := &authv1.ApiKeyUpdateSpec{}
	var paths []string
	if patch.Label != nil {
		spec.Label = *patch.Label
		paths = append(paths, "label")
	}
	if patch.Icon != nil {
		spec.Icon = *patch.Icon
		paths = append(paths, "icon")
	}
	if patch.Color != nil {
		spec.Color = *patch.Color
		paths = append(paths, "color")
	}
	if patch.Status != nil {
		status, err := ApiKeyStatusFromLabel(*patch.Status)
		if err != nil {
			return nil, err
		}
		spec.Status = status
		paths = append(paths, "status")
	}
	if patch.IpWhitelist != nil {
		spec.IpWhitelist = *patch.IpWhitelist
		paths = append(paths, "ip_whitelist")
	}
	if patch.ExpiresAt != nil {
		ts, err := timestampFromClearable(patch.ExpiresAt, "expires_at")
		if err != nil {
			return nil, err
		}
		spec.ExpiresAt = ts
		paths = append(paths, "expires_at")
	}
	mask, err := newUpdateMask(paths)
	if err != nil {
		return nil, err
	}
	return &authv1.UpdateApiKeyRequest{
		KeyId:            keyID,
		ApiKey:           spec,
		UpdateMask:       mask,
		ExpectedRevision: patch.ExpectedRevision,
	}, nil
}
