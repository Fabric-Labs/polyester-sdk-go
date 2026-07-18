package decode_test

import (
	"testing"

	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	"github.com/Fabric-Labs/polyester-sdk-go/codecs/decode"
	authv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/auth/v1"
)

func TestMeFromProto(t *testing.T) {
	apiKeyID := uint64(99)
	msg := &authv1.MeResponse{
		AccountId:               42,
		ApiKeyId:                &apiKeyID,
		Username:                "alice",
		RootSmartAccountAddress: "0xabc",
	}
	me := decode.MeFromProto(msg)
	if me.AccountID != codecs.FormatUint64ID(42) {
		t.Fatalf("account_id=%q", me.AccountID)
	}
	if me.APIKeyID != codecs.FormatUint64ID(99) {
		t.Fatalf("api_key_id=%q", me.APIKeyID)
	}
	if me.Username != "alice" || me.RootSmartAccountAddress != "0xabc" {
		t.Fatalf("me=%+v", me)
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
