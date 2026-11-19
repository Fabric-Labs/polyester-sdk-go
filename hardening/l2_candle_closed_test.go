package hardening_test

import (
	"context"
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
	"google.golang.org/protobuf/proto"
)

// candleClosedServer answers candle reads newest-first with the given newest
// bucket's is_closed, followed by one closed bucket.
type candleClosedServer struct {
	*selectorServer
	newestClosed bool
}

func (s *candleClosedServer) GetCandles(_ context.Context, req *connect.Request[marketdatav1.GetCandlesRequest]) (*connect.Response[marketdatav1.GetCandlesResponse], error) {
	points := func() []*marketdatav1.CandlePoint {
		return []*marketdatav1.CandlePoint{
			{TsSec: 120, Open: 1_500_000_000, High: 1_500_000_000, Low: 1_500_000_000, Close: 1_500_000_000, IsClosed: s.newestClosed},
			{TsSec: 60, Open: 1_000_000_000, High: 2_000_000_000, Low: 500_000_000, Close: 1_500_000_000, Volume: 300_000_000, IsClosed: true},
		}
	}
	resp := &marketdatav1.GetCandlesResponse{SymbolId: req.Msg.GetSymbolId(), Timeframe: req.Msg.GetTimeframe(), Candles: points()}
	if req.Msg.GetIncludeReference() {
		resp.ReferenceCandles = points()
	}
	return connect.NewResponse(resp), nil
}

func (s *candleClosedServer) GetCandlesColumns(_ context.Context, req *connect.Request[marketdatav1.GetCandlesColumnsRequest]) (*connect.Response[marketdatav1.GetCandlesColumnsResponse], error) {
	resp := &marketdatav1.GetCandlesColumnsResponse{
		SymbolId: req.Msg.GetSymbolId(), Timeframe: req.Msg.GetTimeframe(),
		TsSec: []uint64{60, 120}, Open: []int64{1_000_000_000, 1_500_000_000}, High: []int64{2_000_000_000, 1_500_000_000},
		Low: []int64{500_000_000, 1_500_000_000}, Close: []int64{1_500_000_000, 1_500_000_000}, Volume: []int64{300_000_000, 0},
	}
	if req.Msg.GetIncludeReference() {
		resp.ReferenceTsSec, resp.ReferenceOpen, resp.ReferenceHigh = resp.TsSec, resp.Open, resp.High
		resp.ReferenceLow, resp.ReferenceClose, resp.ReferenceVolume = resp.Low, resp.Close, resp.Volume
	}
	return connect.NewResponse(resp), nil
}

