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

	apiPolicy := decode.ApiPolicyMessageFromProto(&authv1.ApiPolicyView{Id: 4, Revision: 6})
	if apiPolicy == nil || apiPolicy.Revision != 6 {
		t.Fatalf("api policy=%+v", apiPolicy)
	}

	entries := decode.ListEntriesFromProto(&authv1.ListAddressBookEntriesResponse{
		Entries: []*authv1.AddressBookEntry{{AddressBookEntryId: 9, Revision: 15}},
	})
	if len(entries.Entries) != 1 || entries.Entries[0].Revision != 15 {
		t.Fatalf("entries=%+v", entries.Entries)
	}
}
