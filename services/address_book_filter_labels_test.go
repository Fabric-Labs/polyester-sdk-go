package services

import (
	"context"
	stderrors "errors"
	"strings"
	"testing"

	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	authv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/auth/v1"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// An unknown kind or direction filter is refused before sending; it used to
// be sent as "unspecified", which the server reads as no filter at all.
func TestAddressBookFiltersRefuseUnknownLabelsBeforeSending(t *testing.T) {
	ctx := context.Background()
	bogus := "totally_invalid_kind"
	for name, call := range map[string]func(*AddressBookService) error{
		"ListEntries kind": func(s *AddressBookService) error {
			_, err := s.ListEntries(ctx, nil, nil, &bogus, 10, nil)
			return err
		},
		"ListTransferCounterparties direction": func(s *AddressBookService) error {
			_, err := s.ListTransferCounterparties(ctx, nil, nil, &bogus, nil, 10)
			return err
		},
		"ListTransferCounterparties kind": func(s *AddressBookService) error {
			_, err := s.ListTransferCounterparties(ctx, nil, nil, nil, &bogus, 10)
			return err
		},
		"ListTransferDestinations kind": func(s *AddressBookService) error {
			_, err := s.ListTransferDestinations(ctx, nil, nil, bogus, 10, nil)
			return err
		},
	} {
		t.Run(name, func(t *testing.T) {
			factory, captures := scopeCaptureFactory(t)
			err := call(NewAddressBookService(factory, nil, nil, nil))
			var validation *sdkerrors.ValidationError
			if !stderrors.As(err, &validation) {
				t.Fatalf("err=%v, want *errors.ValidationError", err)
			}
			if !strings.Contains(validation.Msg, "external") && !strings.Contains(validation.Msg, "deposit_from") {
				t.Errorf("refusal %q does not name the accepted values", validation.Msg)
			}
			select {
			case req := <-captures:
				t.Fatalf("sent %v despite the unknown label", req)
			default:
			}
		})
	}
}

// Every accepted label, and an omitted filter, is sent as before.
func TestAddressBookFiltersSendAcceptedLabels(t *testing.T) {
	ctx := context.Background()
	field := func(t *testing.T, msg proto.Message, name string) protoreflect.EnumNumber {
		t.Helper()
		m := msg.ProtoReflect()
		return m.Get(m.Descriptor().Fields().ByName(protoreflect.Name(name))).Enum()
	}
	kinds := map[string]authv1.AddressBookEntryKind{
		"external": authv1.AddressBookEntryKind_EXTERNAL_CHAIN, "External_Chain": authv1.AddressBookEntryKind_EXTERNAL_CHAIN,
		"internal": authv1.AddressBookEntryKind_INTERNAL_ACCOUNT, "INTERNAL_ACCOUNT": authv1.AddressBookEntryKind_INTERNAL_ACCOUNT,
		"": authv1.AddressBookEntryKind_ENTRY_KIND_UNSPECIFIED,
	}
	for label, want := range kinds {
		factory, captures := scopeCaptureFactory(t)
		kind := label
		_, _ = NewAddressBookService(factory, nil, nil, nil).ListEntries(ctx, nil, nil, &kind, 10, nil)
		if got := field(t, <-captures, "kind"); got != protoreflect.EnumNumber(want) {
			t.Errorf("ListEntries kind %q sent %v, want %v", label, got, want)
		}
	}
	directions := map[string]authv1.TransferCounterpartyDirection{
		"deposit_from": authv1.TransferCounterpartyDirection_DEPOSIT_FROM, "incoming": authv1.TransferCounterpartyDirection_DEPOSIT_FROM,
		"withdraw_to": authv1.TransferCounterpartyDirection_WITHDRAW_TO, "OUTGOING": authv1.TransferCounterpartyDirection_WITHDRAW_TO,
		"internal_transfer_from": authv1.TransferCounterpartyDirection_INTERNAL_TRANSFER_FROM,
		"internal_transfer_to":   authv1.TransferCounterpartyDirection_INTERNAL_TRANSFER_TO,
	}
	for label, want := range directions {
		factory, captures := scopeCaptureFactory(t)
		direction := label
		_, _ = NewAddressBookService(factory, nil, nil, nil).ListTransferCounterparties(ctx, nil, nil, &direction, nil, 10)
		if got := field(t, <-captures, "direction"); got != protoreflect.EnumNumber(want) {
			t.Errorf("ListTransferCounterparties direction %q sent %v, want %v", label, got, want)
		}
	}
	factory, captures := scopeCaptureFactory(t)
	_, _ = NewAddressBookService(factory, nil, nil, nil).ListTransferDestinations(ctx, nil, nil, "", 10, nil)
	if got := field(t, <-captures, "kind"); got != protoreflect.EnumNumber(authv1.AddressBookEntryKind_INTERNAL_ACCOUNT) {
		t.Errorf("ListTransferDestinations default kind sent %v, want INTERNAL_ACCOUNT", got)
	}
}
