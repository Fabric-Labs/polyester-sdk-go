package services

import (
	"context"

	"github.com/Fabric-Labs/polyester-sdk-go/catalogs"
	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	"github.com/Fabric-Labs/polyester-sdk-go/codecs/decode"
	ledgerrdv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/ledger/read/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/gen/ledger/read/v1/ledgerrdv1connect"
	ledgerv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/ledger/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
	"github.com/Fabric-Labs/polyester-sdk-go/realtime"
	"github.com/Fabric-Labs/polyester-sdk-go/transport"
)

type BalancesService struct {
	transport        *transport.Factory
	catalogs         *catalogs.Manager
	scoped           ScopedSubAccount
	defaultAccountID *string
	realtime         RealtimeClient
}

func NewBalancesService(factory *transport.Factory, cats *catalogs.Manager, defaultSubAccountID *string, realtime RealtimeClient, defaultAccountID *string) *BalancesService {
	if cats == nil {
		cats = catalogs.NewManager()
	}
	return &BalancesService{transport: factory, catalogs: cats, scoped: ScopedSubAccount{DefaultSubAccountID: defaultSubAccountID}, realtime: realtime, defaultAccountID: defaultAccountID}
}

func (s *BalancesService) client() ledgerrdv1connect.LedgerReadServiceClient {
	return ledgerrdv1connect.NewLedgerReadServiceClient(s.transport.HTTP, s.transport.Config.APIURL, s.transport.ConnectOptions(true)...)
}

// List reads the account's balances (GetBalances). Its Trading values are one
// atomic cut at the position (TradingEpoch, TradingSequence) the result
// carries, and reflect every balance update published before the call began;
// Subscribe describes how to continue from it with the balance stream. A call
// costs 1 unit of the trading read limit, whatever the number of assets. When
// the server cannot answer at a current position yet it refuses the read as
// unavailable: retry it with a short backoff.
func (s *BalancesService) List(ctx context.Context, account AccountScope, subAccountID *string) (models.BalancesList, error) {
	req := &ledgerrdv1.GetBalancesRequest{}
	sub, err := s.scoped.ResolveSubAccountID(subAccountID, account)
	if err != nil {
		return models.BalancesList{}, err
	}
	if parsed, err := codecs.ParseOptionalSubaccountID(sub); err != nil {
		return models.BalancesList{}, err
	} else if parsed != nil {
		req.SubaccountId = parsed
	}
	return UnaryAuth(ctx, s.transport, s.client().GetBalances, req, decode.BalancesListFromProto)
}

func (s *BalancesService) GetBalanceHistory(ctx context.Context, account AccountScope, rangeKey string, subAccountID *string, ledger uint32, accountCodes []uint32) (models.BalanceHistory, error) {
	rng, err := codecs.ResolveBalanceRange(rangeKey)
	if err != nil {
		return models.BalanceHistory{}, err
	}
	req := &ledgerrdv1.GetBalanceHistoryRequest{Range: rng, Ledger: ledger, AccountCodes: accountCodesProto(accountCodes)}
	sub, err := s.scoped.ResolveSubAccountID(subAccountID, account)
	if err != nil {
		return models.BalanceHistory{}, err
	}
	if parsed, err := codecs.ParseOptionalSubaccountID(sub); err != nil {
		return models.BalanceHistory{}, err
	} else if parsed != nil {
		req.SubaccountId = parsed
	}
	return UnaryAuthDecoded(ctx, s.transport, s.client().GetBalanceHistory, req, decode.BalanceHistoryFromProto)
}

func (s *BalancesService) GetEquityHistory(ctx context.Context, account AccountScope, rangeKey string, subAccountID *string, accountCodes []uint32, groupBy string) (models.EquityHistory, error) {
	rng, err := codecs.ResolveBalanceRange(rangeKey)
	if err != nil {
		return models.EquityHistory{}, err
	}
	g, err := codecs.ResolveEquityGroupBy(groupBy)
	if err != nil {
		return models.EquityHistory{}, err
	}
	req := &ledgerrdv1.GetEquityHistorySeriesRequest{Range: rng, GroupBy: g, AccountCodes: accountCodesProto(accountCodes)}
	sub, err := s.scoped.ResolveSubAccountID(subAccountID, account)
	if err != nil {
		return models.EquityHistory{}, err
	}
	if parsed, err := codecs.ParseOptionalSubaccountID(sub); err != nil {
		return models.EquityHistory{}, err
	} else if parsed != nil {
		req.SubaccountId = parsed
	}
	return UnaryAuth(ctx, s.transport, s.client().GetEquityHistorySeries, req, decode.EquityHistoryFromProto)
}

