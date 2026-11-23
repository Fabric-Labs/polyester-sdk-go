package decode

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	authv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/auth/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

// activityTime is a detail time that is not whole milliseconds, so a
// decoder that truncates it fails.
func activityTime(seconds int64, nanos int32) (*timestamppb.Timestamp, *time.Time) {
	value := time.Unix(seconds, int64(nanos)).UTC()
	return &timestamppb.Timestamp{Seconds: seconds, Nanos: nanos}, &value
}

// decodeActivityBothWays decodes one event through the account feed and the
// sub-account feed, which must read it the same.
func decodeActivityBothWays(t *testing.T, event *authv1.ActivityEvent) models.SubAccountActivityEvent {
	t.Helper()
	account := AccountActivityListFromProto(&authv1.ListAccountActivityResponse{Events: []*authv1.ActivityEvent{event}})
	sub := SubaccountActivityListFromProto(&authv1.ListSubaccountEventsResponse{Events: []*authv1.ActivityEvent{event}})
	if len(account.Events) != 1 || len(sub.Events) != 1 {
		t.Fatalf("events = %d (account), %d (sub-account), want 1 each", len(account.Events), len(sub.Events))
	}
	if !reflect.DeepEqual(account.Events[0], sub.Events[0]) {
		t.Fatalf("account and sub-account feeds differ:\naccount: %+v\nsub:     %+v", account.Events[0], sub.Events[0])
	}
	return account.Events[0]
}

// requireEveryFieldSet fails when a decoded value leaves a field at its zero
// value, so a fixture that sets every contract field proves every one
// arrives. A side of a bool change is exempt: one side is always false, and
// the exact comparison checks it.
func requireEveryFieldSet(t *testing.T, path string, value reflect.Value) {
	t.Helper()
	if value.Kind() == reflect.Bool && (strings.HasSuffix(path, ".From") || strings.HasSuffix(path, ".To")) {
		return
	}
	if value.Kind() == reflect.Pointer {
		if value.IsNil() {
			t.Errorf("%s is nil", path)
			return
		}
		value = value.Elem()
	}
	if value.Kind() != reflect.Struct || value.Type() == reflect.TypeFor[time.Time]() {
		if value.IsZero() {
			t.Errorf("%s is zero", path)
		}
		return
	}
	for i := range value.NumField() {
		requireEveryFieldSet(t, path+"."+value.Type().Field(i).Name, value.Field(i))
	}
}

// Every item kind the contract defines decodes to its kind and ID: a public
// ID for an opaque ID, the string ID as sent for an API key and an MFA
// factor, with the item's name.
func TestActivityEventDecodesEveryItemKind(t *testing.T) {
	cases := []struct {
		id   any
		kind models.ActivityItemKind
		want string
	}{
		{&authv1.ActivityItem_AccountId{AccountId: 9000000001}, models.ActivityItemAccount, codecs.FormatID(9000000001)},
		{&authv1.ActivityItem_SubaccountId{SubaccountId: 9000000002}, models.ActivityItemSubaccount, codecs.FormatID(9000000002)},
		{&authv1.ActivityItem_InviteId{InviteId: 9000000003}, models.ActivityItemInvite, codecs.FormatID(9000000003)},
		{&authv1.ActivityItem_SubaccountPolicyId{SubaccountPolicyId: 9000000004}, models.ActivityItemSubaccountPolicy, codecs.FormatID(9000000004)},
		{&authv1.ActivityItem_ApiPolicyId{ApiPolicyId: 9000000005}, models.ActivityItemAPIPolicy, codecs.FormatID(9000000005)},
		{&authv1.ActivityItem_ApiKeyId{ApiKeyId: "ak_0123456789abcdef0123456789abcdef"}, models.ActivityItemAPIKey, "ak_0123456789abcdef0123456789abcdef"},
		{&authv1.ActivityItem_MfaFactorId{MfaFactorId: "mfa_totp_1"}, models.ActivityItemMFAFactor, "mfa_totp_1"},
		{&authv1.ActivityItem_AddressBookEntryId{AddressBookEntryId: 9000000008}, models.ActivityItemAddressBookEntry, codecs.FormatID(9000000008)},
		{&authv1.ActivityItem_BrokerAllowlistChangeId{BrokerAllowlistChangeId: 9000000009}, models.ActivityItemBrokerAllowlistChange, codecs.FormatID(9000000009)},
		{&authv1.ActivityItem_ConnectedAppId{ConnectedAppId: 9000000010}, models.ActivityItemConnectedApp, codecs.FormatID(9000000010)},
	}
	if got, want := len(cases), (&authv1.ActivityItem{}).ProtoReflect().Descriptor().Oneofs().ByName("id").Fields().Len(); got != want {
		t.Fatalf("%d item kinds tested, the contract has %d", got, want)
	}
	for _, tc := range cases {
		t.Run(string(tc.kind), func(t *testing.T) {
			item := &authv1.ActivityItem{Name: "name of " + string(tc.kind)}
			reflect.ValueOf(item).Elem().FieldByName("Id").Set(reflect.ValueOf(tc.id))
			got := decodeActivityBothWays(t, &authv1.ActivityEvent{Item: item})
			want := &models.ActivityItem{Kind: tc.kind, ID: tc.want, Name: "name of " + string(tc.kind)}
			if !reflect.DeepEqual(got.Item, want) {
				t.Fatalf("Item = %+v, want %+v", got.Item, want)
			}
			if got.Details != nil {
				t.Fatalf("Details = %+v, want nil", got.Details)
			}
		})
	}
}

