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
		Icon:             msg.GetIcon(),
		Color:            msg.GetColor(),
		CreatedByActor:   msg.GetCreatedByActor(),
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

// ApiKeysListFromProto decodes the account's API keys. A key without its ID
// returns a *errors.ResponseContractError.
func ApiKeysListFromProto(msg *authv1.ListApiKeysResponse) (models.ApiKeysList, error) {
	if err := requireAPIKeyIDs("ListApiKeys", msg.GetApiKeys()...); err != nil {
		return models.ApiKeysList{}, err
	}
	return models.ApiKeysList{Keys: apiKeys(msg.GetApiKeys())}, nil
}

// requireAPIKeyIDs refuses an API key read without the key's ID.
func requireAPIKeyIDs(operation string, keys ...*authv1.ApiKey) error {
	for _, k := range keys {
		if err := requireStringID(operation, "key_id", k.GetKeyId()); err != nil {
			return err
		}
	}
	return nil
}

// apiKeys decodes a list of API keys; empty, never nil.
func apiKeys(keys []*authv1.ApiKey) []models.ApiKeySummary {
	out := make([]models.ApiKeySummary, 0, len(keys))
	for _, k := range keys {
		if row := apiKey(k); row != nil {
			out = append(out, *row)
		}
	}
	return out
}

// ApiKeyFromGetProto decodes one API key; nil when the reply has none. A key
// without its ID returns a *errors.ResponseContractError.
func ApiKeyFromGetProto(msg *authv1.GetApiKeyResponse) (*models.ApiKeySummary, error) {
	if key := msg.GetApiKey(); key != nil {
		if err := requireAPIKeyIDs("GetApiKey", key); err != nil {
			return nil, err
		}
	}
	return apiKey(msg.GetApiKey()), nil
}
