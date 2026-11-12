package decode

import (
	"encoding/hex"
	"time"

	authv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/auth/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func timestampTime(ts *timestamppb.Timestamp) *time.Time {
	if ts == nil {
		return nil
	}
	value := ts.AsTime()
	return &value
}

// ipWhitelist copies a key's CIDR allowlist; nil when it has none.
func ipWhitelist(cidrs []string) []string {
	if len(cidrs) == 0 {
		return nil
	}
	return append([]string(nil), cidrs...)
}

func apiKey(msg *authv1.ApiKey) *models.ApiKeySummary {
	if msg == nil {
		return nil
	}
	return &models.ApiKeySummary{
		KeyID:            msg.GetKeyId(),
		Label:            msg.GetLabel(),
		Status:           msg.GetStatus().String(),
		Purpose:          msg.GetPurpose().String(),
		PublicKeyEd25519: hex.EncodeToString(msg.GetPublicKeyEd25519()),
		SubaccountID:     optionalID(msg.GetSubaccountId()),
		PolicyID:         optionalID(msg.GetPolicyId()),
		IPWhitelist:      ipWhitelist(msg.GetIpWhitelist()),
		Revision:         msg.GetRevision(),
		CreatedAt:        timestampTime(msg.GetCreatedAt()),
		LastUsedAt:       timestampTime(msg.GetLastUsedAt()),
		UpdatedAt:        timestampTime(msg.GetUpdatedAt()),
		ExpiresAt:        timestampTime(msg.GetExpiresAt()),
	}
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