// An item with no ID (a sign-in session, a failed sign-in, the recovery
// codes) keeps its name, with no kind or ID; an event without an item or
// details reads them as nil.
func TestActivityEventItemWithoutIDAndAbsentItemAndDetails(t *testing.T) {
	got := decodeActivityBothWays(t, &authv1.ActivityEvent{Item: &authv1.ActivityItem{Name: "Firefox on Linux"}})
	if want := (&models.ActivityItem{Name: "Firefox on Linux"}); !reflect.DeepEqual(got.Item, want) {
		t.Fatalf("Item = %+v, want %+v", got.Item, want)
	}
	got = decodeActivityBothWays(t, &authv1.ActivityEvent{
		EntityKind: authv1.ActivityEntityKind_ACTIVITY_ENTITY_API_KEY, EventAction: authv1.ActivityEventAction_ACTIVITY_ACTION_DELETED,
		Actor: &authv1.ActivityActor{Kind: authv1.ActivityActorKind_ACTIVITY_ACTOR_USER},
	})
	if got.Item != nil || got.Details != nil {
		t.Fatalf("Item = %+v, Details = %+v, want both nil", got.Item, got.Details)
	}
	// Details sent with a kind this SDK does not know (the oneof unset) read
	// as nil, as no change this SDK can show.
	got = decodeActivityBothWays(t, &authv1.ActivityEvent{Details: &authv1.ActivityDetails{}})
	if got.Details != nil {
		t.Fatalf("Details = %+v, want nil", got.Details)
	}
}

// activityDetailsCase is one kind of details with every contract field set,
// and the details the SDK must return for it.
type activityDetailsCase struct {
	name string
	msg  *authv1.ActivityDetails
	want *models.ActivityDetails
}

