package decode_test

import (
	"errors"
	"testing"

	"github.com/Fabric-Labs/polyester-sdk-go/codecs/decode"
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	authv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/auth/v1"
	orderv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/orders/v1"
	triggersv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/triggers/v1"
)

func ignoreResult[T any](_ T, err error) error { return err }

// A reply that must name what it created or changed refuses ID 0, the proto
// default of an absent field, with a ResponseContractError instead of
// reporting the ID "0"; the same reply with a real ID decodes.
func TestReplyDecodersRefuseZeroRequiredID(t *testing.T) {
	cases := []struct {
		name  string
		zero  func() error
		valid func() error
	}{
		{"CreateTrigger",
			func() error {
				return ignoreResult(decode.TriggerMutationFromProto(&triggersv1.CreateTriggerResponse{}))
			},
			func() error {
				return ignoreResult(decode.TriggerMutationFromProto(&triggersv1.CreateTriggerResponse{TriggerId: 7}))
			}},
		{"CancelTrigger",
			func() error {
				return ignoreResult(decode.TriggerMutationFromCancel(&triggersv1.CancelTriggerResponse{}))
			},
			func() error {
				return ignoreResult(decode.TriggerMutationFromCancel(&triggersv1.CancelTriggerResponse{TriggerId: 7}))
			}},
		{"PauseTrigger",
			func() error { return ignoreResult(decode.TriggerMutationFromPause(&triggersv1.PauseTriggerResponse{})) },
			func() error {
				return ignoreResult(decode.TriggerMutationFromPause(&triggersv1.PauseTriggerResponse{TriggerId: 7}))
			}},
		{"ResumeTrigger",
			func() error {
				return ignoreResult(decode.TriggerMutationFromResume(&triggersv1.ResumeTriggerResponse{}))
			},
			func() error {
				return ignoreResult(decode.TriggerMutationFromResume(&triggersv1.ResumeTriggerResponse{TriggerId: 7}))
			}},
		{"ModifyTrigger",
			func() error {
				return ignoreResult(decode.TriggerMutationFromModify(&triggersv1.ModifyTriggerResponse{}))
			},
			func() error {
				return ignoreResult(decode.TriggerMutationFromModify(&triggersv1.ModifyTriggerResponse{TriggerId: 7}))
			}},
		{"CreateOco order_list_id",
			func() error {
				return ignoreResult(decode.CreateOcoFromProto(&triggersv1.CreateOcoResponse{TakeProfitTriggerId: 2, StopLossTriggerId: 3}))
			},
			func() error {
				return ignoreResult(decode.CreateOcoFromProto(&triggersv1.CreateOcoResponse{OrderListId: 1, TakeProfitTriggerId: 2, StopLossTriggerId: 3}))
			}},
		{"CreateOco take_profit_trigger_id",
			func() error {
				return ignoreResult(decode.CreateOcoFromProto(&triggersv1.CreateOcoResponse{OrderListId: 1, StopLossTriggerId: 3}))
			}, nil},
		{"CreateOco stop_loss_trigger_id",
			func() error {
				return ignoreResult(decode.CreateOcoFromProto(&triggersv1.CreateOcoResponse{OrderListId: 1, TakeProfitTriggerId: 2}))
			}, nil},
		{"CancelOco",
			func() error { return ignoreResult(decode.CancelOcoFromProto(&triggersv1.CancelOcoResponse{})) },
			func() error {
				return ignoreResult(decode.CancelOcoFromProto(&triggersv1.CancelOcoResponse{OrderListId: 1}))
			}},
		{"BatchCancelTriggers item",
			func() error {
				return ignoreResult(decode.BatchCancelTriggersFromProto(&triggersv1.BatchCancelTriggersResponse{
					Results:       []*triggersv1.BatchCancelTriggersResultItem{{Status: triggersv1.BatchCancelTriggersResultItem_ACCEPTED}},
					AcceptedCount: 1,
				}))
			},
			func() error {
				return ignoreResult(decode.BatchCancelTriggersFromProto(&triggersv1.BatchCancelTriggersResponse{
					Results:       []*triggersv1.BatchCancelTriggersResultItem{{Status: triggersv1.BatchCancelTriggersResultItem_ACCEPTED, TriggerId: 7}},
					AcceptedCount: 1,
				}))
			}},
		{"BatchCreateOrders accepted item",
			func() error {
				return ignoreResult(decode.BatchCreateFromProto(&orderv1.BatchCreateOrdersResponse{
					Results: []*orderv1.BatchCreateResultItem{{Outcome: &orderv1.BatchCreateResultItem_Accepted{
						Accepted: &orderv1.BatchCreateAccepted{},
					}}},
					AcceptedCount: 1,
				}))
			},
			func() error {
				return ignoreResult(decode.BatchCreateFromProto(&orderv1.BatchCreateOrdersResponse{
					Results: []*orderv1.BatchCreateResultItem{{Outcome: &orderv1.BatchCreateResultItem_Accepted{
						Accepted: &orderv1.BatchCreateAccepted{OrderId: 9},
					}}},
					AcceptedCount: 1,
				}))
			}},
		{"CancelAllOrders cancelled order",
			func() error {
				return ignoreResult(decode.CancelAllFromProto(&orderv1.CancelAllOrdersResponse{
					Status: orderv1.CancelAllOrdersResponse_SUBMITTED, MatchedOrders: 1, SubmittedCancels: 1, TotalCancelled: 1,
					CancelledOrders: []*orderv1.CancelledOrder{{ClientOrderId: "c1"}},
				}))
			},
			func() error {
				return ignoreResult(decode.CancelAllFromProto(&orderv1.CancelAllOrdersResponse{
					Status: orderv1.CancelAllOrdersResponse_SUBMITTED, MatchedOrders: 1, SubmittedCancels: 1, TotalCancelled: 1,
					CancelledOrders: []*orderv1.CancelledOrder{{OrderId: 9, ClientOrderId: "c1"}},
				}))
			}},
		{"CreateAddressBookEntry",
			func() error {
				return ignoreResult(decode.EntryFromCreateProto(&authv1.CreateAddressBookEntryResponse{Entry: &authv1.AddressBookEntry{}}))
			},
			func() error {
				return ignoreResult(decode.EntryFromCreateProto(&authv1.CreateAddressBookEntryResponse{Entry: &authv1.AddressBookEntry{AddressBookEntryId: 4}}))
			}},
		{"UpdateAddressBookEntry",
			func() error {
				return ignoreResult(decode.EntryFromUpdateProto(&authv1.UpdateAddressBookEntryResponse{Entry: &authv1.AddressBookEntry{}}))
			},
			func() error {
				return ignoreResult(decode.EntryFromUpdateProto(&authv1.UpdateAddressBookEntryResponse{Entry: &authv1.AddressBookEntry{AddressBookEntryId: 4}}))
			}},
		{"CreateAddressBookTag",
			func() error {
				return ignoreResult(decode.TagFromCreateProto(&authv1.CreateAddressBookTagResponse{Tag: &authv1.AddressBookTag{}}))
			},
			func() error {
				return ignoreResult(decode.TagFromCreateProto(&authv1.CreateAddressBookTagResponse{Tag: &authv1.AddressBookTag{TagId: 5}}))
			}},
		{"UpdateAddressBookTag",
			func() error {
				return ignoreResult(decode.TagFromUpdateProto(&authv1.UpdateAddressBookTagResponse{Tag: &authv1.AddressBookTag{}}))
			},
			func() error {
				return ignoreResult(decode.TagFromUpdateProto(&authv1.UpdateAddressBookTagResponse{Tag: &authv1.AddressBookTag{TagId: 5}}))
			}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var contract *sdkerrors.ResponseContractError
			if err := tc.zero(); !errors.As(err, &contract) {
				t.Fatalf("zero ID: got %v, want a ResponseContractError", err)
			}
			if tc.valid != nil {
				if err := tc.valid(); err != nil {
					t.Fatalf("real ID: %v", err)
				}
			}
		})
	}
}

// A decoded trigger mutation keeps the reply's ID and status.
func TestTriggerMutationKeepsRealID(t *testing.T) {
	got, err := decode.TriggerMutationFromCancel(&triggersv1.CancelTriggerResponse{TriggerId: 7, Status: triggersv1.TriggerStatus_STATUS_CANCELED})
	if err != nil {
		t.Fatal(err)
	}
	if got.TriggerID != "8" || got.Status != "canceled" {
		t.Fatalf("got %+v", got)
	}
	created, err := decode.TriggerMutationFromProto(&triggersv1.CreateTriggerResponse{TriggerId: 7})
	if err != nil {
		t.Fatal(err)
	}
	if created.TriggerID != "8" || created.Status != "accepted" {
		t.Fatalf("got %+v", created)
	}
}
