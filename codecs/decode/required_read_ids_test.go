package decode_test

import (
	"errors"
	"testing"

	"github.com/Fabric-Labs/polyester-sdk-go/codecs/decode"
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	alertsv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/alerts/v1"
	authv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/auth/v1"
	analyticsv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/chain/analytics/v1"
	lifecyclev1 "github.com/Fabric-Labs/polyester-sdk-go/gen/chain/lifecycle/v1"
	changelogv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/changelog/v1"
	ledgerrdv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/ledger/read/v1"
	orderv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/orders/v1"
	systemv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/system/v1"
	triggersv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/triggers/v1"
)

// Every read reply that carries an object's required ID (see
// testdata/reply_id_inventory.tsv) refuses 0 (or an empty string ID) with a
// ResponseContractError; the same reply with a real ID decodes. A nested
// message the reply may leave out (an order's lineage, the caller's session,
// a sub-account's policy, a destination's saved entry) is checked only when
// present.
func TestReadReplyDecodersRefuseZeroRequiredID(t *testing.T) {
	entry := func(id, tagID uint64) *authv1.AddressBookEntry {
		return &authv1.AddressBookEntry{AddressBookEntryId: id, Tags: []*authv1.AddressBookTag{{TagId: tagID}}}
	}
	order := func(id, lineageID uint64) *orderv1.Order {
		return &orderv1.Order{OrderId: id, Lineage: &orderv1.OrderLineage{Id: lineageID, Generation: 1}}
	}
	trade := func(lineageID uint64) *orderv1.UserTrade {
		return &orderv1.UserTrade{OrderId: 9, Lineage: &orderv1.OrderLineage{Id: lineageID, Generation: 1}}
	}
	invite := func(id, subaccountID uint64) *authv1.SubaccountInvite {
		return &authv1.SubaccountInvite{Id: id, SubaccountId: subaccountID}
	}
	subaccount := func(id uint64, keyID string, inv *authv1.SubaccountInvite, policyID uint64) *authv1.GetSubaccountResponse {
		return &authv1.GetSubaccountResponse{
			Subaccount: &authv1.Subaccount{Id: id},
			ApiKeys:    []*authv1.ApiKey{{KeyId: keyID}},
			Invites:    []*authv1.SubaccountInvite{inv},
			Policy:     &authv1.SubaccountPolicyView{Id: policyID},
		}
	}
	post := func(changeID uint64) *changelogv1.GetPostResponse {
		return &changelogv1.GetPostResponse{Post: &changelogv1.Post{
			Summary:  &changelogv1.PostSummary{},
			Sections: []*changelogv1.Section{{Changes: []*changelogv1.Change{{Id: changeID}}}},
		}}
	}
	group := func(changeID uint64) *changelogv1.GetDeploymentGroupResponse {
		return &changelogv1.GetDeploymentGroupResponse{Group: &changelogv1.DeploymentGroup{
			Summary:   &changelogv1.DeploymentGroupSummary{},
			Attention: []*changelogv1.DeploymentGroupAttention{{Change: &changelogv1.Change{Id: changeID}}},
		}}
	}
	flow := func(id string) *lifecyclev1.GetFlowResponse {
		return &lifecyclev1.GetFlowResponse{Flow: &lifecyclev1.FlowDetailView{Summary: &lifecyclev1.FlowSummaryView{FlowId: id}}}
	}
	cases := []struct {
		name        string
		zero, valid func() error
	}{
		{"GetTrigger",
			func() error {
				return ignoreResult(decode.GetTriggerFromProto(&triggersv1.GetTriggerResponse{Trigger: &triggersv1.Trigger{}}))
			},
			func() error {
				return ignoreResult(decode.GetTriggerFromProto(&triggersv1.GetTriggerResponse{Trigger: &triggersv1.Trigger{TriggerId: 7}}))
			}},
		{"ListTriggers",
			func() error {
				return ignoreResult(decode.TriggersListFromProto(&triggersv1.ListTriggersResponse{Triggers: []*triggersv1.Trigger{{}}}))
			},
			func() error {
				return ignoreResult(decode.TriggersListFromProto(&triggersv1.ListTriggersResponse{Triggers: []*triggersv1.Trigger{{TriggerId: 7}}}))
			}},
		{"ListTriggerEvents",
			func() error {
				return ignoreResult(decode.TriggerEventsListFromProto(&triggersv1.ListTriggerEventsResponse{Events: []*triggersv1.TriggerEvent{{}}}))
			},
			func() error {
				return ignoreResult(decode.TriggerEventsListFromProto(&triggersv1.ListTriggerEventsResponse{Events: []*triggersv1.TriggerEvent{{TriggerId: 7}}}))
			}},
		{"GetOrder order_id",
			func() error {
				return ignoreResult(decode.GetOrderFromProto(&orderv1.GetOrderResponse{Order: order(0, 3)}))
			},
			func() error {
				return ignoreResult(decode.GetOrderFromProto(&orderv1.GetOrderResponse{Order: order(9, 3), Trades: []*orderv1.UserTrade{trade(3)}}))
			}},
		{"GetOrder order lineage.id",
			func() error {
				return ignoreResult(decode.GetOrderFromProto(&orderv1.GetOrderResponse{Order: order(9, 0)}))
			}, nil},
		{"GetOrder trade lineage.id",
			func() error {
				return ignoreResult(decode.GetOrderFromProto(&orderv1.GetOrderResponse{Order: order(9, 3), Trades: []*orderv1.UserTrade{trade(0)}}))
			}, nil},
		{"GetOpenOrders order_id",
			func() error {
				return ignoreResult(decode.OrdersListFromOpen(&orderv1.GetOpenOrdersResponse{Orders: []*orderv1.Order{order(0, 3)}}))
			},
			func() error {
				return ignoreResult(decode.OrdersListFromOpen(&orderv1.GetOpenOrdersResponse{Orders: []*orderv1.Order{order(9, 3)}}))
			}},
		{"GetOpenOrders lineage.id",
			func() error {
				return ignoreResult(decode.OrdersListFromOpen(&orderv1.GetOpenOrdersResponse{Orders: []*orderv1.Order{order(9, 0)}}))
			}, nil},
		{"GetOrderHistory order_id",
			func() error {
				return ignoreResult(decode.OrdersListFromHistory(&orderv1.GetOrderHistoryResponse{Orders: []*orderv1.Order{order(0, 3)}}))
			},
			func() error {
				return ignoreResult(decode.OrdersListFromHistory(&orderv1.GetOrderHistoryResponse{Orders: []*orderv1.Order{{OrderId: 9}}}))
			}},
		{"GetOrderHistory lineage.id",
			func() error {
				return ignoreResult(decode.OrdersListFromHistory(&orderv1.GetOrderHistoryResponse{Orders: []*orderv1.Order{order(9, 0)}}))
			}, nil},
		{"GetUserTrades lineage.id",
			func() error {
				return ignoreResult(decode.UserTradesListFromProto(&orderv1.GetUserTradesResponse{Trades: []*orderv1.UserTrade{trade(0)}}))
			},
			func() error {
				return ignoreResult(decode.UserTradesListFromProto(&orderv1.GetUserTradesResponse{Trades: []*orderv1.UserTrade{trade(3), {OrderId: 9}}}))
			}},
		{"ListAlerts",
			func() error {
				return ignoreResult(decode.PriceAlertsPageFromProto(&alertsv1.ListAlertsResponse{Alerts: []*alertsv1.PriceAlert{{}}}, nil))
			},
			func() error {
				return ignoreResult(decode.PriceAlertsPageFromProto(&alertsv1.ListAlertsResponse{Alerts: []*alertsv1.PriceAlert{{AlertId: 7}}}, nil))
			}},
		{"ListAddressBookEntries entry",
			func() error {
				return ignoreResult(decode.ListEntriesFromProto(&authv1.ListAddressBookEntriesResponse{Entries: []*authv1.AddressBookEntry{entry(0, 5)}}))
			},
			func() error {
				return ignoreResult(decode.ListEntriesFromProto(&authv1.ListAddressBookEntriesResponse{Entries: []*authv1.AddressBookEntry{entry(4, 5)}}))
			}},
		{"ListAddressBookEntries tag",
			func() error {
				return ignoreResult(decode.ListEntriesFromProto(&authv1.ListAddressBookEntriesResponse{Entries: []*authv1.AddressBookEntry{entry(4, 0)}}))
			}, nil},
		{"CreateAddressBookEntry tag",
			func() error {
				return ignoreResult(decode.EntryFromCreateProto(&authv1.CreateAddressBookEntryResponse{Entry: entry(4, 0)}))
			},
			func() error {
				return ignoreResult(decode.EntryFromCreateProto(&authv1.CreateAddressBookEntryResponse{Entry: entry(4, 5)}))
			}},
		{"UpdateAddressBookEntry tag",
			func() error {
				return ignoreResult(decode.EntryFromUpdateProto(&authv1.UpdateAddressBookEntryResponse{Entry: entry(4, 0)}))
			}, nil},
		{"ListTransferDestinations entry",
			func() error {
				return ignoreResult(decode.ListDestinationsFromProto(&authv1.ListTransferDestinationsResponse{Destinations: []*authv1.TransferDestination{{AddressBookEntry: entry(0, 5)}}}))
			},
			func() error {
				return ignoreResult(decode.ListDestinationsFromProto(&authv1.ListTransferDestinationsResponse{Destinations: []*authv1.TransferDestination{{AddressBookEntry: entry(4, 5)}, {}}}))
			}},
		{"ListTransferDestinations tag",
			func() error {
				return ignoreResult(decode.ListDestinationsFromProto(&authv1.ListTransferDestinationsResponse{Destinations: []*authv1.TransferDestination{{AddressBookEntry: entry(4, 0)}}}))
			}, nil},
		{"GetAddressBookView external",
			func() error {
				return ignoreResult(decode.AddressBookViewFromProto(&authv1.GetAddressBookViewResponse{Entries: &authv1.AddressBookEntriesView{External: []*authv1.ExternalAddressBookEntry{{}}}}))
			},
			func() error {
				return ignoreResult(decode.AddressBookViewFromProto(&authv1.GetAddressBookViewResponse{Entries: &authv1.AddressBookEntriesView{
					External: []*authv1.ExternalAddressBookEntry{{AddressBookEntryId: 4}},
					Internal: []*authv1.InternalAddressBookEntry{{AddressBookEntryId: 5}},
				}}))
			}},
		{"GetAddressBookView tag",
			func() error {
				return ignoreResult(decode.AddressBookViewFromProto(&authv1.GetAddressBookViewResponse{Tags: []*authv1.AddressBookTagSummary{{}}}))
			},
			func() error {
				return ignoreResult(decode.AddressBookViewFromProto(&authv1.GetAddressBookViewResponse{Tags: []*authv1.AddressBookTagSummary{{TagId: 7}}}))
			}},
		{"GetAddressBookView internal",
			func() error {
				return ignoreResult(decode.AddressBookViewFromProto(&authv1.GetAddressBookViewResponse{Entries: &authv1.AddressBookEntriesView{Internal: []*authv1.InternalAddressBookEntry{{}}}}))
			}, nil},
		{"ListInternalTransferWhitelistEntries",
			func() error {
				return ignoreResult(decode.ListInternalWhitelistFromProto(&authv1.ListInternalTransferWhitelistEntriesResponse{Entries: []*authv1.InternalTransferWhitelistEntry{{}}}))
			},
			func() error {
				return ignoreResult(decode.ListInternalWhitelistFromProto(&authv1.ListInternalTransferWhitelistEntriesResponse{Entries: []*authv1.InternalTransferWhitelistEntry{{EntryId: 4}}}))
			}},
		{"ListTransferCounterparties",
			func() error {
				return ignoreResult(decode.ListCounterpartiesFromProto(&authv1.ListTransferCounterpartiesResponse{Counterparties: []*authv1.TransferCounterparty{{}}}))
			},
			func() error {
				return ignoreResult(decode.ListCounterpartiesFromProto(&authv1.ListTransferCounterpartiesResponse{Counterparties: []*authv1.TransferCounterparty{{CounterpartyId: 4}}}))
			}},
		{"GetApiKey",
			func() error {
				return ignoreResult(decode.ApiKeyFromGetProto(&authv1.GetApiKeyResponse{ApiKey: &authv1.ApiKey{}}))
			},
			func() error {
				return ignoreResult(decode.ApiKeyFromGetProto(&authv1.GetApiKeyResponse{ApiKey: &authv1.ApiKey{KeyId: "ak_1"}}))
			}},
		{"ListApiKeys",
			func() error {
				return ignoreResult(decode.ApiKeysListFromProto(&authv1.ListApiKeysResponse{ApiKeys: []*authv1.ApiKey{{}}}))
			},
			func() error {
				return ignoreResult(decode.ApiKeysListFromProto(&authv1.ListApiKeysResponse{ApiKeys: []*authv1.ApiKey{{KeyId: "ak_1"}}}))
			}},
		{"Me account_id",
			func() error { return ignoreResult(decode.MeFromProto(&authv1.MeResponse{})) },
			func() error {
				if err := ignoreResult(decode.MeFromProto(&authv1.MeResponse{AccountId: 4})); err != nil {
					return err
				}
				return ignoreResult(decode.MeFromProto(&authv1.MeResponse{AccountId: 4, Session: &authv1.SessionInfo{SessionId: "s1"}}))
			}},
		{"Me session.session_id",
			func() error {
				return ignoreResult(decode.MeFromProto(&authv1.MeResponse{AccountId: 4, Session: &authv1.SessionInfo{}}))
			}, nil},
		{"GetSubaccount subaccount id",
			func() error {
				return ignoreResult(decode.GetSubaccountFromProto(subaccount(0, "ak_1", invite(5, 6), 7)))
			},
			func() error {
				return ignoreResult(decode.GetSubaccountFromProto(subaccount(4, "ak_1", invite(5, 6), 7)))
			}},
		{"GetSubaccount api key",
			func() error { return ignoreResult(decode.GetSubaccountFromProto(subaccount(4, "", invite(5, 6), 7))) }, nil},
		{"GetSubaccount invite id",
			func() error {
				return ignoreResult(decode.GetSubaccountFromProto(subaccount(4, "ak_1", invite(0, 6), 7)))
			}, nil},
		{"GetSubaccount invite subaccount_id",
			func() error {
				return ignoreResult(decode.GetSubaccountFromProto(subaccount(4, "ak_1", invite(5, 0), 7)))
			}, nil},
		{"GetSubaccount policy",
			func() error {
				return ignoreResult(decode.GetSubaccountFromProto(subaccount(4, "ak_1", invite(5, 6), 0)))
			}, nil},
		{"GetSubaccount member account_id",
			func() error {
				r := subaccount(4, "ak_1", invite(5, 6), 7)
				r.Members = []*authv1.SubaccountMemberView{{}}
				return ignoreResult(decode.GetSubaccountFromProto(r))
			},
			func() error {
				r := subaccount(4, "ak_1", invite(5, 6), 7)
				r.Members = []*authv1.SubaccountMemberView{{AccountId: 8}}
				return ignoreResult(decode.GetSubaccountFromProto(r))
			}},
		{"ListSubaccountMembers",
			func() error {
				return ignoreResult(decode.SubaccountMembersListFromProto(&authv1.ListSubaccountMembersResponse{Members: []*authv1.SubaccountMemberView{{}}}))
			},
			func() error {
				return ignoreResult(decode.SubaccountMembersListFromProto(&authv1.ListSubaccountMembersResponse{Members: []*authv1.SubaccountMemberView{{AccountId: 8}}}))
			}},
		{"ListSubaccounts",
			func() error {
				return ignoreResult(decode.SubaccountsListFromProto(&authv1.ListSubaccountsResponse{Subaccounts: []*authv1.Subaccount{{}}}))
			},
			func() error {
				return ignoreResult(decode.SubaccountsListFromProto(&authv1.ListSubaccountsResponse{Subaccounts: []*authv1.Subaccount{{Id: 4}}}))
			}},
		{"ListSubaccountInvites id",
			func() error {
				return ignoreResult(decode.SubaccountInvitesListFromProto(&authv1.ListSubaccountInvitesResponse{Invites: []*authv1.SubaccountInvite{invite(0, 6)}}))
			},
			func() error {
				return ignoreResult(decode.SubaccountInvitesListFromProto(&authv1.ListSubaccountInvitesResponse{Invites: []*authv1.SubaccountInvite{invite(5, 6)}}))
			}},
		{"ListSubaccountInvites subaccount_id",
			func() error {
				return ignoreResult(decode.SubaccountInvitesListFromProto(&authv1.ListSubaccountInvitesResponse{Invites: []*authv1.SubaccountInvite{invite(5, 0)}}))
			}, nil},
		{"GetZippedAssetSupplyGroup",
			func() error {
				return ignoreResult(decode.ZippedAssetSupplyGroupFromProto(&analyticsv1.GetZippedAssetSupplyGroupResponse{}))
			},
			func() error {
				return ignoreResult(decode.ZippedAssetSupplyGroupFromProto(&analyticsv1.GetZippedAssetSupplyGroupResponse{GroupId: "g1"}))
			}},
		{"GetFlowById",
			func() error { return ignoreResult(decode.FlowFromGetResponse(flow(""))) },
			func() error { return ignoreResult(decode.FlowFromGetResponse(flow("f1"))) }},
		{"ListFlows",
			func() error {
				return ignoreResult(decode.FlowsListFromProto(&lifecyclev1.ListFlowsResponse{Flows: []*lifecyclev1.FlowSummaryView{{}}}))
			},
			func() error {
				return ignoreResult(decode.FlowsListFromProto(&lifecyclev1.ListFlowsResponse{Flows: []*lifecyclev1.FlowSummaryView{{FlowId: "f1"}}}))
			}},
		{"ListFlowsByTx",
			func() error {
				return ignoreResult(decode.FlowsByTxListFromProto(&lifecyclev1.ListFlowsByTxResponse{Matches: []*lifecyclev1.FlowTxMatchView{{}}}))
			},
			func() error {
				return ignoreResult(decode.FlowFromGetByTxResponse(&lifecyclev1.ListFlowsByTxResponse{Matches: []*lifecyclev1.FlowTxMatchView{{FlowId: "f1"}}}))
			}},
		{"ListChanges",
			func() error {
				return ignoreResult(decode.ChangelogChangesPageFromProto(&changelogv1.ListChangesResponse{Changes: []*changelogv1.ChangeEntry{{Change: &changelogv1.Change{}}}}))
			},
			func() error {
				return ignoreResult(decode.ChangelogChangesPageFromProto(&changelogv1.ListChangesResponse{Changes: []*changelogv1.ChangeEntry{{Change: &changelogv1.Change{Id: 7}}}}))
			}},
		{"GetPost",
			func() error { return ignoreResult(decode.ChangelogPostFromProto(post(0))) },
			func() error { return ignoreResult(decode.ChangelogPostFromProto(post(7))) }},
		{"GetDeploymentGroup",
			func() error { return ignoreResult(decode.ChangelogDeploymentGroupFromProto(group(0))) },
			func() error { return ignoreResult(decode.ChangelogDeploymentGroupFromProto(group(7))) }},
		{"ListTransfers",
			func() error {
				return ignoreResult(decode.TransfersListFromProto(&ledgerrdv1.ListTransfersResponse{Transfers: []*ledgerrdv1.TransferRow{{}}}))
			},
			func() error {
				return ignoreResult(decode.TransfersListFromProto(&ledgerrdv1.ListTransfersResponse{Transfers: []*ledgerrdv1.TransferRow{{TransferId: "7"}}}))
			}},
		{"ListStatusEvents",
			func() error {
				return ignoreResult(decode.StatusEventsPageFromProto(&systemv1.ListStatusEventsResponse{Events: []*systemv1.StatusEvent{{}}}))
			},
			func() error {
				return ignoreResult(decode.StatusEventsPageFromProto(&systemv1.ListStatusEventsResponse{Events: []*systemv1.StatusEvent{{Id: 7, Updates: []*systemv1.StatusEventUpdate{{Id: 1}}}}}))
			}},
		{"ListStatusEvents update id",
			func() error {
				return ignoreResult(decode.StatusEventsPageFromProto(&systemv1.ListStatusEventsResponse{Events: []*systemv1.StatusEvent{{Id: 7, Updates: []*systemv1.StatusEventUpdate{{}}}}}))
			}, nil},
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

// A successful get without the object it exists to return breaks the
// contract: a missing object is refused, never read as "none" (a real
// not-found arrives as an API error).
func TestGetRepliesWithoutTheirObjectAreRefused(t *testing.T) {
	cases := map[string]func() error{
		"GetTrigger":    func() error { return ignoreResult(decode.GetTriggerFromProto(&triggersv1.GetTriggerResponse{})) },
		"GetOrder":      func() error { return ignoreResult(decode.GetOrderFromProto(&orderv1.GetOrderResponse{})) },
		"GetApiKey":     func() error { return ignoreResult(decode.ApiKeyFromGetProto(&authv1.GetApiKeyResponse{})) },
		"GetSubaccount": func() error { return ignoreResult(decode.GetSubaccountFromProto(&authv1.GetSubaccountResponse{})) },
		"GetFlowById":   func() error { return ignoreResult(decode.FlowFromGetResponse(&lifecyclev1.GetFlowResponse{})) },
		"GetFlowById summary": func() error {
			return ignoreResult(decode.FlowFromGetResponse(&lifecyclev1.GetFlowResponse{Flow: &lifecyclev1.FlowDetailView{}}))
		},
		"GetPost": func() error { return ignoreResult(decode.ChangelogPostFromProto(&changelogv1.GetPostResponse{})) },
		"GetDeploymentGroup": func() error {
			return ignoreResult(decode.ChangelogDeploymentGroupFromProto(&changelogv1.GetDeploymentGroupResponse{}))
		},
	}
	for name, call := range cases {
		var contract *sdkerrors.ResponseContractError
		if err := call(); !errors.As(err, &contract) {
			t.Errorf("%s without its object: got %v, want a ResponseContractError", name, err)
		}
	}
}