func newCandleClosedClient(t *testing.T, newestClosed bool, wire, wsURL string) *polyester.Client {
	t.Helper()
	server := &candleClosedServer{selectorServer: &selectorServer{}, newestClosed: newestClosed}
	mux := http.NewServeMux()
	mux.Handle(marketdatav1connect.NewMarketDataServiceHandler(server))
	mux.Handle(chainzipperv1connect.NewZipperServiceHandler(server))
	srv := hardening.NewTestServer(mux)
	t.Cleanup(srv.Close)
	client, err := polyester.New(polyester.Config{APIURL: srv.URL, WSURL: wsURL, WireFormat: wire, Timeout: 5 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	return client
}

// GetCandles rows, their reference rows and GetCurrentCandle report each
// bucket's is_closed in both wire formats: false while the newest bucket is
// open, true once it is closed, and true for every older bucket.
func TestL2CandleReadsReportIsClosed(t *testing.T) {
	for _, wire := range []string{"json", "binary"} {
		for _, newestClosed := range []bool{false, true} {
			client := newCandleClosedClient(t, newestClosed, wire, "ws://127.0.0.1:1/unused")
			id := uint32(1)
			ctx := context.Background()
			check := func(read string, candles []models.Candle) {
				t.Helper()
				if len(candles) != 2 {
					t.Fatalf("%s/%s: %d candles, want 2", wire, read, len(candles))
				}
				if candles[0].IsClosed != newestClosed {
					t.Errorf("%s/%s: newest bucket is_closed %t, want %t", wire, read, candles[0].IsClosed, newestClosed)
				}
				if !candles[1].IsClosed {
					t.Errorf("%s/%s: older bucket is_closed false, want true", wire, read)
				}
			}
			rows, err := client.MarketData.GetCandles(ctx, nil, &id, "1m", 2, nil, nil, true)
			if err != nil {
				t.Fatal(err)
			}
			check("rows", rows.Candles)
			withReference, err := client.MarketData.GetCandlesWithReference(ctx, nil, &id, "1m", 2, nil, nil, true, true)
			if err != nil {
				t.Fatal(err)
			}
			check("rows-with-reference", withReference.Candles)
			check("reference", withReference.ReferenceCandles)
			current, err := client.MarketData.GetCurrentCandle(ctx, nil, &id, "1m")
			if err != nil {
				t.Fatal(err)
			}
			if current == nil || current.TsSec != 120 || current.IsClosed != newestClosed {
				t.Errorf("%s/current: %+v, want ts 120 is_closed %t", wire, current, newestClosed)
			}
		}
	}
}

// SubscribeCandles reports is_closed on each live update: false while the
// bucket is open and true on its terminal update.
func TestL2SubscribeCandlesReportsIsClosed(t *testing.T) {
	publications := make(chan []byte, 2)
	ws := hardening.SpawnCentrifugoPublish(nil, publications)
	t.Cleanup(ws.Close)
	client := newCandleClosedClient(t, true, "binary", ws.WSURL())
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	symbol := "BTC-USDT"
	sub, err := client.MarketData.SubscribeCandles(ctx, &symbol, nil, "1m")
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close()
	for _, closed := range []bool{false, true} {
		payload, err := proto.Marshal(&marketdatav1.CandlePoint{TsSec: 60, Open: 1_000_000_000, High: 1_000_000_000, Low: 1_000_000_000, Close: 1_000_000_000, IsClosed: closed})
		if err != nil {
			t.Fatal(err)
		}
		publications <- payload
	}
	for _, want := range []bool{false, true} {
		select {
		case candle := <-sub.Messages():
			if candle.TsSec != 60 || candle.IsClosed != want {
				t.Errorf("live update ts %d is_closed %t, want ts 60 is_closed %t", candle.TsSec, candle.IsClosed, want)
			}
		case <-ctx.Done():
			t.Fatalf("no live update with is_closed %t", want)
		}
	}
}

// GetCandlesColumns and GetCandlesColumnsWithReference, whose response carries
// no closed flag, report every row closed without incomplete candles, and the
// newest row (ts 120) not known to be closed with them, in both wire formats.
func TestL2CandleColumnsReportIsClosedByIncomplete(t *testing.T) {
	for _, wire := range []string{"json", "binary"} {
		client := newCandleClosedClient(t, false, wire, "ws://127.0.0.1:1/unused")
		id := uint32(1)
		for _, includeIncomplete := range []bool{false, true} {
			result, err := client.MarketData.GetCandlesColumnsWithReference(context.Background(), nil, &id, "1m", 2, nil, nil, includeIncomplete, nil, true)
			if err != nil {
				t.Fatal(err)
			}
			plain, err := client.MarketData.GetCandlesColumns(context.Background(), nil, &id, "1m", 2, nil, nil, includeIncomplete, nil)
			if err != nil {
				t.Fatal(err)
			}
			for name, rows := range map[string][]models.Candle{"columns": plain.Candles, "with-reference": result.Candles, "reference": result.ReferenceCandles} {
				if len(rows) != 2 || rows[0].TsSec != 60 || rows[1].TsSec != 120 {
					t.Fatalf("%s/%s: rows %+v, want ts 60 then 120", wire, name, rows)
				}
				if !rows[0].IsClosed || rows[1].IsClosed == includeIncomplete {
					t.Errorf("%s/%s include_incomplete=%t: is_closed %t, %t; want true, %t", wire, name, includeIncomplete, rows[0].IsClosed, rows[1].IsClosed, !includeIncomplete)
				}
			}
		}
	}
}
