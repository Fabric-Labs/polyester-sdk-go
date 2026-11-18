package decode_test

import (
	"errors"
	"testing"

	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	"github.com/Fabric-Labs/polyester-sdk-go/codecs/decode"
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	authv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/auth/v1"
	"google.golang.org/protobuf/proto"
)

func TestMeFromProto(t *testing.T) {
	apiKeyID := "ak_0123456789abcdef0123456789abcdef"
	msg := &authv1.MeResponse{
		AccountId:               42,
		ApiKeyId:                &apiKeyID,
		Username:                "alice",
		RootSmartAccountAddress: "0xabc",
	}
	me := mustReply(decode.MeFromProto(msg))
	if me.AccountID != codecs.FormatUint64ID(42) {
		t.Fatalf("account_id=%q", me.AccountID)
	}
	if me.APIKeyID != apiKeyID {
		t.Fatalf("api_key_id=%q", me.APIKeyID)
	}
	if me.Username != "alice" || me.RootSmartAccountAddress != "0xabc" {
		t.Fatalf("me=%+v", me)
	}
}

func TestSubaccountStatusFromProto(t *testing.T) {
	active := decode.SubaccountMessageFromProto(&authv1.Subaccount{
		Id:     12,
		Status: authv1.SubaccountStatus_SUBACCOUNT_STATUS_ACTIVE,
	})
	if active.Status != "active" {
		t.Fatalf("active=%q", active.Status)
	}
	disabled := decode.SubaccountMessageFromProto(&authv1.Subaccount{
		Status: authv1.SubaccountStatus_SUBACCOUNT_STATUS_DISABLED,
	})
	if disabled.Status != "disabled" {
		t.Fatalf("disabled=%q", disabled.Status)
	}
	unspecified := decode.SubaccountMessageFromProto(&authv1.Subaccount{})
	if unspecified.Status != "" {
		t.Fatalf("unspecified=%q", unspecified.Status)
	}
}

func TestSubaccountActivityFromProtoNormalizesTypedEnums(t *testing.T) {
	msg := &authv1.ListSubaccountEventsResponse{
		Events: []*authv1.ActivityEvent{{
			EntityKind:  authv1.ActivityEntityKind_ACTIVITY_ENTITY_INVITE,
			EventAction: authv1.ActivityEventAction_ACTIVITY_ACTION_CREATED,
		}},
	}
	result := decode.SubaccountActivityListFromProto(msg)
	if len(result.Events) != 1 || result.Events[0].EventType != "invite:created" {
		t.Fatalf("events=%+v", result.Events)
	}
}

// Me names the account an API key is scoped to, and a sub-account key's
// sub-account; a sub-account key without its sub-account ID breaks the
// contract.
func TestMeNamesTheAPIKeyScope(t *testing.T) {
	sub, err := decode.MeFromProto(&authv1.MeResponse{AccountId: 7, ApiKeyId: proto.String("ak_1"), ApiKeyScope: authv1.AccountScopeType_SCOPE_SUBACCOUNT, SubaccountId: proto.Uint64(9000000001)})
	if err != nil || sub.APIKeyScope != "subaccount" || sub.SubaccountID != codecs.FormatID(9000000001) || sub.ChannelAccountID() != codecs.FormatID(9000000001) {
		t.Fatalf("sub-account key: %+v, %v", sub, err)
	}
	root, err := decode.MeFromProto(&authv1.MeResponse{AccountId: 7, ApiKeyId: proto.String("ak_2"), ApiKeyScope: authv1.AccountScopeType_SCOPE_ROOT})
	if err != nil || root.APIKeyScope != "root" || root.SubaccountID != "" || root.ChannelAccountID() != codecs.FormatID(7) {
		t.Fatalf("root key: %+v, %v", root, err)
	}
	session, err := decode.MeFromProto(&authv1.MeResponse{AccountId: 7})
	if err != nil || session.APIKeyScope != "" || session.SubaccountID != "" {
		t.Fatalf("session: %+v, %v", session, err)
	}
	var contract *sdkerrors.ResponseContractError
	if _, err := decode.MeFromProto(&authv1.MeResponse{AccountId: 7, ApiKeyScope: authv1.AccountScopeType_SCOPE_SUBACCOUNT}); !errors.As(err, &contract) {
		t.Fatalf("sub-account key without its ID: err = %v", err)
	}
}