func activityDetailsCases() []activityDetailsCase {
	keyExpiryFromPB, keyExpiryFrom := activityTime(1_794_700_000, 250_000_001)
	keyExpiryToPB, keyExpiryTo := activityTime(1_797_300_000, 999_999_999)
	reviewFromPB, reviewFrom := activityTime(1_794_800_000, 1)
	reviewToPB, reviewTo := activityTime(1_794_900_000, 123_456_789)
	policyExpiryFromPB, policyExpiryFrom := activityTime(1_795_000_000, 500_000_000)
	policyExpiryToPB, policyExpiryTo := activityTime(1_795_100_000, 7)
	return []activityDetailsCase{
		{
			name: "account",
			msg: &authv1.ActivityDetails{Kind: &authv1.ActivityDetails_Account{Account: &authv1.ActivityAccountDetails{
				Username:          &authv1.ActivityTextChange{From: "polyester_4821", To: "alice"},
				GeneratedUsername: true,
			}}},
			want: &models.ActivityDetails{Account: &models.ActivityAccountDetails{
				Username:          &models.ActivityChange[string]{From: "polyester_4821", To: "alice"},
				GeneratedUsername: true,
			}},
		},
		{
			name: "session",
			msg: &authv1.ActivityDetails{Kind: &authv1.ActivityDetails_Session{Session: &authv1.ActivitySessionDetails{
				Method:         authv1.ActivitySignInMethod_ACTIVITY_SIGN_IN_CONNECTED_APP,
				WalletAddress:  "0xabcdef0123456789abcdef0123456789abcdef01",
				Failure:        authv1.ActivitySignInFailure_ACTIVITY_SIGN_IN_FAILURE_CHALLENGE_USED,
				Scopes:         []string{"read:balances", "trade:spot"},
				ElevatedAction: authv1.ActivityElevatedAction_ACTIVITY_ELEVATED_TELEGRAM_LINK,
			}}},
			want: &models.ActivityDetails{Session: &models.ActivitySessionDetails{
				Method:         authv1.ActivitySignInMethod_ACTIVITY_SIGN_IN_CONNECTED_APP,
				WalletAddress:  "0xabcdef0123456789abcdef0123456789abcdef01",
				Failure:        authv1.ActivitySignInFailure_ACTIVITY_SIGN_IN_FAILURE_CHALLENGE_USED,
				Scopes:         []string{"read:balances", "trade:spot"},
				ElevatedAction: authv1.ActivityElevatedAction_ACTIVITY_ELEVATED_TELEGRAM_LINK,
			}},
		},
		{
			name: "api_key",
			msg: &authv1.ActivityDetails{Kind: &authv1.ActivityDetails_ApiKey{ApiKey: &authv1.ActivityApiKeyDetails{
				Label:         &authv1.ActivityTextChange{From: "bot", To: "market maker"},
				Status:        &authv1.ActivityApiKeyStatusChange{From: authv1.ApiKeyStatus_ACTIVE, To: authv1.ApiKeyStatus_DISABLED},
				Purpose:       &authv1.ActivityApiKeyPurposeChange{From: authv1.ApiKeyPurpose_STANDARD, To: authv1.ApiKeyPurpose_STRATEGY_BOT},
				Icon:          &authv1.ActivityTextChange{From: "robot", To: "chart"},
				Color:         &authv1.ActivityTextChange{From: "#112233", To: "#445566"},
				IpAllowlist:   &authv1.ActivityListChange{Added: []string{"203.0.113.0/24", "198.51.100.7"}, Removed: []string{"192.0.2.1"}},
				ExpiresAt:     &authv1.ActivityTimeChange{From: keyExpiryFromPB, To: keyExpiryToPB},
				ApiPolicyId:   &authv1.ActivityIdChange{FromId: 9000000021, ToId: 9000000022},
				StrategyBotId: 9000000023,
			}}},
			want: &models.ActivityDetails{APIKey: &models.ActivityAPIKeyDetails{
				Label:         &models.ActivityChange[string]{From: "bot", To: "market maker"},
				Status:        &models.ActivityChange[authv1.ApiKeyStatus]{From: authv1.ApiKeyStatus_ACTIVE, To: authv1.ApiKeyStatus_DISABLED},
				Purpose:       &models.ActivityChange[authv1.ApiKeyPurpose]{From: authv1.ApiKeyPurpose_STANDARD, To: authv1.ApiKeyPurpose_STRATEGY_BOT},
				Icon:          &models.ActivityChange[string]{From: "robot", To: "chart"},
				Color:         &models.ActivityChange[string]{From: "#112233", To: "#445566"},
				IPAllowlist:   &models.ActivityListChange[string]{Added: []string{"203.0.113.0/24", "198.51.100.7"}, Removed: []string{"192.0.2.1"}},
				ExpiresAt:     &models.ActivityChange[*time.Time]{From: keyExpiryFrom, To: keyExpiryTo},
				APIPolicyID:   &models.ActivityChange[string]{From: codecs.FormatID(9000000021), To: codecs.FormatID(9000000022)},
				StrategyBotID: codecs.FormatID(9000000023),
			}},
		},
		{
			name: "policy",
			msg: &authv1.ActivityDetails{Kind: &authv1.ActivityDetails_Policy{Policy: &authv1.ActivityPolicyDetails{
				Name:                   &authv1.ActivityTextChange{From: "read only", To: "trader"},
				Description:            &authv1.ActivityTextChange{From: "reads", To: "reads and trades"},
				Actions:                &authv1.ActivityActionsChange{Added: []authv1.PolicyAction{authv1.PolicyAction_TRADE_SPOT, authv1.PolicyAction_READ_PNL}, Removed: []authv1.PolicyAction{authv1.PolicyAction_EXTERNAL_WITHDRAW}},
				AllSpotMarkets:         &authv1.ActivityBoolChange{From: false, To: true},
				MaxOrderNotional:       &authv1.ActivityUintChange{From: 1_000_000_000, To: 25_000_000_000},
				MaxOpenOrders:          &authv1.ActivityUintChange{From: 10, To: 200},
				TradingHalted:          &authv1.ActivityBoolChange{From: true, To: false},
				Locked:                 &authv1.ActivityBoolChange{From: false, To: true},
				ReviewAt:               &authv1.ActivityTimeChange{From: reviewFromPB, To: reviewToPB},
				ExpiresAt:              &authv1.ActivityTimeChange{From: policyExpiryFromPB, To: policyExpiryToPB},
				AssistantPreset:        authv1.AssistantKeyPreset_TRADE_WITH_APPROVAL,
				SourceTemplateId:       9000000031,
				SourceTemplateRevision: 4,
				SubaccountPolicyId:     &authv1.ActivityIdChange{FromId: 9000000032, ToId: 9000000033},
				StrategyBotId:          9000000034,
			}}},
			want: &models.ActivityDetails{Policy: &models.ActivityPolicyDetails{
				Name:                   &models.ActivityChange[string]{From: "read only", To: "trader"},
				Description:            &models.ActivityChange[string]{From: "reads", To: "reads and trades"},
				Actions:                &models.ActivityListChange[authv1.PolicyAction]{Added: []authv1.PolicyAction{authv1.PolicyAction_TRADE_SPOT, authv1.PolicyAction_READ_PNL}, Removed: []authv1.PolicyAction{authv1.PolicyAction_EXTERNAL_WITHDRAW}},
				AllSpotMarkets:         &models.ActivityChange[bool]{From: false, To: true},
				MaxOrderNotional:       &models.ActivityChange[uint64]{From: 1_000_000_000, To: 25_000_000_000},
				MaxOpenOrders:          &models.ActivityChange[uint64]{From: 10, To: 200},
				TradingHalted:          &models.ActivityChange[bool]{From: true, To: false},
				Locked:                 &models.ActivityChange[bool]{From: false, To: true},
				ReviewAt:               &models.ActivityChange[*time.Time]{From: reviewFrom, To: reviewTo},
				ExpiresAt:              &models.ActivityChange[*time.Time]{From: policyExpiryFrom, To: policyExpiryTo},
				AssistantPreset:        authv1.AssistantKeyPreset_TRADE_WITH_APPROVAL,
				SourceTemplateID:       codecs.FormatID(9000000031),
				SourceTemplateRevision: 4,
				SubaccountPolicyID:     &models.ActivityChange[string]{From: codecs.FormatID(9000000032), To: codecs.FormatID(9000000033)},
				StrategyBotID:          codecs.FormatID(9000000034),
			}},
		},
		{
			name: "subaccount",
			msg: &authv1.ActivityDetails{Kind: &authv1.ActivityDetails_Subaccount{Subaccount: &authv1.ActivitySubaccountDetails{
				Label:            &authv1.ActivityTextChange{From: "desk", To: "desk 2"},
				Status:           &authv1.ActivitySubaccountStatusChange{From: authv1.SubaccountStatus_SUBACCOUNT_STATUS_ACTIVE, To: authv1.SubaccountStatus_SUBACCOUNT_STATUS_DISABLED},
				Icon:             &authv1.ActivityTextChange{From: "briefcase", To: "rocket"},
				Color:            &authv1.ActivityTextChange{From: "#000000", To: "#ffffff"},
				RequireMemberMfa: &authv1.ActivityBoolChange{From: false, To: true},
			}}},
			want: &models.ActivityDetails{Subaccount: &models.ActivitySubaccountDetails{
				Label:            &models.ActivityChange[string]{From: "desk", To: "desk 2"},
				Status:           &models.ActivityChange[authv1.SubaccountStatus]{From: authv1.SubaccountStatus_SUBACCOUNT_STATUS_ACTIVE, To: authv1.SubaccountStatus_SUBACCOUNT_STATUS_DISABLED},
				Icon:             &models.ActivityChange[string]{From: "briefcase", To: "rocket"},
				Color:            &models.ActivityChange[string]{From: "#000000", To: "#ffffff"},
				RequireMemberMFA: &models.ActivityChange[bool]{From: false, To: true},
			}},
		},
		{
			name: "member",
			msg: &authv1.ActivityDetails{Kind: &authv1.ActivityDetails_Member{Member: &authv1.ActivityMemberDetails{
				Role:              &authv1.ActivitySubaccountRoleChange{From: authv1.SubaccountRole_VIEWER, To: authv1.SubaccountRole_TRADER},
				BrokerRole:        &authv1.ActivityBrokerRoleChange{From: authv1.ActivityBrokerRole_ACTIVITY_BROKER_FINANCE, To: authv1.ActivityBrokerRole_ACTIVITY_BROKER_OWNER},
				OwnershipTransfer: true,
			}}},
			want: &models.ActivityDetails{Member: &models.ActivityMemberDetails{
				Role:              &models.ActivityChange[authv1.SubaccountRole]{From: authv1.SubaccountRole_VIEWER, To: authv1.SubaccountRole_TRADER},
				BrokerRole:        &models.ActivityChange[authv1.ActivityBrokerRole]{From: authv1.ActivityBrokerRole_ACTIVITY_BROKER_FINANCE, To: authv1.ActivityBrokerRole_ACTIVITY_BROKER_OWNER},
				OwnershipTransfer: true,
			}},
		},
		{
			name: "invite",
			msg: &authv1.ActivityDetails{Kind: &authv1.ActivityDetails_Invite{Invite: &authv1.ActivityInviteDetails{
				GranteeAccountId: 9000000041,
				Role:             authv1.SubaccountRole_TREASURY,
				BrokerRole:       authv1.ActivityBrokerRole_ACTIVITY_BROKER_AFFILIATE_MANAGER,
				Status:           &authv1.ActivityInviteStatusChange{From: authv1.SubaccountInviteStatus_SUBACCOUNT_INVITE_STATUS_PENDING, To: authv1.SubaccountInviteStatus_SUBACCOUNT_INVITE_STATUS_ACCEPTED},
			}}},
			want: &models.ActivityDetails{Invite: &models.ActivityInviteDetails{
				GranteeAccountID: codecs.FormatID(9000000041),
				Role:             authv1.SubaccountRole_TREASURY,
				BrokerRole:       authv1.ActivityBrokerRole_ACTIVITY_BROKER_AFFILIATE_MANAGER,
				Status:           &models.ActivityChange[authv1.SubaccountInviteStatus]{From: authv1.SubaccountInviteStatus_SUBACCOUNT_INVITE_STATUS_PENDING, To: authv1.SubaccountInviteStatus_SUBACCOUNT_INVITE_STATUS_ACCEPTED},
			}},
		},
		{
			name: "destination",
			msg: &authv1.ActivityDetails{Kind: &authv1.ActivityDetails_Destination{Destination: &authv1.ActivityDestinationDetails{
				Label:       &authv1.ActivityTextChange{From: "cold wallet", To: "cold wallet (old)"},
				NoteChanged: true,
				TagIds:      &authv1.ActivityIdListChange{AddedIds: []uint64{9000000051, 9000000052}, RemovedIds: []uint64{9000000053}},
			}}},
			want: &models.ActivityDetails{Destination: &models.ActivityDestinationDetails{
				Label:       &models.ActivityChange[string]{From: "cold wallet", To: "cold wallet (old)"},
				NoteChanged: true,
				TagIDs:      &models.ActivityListChange[string]{Added: []string{codecs.FormatID(9000000051), codecs.FormatID(9000000052)}, Removed: []string{codecs.FormatID(9000000053)}},
			}},
		},
		{
			name: "broker_allowlist",
			msg: &authv1.ActivityDetails{Kind: &authv1.ActivityDetails_BrokerAllowlist{BrokerAllowlist: &authv1.ActivityBrokerAllowlistDetails{
				Action:             authv1.ActivityAllowlistAction_ACTIVITY_ALLOWLIST_REMOVE,
				Destination:        "0x1111111111111111111111111111111111111111",
				DestinationChainId: 8453,
				Label:              "treasury",
			}}},
			want: &models.ActivityDetails{BrokerAllowlist: &models.ActivityBrokerAllowlistDetails{
				Action:             authv1.ActivityAllowlistAction_ACTIVITY_ALLOWLIST_REMOVE,
				Destination:        "0x1111111111111111111111111111111111111111",
				DestinationChainID: 8453,
				Label:              "treasury",
			}},
		},
		{
			name: "security",
			msg: &authv1.ActivityDetails{Kind: &authv1.ActivityDetails_Security{Security: &authv1.ActivitySecurityDetails{
				FactorType:           authv1.MFAFactorType_MFA_FACTOR_TYPE_PASSKEY,
				Label:                &authv1.ActivityTextChange{From: "laptop", To: "work laptop"},
				ChallengePurpose:     authv1.MFAChallengePurpose_MFA_CHALLENGE_PURPOSE_SESSION_ELEVATION,
				RecoveryCodesRotated: true,
			}}},
			want: &models.ActivityDetails{Security: &models.ActivitySecurityDetails{
				FactorType:           authv1.MFAFactorType_MFA_FACTOR_TYPE_PASSKEY,
				Label:                &models.ActivityChange[string]{From: "laptop", To: "work laptop"},
				ChallengePurpose:     authv1.MFAChallengePurpose_MFA_CHALLENGE_PURPOSE_SESSION_ELEVATION,
				RecoveryCodesRotated: true,
			}},
		},
	}
}

