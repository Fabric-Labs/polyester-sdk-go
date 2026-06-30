package decode

import (
	"encoding/hex"

	authv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/auth/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

func apiKey(msg *authv1.ApiKey) *models.ApiKeySummary {
	if msg == nil {
		return nil
	}
	return &models.ApiKeySummary{KeyID: msg.GetKeyId(), Label: msg.GetLabel(), Status: msg.GetStatus().String(), PublicKeyEd25519: hex.EncodeToString(msg.GetPublicKeyEd25519())}
}

// ApiKeyMessageFromProto decodes one API key message.
func ApiKeyMessageFromProto(msg *authv1.ApiKey) *models.ApiKeySummary { return apiKey(msg) }

func ApiKeysListFromProto(msg *authv1.ListApiKeysResponse) models.ApiKeysList {
	out := make([]models.ApiKeySummary, 0, len(msg.GetApiKeys()))
	for _, k := range msg.GetApiKeys() {
		if row := apiKey(k); row != nil {
			out = append(out, *row)
		}
	}
	return models.ApiKeysList{Keys: out}
}

func ApiKeyFromGetProto(msg *authv1.GetApiKeyResponse) *models.ApiKeySummary {
	return apiKey(msg.GetApiKey())
}
func ApiKeyFromCreateProto(msg *authv1.CreateApiKeyResponse) *models.ApiKeySummary {
	return apiKey(msg.GetApiKey())
}
func ApiKeyFromUpdateProto(msg *authv1.UpdateApiKeyResponse) *models.ApiKeySummary {
	return apiKey(msg.GetApiKey())
}
