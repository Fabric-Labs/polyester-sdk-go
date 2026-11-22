package services

import (
	"context"
	"errors"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Fabric-Labs/polyester-sdk-go/auth"
	"github.com/Fabric-Labs/polyester-sdk-go/catalogs"
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	marketdatav1 "github.com/Fabric-Labs/polyester-sdk-go/gen/marketdata/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/internal/hardening"
	"github.com/Fabric-Labs/polyester-sdk-go/transport"
)

// acceptedCandleTimeframes is every candle timeframe the SDK accepts, exactly
// as written, with the enum it reads as and its live channel label. The
// Python and Rust SDKs accept the same set.
var acceptedCandleTimeframes = map[string]struct {
	value   marketdatav1.Timeframe
	channel string
}{
	"1s": {marketdatav1.Timeframe_SEC_1, "1s"}, "SEC_1": {marketdatav1.Timeframe_SEC_1, "1s"},
	"1m": {marketdatav1.Timeframe_MIN_1, "1m"}, "MIN_1": {marketdatav1.Timeframe_MIN_1, "1m"},
	"3m": {marketdatav1.Timeframe_MIN_3, "3m"}, "MIN_3": {marketdatav1.Timeframe_MIN_3, "3m"},
	"5m": {marketdatav1.Timeframe_MIN_5, "5m"}, "MIN_5": {marketdatav1.Timeframe_MIN_5, "5m"},
	"15m": {marketdatav1.Timeframe_MIN_15, "15m"}, "MIN_15": {marketdatav1.Timeframe_MIN_15, "15m"},
	"30m": {marketdatav1.Timeframe_MIN_30, "30m"}, "MIN_30": {marketdatav1.Timeframe_MIN_30, "30m"},
	"1h": {marketdatav1.Timeframe_HOUR_1, "1h"}, "HOUR_1": {marketdatav1.Timeframe_HOUR_1, "1h"},
	"2h": {marketdatav1.Timeframe_HOUR_2, "2h"}, "HOUR_2": {marketdatav1.Timeframe_HOUR_2, "2h"},
	"4h": {marketdatav1.Timeframe_HOUR_4, "4h"}, "HOUR_4": {marketdatav1.Timeframe_HOUR_4, "4h"},
	"6h": {marketdatav1.Timeframe_HOUR_6, "6h"}, "HOUR_6": {marketdatav1.Timeframe_HOUR_6, "6h"},
	"8h": {marketdatav1.Timeframe_HOUR_8, "8h"}, "HOUR_8": {marketdatav1.Timeframe_HOUR_8, "8h"},
	"12h": {marketdatav1.Timeframe_HOUR_12, "12h"}, "HOUR_12": {marketdatav1.Timeframe_HOUR_12, "12h"},
	"1d": {marketdatav1.Timeframe_DAY_1, "1d"}, "DAY_1": {marketdatav1.Timeframe_DAY_1, "1d"},
	"3d": {marketdatav1.Timeframe_DAY_3, "3d"}, "DAY_3": {marketdatav1.Timeframe_DAY_3, "3d"},
	"1w": {marketdatav1.Timeframe_WEEK_1, "1w"}, "WEEK_1": {marketdatav1.Timeframe_WEEK_1, "1w"},
	"1mo": {marketdatav1.Timeframe_MONTH_1, "1mo"}, "MONTH_1": {marketdatav1.Timeframe_MONTH_1, "1mo"},
}

// refusedCandleTimeframes are refused: another case or spelling of a label
// ("1M" is neither one minute nor one month here), an enum name in another
// case or without its underscore, the unspecified enum value, padding, and
// names that are not timeframes at all.
var refusedCandleTimeframes = []string{
	"1M", "1H", "1D", "1S", "1W", "1MO", "1Mo", "15M",
	"min_1", "min1", "MIN1", "Min_1", "month_1", "month1", "sec1",
	"TIMEFRAME_UNSPECIFIED", "BUY", "SELL", "buy", "sell",
	" 1m", "1m ", "", "1", "60", "1min", "1month",
}

func TestCandleTimeframesAcceptOnlyTheDocumentedSet(t *testing.T) {
	if len(candleTimeframes) != len(acceptedCandleTimeframes) {
		t.Fatalf("accepted %d timeframes, want %d", len(candleTimeframes), len(acceptedCandleTimeframes))
	}
	for label, want := range acceptedCandleTimeframes {
		got, err := resolveCandleTimeframe(label)
		if err != nil || got != want.value {
			t.Fatalf("resolveCandleTimeframe(%q) = %v, %v; want %v", label, got, err, want.value)
		}
		channel, err := resolveCandleChannelTimeframe(label)
		if err != nil || channel != want.channel {
			t.Fatalf("resolveCandleChannelTimeframe(%q) = %q, %v; want %q", label, channel, err, want.channel)
		}
	}
	for _, label := range refusedCandleTimeframes {
		var validation *sdkerrors.ValidationError
		if got, err := resolveCandleTimeframe(label); !errors.As(err, &validation) {
			t.Fatalf("resolveCandleTimeframe(%q) = %v, %v; want a ValidationError", label, got, err)
		}
		if got, err := resolveCandleChannelTimeframe(label); !errors.As(err, &validation) {
			t.Fatalf("resolveCandleChannelTimeframe(%q) = %q, %v; want a ValidationError", label, got, err)
		}
	}
}

// A refused timeframe is refused before the catalog wait, any request or
// any subscription.
func TestCandleCallsRefuseUnknownTimeframeBeforeSending(t *testing.T) {
	var sent atomic.Int64
	srv := hardening.NewTestServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		sent.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)
	creds, err := auth.LoadCredentials("ak_test", auth.GenerateEd25519Keypair().SecretKeyHex.Reveal(), false)
	if err != nil {
		t.Fatal(err)
	}
	cfg := transport.DefaultConfig()
	cfg.APIURL = srv.URL
	cats := catalogs.NewManager()
	_ = cats.BeginHydration() // never finished
	service := NewMarketDataService(transport.NewFactory(cfg, creds, nil), cats, nil)
	symbol := "BTC-USDT"
	token := ""
	calls := map[string]func(ctx context.Context, timeframe string) error{
		"GetCandles": func(ctx context.Context, timeframe string) error {
			_, err := service.GetCandles(ctx, &symbol, nil, timeframe, 10, nil, nil, false)
			return err
		},
		"GetCurrentCandle": func(ctx context.Context, timeframe string) error {
			_, err := service.GetCurrentCandle(ctx, &symbol, nil, timeframe)
			return err
		},
		"GetCandlesColumns": func(ctx context.Context, timeframe string) error {
			_, err := service.GetCandlesColumns(ctx, &symbol, nil, timeframe, 10, nil, nil, false, &token)
			return err
		},
		"SubscribeCandles": func(ctx context.Context, timeframe string) error {
			_, err := service.SubscribeCandles(ctx, &symbol, nil, timeframe)
			return err
		},
	}
	for name, call := range calls {
		for _, timeframe := range []string{"1M", "buy", "sell", "min_1", "TIMEFRAME_UNSPECIFIED"} {
			t.Run(name+"/"+timeframe, func(t *testing.T) {
				ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
				defer cancel()
				var validation *sdkerrors.ValidationError
				if err := call(ctx, timeframe); !errors.As(err, &validation) {
					t.Fatalf("err = %v, want a ValidationError", err)
				}
			})
		}
	}
	if n := sent.Load(); n != 0 {
		t.Fatalf("sent %d requests", n)
	}
}
