package decode_test

import (
	"errors"
	"testing"

	"github.com/Fabric-Labs/polyester-sdk-go/codecs/decode"
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	authv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/auth/v1"
	"google.golang.org/protobuf/proto"
)

// lineageCases are a policy's template lineage as the contract defines it: a
// copy names its template and the template's revision when it was copied, and
// a policy that is no copy names neither. Anything else is malformed.
var lineageCases = []struct {
	name       string
	templateID uint64
	revision   uint64
	valid      bool
}{
	{"not a copy", 0, 0, true},
	{"a copy", 9, 3, true},
	{"a copy without its revision", 9, 0, false},
	{"a revision without its template", 0, 3, false},
}

func checkLineage(t *testing.T, name string, valid bool, err error) {
	t.Helper()
	var contract *sdkerrors.ResponseContractError
	if valid && err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	if !valid && !errors.As(err, &contract) {
		t.Fatalf("%s: err = %v, want a ResponseContractError", name, err)
	}
}

func TestSubaccountPolicyStreamRequiresACompleteTemplateLineage(t *testing.T) {
	for _, tc := range lineageCases {
		payload, _ := proto.Marshal(&authv1.SubaccountPolicyView{Id: 1, SourceTemplateId: tc.templateID, SourceTemplateRevision: tc.revision})
		policy, err := decode.SubaccountPolicyFromBytes(payload)
		checkLineage(t, tc.name, tc.valid, err)
		if tc.valid && policy.SourceTemplateRevision != tc.revision {
			t.Fatalf("%s: revision %d", tc.name, policy.SourceTemplateRevision)
		}
	}
}

func TestAPIPolicyStreamRequiresACompleteTemplateLineage(t *testing.T) {
	for _, tc := range lineageCases {
		payload, _ := proto.Marshal(&authv1.ApiPolicyView{Id: 1, SourceTemplateId: tc.templateID, SourceTemplateRevision: tc.revision})
		policy, err := decode.ApiPolicyFromBytes(payload)
		checkLineage(t, tc.name, tc.valid, err)
		if tc.valid && policy.SourceTemplateRevision != tc.revision {
			t.Fatalf("%s: revision %d", tc.name, policy.SourceTemplateRevision)
		}
	}
}

func TestGetSubaccountPolicyRequiresACompleteTemplateLineage(t *testing.T) {
	for _, tc := range lineageCases {
		_, err := decode.GetSubaccountFromProto(&authv1.GetSubaccountResponse{
			Subaccount: &authv1.Subaccount{Id: 5},
			Policy:     &authv1.SubaccountPolicyView{Id: 1, SourceTemplateId: tc.templateID, SourceTemplateRevision: tc.revision},
		})
		checkLineage(t, tc.name, tc.valid, err)
	}
}
