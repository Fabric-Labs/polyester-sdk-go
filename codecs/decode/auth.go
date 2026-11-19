package decode

import (
	"strings"

	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	authv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/auth/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
	"github.com/Fabric-Labs/polyester-sdk-go/wire"
)

// MeFromProto decodes the caller's identity. A reply without the account ID,
// with a session without its ID, naming a sub-account key without its
// sub-account ID, or a sub-account ID for any other caller, returns a
// *errors.ResponseContractError.
func MeFromProto(msg *authv1.MeResponse) (models.MeResult, error) {
	if err := requireID("Me", "account_id", msg.GetAccountId()); err != nil {
		return models.MeResult{}, err
	}
	if session := msg.GetSession(); session != nil {
		if err := requireStringID("Me", "session.session_id", session.GetSessionId()); err != nil {
			return models.MeResult{}, err
		}
	}
	session, _ := wire.ProtoToMap(msg.GetSession())
	out := models.MeResult{
		AccountID: codecs.FormatUint64ID(msg.GetAccountId()),
		Session:   session,
	}
	if id := strings.TrimSpace(msg.GetApiKeyId()); id != "" {
		out.APIKeyID = id
	}
	if msg.GetUsername() != "" {
		out.Username = msg.GetUsername()
	}
	if msg.GetRootSmartAccountAddress() != "" {
		out.RootSmartAccountAddress = msg.GetRootSmartAccountAddress()
	}
	out.APIKeyScope = enumLabel(msg.GetApiKeyScope(), "SCOPE_")
	// subaccount_id is present exactly when the key is a sub-account key.
	if msg.GetApiKeyScope() == authv1.AccountScopeType_SCOPE_SUBACCOUNT {
		if err := requireID("Me", "subaccount_id", msg.GetSubaccountId()); err != nil {
			return models.MeResult{}, err
		}
		out.SubaccountID = codecs.FormatID(msg.GetSubaccountId())
	} else if msg.SubaccountId != nil {
		return models.MeResult{}, &sdkerrors.ResponseContractError{Operation: "Me", Msg: "a subaccount_id without a sub-account key scope"}
	}
	return out, nil
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