// Every kind of details the contract defines decodes with every field it
// carries: from/to pairs, scalar values, fromId/toId as public IDs,
// added/removed lists, and times to the nanosecond.
func TestActivityEventDecodesEveryDetailsKind(t *testing.T) {
	cases := activityDetailsCases()
	if got, want := len(cases), (&authv1.ActivityDetails{}).ProtoReflect().Descriptor().Oneofs().ByName("kind").Fields().Len(); got != want {
		t.Fatalf("%d details kinds tested, the contract has %d", got, want)
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := decodeActivityBothWays(t, &authv1.ActivityEvent{
				Actor:   &authv1.ActivityActor{Kind: authv1.ActivityActorKind_ACTIVITY_ACTOR_API_KEY, ApiKeyId: "ak_0123456789abcdef0123456789abcdef"},
				Item:    &authv1.ActivityItem{Id: &authv1.ActivityItem_AccountId{AccountId: 9000000001}, Name: "alice"},
				Details: tc.msg,
			})
			if !reflect.DeepEqual(got.Details, tc.want) {
				gotJSON, _ := json.Marshal(got.Details)
				wantJSON, _ := json.Marshal(tc.want)
				t.Fatalf("Details =\n%s\nwant\n%s", gotJSON, wantJSON)
			}
			if got.Actor == nil || got.Actor.APIKeyID != "ak_0123456789abcdef0123456789abcdef" || got.Item == nil || got.Item.Name != "alice" {
				t.Fatalf("Actor = %+v, Item = %+v: the actor and item must survive beside the details", got.Actor, got.Item)
			}
			// Exactly the one kind is set, and every field of it is filled.
			set := 0
			details := reflect.ValueOf(got.Details).Elem()
			for i := range details.NumField() {
				if !details.Field(i).IsNil() {
					set++
					requireEveryFieldSet(t, details.Type().Field(i).Name, details.Field(i))
				}
			}
			if set != 1 {
				t.Fatalf("%d details kinds set, want 1", set)
			}
		})
	}
}

