package services

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/Fabric-Labs/polyester-sdk-go/auth"
	"github.com/Fabric-Labs/polyester-sdk-go/catalogs"
	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	pnlv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/pnl/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/gen/pnl/v1/pnlv1connect"
	"github.com/Fabric-Labs/polyester-sdk-go/hardening"
	"github.com/Fabric-Labs/polyester-sdk-go/transport"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type pnlHandler struct {
	pnlv1connect.UnimplementedProfitAndLossServiceHandler
	mu     sync.Mutex
	gets   []*pnlv1.GetProfitAndLossRequest
	stats  []*pnlv1.GetTradingStatisticsRequest
	dailys []*pnlv1.ListDailyProfitAndLossRequest
}

func optional(s string) *string { return &s }

var pnlAsOf = &timestamppb.Timestamp{Seconds: 1_795_000_000}

func (h *pnlHandler) GetProfitAndLoss(_ context.Context, req *connect.Request[pnlv1.GetProfitAndLossRequest]) (*connect.Response[pnlv1.GetProfitAndLossResponse], error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.gets = append(h.gets, proto.Clone(req.Msg).(*pnlv1.GetProfitAndLossRequest))
	return connect.NewResponse(&pnlv1.GetProfitAndLossResponse{
		WindowStart:    &timestamppb.Timestamp{Seconds: 1_794_441_600},
		AsOf:           pnlAsOf,
		RealizedPnlUsd: "-12.5",
		FeesUsd:        "-0.25",
		VolumeUsd:      "1000",
		Products:       []*pnlv1.ProductProfitAndLoss{{Product: pnlv1.ProfitAndLossProduct_PROFIT_AND_LOSS_PRODUCT_SPOT, RealizedPnlUsd: "-12.5", FeesUsd: "-0.25", VolumeUsd: "1000"}},
		Markets:        []*pnlv1.MarketProfitAndLoss{{SymbolId: 5, Product: pnlv1.ProfitAndLossProduct_PROFIT_AND_LOSS_PRODUCT_SPOT, RealizedPnlUsd: "-12.5", FeesUsd: "-0.25", VolumeUsd: "1000", TradeCount: 3}},
		Holdings: []*pnlv1.HoldingProfitAndLoss{
			{AssetId: 1, PositionQuantity: "0.5", UntrackedQuantity: "0.1", CostUsd: "24000", EntryPriceUsd: optional("60000"), MarkPriceUsd: optional("61000"), UnrealizedPnlUsd: optional("0")},
			{AssetId: 2, PositionQuantity: "3", UntrackedQuantity: "3", CostUsd: "0"},
		},
		Notices: []pnlv1.ProfitAndLossNotice{pnlv1.ProfitAndLossNotice_PROFIT_AND_LOSS_NOTICE_MARK_UNAVAILABLE, 99},
	}), nil
}

func (h *pnlHandler) GetTradingStatistics(_ context.Context, req *connect.Request[pnlv1.GetTradingStatisticsRequest]) (*connect.Response[pnlv1.GetTradingStatisticsResponse], error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.stats = append(h.stats, proto.Clone(req.Msg).(*pnlv1.GetTradingStatisticsRequest))
	return connect.NewResponse(&pnlv1.GetTradingStatisticsResponse{
		AsOf: pnlAsOf, TradeCount: 7, VolumeUsd: "10", FeesUsd: "0.01", RealizedPnlUsd: "1",
		ReturnPct: optional("9.95"), MaxDrawdownPct: optional("0"), ReturnDays: 2,
		Notices: []pnlv1.ProfitAndLossNotice{pnlv1.ProfitAndLossNotice_PROFIT_AND_LOSS_NOTICE_SHARPE_INSUFFICIENT_DAYS},
	}), nil
}

