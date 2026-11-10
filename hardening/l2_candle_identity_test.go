package hardening_test

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"connectrpc.com/connect"
	polyester "github.com/Fabric-Labs/polyester-sdk-go"
	"github.com/Fabric-Labs/polyester-sdk-go/gen/chain/zipper/v1/chainzipperv1connect"
	marketdatav1 "github.com/Fabric-Labs/polyester-sdk-go/gen/marketdata/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/gen/marketdata/v1/marketdatav1connect"
	"github.com/Fabric-Labs/polyester-sdk-go/hardening"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

// candleIdentityServer serves the two-market catalog of selectorServer and
// answers candle reads with the requested market and timeframe, with one
// candle per series or with empty series.
type candleIdentityServer struct {
	*selectorServer
	empty bool
}

func (s *candleIdentityServer) points() []*marketdatav1.CandlePoint {
	if s.empty {
		return nil
	}
	return []*marketdatav1.CandlePoint{{TsSec: 60, Open: 1_000_000_000, High: 2_000_000_000, Low: 500_000_000, Close: 1_500_000_000, Volume: 300_000_000}}
}

func (s *candleIdentityServer) GetCandles(_ context.Context, req *connect.Request[marketdatav1.GetCandlesRequest]) (*connect.Response[marketdatav1.GetCandlesResponse], error) {
	resp := &marketdatav1.GetCandlesResponse{SymbolId: req.Msg.GetSymbolId(), Timeframe: req.Msg.GetTimeframe(), Candles: s.points()}
	if req.Msg.GetIncludeReference() {
		resp.ReferenceCandles = s.points()
	}
	if !s.empty {
		resp.NextPageToken = "cursor-2"
	}
	return connect.NewResponse(resp), nil
}

func (s *candleIdentityServer) GetCandlesColumns(_ context.Context, req *connect.Request[marketdatav1.GetCandlesColumnsRequest]) (*connect.Response[marketdatav1.GetCandlesColumnsResponse], error) {
	resp := &marketdatav1.GetCandlesColumnsResponse{SymbolId: req.Msg.GetSymbolId(), Timeframe: req.Msg.GetTimeframe()}
	for _, p := range s.points() {
		resp.TsSec = append(resp.TsSec, p.TsSec)
		resp.Open = append(resp.Open, p.Open)
		resp.High = append(resp.High, p.High)
		resp.Low = append(resp.Low, p.Low)
		resp.Close = append(resp.Close, p.Close)
		resp.Volume = append(resp.Volume, p.Volume)
	}
	if !s.empty {
		resp.NextPageToken = "cursor-2"
	}
	return connect.NewResponse(resp), nil
}

// MarketData.GetCandles and GetCandlesWithReference report the market and
// timeframe the response names, as GetCandlesColumns does, for populated and
// empty series in both wire formats.
func TestL2RowCandlesReportResponseIdentity(t *testing.T) {
	for _, wire := range []string{"json", "binary"} {
		for _, empty := range []bool{false, true} {
			server := &candleIdentityServer{selectorServer: &selectorServer{}, empty: empty}
			mux := http.NewServeMux()
			mux.Handle(marketdatav1connect.NewMarketDataServiceHandler(server))
			mux.Handle(chainzipperv1connect.NewZipperServiceHandler(server))
			srv := hardening.NewTestServer(mux)
			client, err := polyester.New(polyester.Config{APIURL: srv.URL, WireFormat: wire, HydrateCatalogs: true, Timeout: 5 * time.Second})
			if err != nil {
				t.Fatal(err)
			}
			for _, market := range []struct {
				id     uint32
				symbol string
			}{{1, "BTC-USDT"}, {8, "LTC-USDT"}} {
				for _, timeframe := range []string{"1m", "5m"} {
					name := fmt.Sprintf("%s/empty=%t/%s/%s", wire, empty, market.symbol, timeframe)
					t.Run(name, func(t *testing.T) {
						id := market.id
						reads := map[string]func() (models.CandlesResult, error){
							"rows": func() (models.CandlesResult, error) {
								return client.MarketData.GetCandles(context.Background(), nil, &id, timeframe, 3, nil, nil, false)
							},
							"rows-with-reference": func() (models.CandlesResult, error) {
								return client.MarketData.GetCandlesWithReference(context.Background(), nil, &id, timeframe, 3, nil, nil, false, true)
							},
							"columns": func() (models.CandlesResult, error) {
								return client.MarketData.GetCandlesColumns(context.Background(), nil, &id, timeframe, 3, nil, nil, false, nil)
							},
						}
						for read, call := range reads {
							got, err := call()
							if err != nil {
								t.Fatalf("%s: %v", read, err)
							}
							if got.SymbolID != market.id || got.Timeframe != timeframe {
								t.Errorf("%s: SymbolID=%d Timeframe=%q, want %d %q", read, got.SymbolID, got.Timeframe, market.id, timeframe)
							}
							wantCandles, wantCursor := 1, "cursor-2"
							if empty {
								wantCandles, wantCursor = 0, ""
							}
							if len(got.Candles) != wantCandles || got.NextPageToken != wantCursor {
								t.Errorf("%s: %d candle(s), cursor %q; want %d, %q", read, len(got.Candles), got.NextPageToken, wantCandles, wantCursor)
							}
							if !empty && got.Candles[0].Close == "" {
								t.Errorf("%s: candle lost its close: %+v", read, got.Candles[0])
							}
						}
					})
				}
			}
			_ = client.Close()
			srv.Close()
		}
	}
}