// Detail times keep their nanoseconds: two times a nanosecond apart stay
// apart, and none is cut to the millisecond.
func TestActivityDetailTimesKeepNanoseconds(t *testing.T) {
	fromPB, from := activityTime(1_794_700_000, 250_000_000)
	toPB, to := activityTime(1_794_700_000, 250_000_001)
	got := decodeActivityBothWays(t, &authv1.ActivityEvent{Details: &authv1.ActivityDetails{Kind: &authv1.ActivityDetails_ApiKey{ApiKey: &authv1.ActivityApiKeyDetails{
		ExpiresAt: &authv1.ActivityTimeChange{From: fromPB, To: toPB},
	}}}})
	change := got.Details.APIKey.ExpiresAt
	if change == nil || change.From == nil || change.To == nil {
		t.Fatalf("ExpiresAt = %+v, want both sides", change)
	}
	if !change.From.Equal(*from) || !change.To.Equal(*to) {
		t.Fatalf("ExpiresAt = %v -> %v, want %v -> %v", change.From, change.To, from, to)
	}
	if change.To.Sub(*change.From) != time.Nanosecond {
		t.Fatalf("ExpiresAt sides differ by %v, want 1ns", change.To.Sub(*change.From))
	}
	if change.To.Unix() != 1_794_700_000 || change.To.Nanosecond() != 250_000_001 {
		t.Fatalf("ExpiresAt.To = %d s %d ns, want 1794700000 s 250000001 ns", change.To.Unix(), change.To.Nanosecond())
	}
}

