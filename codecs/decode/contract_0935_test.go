package decode

import (
	"reflect"
	"testing"
	"time"

	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	authv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/auth/v1"
	withdrawv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/chain/withdraw/v1"
	marketdatav1 "github.com/Fabric-Labs/polyester-sdk-go/gen/marketdata/v1"
	orderv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/orders/v1"
	typev1 "github.com/Fabric-Labs/polyester-sdk-go/gen/polyester/type/v1"
	triggersv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/triggers/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// A wallet read lists the zTokens whose balance could not be read; such a
// read is incomplete, and an asset missing from Balances is then unknown.
func TestWalletBalancesListTheUnavailableAssets(t *testing.T) {
	msg := &withdrawv1.GetWalletBalancesResponse{
		SmartAccountAddress: "0x5a5a",
		Balances:            []*withdrawv1.WalletBalance{{ZippedAssetId: 7, UnifiedAssetId: 3, ChainId: 2, ZtokenAddress: "0x11", AmountE18: &typev1.U128{Lo: 1_500_000_000_000_000_000}}},
		UnavailableAssets: []*withdrawv1.UnavailableWalletAsset{
			{ZippedAssetId: 700007, ChainId: 8453, ZtokenAddress: "0x07", Reason: withdrawv1.WalletAssetUnavailableReason_NO_CONTRACT},
			{ZippedAssetId: 700008, Reason: withdrawv1.WalletAssetUnavailableReason_READ_FAILED},
			{ZippedAssetId: 700009, Reason: 99},
		},
	}
	got, err := WalletBalancesFromProto(msg)
	if err != nil {
		t.Fatal(err)
	}
	if got.Complete() || len(got.UnavailableAssets) != 3 || got.Balances[0].Amount != "1.5" {
		t.Fatalf("balances = %+v", got)
	}
	first := got.UnavailableAssets[0]
	if first.ZippedAssetID != 700007 || first.UnifiedAssetID != 0 || first.ChainID != 8453 || first.ZTokenAddress != "0x07" || first.Reason != "no_contract" {
		t.Fatalf("unavailable asset = %+v", first)
	}
	if got.UnavailableAssets[1].Reason != "read_failed" || got.UnavailableAssets[2].Reason != "unknown(99)" {
		t.Fatalf("reasons = %+v", got.UnavailableAssets)
	}
	msg.UnavailableAssets = nil
	if got, err = WalletBalancesFromProto(msg); err != nil || !got.Complete() {
		t.Fatalf("complete read = %+v, %v", got, err)
	}
}

// Market datapoints keep their exact decimal values, sources and times; an
// absent value is unknown, and an unlimited maximum is a presence-only fact.
func TestAssetInfoCarriesItsMarketStats(t *testing.T) {
	asOf := time.Date(2026, 11, 17, 4, 27, 33, 22_000_000, time.UTC)
	list, err := AssetInfoListFromProto(&marketdatav1.ListAssetInfoResponse{AssetInfo: []*marketdatav1.AssetInfo{
		{AssetId: 1, MarketStats: &marketdatav1.AssetMarketStats{
			CirculatingSupply:  &marketdatav1.AssetMarketValue{Value: "20113162", Source: marketdatav1.AssetDataSource_COINMARKETCAP, AsOf: timestamppb.New(asOf)},
			MarketCapUsd:       &marketdatav1.AssetMarketValue{Value: "1307009616822.71", Source: marketdatav1.AssetDataSource_POLYESTER, AsOf: timestamppb.New(asOf)},
			MaxSupplyUnlimited: &marketdatav1.AssetMarketFlag{Source: marketdatav1.AssetDataSource_COINMARKETCAP, AsOf: timestamppb.New(asOf)},
		}},
		{AssetId: 2},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 || list[1].MarketStats != nil {
		t.Fatalf("list = %+v", list)
	}
	stats := list[0].MarketStats
	if stats == nil || stats.CirculatingSupply == nil || stats.CirculatingSupply.Value != "20113162" || stats.CirculatingSupply.Source != "coinmarketcap" || !stats.CirculatingSupply.AsOf.Equal(asOf) {
		t.Fatalf("circulating supply = %+v", stats)
	}
	if stats.MarketCapUSD.Value != "1307009616822.71" || stats.MarketCapUSD.Source != "polyester" || !stats.MarketCapUSD.AsOf.Equal(asOf) {
		t.Fatalf("market cap = %+v", stats.MarketCapUSD)
	}
	if stats.MaxSupply != nil || stats.MaxSupplyUnlimited == nil || stats.MaxSupplyUnlimited.Source != "coinmarketcap" || stats.TotalSupply != nil || stats.Rank != nil {
		t.Fatalf("stats = %+v", stats)
	}
}

// A ladder says how its levels were placed, and how placement went once it
// has ended.
func TestLadderStateCarriesItsLevelCountsAndOutcome(t *testing.T) {
	trigger := mustDecoded(TriggerFromProto(&triggersv1.Trigger{
		TriggerId: 1, SymbolId: 1,
		RuntimeDetails: &triggersv1.Trigger_LadderState{LadderState: &triggersv1.LadderDetails{
			LadderLevels: 3, ExecutedLevels: 1, RequestedLevels: 3, PlacedLevels: 2, RejectedLevels: 1, CanceledLevels: 1,
			Outcome: triggersv1.LadderOutcome_MIXED_REJECTIONS,
		}},
	}))
	d := trigger.Details
	if d == nil || d.RequestedLevels != 3 || d.PlacedLevels != 2 || d.RejectedLevels != 1 || d.CanceledLevels != 1 || d.LadderOutcome != "mixed_rejections" {
		t.Fatalf("details = %+v", d)
	}
	running := mustDecoded(TriggerFromProto(&triggersv1.Trigger{TriggerId: 1, SymbolId: 1, RuntimeDetails: &triggersv1.Trigger_LadderState{LadderState: &triggersv1.LadderDetails{LadderLevels: 3}}}))
	if running.Details.LadderOutcome != "" {
		t.Fatalf("running ladder outcome = %q", running.Details.LadderOutcome)
	}
}

// A preview refused for its price band names the band limit; other
// rejections carry none.
func TestPreviewPriceBandRejectionCarriesTheLimit(t *testing.T) {
	limit := int64(1_837_500_000_000)
	got, err := PreviewOrderFromProto(&orderv1.PreviewOrderResponse{
		Admissible:  new(bool),
		Rejection:   &orderv1.ErrorDetail{Code: orderv1.ErrorCode_ERROR_CODE_PRICE_BAND_VIOLATION, PriceBandLimitTicks: &limit},
		EvaluatedAt: timestamppb.Now(),
	}, 6, "ETH-USDT", nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.Rejection == nil || got.Rejection.PriceBandLimit == nil || got.Rejection.PriceBandLimit.Ticks() != limit {
		t.Fatalf("rejection = %+v", got.Rejection)
	}
	if text := got.Rejection.PriceBandLimit.Format(); text != "1837.5" {
		t.Fatalf("limit = %q", text)
	}
	got, err = PreviewOrderFromProto(&orderv1.PreviewOrderResponse{
		Admissible:  new(bool),
		Rejection:   &orderv1.ErrorDetail{Code: orderv1.ErrorCode_ERROR_CODE_BAD_PRICE},
		EvaluatedAt: timestamppb.Now(),
	}, 6, "ETH-USDT", nil)
	if err != nil || got.Rejection.PriceBandLimit != nil {
		t.Fatalf("rejection = %+v, %v", got.Rejection, err)
	}
}

// The account activity feed names each event's sub-account in public-ID
// form, empty when absent, and decodes its actor: the account as a public
// ID, the API key as sent, and each kind and reason as a label.
func TestAccountActivityNamesTheSubAccountAndActor(t *testing.T) {
	list := AccountActivityListFromProto(&authv1.ListAccountActivityResponse{
		Events: []*authv1.ActivityEvent{
			{EntityKind: authv1.ActivityEntityKind_ACTIVITY_ENTITY_SESSION, EventAction: authv1.ActivityEventAction_ACTIVITY_ACTION_FAILED, CreatedAt: timestamppb.New(time.UnixMilli(1_763_337_600_000)),
				Actor: &authv1.ActivityActor{Kind: authv1.ActivityActorKind_ACTIVITY_ACTOR_UNAUTHENTICATED}},
			{EntityKind: authv1.ActivityEntityKind_ACTIVITY_ENTITY_API_KEY, EventAction: authv1.ActivityEventAction_ACTIVITY_ACTION_CREATED, SubaccountId: 9000000001,
				Source: 99, Ip: "203.0.113.7", UserAgent: "ua",
				Actor: &authv1.ActivityActor{Kind: authv1.ActivityActorKind_ACTIVITY_ACTOR_API_KEY, AccountId: 9000000003, Username: "alice",
					ApiKeyId: "ak_0123456789abcdef0123456789abcdef", ApiKeyKind: 77, ApiKeyLabel: "bot"}},
			{EntityKind: authv1.ActivityEntityKind_ACTIVITY_ENTITY_API_KEY, EventAction: authv1.ActivityEventAction_ACTIVITY_ACTION_UPDATED,
				Actor: &authv1.ActivityActor{Kind: authv1.ActivityActorKind_ACTIVITY_ACTOR_SYSTEM, SystemReason: authv1.ActivitySystemReason_ACTIVITY_SYSTEM_ASSISTANT_PRESET_UPDATED}},
			{EntityKind: authv1.ActivityEntityKind_ACTIVITY_ENTITY_SESSION, EventAction: authv1.ActivityEventAction_ACTIVITY_ACTION_CREATED},
		},
		NextPageToken: "p2",
	})
	if len(list.Events) != 4 || list.NextPageToken != "p2" {
		t.Fatalf("list = %+v", list)
	}
	main, sub, system, noActor := list.Events[0], list.Events[1], list.Events[2], list.Events[3]
	if main.EventType != "session:failed" || main.TsMs != 1_763_337_600_000 || main.SubaccountID != "" || main.CreatedAt == nil {
		t.Fatalf("main account event = %+v", main)
	}
	if main.Source != "" || main.IP != "" || !reflect.DeepEqual(main.Actor, &models.SubAccountActivityActor{Kind: "unauthenticated"}) {
		t.Fatalf("main account event = %+v, actor %+v", main, main.Actor)
	}
	if sub.CreatedAt != nil || sub.Source != "unknown(99)" || sub.IP != "203.0.113.7" || sub.UserAgent != "ua" {
		t.Fatalf("sub-account event = %+v", sub)
	}
	wantKey := &models.SubAccountActivityActor{Kind: "api_key", AccountID: codecs.FormatID(9000000003), Username: "alice",
		APIKeyID: "ak_0123456789abcdef0123456789abcdef", APIKeyKind: "unknown(77)", APIKeyLabel: "bot"}
	if sub.EventType != "api_key:created" || sub.SubaccountID != codecs.FormatID(9000000001) || !reflect.DeepEqual(sub.Actor, wantKey) {
		t.Fatalf("sub-account event = %+v, actor %+v", sub, sub.Actor)
	}
	if !reflect.DeepEqual(system.Actor, &models.SubAccountActivityActor{Kind: "system", SystemReason: "assistant_preset_updated"}) {
		t.Fatalf("system actor = %+v", system.Actor)
	}
	if noActor.Actor != nil {
		t.Fatalf("actor of an event without one = %+v", noActor.Actor)
	}
}