func (h *pnlHandler) ListDailyProfitAndLoss(_ context.Context, req *connect.Request[pnlv1.ListDailyProfitAndLossRequest]) (*connect.Response[pnlv1.ListDailyProfitAndLossResponse], error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.dailys = append(h.dailys, proto.Clone(req.Msg).(*pnlv1.ListDailyProfitAndLossRequest))
	start := req.Msg.GetStartTime().AsTime()
	if req.Msg.GetPageToken() == "" {
		return connect.NewResponse(&pnlv1.ListDailyProfitAndLossResponse{
			Days: []*pnlv1.DailyProfitAndLoss{{
				Day: timestamppb.New(start), PnlUsd: optional("1.5"), RealizedPnlUsd: "1", FeesUsd: "0", VolumeUsd: "0",
				StartEquityUsd: optional("100"), EndEquityUsd: optional("101.5"), NetFlowUsd: optional("0"),
			}},
			NextPageToken: "page-2", AsOf: pnlAsOf,
		}), nil
	}
	return connect.NewResponse(&pnlv1.ListDailyProfitAndLossResponse{
		Days: []*pnlv1.DailyProfitAndLoss{{
			Day: timestamppb.New(start.AddDate(0, 0, 1)), RealizedPnlUsd: "0", FeesUsd: "0", VolumeUsd: "0", TradeCount: 0,
			StartEquityUsd: optional("101.5"),
		}},
		AsOf: pnlAsOf,
	}), nil
}

func newPnLService(t *testing.T, handler pnlv1connect.ProfitAndLossServiceHandler) *ProfitAndLossService {
	t.Helper()
	mux := http.NewServeMux()
	path, h := pnlv1connect.NewProfitAndLossServiceHandler(handler)
	mux.Handle(path, h)
	srv := hardening.NewTestServer(mux)
	t.Cleanup(srv.Close)
	creds, err := auth.LoadCredentials("ak_test", auth.GenerateEd25519Keypair().SecretKeyHex.Reveal(), false)
	if err != nil {
		t.Fatal(err)
	}
	cfg := transport.DefaultConfig()
	cfg.APIURL = srv.URL
	cats := catalogs.NewManager()
	if err := cats.HydrateSpotConfig(map[string]any{"pairs": []any{map[string]any{
		"symbol": "BTC-USDT", "symbol_id": float64(5), "base_quantity_scale": float64(8),
	}}}); err != nil {
		t.Fatal(err)
	}
	return NewProfitAndLossService(transport.NewFactory(cfg, creds, nil), cats, nil)
}

// Get sends the window, product, market and sub-account, and keeps every
// figure as sent: an omitted figure stays nil, distinct from "0".
func TestProfitAndLossGetSendsSelectionAndKeepsOmittedFigures(t *testing.T) {
	handler := &pnlHandler{}
	service := newPnLService(t, handler)
	sub := codecs.FormatID(42)
	symbol := "BTC-USDT"

	got, err := service.Get(context.Background(), ProfitAndLossParams{SubAccountID: &sub, Window: "DAYS_7", Product: "spot", Symbol: &symbol})
	if err != nil {
		t.Fatal(err)
	}
	subID := uint64(42)
	want := &pnlv1.GetProfitAndLossRequest{SubaccountId: &subID, Window: pnlv1.ProfitAndLossWindow_PROFIT_AND_LOSS_WINDOW_DAYS_7, Product: pnlv1.ProfitAndLossProduct_PROFIT_AND_LOSS_PRODUCT_SPOT, SymbolId: 5}
	if len(handler.gets) != 1 || !proto.Equal(handler.gets[0], want) {
		t.Fatalf("requests = %v, want %v", handler.gets, want)
	}
	if got.PnLUSD != nil || got.UnrealizedPnLUSD != nil || got.RealizedPnLUSD != "-12.5" || got.FeesUSD != "-0.25" || got.VolumeUSD != "1000" {
		t.Fatalf("totals = %+v", got)
	}
	if got.AsOf == nil || !got.AsOf.Equal(time.Unix(1_795_000_000, 0)) || got.WindowStart == nil || got.MarkTime != nil {
		t.Fatalf("times = %v %v %v", got.WindowStart, got.AsOf, got.MarkTime)
	}
	if len(got.Products) != 1 || got.Products[0].Product != "spot" || len(got.Markets) != 1 || got.Markets[0].SymbolID != 5 || got.Markets[0].TradeCount != 3 {
		t.Fatalf("products = %+v, markets = %+v", got.Products, got.Markets)
	}
	held, unpriced := got.Holdings[0], got.Holdings[1]
	if *held.EntryPriceUSD != "60000" || *held.MarkPriceUSD != "61000" || *held.UnrealizedPnLUSD != "0" || held.UntrackedQuantity != "0.1" {
		t.Fatalf("holding = %+v", held)
	}
	if unpriced.EntryPriceUSD != nil || unpriced.MarkPriceUSD != nil || unpriced.UnrealizedPnLUSD != nil {
		t.Fatalf("unpriced holding = %+v", unpriced)
	}
	if len(got.Notices) != 2 || got.Notices[0] != "mark_unavailable" || got.Notices[1] != "unknown(99)" {
		t.Fatalf("notices = %v", got.Notices)
	}
}

