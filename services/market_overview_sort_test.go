package services

import (
	"context"
	stderrors "errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	overviewpb "github.com/Fabric-Labs/polyester-sdk-go/gen/marketoverview/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/internal/hardening"
	"github.com/Fabric-Labs/polyester-sdk-go/transport"
	"google.golang.org/protobuf/proto"
)

// overviewListCapture answers every ListMarketOverview with an empty page
// that has a next page token, and records each request.
func overviewListCapture(t *testing.T) (*MarketOverviewService, func() []*overviewpb.ListMarketOverviewRequest) {
	t.Helper()
	var mu sync.Mutex
	var sent []*overviewpb.ListMarketOverviewRequest
	server := hardening.NewTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		req := &overviewpb.ListMarketOverviewRequest{}
		if err := proto.Unmarshal(body, req); err != nil {
			t.Error(err)
		}
		mu.Lock()
		sent = append(sent, req)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/proto")
		out, _ := proto.Marshal(&overviewpb.ListMarketOverviewResponse{NextPageToken: "page-2"})
		_, _ = w.Write(out)
	}))
	t.Cleanup(server.Close)
	factory := transport.NewFactory(transport.Config{APIURL: server.URL, Timeout: 10 * time.Second}, nil, nil)
	return NewMarketOverviewService(factory, marketDataCatalogs(t), nil), func() []*overviewpb.ListMarketOverviewRequest {
		mu.Lock()
		defer mu.Unlock()
		return sent
	}
}

// ListWith sends the contract's sort key, direction and sparkline intervals
// (ListMarketOverviewRequest fields 4, 5 and 7).
func TestOverviewListWithSendsSortAndSparklineIntervals(t *testing.T) {
	orderBy := map[string]overviewpb.MarketOrderBy{
		"change_24h_bps": overviewpb.MarketOrderBy_ORDER_BY_CHANGE_24H_BPS,
		"volume_24h_usd": overviewpb.MarketOrderBy_ORDER_BY_VOLUME_24H_USD,
		"last_price":     overviewpb.MarketOrderBy_ORDER_BY_LAST_PRICE,
		"date_added":     overviewpb.MarketOrderBy_ORDER_BY_DATE_ADDED,
	}
	sortBy := map[string]overviewpb.SortDirection{"asc": overviewpb.SortDirection_SORT_ASC, "desc": overviewpb.SortDirection_SORT_DESC}
	for label, want := range orderBy {
		service, sent := overviewListCapture(t)
		if _, err := service.ListWith(context.Background(), ListMarketOverviewParams{Limit: 50, OrderBy: label}); err != nil {
			t.Fatalf("OrderBy %q: %v", label, err)
		}
		if got := sent()[0]; got.GetOrderBy() != want || got.GetSort() != overviewpb.SortDirection_SORT_DIRECTION_UNSPECIFIED {
			t.Fatalf("OrderBy %q sent order_by=%v sort=%v", label, got.GetOrderBy(), got.GetSort())
		}
	}
	for label, want := range sortBy {
		service, sent := overviewListCapture(t)
		if _, err := service.ListWith(context.Background(), ListMarketOverviewParams{Limit: 50, OrderBy: "last_price", Sort: label}); err != nil {
			t.Fatalf("Sort %q: %v", label, err)
		}
		if got := sent()[0].GetSort(); got != want {
			t.Fatalf("Sort %q sent %v", label, got)
		}
	}
	service, sent := overviewListCapture(t)
	intervals := []string{"SPARKLINE_1H", "SPARKLINE_24H", "SPARKLINE_1W", "SPARKLINE_1M"}
	if _, err := service.ListWith(context.Background(), ListMarketOverviewParams{Limit: 50, IncludeSparklines: true, SparklineIntervals: intervals}); err != nil {
		t.Fatal(err)
	}
	wantIntervals := []overviewpb.SparklineInterval{overviewpb.SparklineInterval_SPARKLINE_1H, overviewpb.SparklineInterval_SPARKLINE_24H, overviewpb.SparklineInterval_SPARKLINE_1W, overviewpb.SparklineInterval_SPARKLINE_1M}
	got := sent()[0].GetSparklineIntervals()
	if len(got) != len(wantIntervals) {
		t.Fatalf("sparkline_intervals sent %v", got)
	}
	for i := range got {
		if got[i] != wantIntervals[i] {
			t.Fatalf("sparkline_intervals sent %v", got)
		}
	}
}

// Unset, nothing is sent and the server keeps its default order; List sends
// the same request as before. The next page is asked with the same sort.
func TestOverviewListUnsetSortKeepsTheDefaultAndPagesKeepTheSort(t *testing.T) {
	service, sent := overviewListCapture(t)
	if _, err := service.List(context.Background(), nil, 50, "", false); err != nil {
		t.Fatal(err)
	}
	if !proto.Equal(sent()[0], &overviewpb.ListMarketOverviewRequest{Limit: 50}) {
		t.Fatalf("List sent %v", sent()[0])
	}
	params := ListMarketOverviewParams{Limit: 10, OrderBy: "date_added", Sort: "asc"}
	first, err := service.ListWith(context.Background(), params)
	if err != nil {
		t.Fatal(err)
	}
	params.PageToken = first.NextPageToken
	if _, err := service.ListWith(context.Background(), params); err != nil {
		t.Fatal(err)
	}
	second := sent()[2]
	if second.GetPageToken() != "page-2" || second.GetOrderBy() != overviewpb.MarketOrderBy_ORDER_BY_DATE_ADDED || second.GetSort() != overviewpb.SortDirection_SORT_ASC {
		t.Fatalf("next page sent %v", second)
	}
}

// An unknown value is refused before anything is sent.
func TestOverviewListWithRefusesUnknownValues(t *testing.T) {
	for name, params := range map[string]ListMarketOverviewParams{
		"order_by":              {OrderBy: "volume"},
		"order_by unspecified":  {OrderBy: "unspecified"},
		"sort":                  {Sort: "ascending"},
		"sparkline interval":    {SparklineIntervals: []string{"1h"}},
		"sparkline unspecified": {SparklineIntervals: []string{"SPARKLINE_INTERVAL_UNSPECIFIED"}},
	} {
		service, sent := overviewListCapture(t)
		params.Limit = 50
		_, err := service.ListWith(context.Background(), params)
		var validation *sdkerrors.ValidationError
		if !stderrors.As(err, &validation) {
			t.Fatalf("%s: err = %v, want a ValidationError", name, err)
		}
		if len(sent()) != 0 {
			t.Fatalf("%s: sent %d requests", name, len(sent()))
		}
	}
}

// List checks its arguments in the order it always has: a bad limit is
// refused before too many page tokens.
func TestOverviewListChecksTheLimitBeforeThePageTokens(t *testing.T) {
	service, sent := overviewListCapture(t)
	_, err := service.List(context.Background(), nil, -1, "", false, "a", "b")
	var validation *sdkerrors.ValidationError
	if !stderrors.As(err, &validation) || !strings.Contains(validation.Msg, "limit") || strings.Contains(validation.Msg, "page token") {
		t.Fatalf("List(limit=-1, two tokens): err = %v, want the limit refusal", err)
	}
	_, err = service.List(context.Background(), nil, 50, "", false, "a", "b")
	if !stderrors.As(err, &validation) || validation.Msg != "at most one page token is allowed" {
		t.Fatalf("List(two tokens): err = %v", err)
	}
	if len(sent()) != 0 {
		t.Fatalf("sent %d requests", len(sent()))
	}
}
