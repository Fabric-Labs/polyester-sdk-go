package services

import (
	"connectrpc.com/connect"
	"context"
	heatmap "github.com/Fabric-Labs/polyester-sdk-go/gen/marketdata/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/gen/marketdata/v1/marketdatav1connect"
	"github.com/Fabric-Labs/polyester-sdk-go/transport"
	"google.golang.org/protobuf/proto"
	"sync"
	"testing"
	"time"

	"github.com/Fabric-Labs/polyester-sdk-go/hardening"
)

type heatmapPagingHandler struct {
	marketdatav1connect.UnimplementedHeatmapServiceHandler
	mu    sync.Mutex
	calls []*heatmap.GetOrderbookHeatmapRequest
}

func (h *heatmapPagingHandler) GetOrderbookHeatmap(_ context.Context, r *connect.Request[heatmap.GetOrderbookHeatmapRequest]) (*connect.Response[heatmap.GetOrderbookHeatmapResponse], error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.calls = append(h.calls, proto.Clone(r.Msg).(*heatmap.GetOrderbookHeatmapRequest))
	token := ""
	if len(h.calls) == 1 {
		token = "eyJoZWF0bWFwIjoxfQ=="
	}
	return connect.NewResponse(&heatmap.GetOrderbookHeatmapResponse{SymbolId: 1, NextPageToken: token, LastPersistedTsSec: 100}), nil
}
func TestReadHeatmapOpaquePaging(t *testing.T) {
	for _, wire := range []string{"binary", "json"} {
		for _, explicit := range []bool{false, true} {
			t.Run(wire+map[bool]string{true: "explicit", false: "default"}[explicit], func(t *testing.T) {
				h := &heatmapPagingHandler{}
				_, handler := marketdatav1connect.NewHeatmapServiceHandler(h)
				server := hardening.NewTestServer(handler)
				defer server.Close()
				cfg := transport.DefaultConfig()
				cfg.APIURL = server.URL
				cfg.WireFormat = transport.WireFormatFromString(wire)
				service := NewHeatmapService(transport.NewFactory(cfg, nil, nil), nil, nil)
				var start, end *time.Time
				if explicit {
					s, e := time.Unix(100, 0), time.Unix(400, 0)
					start, end = &s, &e
				}
				id := uint32(1)
				first, err := service.Get(context.Background(), nil, &id, "1m", 50, 1, "peak", nil, start, end)
				if err != nil {
					t.Fatal(err)
				}
				token, ok := first.Raw["next_page_token"].(string)
				if !ok || token != "eyJoZWF0bWFwIjoxfQ==" {
					t.Fatalf("missing token: %+v", first)
				}
				second, err := service.Get(context.Background(), nil, &id, "1m", 50, 1, "peak", nil, start, end, token)
				if err != nil {
					t.Fatal(err)
				}
				if tok, _ := second.Raw["next_page_token"].(string); tok != "" {
					t.Fatal(second)
				}
				h.mu.Lock()
				defer h.mu.Unlock()
				if len(h.calls) != 2 {
					t.Fatalf("calls=%d", len(h.calls))
				}
				a, b := h.calls[0], h.calls[1]
				if a.PageToken != "" || b.PageToken != token || a.TimeRange == nil {
					t.Fatal(h.calls)
				}
				if explicit {
					if !proto.Equal(a.TimeRange, b.TimeRange) {
						t.Fatal("range changed")
					}
				} else if b.TimeRange != nil {
					t.Fatal("continuation invented a time window")
				}
				a.TimeRange = nil
				b.TimeRange = nil
				b.PageToken = ""
				if !proto.Equal(a, b) || a.SymbolId != 1 || a.Limit != 1 {
					t.Fatal("filters changed")
				}
			})
		}
	}
}

func TestReadHeatmapTokenPartialBoundsStayExplicit(t *testing.T) {
	for _, bound := range []string{"from", "start", "end"} {
		t.Run(bound, func(t *testing.T) {
			h := &heatmapPagingHandler{}
			_, handler := marketdatav1connect.NewHeatmapServiceHandler(h)
			server := hardening.NewTestServer(handler)
			defer server.Close()
			cfg := transport.DefaultConfig()
			cfg.APIURL = server.URL
			service := NewHeatmapService(transport.NewFactory(cfg, nil, nil), nil, nil)
			var from *int64
			var start, end *time.Time
			value := int64(100)
			instant := time.Unix(value, 0)
			switch bound {
			case "from":
				from = &value
			case "start":
				start = &instant
			case "end":
				end = &instant
			}
			id := uint32(1)
			_, err := service.Get(context.Background(), nil, &id, "1m", 50, 1, "peak", from, start, end, "opaque")
			if err != nil {
				t.Fatal(err)
			}
			h.mu.Lock()
			defer h.mu.Unlock()
			if len(h.calls) != 1 {
				t.Fatalf("calls=%d", len(h.calls))
			}
			tr := h.calls[0].TimeRange
			if tr == nil || (tr.StartTime != nil) != (bound != "end") || (tr.EndTime != nil) != (bound == "end") {
				t.Fatal(tr)
			}
			if bound == "end" {
				if tr.EndTime.Seconds != 100 {
					t.Fatal(tr)
				}
			} else if tr.StartTime.Seconds != 100 {
				t.Fatal(tr)
			}
		})
	}
}