// Without a sub-account or filters the main account reads every product and
// market; the return figures come back as sent.
func TestProfitAndLossTradingStatisticsReadsReturnFigures(t *testing.T) {
	handler := &pnlHandler{}
	service := newPnLService(t, handler)
	got, err := service.GetTradingStatistics(context.Background(), ProfitAndLossParams{Window: "lifetime"})
	if err != nil {
		t.Fatal(err)
	}
	want := &pnlv1.GetTradingStatisticsRequest{Window: pnlv1.ProfitAndLossWindow_PROFIT_AND_LOSS_WINDOW_LIFETIME}
	if len(handler.stats) != 1 || !proto.Equal(handler.stats[0], want) {
		t.Fatalf("requests = %v", handler.stats)
	}
	if got.TradeCount != 7 || *got.ReturnPct != "9.95" || *got.MaxDrawdownPct != "0" || got.Sharpe != nil || got.ReturnDays != 2 ||
		len(got.Notices) != 1 || got.Notices[0] != "sharpe_insufficient_days" {
		t.Fatalf("statistics = %+v", got)
	}
}

// AllDaily follows the pages with the same days, and each day keeps its
// equity and flow figures, nil where the server omitted them.
func TestProfitAndLossAllDailyFollowsPages(t *testing.T) {
	handler := &pnlHandler{}
	service := newPnLService(t, handler)
	start := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC)

	var days []time.Time
	var last *string
	for day, err := range service.AllDaily(context.Background(), ListDailyProfitAndLossParams{StartTime: start, EndTime: end, Limit: 1}) {
		if err != nil {
			t.Fatal(err)
		}
		days = append(days, *day.Day)
		last = day.EndEquityUSD
		if len(days) == 1 && (*day.PnLUSD != "1.5" || *day.StartEquityUSD != "100" || *day.NetFlowUSD != "0") {
			t.Fatalf("first day = %+v", day)
		}
	}
	if len(days) != 2 || !days[0].Equal(start) || !days[1].Equal(start.AddDate(0, 0, 1)) || last != nil {
		t.Fatalf("days = %v, last end equity = %v", days, last)
	}
	if len(handler.dailys) != 2 || handler.dailys[0].GetPageToken() != "" || handler.dailys[1].GetPageToken() != "page-2" {
		t.Fatalf("requests = %v", handler.dailys)
	}
	for _, req := range handler.dailys {
		if !req.GetStartTime().AsTime().Equal(start) || !req.GetEndTime().AsTime().Equal(end) || req.GetLimit() != 1 {
			t.Fatalf("request = %v", req)
		}
	}
}