// Unset sides and values read as the contract leaves them: "" for text and
// IDs, nil for times, the ..._UNSPECIFIED constant for enums; an enum value
// newer than this SDK keeps its number.
func TestActivityDetailsKeepUnsetAndUnknownValues(t *testing.T) {
	expiryPB, expiry := activityTime(1_797_300_000, 42)
	got := decodeActivityBothWays(t, &authv1.ActivityEvent{Details: &authv1.ActivityDetails{Kind: &authv1.ActivityDetails_ApiKey{ApiKey: &authv1.ActivityApiKeyDetails{
		Label:       &authv1.ActivityTextChange{To: "new"},
		Status:      &authv1.ActivityApiKeyStatusChange{From: authv1.ApiKeyStatus_API_KEY_STATUS_UNSPECIFIED, To: authv1.ApiKeyStatus(99)},
		ExpiresAt:   &authv1.ActivityTimeChange{To: expiryPB},
		ApiPolicyId: &authv1.ActivityIdChange{ToId: 9000000022},
	}}}})
	want := &models.ActivityAPIKeyDetails{
		Label:       &models.ActivityChange[string]{To: "new"},
		Status:      &models.ActivityChange[authv1.ApiKeyStatus]{From: authv1.ApiKeyStatus_API_KEY_STATUS_UNSPECIFIED, To: authv1.ApiKeyStatus(99)},
		ExpiresAt:   &models.ActivityChange[*time.Time]{To: expiry},
		APIPolicyID: &models.ActivityChange[string]{To: codecs.FormatID(9000000022)},
	}
	if !reflect.DeepEqual(got.Details.APIKey, want) {
		t.Fatalf("APIKey = %+v, want %+v", got.Details.APIKey, want)
	}

	got = decodeActivityBothWays(t, &authv1.ActivityEvent{Details: &authv1.ActivityDetails{Kind: &authv1.ActivityDetails_Policy{Policy: &authv1.ActivityPolicyDetails{
		Actions:         &authv1.ActivityActionsChange{Added: []authv1.PolicyAction{authv1.PolicyAction(9999)}},
		AssistantPreset: authv1.AssistantKeyPreset(77),
	}}}})
	if a := got.Details.Policy.Actions; a == nil || !reflect.DeepEqual(a.Added, []authv1.PolicyAction{9999}) || a.Removed != nil {
		t.Fatalf("Actions = %+v, want added [9999]", a)
	}
	if got.Details.Policy.AssistantPreset != 77 {
		t.Fatalf("AssistantPreset = %v, want 77", got.Details.Policy.AssistantPreset)
	}

	got = decodeActivityBothWays(t, &authv1.ActivityEvent{Details: &authv1.ActivityDetails{Kind: &authv1.ActivityDetails_Member{Member: &authv1.ActivityMemberDetails{
		Role:       &authv1.ActivitySubaccountRoleChange{To: authv1.SubaccountRole_ADMIN},
		BrokerRole: &authv1.ActivityBrokerRoleChange{From: authv1.ActivityBrokerRole(42)},
	}}}})
	wantMember := &models.ActivityMemberDetails{
		Role:       &models.ActivityChange[authv1.SubaccountRole]{From: authv1.SubaccountRole_SUBACCOUNT_ROLE_UNSPECIFIED, To: authv1.SubaccountRole_ADMIN},
		BrokerRole: &models.ActivityChange[authv1.ActivityBrokerRole]{From: 42, To: authv1.ActivityBrokerRole_ACTIVITY_BROKER_ROLE_UNSPECIFIED},
	}
	if !reflect.DeepEqual(got.Details.Member, wantMember) {
		t.Fatalf("Member = %+v, want %+v", got.Details.Member, wantMember)
	}
}
