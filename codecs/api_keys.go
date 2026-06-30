package codecs

import (
	"strings"

	"github.com/Fabric-Labs/polyester-sdk-go/errors"
	authv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/auth/v1"
)

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
