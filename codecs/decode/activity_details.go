package decode

import (
	"slices"
	"time"

	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	authv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/auth/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

// activityItem decodes the item an activity event affected; nil when
// absent. Opaque IDs read as public IDs, API key and MFA factor IDs as sent.
func activityItem(msg *authv1.ActivityItem) *models.ActivityItem {
	if msg == nil {
		return nil
	}
	item := &models.ActivityItem{Name: msg.GetName()}
	switch id := msg.GetId().(type) {
	case *authv1.ActivityItem_AccountId:
		item.Kind, item.ID = models.ActivityItemAccount, codecs.FormatID(id.AccountId)
	case *authv1.ActivityItem_SubaccountId:
		item.Kind, item.ID = models.ActivityItemSubaccount, codecs.FormatID(id.SubaccountId)
	case *authv1.ActivityItem_InviteId:
		item.Kind, item.ID = models.ActivityItemInvite, codecs.FormatID(id.InviteId)
	case *authv1.ActivityItem_SubaccountPolicyId:
		item.Kind, item.ID = models.ActivityItemSubaccountPolicy, codecs.FormatID(id.SubaccountPolicyId)
	case *authv1.ActivityItem_ApiPolicyId:
		item.Kind, item.ID = models.ActivityItemAPIPolicy, codecs.FormatID(id.ApiPolicyId)
	case *authv1.ActivityItem_ApiKeyId:
		item.Kind, item.ID = models.ActivityItemAPIKey, id.ApiKeyId
	case *authv1.ActivityItem_MfaFactorId:
		item.Kind, item.ID = models.ActivityItemMFAFactor, id.MfaFactorId
	case *authv1.ActivityItem_AddressBookEntryId:
		item.Kind, item.ID = models.ActivityItemAddressBookEntry, codecs.FormatID(id.AddressBookEntryId)
	case *authv1.ActivityItem_BrokerAllowlistChangeId:
		item.Kind, item.ID = models.ActivityItemBrokerAllowlistChange, codecs.FormatID(id.BrokerAllowlistChangeId)
	case *authv1.ActivityItem_ConnectedAppId:
		item.Kind, item.ID = models.ActivityItemConnectedApp, codecs.FormatID(id.ConnectedAppId)
	}
	return item
}

// activityDetails decodes what an activity event changed; nil when absent or
// of a kind this SDK does not know.
func activityDetails(msg *authv1.ActivityDetails) *models.ActivityDetails {
	switch kind := msg.GetKind().(type) {
	case *authv1.ActivityDetails_Account:
		d := kind.Account
		return &models.ActivityDetails{Account: &models.ActivityAccountDetails{
			Username:          textChange(d.GetUsername()),
			GeneratedUsername: d.GetGeneratedUsername(),
		}}
	case *authv1.ActivityDetails_Session:
		d := kind.Session
		return &models.ActivityDetails{Session: &models.ActivitySessionDetails{
			Method:         d.GetMethod(),
			WalletAddress:  d.GetWalletAddress(),
			Failure:        d.GetFailure(),
			Scopes:         slices.Clone(d.GetScopes()),
			ElevatedAction: d.GetElevatedAction(),
		}}
	case *authv1.ActivityDetails_ApiKey:
		d := kind.ApiKey
		details := &models.ActivityAPIKeyDetails{
			Label:         textChange(d.GetLabel()),
			Icon:          textChange(d.GetIcon()),
			Color:         textChange(d.GetColor()),
			ExpiresAt:     timeChange(d.GetExpiresAt()),
			APIPolicyID:   idChange(d.GetApiPolicyId()),
			StrategyBotID: optionalPublicID(d.GetStrategyBotId()),
		}
		if c := d.GetStatus(); c != nil {
			details.Status = &models.ActivityChange[authv1.ApiKeyStatus]{From: c.GetFrom(), To: c.GetTo()}
		}
		if c := d.GetPurpose(); c != nil {
			details.Purpose = &models.ActivityChange[authv1.ApiKeyPurpose]{From: c.GetFrom(), To: c.GetTo()}
		}
		if c := d.GetIpAllowlist(); c != nil {
			details.IPAllowlist = &models.ActivityListChange[string]{Added: slices.Clone(c.GetAdded()), Removed: slices.Clone(c.GetRemoved())}
		}
		return &models.ActivityDetails{APIKey: details}
	case *authv1.ActivityDetails_Policy:
		d := kind.Policy
		details := &models.ActivityPolicyDetails{
			Name:                   textChange(d.GetName()),
			Description:            textChange(d.GetDescription()),
			AllSpotMarkets:         boolChange(d.GetAllSpotMarkets()),
			MaxOrderNotional:       uintChange(d.GetMaxOrderNotional()),
			MaxOpenOrders:          uintChange(d.GetMaxOpenOrders()),
			TradingHalted:          boolChange(d.GetTradingHalted()),
			Locked:                 boolChange(d.GetLocked()),
			ReviewAt:               timeChange(d.GetReviewAt()),
			ExpiresAt:              timeChange(d.GetExpiresAt()),
			AssistantPreset:        d.GetAssistantPreset(),
			SourceTemplateID:       optionalPublicID(d.GetSourceTemplateId()),
			SourceTemplateRevision: d.GetSourceTemplateRevision(),
			SubaccountPolicyID:     idChange(d.GetSubaccountPolicyId()),
			StrategyBotID:          optionalPublicID(d.GetStrategyBotId()),
		}
		if c := d.GetActions(); c != nil {
			details.Actions = &models.ActivityListChange[authv1.PolicyAction]{Added: slices.Clone(c.GetAdded()), Removed: slices.Clone(c.GetRemoved())}
		}
		return &models.ActivityDetails{Policy: details}
	case *authv1.ActivityDetails_Subaccount:
		d := kind.Subaccount
		details := &models.ActivitySubaccountDetails{
			Label:            textChange(d.GetLabel()),
			Icon:             textChange(d.GetIcon()),
			Color:            textChange(d.GetColor()),
			RequireMemberMFA: boolChange(d.GetRequireMemberMfa()),
		}
		if c := d.GetStatus(); c != nil {
			details.Status = &models.ActivityChange[authv1.SubaccountStatus]{From: c.GetFrom(), To: c.GetTo()}
		}
		return &models.ActivityDetails{Subaccount: details}
	case *authv1.ActivityDetails_Member:
		d := kind.Member
		details := &models.ActivityMemberDetails{OwnershipTransfer: d.GetOwnershipTransfer()}
		if c := d.GetRole(); c != nil {
			details.Role = &models.ActivityChange[authv1.SubaccountRole]{From: c.GetFrom(), To: c.GetTo()}
		}
		if c := d.GetBrokerRole(); c != nil {
			details.BrokerRole = &models.ActivityChange[authv1.ActivityBrokerRole]{From: c.GetFrom(), To: c.GetTo()}
		}
		return &models.ActivityDetails{Member: details}
	case *authv1.ActivityDetails_Invite:
		d := kind.Invite
		details := &models.ActivityInviteDetails{
			GranteeAccountID: optionalPublicID(d.GetGranteeAccountId()),
			Role:             d.GetRole(),
			BrokerRole:       d.GetBrokerRole(),
		}
		if c := d.GetStatus(); c != nil {
			details.Status = &models.ActivityChange[authv1.SubaccountInviteStatus]{From: c.GetFrom(), To: c.GetTo()}
		}
		return &models.ActivityDetails{Invite: details}
	case *authv1.ActivityDetails_Destination:
		d := kind.Destination
		details := &models.ActivityDestinationDetails{
			Label:       textChange(d.GetLabel()),
			NoteChanged: d.GetNoteChanged(),
		}
		if c := d.GetTagIds(); c != nil {
			details.TagIDs = &models.ActivityListChange[string]{Added: publicIDs(c.GetAddedIds()), Removed: publicIDs(c.GetRemovedIds())}
		}
		return &models.ActivityDetails{Destination: details}
	case *authv1.ActivityDetails_BrokerAllowlist:
		d := kind.BrokerAllowlist
		return &models.ActivityDetails{BrokerAllowlist: &models.ActivityBrokerAllowlistDetails{
			Action:             d.GetAction(),
			Destination:        d.GetDestination(),
			DestinationChainID: d.GetDestinationChainId(),
			Label:              d.GetLabel(),
		}}
	case *authv1.ActivityDetails_Security:
		d := kind.Security
		return &models.ActivityDetails{Security: &models.ActivitySecurityDetails{
			FactorType:           d.GetFactorType(),
			Label:                textChange(d.GetLabel()),
			ChallengePurpose:     d.GetChallengePurpose(),
			RecoveryCodesRotated: d.GetRecoveryCodesRotated(),
		}}
	default:
		return nil
	}
}

func textChange(c *authv1.ActivityTextChange) *models.ActivityChange[string] {
	if c == nil {
		return nil
	}
	return &models.ActivityChange[string]{From: c.GetFrom(), To: c.GetTo()}
}

func boolChange(c *authv1.ActivityBoolChange) *models.ActivityChange[bool] {
	if c == nil {
		return nil
	}
	return &models.ActivityChange[bool]{From: c.GetFrom(), To: c.GetTo()}
}

func uintChange(c *authv1.ActivityUintChange) *models.ActivityChange[uint64] {
	if c == nil {
		return nil
	}
	return &models.ActivityChange[uint64]{From: c.GetFrom(), To: c.GetTo()}
}

// timeChange keeps each side's seconds and nanoseconds; an unset side is nil.
func timeChange(c *authv1.ActivityTimeChange) *models.ActivityChange[*time.Time] {
	if c == nil {
		return nil
	}
	return &models.ActivityChange[*time.Time]{From: timestampTime(c.GetFrom()), To: timestampTime(c.GetTo())}
}

// idChange reads each side as a public ID; an unset side is "".
func idChange(c *authv1.ActivityIdChange) *models.ActivityChange[string] {
	if c == nil {
		return nil
	}
	return &models.ActivityChange[string]{From: optionalPublicID(c.GetFromId()), To: optionalPublicID(c.GetToId())}
}

func optionalPublicID(id uint64) string {
	if id == 0 {
		return ""
	}
	return codecs.FormatID(id)
}

func publicIDs(ids []uint64) []string {
	if len(ids) == 0 {
		return nil
	}
	out := make([]string, len(ids))
	for i, id := range ids {
		out[i] = codecs.FormatID(id)
	}
	return out
}
