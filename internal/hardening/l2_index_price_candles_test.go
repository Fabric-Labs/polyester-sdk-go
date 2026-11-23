package hardening_test

import (
	"context"
	stderrors "errors"
	"net/http"
	"reflect"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"
	polyester "github.com/Fabric-Labs/polyester-sdk-go"
	"github.com/Fabric-Labs/polyester-sdk-go/auth"
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	"github.com/Fabric-Labs/polyester-sdk-go/gen/chain/zipper/v1/chainzipperv1connect"
	marketdatav1 "github.com/Fabric-Labs/polyester-sdk-go/gen/marketdata/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/gen/marketdata/v1/marketdatav1connect"
	"github.com/Fabric-Labs/polyester-sdk-go/internal/hardening"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// indexPriceCandleServer answers GetIndexPriceCandles with two candles,
// newest first: an index price candle and, before the listing, a reference
// candle. It records each request it receives. LTC-USDT's
// reference_price_scale is 6.
type indexPriceCandleServer struct {
	*selectorServer

	mu       sync.Mutex
	requests []*marketdatav1.GetIndexPriceCandlesRequest
	headers  []http.Header
}

func (s *indexPriceCandleServer) GetSpotConfig(ctx context.Context, req *connect.Request[marketdatav1.GetSpotConfigRequest]) (*connect.Response[marketdatav1.GetSpotConfigResponse], error) {
	resp, err := s.selectorServer.GetSpotConfig(ctx, req)
	if err != nil {
		return nil, err
	}
	resp.Msg.Pairs[1].ReferencePriceScale = 6
	return resp, nil
}

func (s *indexPriceCandleServer) GetIndexPriceCandles(_ context.Context, req *connect.Request[marketdatav1.GetIndexPriceCandlesRequest]) (*connect.Response[marketdatav1.GetIndexPriceCandlesResponse], error) {
	s.mu.Lock()
	s.requests = append(s.requests, proto.CloneOf(req.Msg))
	s.headers = append(s.headers, req.Header().Clone())
	s.mu.Unlock()
	resp := &marketdatav1.GetIndexPriceCandlesResponse{
		SymbolId: req.Msg.GetSymbolId(), Timeframe: req.Msg.GetTimeframe(),
		TsSec: []uint64{7200, 3600, 0},
		Open:  []int64{2_500_000, 2_000_000, 1_000_000},
		High:  []int64{3_000_000, 2_600_000, 1_000_000},
		Low:   []int64{2_400_000, 1_900_000, 1_000_000},
		Close: []int64{2_750_000, 2_500_000, 1_000_000},
		Source: []marketdatav1.IndexPriceCandleSource{
			marketdatav1.IndexPriceCandleSource_INDEX_PRICE_CANDLE_SOURCE_INDEX,
			marketdatav1.IndexPriceCandleSource_INDEX_PRICE_CANDLE_SOURCE_REFERENCE,
			9,
		},
	}
	if req.Msg.GetPageToken() == "" {
		resp.NextPageToken = "older"
	}
	return connect.NewResponse(resp), nil
}

func (s *indexPriceCandleServer) sent() []*marketdatav1.GetIndexPriceCandlesRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]*marketdatav1.GetIndexPriceCandlesRequest(nil), s.requests...)
}

func newIndexPriceCandleClient(t *testing.T, wire string, credentials bool) (*polyester.Client, *indexPriceCandleServer) {
	t.Helper()
	server := &indexPriceCandleServer{selectorServer: &selectorServer{}}
	mux := http.NewServeMux()
	mux.Handle(marketdatav1connect.NewMarketDataServiceHandler(server))
	mux.Handle(chainzipperv1connect.NewZipperServiceHandler(server))
	srv := hardening.NewTestServer(mux)
	t.Cleanup(srv.Close)
	cfg := polyester.Config{APIURL: srv.URL, WSURL: "ws://127.0.0.1:1/unused", WireFormat: wire, Timeout: 5 * time.Second}
	if credentials {
		kp := auth.GenerateEd25519Keypair()
		cfg.APIKeyID, cfg.APIPrivateKey = "ak_test", kp.SecretKeyHex
	}
	client, err := polyester.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	return client, server
}

