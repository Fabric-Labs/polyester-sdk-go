package decode_test

import (
	"testing"

	"github.com/Fabric-Labs/polyester-sdk-go/codecs/decode"
	authv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/auth/v1"
)

func TestDecodeRevisionFields(t *testing.T) {
	sub := decode.SubaccountMessageFromProto(&authv1.Subaccount{Id: 1, Revision: 12})
	if sub.Revision != 12 {
		t.Fatalf("subaccount revision=%d", sub.Revision)
	}

	key := decode.ApiKeyMessageFromProto(&authv1.ApiKey{KeyId: "ak_1", Revision: 3})
	if key == nil || key.Revision != 3 {
		t.Fatalf("api key=%+v", key)
	}

	policy := decode.SubaccountPolicyMessageFromProto(&authv1.SubaccountPolicyView{Id: 2, Revision: 8})
	if policy == nil || policy.Revision != 8 {
		t.Fatalf("subaccount policy=%+v", policy)
	}

	apiPolicy := decode.GetApiPolicyFromProto(&authv1.GetApiPolicyResponse{
		Policy: &authv1.ApiPolicyView{Id: 4, Revision: 6},
	})
	if apiPolicy == nil || apiPolicy.Revision != 6 {
		t.Fatalf("api policy=%+v", apiPolicy)
	}

	entry := decode.EntryFromUpdateProto(&authv1.UpdateAddressBookEntryResponse{
		Entry: &authv1.AddressBookEntry{AddressBookEntryId: 9, Revision: 15},
	})
	if entry.Revision != 15 {
		t.Fatalf("entry revision=%d", entry.Revision)
	}

	updated := decode.UpdateSubaccountFromProto(&authv1.UpdateSubaccountResponse{
		Subaccount: &authv1.Subaccount{Id: 7, Label: "x", Revision: 2},
	})
	if updated == nil || updated.Revision != 2 || updated.Label != "x" {
		t.Fatalf("updated subaccount=%+v", updated)
	}

	created := decode.CreateSubaccountFromProto(&authv1.CreateSubaccountResponse{
		SubaccountId: 11,
		Revision:     1,
	})
	if created.SubaccountID == "" || created.Revision != 1 {
		t.Fatalf("create subaccount=%+v", created)
	}
}
