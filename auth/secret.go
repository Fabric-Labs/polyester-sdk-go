package auth

import "github.com/Fabric-Labs/polyester-sdk-go/models"

const redactedSecret = "[REDACTED]"

// Secret holds a secret string that printing and encoding never reveal. It is
// models.Secret; see there.
type Secret = models.Secret

// NewSecret wraps value. An empty value gives the unset Secret.
func NewSecret(value string) Secret {
	return models.NewSecret(value)
}