// A selection the contract refuses is refused before sending.
func TestProfitAndLossRefusesInvalidSelectionBeforeSending(t *testing.T) {
	handler := &pnlHandler{}
	service := newPnLService(t, handler)
	ctx := context.Background()
	symbol, otherSymbolID := "BTC-USDT", uint32(6)
	start := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	calls := map[string]func() error{
		"no window":      func() error { _, err := service.Get(ctx, ProfitAndLossParams{}); return err },
		"unknown window": func() error { _, err := service.Get(ctx, ProfitAndLossParams{Window: "days_90"}); return err },
		"unknown product": func() error {
			_, err := service.GetTradingStatistics(ctx, ProfitAndLossParams{Window: "today", Product: "perp"})
			return err
		},
		"symbol and another id": func() error {
			_, err := service.Get(ctx, ProfitAndLossParams{Window: "today", Symbol: &symbol, SymbolID: &otherSymbolID})
			return err
		},
		"no end time": func() error {
			_, err := service.ListDaily(ctx, ListDailyProfitAndLossParams{StartTime: start})
			return err
		},
		"start not at midnight": func() error {
			_, err := service.ListDaily(ctx, ListDailyProfitAndLossParams{StartTime: start.Add(time.Hour), EndTime: start.AddDate(0, 0, 1)})
			return err
		},
		"end not at UTC midnight": func() error {
			_, err := service.ListDaily(ctx, ListDailyProfitAndLossParams{StartTime: start, EndTime: start.AddDate(0, 0, 1).In(time.FixedZone("UTC+4", 4*3600)).Add(time.Minute)})
			return err
		},
		"equal ends": func() error {
			_, err := service.ListDaily(ctx, ListDailyProfitAndLossParams{StartTime: start, EndTime: start})
			return err
		},
		"reversed": func() error {
			_, err := service.ListDaily(ctx, ListDailyProfitAndLossParams{StartTime: start, EndTime: start.AddDate(0, 0, -1)})
			return err
		},
		"367 days": func() error {
			_, err := service.ListDaily(ctx, ListDailyProfitAndLossParams{StartTime: start, EndTime: start.AddDate(0, 0, 367)})
			return err
		},
		"outside timestamp range": func() error {
			far := time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC)
			_, err := service.ListDaily(ctx, ListDailyProfitAndLossParams{StartTime: far, EndTime: far.AddDate(0, 0, 1)})
			return err
		},
		"limit above 366": func() error {
			_, err := service.ListDaily(ctx, ListDailyProfitAndLossParams{StartTime: start, EndTime: start.AddDate(0, 0, 1), Limit: 367})
			return err
		},
	}
	for name, call := range calls {
		var validation *sdkerrors.ValidationError
		if err := call(); !errors.As(err, &validation) {
			t.Errorf("%s: err = %v, want a ValidationError", name, err)
		}
	}
	if len(handler.gets)+len(handler.stats)+len(handler.dailys) != 0 {
		t.Fatalf("sent %d, %d, %d requests", len(handler.gets), len(handler.stats), len(handler.dailys))
	}
}

// The longest range the contract takes, 366 days, is sent, and so is one
// whose ends are UTC midnights given in another zone.
func TestProfitAndLossListDailyAcceptsTheLongestRange(t *testing.T) {
	handler := &pnlHandler{}
	service := newPnLService(t, handler)
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	if _, err := service.ListDaily(context.Background(), ListDailyProfitAndLossParams{StartTime: start, EndTime: start.AddDate(0, 0, MaxDailyProfitAndLossDays)}); err != nil {
		t.Fatal(err)
	}
	zone := time.FixedZone("UTC+4", 4*3600)
	if _, err := service.ListDaily(context.Background(), ListDailyProfitAndLossParams{StartTime: start.In(zone), EndTime: start.AddDate(0, 0, 1).In(zone)}); err != nil {
		t.Fatal(err)
	}
	if len(handler.dailys) != 2 || !handler.dailys[0].GetEndTime().AsTime().Equal(start.AddDate(0, 0, 366)) {
		t.Fatalf("requests = %v", handler.dailys)
	}
}
