package services

import (
	"context"
	stderrors "errors"
	typev1 "github.com/Fabric-Labs/polyester-sdk-go/gen/polyester/type/v1"
	"testing"
	"time"

	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	"github.com/Fabric-Labs/polyester-sdk-go/gen/marketdata/v1/marketdatav1connect"
	"github.com/Fabric-Labs/polyester-sdk-go/hardening"
	"github.com/Fabric-Labs/polyester-sdk-go/transport"
)

func newHeatmapRequestService(t *testing.T) (*HeatmapService, *heatmapPagingHandler) {
	t.Helper()
	h := &heatmapPagingHandler{}
	_, handler := marketdatav1connect.NewHeatmapServiceHandler(h)
	server := hardening.NewTestServer(handler)
	t.Cleanup(server.Close)
	cfg := transport.DefaultConfig()
	cfg.APIURL = server.URL
	return NewHeatmapService(transport.NewFactory(cfg, nil, nil), nil, nil), h
}

// Get requests every supported depth as itself and any other depth up to 1000
// as the smallest supported depth covering it, as Orderbook.Get does.
func TestHeatmapGetRequestsTheSmallestSupportedDepthCoveringTheRequest(t *testing.T) {
	cases := map[int]typev1.DepthTier{
		0:    typev1.DepthTier_DEPTH_50,
		1:    typev1.DepthTier_DEPTH_1,
		2:    typev1.DepthTier_DEPTH_5,
		5:    typev1.DepthTier_DEPTH_5,
		10:   typev1.DepthTier_DEPTH_10,
		11:   typev1.DepthTier_DEPTH_20,
		20:   typev1.DepthTier_DEPTH_20,
		50:   typev1.DepthTier_DEPTH_50,
		100:  typev1.DepthTier_DEPTH_100,
		101:  typev1.DepthTier_DEPTH_200,
		200:  typev1.DepthTier_DEPTH_200,
		201:  typev1.DepthTier_DEPTH_500,
		500:  typev1.DepthTier_DEPTH_500,
		501:  typev1.DepthTier_DEPTH_1000,
		1000: typev1.DepthTier_DEPTH_1000,
	}
	for depth, want := range cases {
		service, h := newHeatmapRequestService(t)
		id := uint32(1)
		if _, err := service.Get(context.Background(), nil, &id, "1m", depth, 1, "close", nil, nil, nil); err != nil {
			t.Fatalf("depth %d: %v", depth, err)
		}
		h.mu.Lock()
		got := h.calls[0].Depth
		h.mu.Unlock()
		if got != want {
			t.Errorf("depth %d: wire = %s, want %s", depth, got, want)
		}
	}
}

func TestHeatmapGetRefusesDepthsNoSupportedDepthCovers(t *testing.T) {
	for _, depth := range []int{-1, 1001, 5000} {
		service, h := newHeatmapRequestService(t)
		id := uint32(1)
		_, err := service.Get(context.Background(), nil, &id, "1m", depth, 1, "close", nil, nil, nil)
		var validation *sdkerrors.ValidationError
		if !stderrors.As(err, &validation) {
			t.Errorf("depth %d: err = %v, want *errors.ValidationError", depth, err)
		}
		h.mu.Lock()
		calls := len(h.calls)
		h.mu.Unlock()
		if calls != 0 {
			t.Errorf("depth %d: sent %d requests", depth, calls)
		}
	}
}

// fromTsSec with an explicit end sends the same bounds on the first page and
// on its continuation, so the server accepts the token it issued.
func TestHeatmapGetKeepsAnExplicitEndWithFromTsSecAcrossPages(t *testing.T) {
	service, h := newHeatmapRequestService(t)
	id := uint32(1)
	from := int64(100)
	end := time.Unix(400, 0)
	first, err := service.Get(context.Background(), nil, &id, "1m", 50, 1, "peak", &from, nil, &end)
	if err != nil {
		t.Fatal(err)
	}
	token, _ := first.Raw["next_page_token"].(string)
	if _, err = service.Get(context.Background(), nil, &id, "1m", 50, 1, "peak", &from, nil, &end, token); err != nil {
		t.Fatal(err)
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	for page, call := range h.calls {
		tr := call.GetTimeRange()
		if tr.GetStartTime().GetSeconds() != 100 || tr.GetEndTime().GetSeconds() != 400 {
			t.Errorf("page %d: range %d..%d, want 100..400", page+1, tr.GetStartTime().GetSeconds(), tr.GetEndTime().GetSeconds())
		}
	}
}

// An interval and a quantity mode resolve only against their own enum: a value
// of the other one is refused before any request.
func TestHeatmapGetRefusesAValueOfTheOtherEnum(t *testing.T) {
	for _, tc := range []struct{ interval, mode string }{{"PEAK", "peak"}, {"1m", "INTERVAL_1M"}, {"CLOSE", "close"}, {"1m", "1m"}} {
		service, h := newHeatmapRequestService(t)
		id := uint32(1)
		_, err := service.Get(context.Background(), nil, &id, tc.interval, 50, 1, tc.mode, nil, nil, nil)
		var validation *sdkerrors.ValidationError
		if !stderrors.As(err, &validation) {
			t.Errorf("interval %q mode %q: err = %v, want *errors.ValidationError", tc.interval, tc.mode, err)
		}
		h.mu.Lock()
		calls := len(h.calls)
		h.mu.Unlock()
		if calls != 0 {
			t.Errorf("interval %q mode %q: sent %d requests", tc.interval, tc.mode, calls)
		}
	}
}

// INTERVAL_UNSPECIFIED is no interval the server reads or publishes.
func TestHeatmapGetRefusesTheUnspecifiedInterval(t *testing.T) {
	service, h := newHeatmapRequestService(t)
	id := uint32(1)
	_, err := service.Get(context.Background(), nil, &id, "INTERVAL_UNSPECIFIED", 50, 1, "close", nil, nil, nil)
	var validation *sdkerrors.ValidationError
	if !stderrors.As(err, &validation) {
		t.Fatalf("err = %v, want *errors.ValidationError", err)
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.calls) != 0 {
		t.Fatalf("sent %d requests", len(h.calls))
	}
}

// A heatmap reply names the tier it was requested at: one naming none, an
// unknown one, another tier or the unsupported DEPTH_5000 is an
// *errors.ResponseContractError.
func TestHeatmapGetRefusesAReplyNotAtTheRequestedTier(t *testing.T) {
	for name, served := range map[string]typev1.DepthTier{
		"unset":        typev1.DepthTier_DEPTH_UNSPECIFIED,
		"unknown":      typev1.DepthTier(99),
		"another tier": typev1.DepthTier_DEPTH_200,
		"unsupported":  typev1.DepthTier_DEPTH_5000,
	} {
		service, h := newHeatmapRequestService(t)
		h.served = &served
		id := uint32(1)
		_, err := service.Get(context.Background(), nil, &id, "1m", 50, 1, "close", nil, nil, nil)
		var contract *sdkerrors.ResponseContractError
		if !stderrors.As(err, &contract) {
			t.Fatalf("%s: err = %v, want a ResponseContractError", name, err)
		}
	}
}
