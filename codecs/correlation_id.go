package codecs

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/Fabric-Labs/polyester-sdk-go/errors"
)

const (
	clientIDMaxLen  = 36
	requestIDMaxLen = 64
)

func validateCorrelationID(value, field string, maxLen int) (string, error) {
	trimmed := strings.TrimSpace(value)
	if len(trimmed) == 0 || len(trimmed) > maxLen {
		return "", &errors.ValidationError{
			Msg: fmt.Sprintf("%s must be 1 to %d characters", field, maxLen),
		}
	}
	for _, char := range trimmed {
		allowed := char >= 'a' && char <= 'z' ||
			char >= 'A' && char <= 'Z' ||
			char >= '0' && char <= '9' ||
			strings.ContainsRune("._:/-", char)
		if !allowed {
			return "", &errors.ValidationError{
				Msg: field + " contains invalid characters; allowed: A-Z a-z 0-9 . _ : / -",
			}
		}
	}
	return trimmed, nil
}

func optionalClientID(value *string, field string) (*string, error) {
	if value == nil || strings.TrimSpace(*value) == "" {
		return nil, nil
	}
	validated, err := validateCorrelationID(*value, field, clientIDMaxLen)
	if err != nil {
		return nil, err
	}
	return &validated, nil
}

func requiredClientID(value, field string) (string, error) {
	return validateCorrelationID(value, field, clientIDMaxLen)
}

func coalesceRequestID(requestID *string, prefix string) (string, error) {
	if requestID != nil && strings.TrimSpace(*requestID) != "" {
		return validateCorrelationID(*requestID, "request_id", requestIDMaxLen)
	}
	var b [6]byte
	_, _ = rand.Read(b[:])
	return fmt.Sprintf("%s-%s", prefix, hex.EncodeToString(b[:])), nil
}
