package services

import (
	"context"
	"fmt"
	"iter"
	"time"

	"github.com/Fabric-Labs/polyester-sdk-go/catalogs"
	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	"github.com/Fabric-Labs/polyester-sdk-go/codecs/decode"
	"github.com/Fabric-Labs/polyester-sdk-go/errors"
	pnlv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/pnl/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/gen/pnl/v1/pnlv1connect"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
	"github.com/Fabric-Labs/polyester-sdk-go/transport"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// ProfitAndLossService reads the profit and loss of the main account or one
// sub-account, in USD (pnl.v1.ProfitAndLossService). Spot PnL uses the
// weighted average cost of each asset; every trade is valued at its execution
// time and every fee is a realized expense; deposits, withdrawals and
// transfers are never PnL. Every read needs READ_PNL in the API key's policy.
//
// Without a sub-account, a main-account key reads the main account together
// with every sub-account it owns, and a sub-account key its own sub-account.
type ProfitAndLossService struct {
	transport *transport.Factory
	catalogs  *catalogs.Manager
	scoped    ScopedSubAccount
}

func NewProfitAndLossService(factory *transport.Factory, cats *catalogs.Manager, defaultSubAccountID *string) *ProfitAndLossService {
	if cats == nil {
		cats = catalogs.NewManager()
	}
	return &ProfitAndLossService{transport: factory, catalogs: cats, scoped: ScopedSubAccount{DefaultSubAccountID: defaultSubAccountID}}
}

func (s *ProfitAndLossService) client() pnlv1connect.ProfitAndLossServiceClient {
	return pnlv1connect.NewProfitAndLossServiceClient(s.transport.HTTP, s.transport.Config.APIURL, s.transport.ConnectOptions(true)...)
}

// ProfitAndLossParams selects the account, window and filters of a profit
// and loss or trading statistics read.
type ProfitAndLossParams struct {
	// Account and SubAccountID select the account, as for Balances.List.
	Account      AccountScope
	SubAccountID *string
	// Window is required: "today", "days_7", "days_30" or "lifetime". Every
	// window ends now and starts at 00:00 UTC of a day; lifetime starts on
	// the day of the account's first trade or transfer.
	Window string
	// Product filters by product: "" for every product, or "spot".
	Product string
	// Symbol or SymbolID filters by market (given both, they must name the
	// same market); neither reads every market. Get filters only its Markets
	// by it, not its totals or holdings.
	Symbol   *string
	SymbolID *uint32
}

func (s *ProfitAndLossService) params(ctx context.Context, params ProfitAndLossParams, label string) (subaccountID *uint64, window pnlv1.ProfitAndLossWindow, product pnlv1.ProfitAndLossProduct, symbolID uint32, err error) {
	if window, err = codecs.ResolveProfitAndLossWindow(params.Window); err != nil {
		return
	}
	if product, err = codecs.ResolveProfitAndLossProduct(params.Product); err != nil {
		return
	}
	if err = s.scoped.ApplyOptionalSubaccountIDPtr(&subaccountID, params.Account, params.SubAccountID); err != nil {
		return
	}
	if namesSymbol(params.Symbol) {
		if err = awaitCatalogs(ctx, s.catalogs, true); err != nil {
			return
		}
	}
	symbolID, err = ResolveOptionalSymbolID(s.catalogs, params.Symbol, params.SymbolID, label)
	return
}

// Get reads the account's PnL over a window (GetProfitAndLoss): realized PnL
// after fees, the change in unrealized PnL, fees and volume, by product and by
// market, and every asset held now with its cost, entry price, mark and
// unrealized PnL. A figure that needs an unavailable price is nil, never
// "0", and Notices says why.
func (s *ProfitAndLossService) Get(ctx context.Context, params ProfitAndLossParams) (models.ProfitAndLoss, error) {
	sub, window, product, symbolID, err := s.params(ctx, params, "pnl.get")
	if err != nil {
		return models.ProfitAndLoss{}, err
	}
	req := &pnlv1.GetProfitAndLossRequest{SubaccountId: sub, Window: window, Product: product, SymbolId: symbolID}
	return UnaryAuth(ctx, s.transport, s.client().GetProfitAndLoss, req, decode.ProfitAndLossFromProto)
}

// GetTradingStatistics reads the account's trade count, volume, fees and
// realized PnL over a window, with the time-weighted return, maximum drawdown
// and Sharpe ratio of its closed UTC days (GetTradingStatistics).
func (s *ProfitAndLossService) GetTradingStatistics(ctx context.Context, params ProfitAndLossParams) (models.TradingStatistics, error) {
	sub, window, product, symbolID, err := s.params(ctx, params, "pnl.get_trading_statistics")
	if err != nil {
		return models.TradingStatistics{}, err
	}
	req := &pnlv1.GetTradingStatisticsRequest{SubaccountId: sub, Window: window, Product: product, SymbolId: symbolID}
	return UnaryAuth(ctx, s.transport, s.client().GetTradingStatistics, req, decode.TradingStatisticsFromProto)
}

// ListDailyProfitAndLossParams selects a range of UTC days.
type ListDailyProfitAndLossParams struct {
	// Account and SubAccountID select the account, as for Balances.List.
	Account      AccountScope
	SubAccountID *string
	// StartTime (inclusive) and EndTime (exclusive) are required, each at
	// 00:00 UTC of a day, with EndTime after StartTime and at most
	// MaxDailyProfitAndLossDays days after it. Any other range is refused
	// with *errors.ValidationError before sending.
	StartTime time.Time
	EndTime   time.Time
	// Limit is the page size in days; 0 uses the server's default of 31.
	// At most 366.
	Limit int
	// PageToken continues from an earlier page's NextPageToken, with the
	// same account and days.
	PageToken string
}

// MaxDailyProfitAndLossDays is the most days one ListDaily range may span.
const MaxDailyProfitAndLossDays = 366

// checkDailyProfitAndLossRange refuses a range the contract refuses: both
// ends at 00:00 UTC of a day, within the protobuf timestamp range, the end
// after the start and at most MaxDailyProfitAndLossDays days after it.
func checkDailyProfitAndLossRange(start, end time.Time) error {
	for _, bound := range []struct {
		name  string
		value time.Time
	}{{"start_time", start}, {"end_time", end}} {
		if bound.value.IsZero() {
			return &errors.ValidationError{Msg: "pnl.list_daily requires " + bound.name}
		}
		if err := timestamppb.New(bound.value).CheckValid(); err != nil {
			return &errors.ValidationError{Msg: "pnl.list_daily " + bound.name + " is outside the timestamp range"}
		}
		utc := bound.value.UTC()
		if utc.Hour() != 0 || utc.Minute() != 0 || utc.Second() != 0 || utc.Nanosecond() != 0 {
			return &errors.ValidationError{Msg: "pnl.list_daily " + bound.name + " must be 00:00 UTC of a day"}
		}
	}
	if !end.After(start) {
		return &errors.ValidationError{Msg: "pnl.list_daily end_time must be after start_time"}
	}
	if end.After(start.UTC().AddDate(0, 0, MaxDailyProfitAndLossDays)) {
		return &errors.ValidationError{Msg: fmt.Sprintf("pnl.list_daily covers at most %d days", MaxDailyProfitAndLossDays)}
	}
	return nil
}

// ListDaily reads one page of the account's PnL per UTC day, oldest first
// (ListDailyProfitAndLoss). Every day of the range is listed, days without
// activity included.
func (s *ProfitAndLossService) ListDaily(ctx context.Context, params ListDailyProfitAndLossParams) (models.DailyProfitAndLossList, error) {
	if err := checkDailyProfitAndLossRange(params.StartTime, params.EndTime); err != nil {
		return models.DailyProfitAndLossList{}, err
	}
	limit, err := PaginationLimit(params.Limit, "limit")
	if err != nil {
		return models.DailyProfitAndLossList{}, err
	}
	req := &pnlv1.ListDailyProfitAndLossRequest{
		StartTime: timestamppb.New(params.StartTime),
		EndTime:   timestamppb.New(params.EndTime),
		Limit:     limit,
		PageToken: params.PageToken,
	}
	if err := codecs.CheckPageLimit(req); err != nil {
		return models.DailyProfitAndLossList{}, err
	}
	if err := codecs.CheckContractString(req, "page_token", params.PageToken, "page_token"); err != nil {
		return models.DailyProfitAndLossList{}, err
	}
	if err := s.scoped.ApplyOptionalSubaccountIDPtr(&req.SubaccountId, params.Account, params.SubAccountID); err != nil {
		return models.DailyProfitAndLossList{}, err
	}
	return UnaryAuth(ctx, s.transport, s.client().ListDailyProfitAndLoss, req, decode.DailyProfitAndLossListFromProto)
}

// AllDaily iterates the days of params from params.PageToken (or the first
// day), following each page's NextPageToken. It stops at the first error.
func (s *ProfitAndLossService) AllDaily(ctx context.Context, params ListDailyProfitAndLossParams) iter.Seq2[models.DailyProfitAndLoss, error] {
	return paginate(params.PageToken, func(token string) ([]models.DailyProfitAndLoss, string, error) {
		page := params
		page.PageToken = token
		list, err := s.ListDaily(ctx, page)
		return list.Days, list.NextPageToken, err
	})
}
