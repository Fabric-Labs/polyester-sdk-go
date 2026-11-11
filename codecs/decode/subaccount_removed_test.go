package decode_test

import (
	"testing"
	"time"

	"github.com/Fabric-Labs/polyester-sdk-go/codecs/decode"
	authv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/auth/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func subaccountFixture(revision uint64, removed bool, role authv1.SubaccountRole) *authv1.Subaccount {
	return &authv1.Subaccount{
		Id: 42, Role: role, Label: "desk", Status: authv1.SubaccountStatus_SUBACCOUNT_STATUS_ACTIVE,
		UpdatedAt: timestamppb.New(time.Unix(1_700_000_000, 0)), Revision: revision, Removed: removed,
	}
}

// subaccountDecodes decodes msg through the stream decoder (binary) and the
// list and get decoders after binary and protojson round trips.
func subaccountDecodes(t *testing.T, msg *authv1.Subaccount) map[string]models.SubAccount {
	t.Helper()
	payload, err := proto.Marshal(msg)
	if err != nil {
		t.Fatal(err)
	}
	streamed, err := decode.SubaccountFromBytes(payload)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]models.SubAccount{"stream": streamed}
	jsonPayload, err := protojson.Marshal(msg)
	if err != nil {
		t.Fatal(err)
	}
	var fromJSON authv1.Subaccount
	if err := protojson.Unmarshal(jsonPayload, &fromJSON); err != nil {
		t.Fatal(err)
	}
	var fromBinary authv1.Subaccount
	if err := proto.Unmarshal(payload, &fromBinary); err != nil {
		t.Fatal(err)
	}
	for wire, m := range map[string]*authv1.Subaccount{"binary": &fromBinary, "json": &fromJSON} {
		if m.GetRemoved() != msg.GetRemoved() || m.GetRevision() != msg.GetRevision() {
			t.Fatalf("%s generated round trip lost removed/revision: %+v", wire, m)
		}
		list := decode.SubaccountsListFromProto(&authv1.ListSubaccountsResponse{Subaccounts: []*authv1.Subaccount{m}})
		out[wire+" list"] = list.Subaccounts[0]
		out[wire+" get"] = *decode.GetSubaccountFromProto(&authv1.GetSubaccountResponse{Subaccount: m}).Subaccount
	}
	return out
}

// POLY-9397: a removal update keeps removed and reports the role as the
// contract sends it (unspecified reads as ""), so it never reads like an
// active viewer, even with the same revision and update time.
func TestPoly9397SubaccountUpdateKeepsRemovedAndRole(t *testing.T) {
	for _, revision := range []uint64{0, 3, 9007199254740993} {
		removal := subaccountDecodes(t, subaccountFixture(revision, true, authv1.SubaccountRole_SUBACCOUNT_ROLE_UNSPECIFIED))
		viewer := subaccountDecodes(t, subaccountFixture(revision, false, authv1.SubaccountRole_VIEWER))
		for path, got := range removal {
			if !got.Removed || got.Role != "" || got.Revision != revision {
				t.Errorf("revision %d %s removal: Removed=%v Role=%q Revision=%d, want true, \"\", %d", revision, path, got.Removed, got.Role, got.Revision, revision)
			}
			active := viewer[path]
			if active.Removed || active.Role != "viewer" || active.Revision != revision {
				t.Errorf("revision %d %s viewer: Removed=%v Role=%q Revision=%d, want false, viewer, %d", revision, path, active.Removed, active.Role, active.Revision, revision)
			}
			// UpdatedAt is a pointer: compare the times, then the rest.
			if !got.UpdatedAt.Equal(*active.UpdatedAt) {
				t.Fatalf("revision %d %s: fixtures differ in updated_at", revision, path)
			}
			gotRest, activeRest := got, active
			gotRest.UpdatedAt, activeRest.UpdatedAt = nil, nil
			if gotRest == activeRest {
				t.Errorf("revision %d %s: removal and active viewer decode identically: %+v", revision, path, got)
			}
		}
	}
	// A role change without removal keeps the new role.
	for role, want := range map[authv1.SubaccountRole]string{
		authv1.SubaccountRole_TRADER:           "trader",
		authv1.SubaccountRole_LEVERAGED_TRADER: "leveraged_trader",
		authv1.SubaccountRole_TREASURY:         "treasury",
		authv1.SubaccountRole_ADMIN:            "admin",
		authv1.SubaccountRole_OWNER:            "owner",
	} {
		for path, got := range subaccountDecodes(t, subaccountFixture(3, false, role)) {
			if got.Role != want || got.Removed {
				t.Errorf("%s %v: Role=%q Removed=%v, want %q false", path, role, got.Role, got.Removed, want)
			}
		}
	}
}
