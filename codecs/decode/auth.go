package decode

import (
	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	authv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/auth/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
	"github.com/Fabric-Labs/polyester-sdk-go/wire"
)

func MeFromProto(msg *authv1.MeResponse) models.MeResult {
	session, _ := wire.ProtoToMap(msg.GetSession())
	out := models.MeResult{
		AccountID: codecs.FormatUint64ID(msg.GetAccountId()),
		Session:   session,
	}
	if msg.GetApiKeyId() != 0 {
		out.APIKeyID = codecs.FormatUint64ID(msg.GetApiKeyId())
	}
	if msg.GetUsername() != "" {
		out.Username = msg.GetUsername()
	}
	if msg.GetRootSmartAccountAddress() != "" {
		out.RootSmartAccountAddress = msg.GetRootSmartAccountAddress()
	}
	return out
}

func ProfileFromProto(msg *authv1.UserProfile) models.UserProfile {
	if msg == nil {
		return models.UserProfile{}
	}
	return models.UserProfile{
		Username: msg.GetUsername(), Bio: msg.GetBio(), Website: msg.GetWebsite(),
		Twitter: msg.GetTwitter(), AvatarURL: msg.GetAvatarUrl(),
	}
}

func UsernameHistoryFromProto(msg *authv1.GetUsernameHistoryResponse) models.UsernameHistoryList {
	entries := make([]models.UsernameHistoryEntry, 0, len(msg.GetHistory()))
	for _, e := range msg.GetHistory() {
		changedAt := int64(0)
		if ts := e.GetSetAt(); ts != nil {
			changedAt = ts.AsTime().UnixMilli()
		}
		entries = append(entries, models.UsernameHistoryEntry{Username: e.GetUsername(), ChangedAt: changedAt})
	}
	return models.UsernameHistoryList{Entries: entries}
}

// AccountIdentityFromProto decodes a public identity update.
func AccountIdentityFromProto(msg *authv1.AccountIdentity) models.AccountIdentity {
	if msg == nil {
		return models.AccountIdentity{}
	}
	return models.AccountIdentity{
		AccountID:               codecs.FormatID(msg.GetAccountId()),
		Username:                msg.GetUsername(),
		AvatarURL:               msg.GetAvatarUrl(),
		RootSmartAccountAddress: msg.GetRootSmartAccountAddress(),
	}
}