func accountCodesProto(codes []uint32) []ledgerv1.AccountCode {
	if len(codes) == 0 {
		return nil
	}
	out := make([]ledgerv1.AccountCode, len(codes))
	for i, c := range codes {
		out[i] = ledgerv1.AccountCode(c)
	}
	return out
}

// Subscribe streams the account's balance updates from
// private:ledger:balances:{account_id}:proto, each delivered as the generated
// ledgerrdv1.BalanceUpdate exactly as published. The SDK neither buffers,
// reorders nor reconciles them: keeping exact balances is the caller's loop.
//
// Account tells the two kinds apart, never the epoch:
//
//   - A TRADING update is one numbered step of the account's Trading
//     balances. It carries its position (TradingEpoch, TradingSequence),
//     Complete, the assets it changed with only their Trading fields
//     (trading, reserved, available), the Trading transfer legs it applied
//     and TradingAppliedThroughTsNs. Its epoch is never zero.
//   - A FUNDING update carries one asset's Funding balance and its
//     FundingRevision, and no Trading position (both zero).
//
// To keep exact balances:
//
//  1. Subscribe first, and buffer the updates that arrive.
//  2. Call List (GetBalances) and apply its balances. Remember its position
//     (E, Q): TradingEpoch and TradingSequence. Retry an unavailable answer
//     with a short backoff.
//  3. Drop buffered and later TRADING updates of epoch E with a sequence at
//     or below Q, then apply E's sequences Q+1, Q+2, ... in order, taking
//     only Trading fields from them and keeping the Funding fields you hold.
//  4. Keep a set of dropped epochs: every epoch held before E, and every
//     other epoch seen on an update received before the List call that
//     answered E began. Drop a TRADING update of a dropped epoch without
//     reloading. Reload from step 2 when:
//     - a TRADING update's epoch is neither E nor dropped (the numbering
//     restarted);
//     - its sequence skips a number (an update was missed);
//     - after an update of E with Complete false, E's next sequence does not
//     arrive within 2 seconds (FUNDING updates, repeats and updates of
//     dropped epochs do not stop that timer);
//     - Subscription.TakeResubscribed reports true: the stream reconnected,
//     and publications may have been missed in between. Check it on a timer
//     of its own, not only when an update arrives: an idle account may
//     publish nothing more after the reconnect.
//  5. Apply a FUNDING update when its FundingRevision is higher than the
//     asset's, taking only its Funding fields.
//
// Show Trading values only after an update with Complete set: until then the
// same step continues in the next update. The updates of a new epoch up to
// its first complete one carry every asset the account holds in Trading.
//
// TradingEpoch is an opaque value: compare it for equality only. After a
// move, a new epoch can start behind the previous one's last values until
// its TradingAppliedThroughTsNs reaches the last one seen; a transfer's leg
// can then arrive again in the new epoch, identified by (transfer_id,
// is_debit). Count a transfer as applied only by a receipt in the current
// epoch: its leg, or a TradingAppliedThroughTsNs at or past its ts_ns.
//
// Read the stream and List for the same account. An unset accountID here
// uses Config.DefaultAccountID, while List with an unset account uses
// Config.DefaultSubAccountID or the main account, so name the account in
// both: Subscribe(ctx, mainAccountID) with List(ctx, "main", nil) for the
// main account, Subscribe(ctx, subAccountID) with List(ctx, subAccountID,
// nil) for a sub-account.
func (s *BalancesService) Subscribe(ctx context.Context, accountID any) (*realtime.Subscription[*ledgerrdv1.BalanceUpdate], error) {
	return SubscribeAccountProto(ctx, s.realtime, "private:ledger:balances:{account_id}:proto", accountID, s.defaultAccountID, decode.BalanceUpdateFromBytes)
}