// GetIndexPriceCandles sends the symbol resolved to its ID, the timeframe,
// limit, time range and page token, unsigned even with credentials, and
// decodes each row newest first at the pair's reference_price_scale with the
// series it comes from, in both wire formats.
func TestL2GetIndexPriceCandlesSendsTheRequestAndDecodesRows(t *testing.T) {
	for _, wire := range []string{"json", "binary"} {
		client, server := newIndexPriceCandleClient(t, wire, true)
		ctx := context.Background()
		symbol := "LTC-USDT"
		start := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
		end := start.Add(48 * time.Hour)
		got, err := client.MarketData.GetIndexPriceCandles(ctx, &symbol, nil, "1h", 3, &start, &end)
		if err != nil {
			t.Fatalf("%s: %v", wire, err)
		}
		want := models.IndexPriceCandlesResult{
			SymbolID: 8, Timeframe: "1h", NextPageToken: "older",
			Candles: []models.IndexPriceCandle{
				{TsSec: 7200, Open: "2.5", High: "3", Low: "2.4", Close: "2.75", Source: "index"},
				{TsSec: 3600, Open: "2", High: "2.6", Low: "1.9", Close: "2.5", Source: "reference"},
				{TsSec: 0, Open: "1", High: "1", Low: "1", Close: "1", Source: "unknown(9)"},
			},
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("%s: result = %+v, want %+v", wire, got, want)
		}

		// The next page: the page token as returned, the other arguments
		// unchanged, and the symbol given by ID.
		id := uint32(8)
		if _, err := client.MarketData.GetIndexPriceCandles(ctx, nil, &id, "HOUR_1", 3, &start, &end, got.NextPageToken); err != nil {
			t.Fatalf("%s: next page: %v", wire, err)
		}
		sent := server.sent()
		if len(sent) != 2 {
			t.Fatalf("%s: %d requests, want 2", wire, len(sent))
		}
		first := &marketdatav1.GetIndexPriceCandlesRequest{
			SymbolId: 8, Timeframe: marketdatav1.Timeframe_HOUR_1, Limit: 3,
			StartTime: timestamppb.New(start), EndTime: timestamppb.New(end),
		}
		next := proto.CloneOf(first)
		next.PageToken = "older"
		if !proto.Equal(sent[0], first) || !proto.Equal(sent[1], next) {
			t.Fatalf("%s: requests = %v, %v; want %v, %v", wire, sent[0], sent[1], first, next)
		}
		server.mu.Lock()
		for _, h := range server.headers {
			if h.Get("X-API-KEY-ID") != "" || h.Get("X-API-SIGNATURE") != "" {
				t.Errorf("%s: public read was signed: %v", wire, h)
			}
		}
		server.mu.Unlock()
	}
}

// Arguments the SDK can check are refused with an *errors.ValidationError
// before anything is sent. The server's own refusals ("1s", a time range) are left
// to it.
func TestL2GetIndexPriceCandlesRefusesBeforeSending(t *testing.T) {
	client, server := newIndexPriceCandleClient(t, "binary", false)
	ctx := context.Background()
	btc, unknown := "BTC-USDT", "DOGE-USDT"
	cases := map[string]func() error{
		"unknown timeframe": func() error {
			_, err := client.MarketData.GetIndexPriceCandles(ctx, &btc, nil, "1M", 10, nil, nil)
			return err
		},
		"two page tokens": func() error {
			_, err := client.MarketData.GetIndexPriceCandles(ctx, &btc, nil, "1h", 10, nil, nil, "a", "b")
			return err
		},
		"limit over 10000": func() error {
			_, err := client.MarketData.GetIndexPriceCandles(ctx, &btc, nil, "1h", 10001, nil, nil)
			return err
		},
		"negative limit": func() error {
			_, err := client.MarketData.GetIndexPriceCandles(ctx, &btc, nil, "1h", -1, nil, nil)
			return err
		},
		"unknown symbol": func() error {
			_, err := client.MarketData.GetIndexPriceCandles(ctx, &unknown, nil, "1h", 10, nil, nil)
			return err
		},
	}
	for name, call := range cases {
		var validation *sdkerrors.ValidationError
		if err := call(); !stderrors.As(err, &validation) {
			t.Errorf("%s: err = %v, want a ValidationError", name, err)
		}
	}
	if sent := server.sent(); len(sent) != 0 {
		t.Fatalf("%d refused requests were sent: %v", len(sent), sent)
	}
}
