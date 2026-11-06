package services

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"connectrpc.com/connect"
	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	triggersv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/triggers/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/gen/triggers/v1/triggersv1connect"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
	"google.golang.org/protobuf/reflect/protoreflect"
)

type optionCapture struct {
	triggersv1connect.UnimplementedTriggersServiceHandler
	mu       sync.Mutex
	requests []*triggersv1.CreateTriggerRequest
}

func (h *optionCapture) CreateTrigger(_ context.Context, req *connect.Request[triggersv1.CreateTriggerRequest]) (*connect.Response[triggersv1.CreateTriggerResponse], error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.requests = append(h.requests, req.Msg)
	return connect.NewResponse(&triggersv1.CreateTriggerResponse{TriggerId: 1}), nil
}

func optionPtr[T any](v T) *T { return &v }

type triggerOptionCase struct {
	name, allowed, wire string
	set                 func(*codecs.CreateTriggerOptions)
	value               int64
}

func TestTriggerTypeOptionMatrix(t *testing.T) {
	price := models.PriceFromDecimal("90")
	cases := []triggerOptionCase{
		{"trailing_distance_ticks", "trailing_stop", "trailing_distance_ticks", func(o *codecs.CreateTriggerOptions) {
			o.TrailingDistanceBps = nil
			o.TrailingDistanceTicks = optionPtr(int64(5))
		}, 5},
		{"trailing_distance_bps", "trailing_stop", "trailing_distance_bps", func(o *codecs.CreateTriggerOptions) { o.TrailingDistanceBps = optionPtr(int32(50)) }, 50},
		{"activation_price", "trailing_stop", "activation_price_ticks", func(o *codecs.CreateTriggerOptions) { o.ActivationPrice = &price }, 90_000_000_000},
		{"twap_duration_ms", "twap", "duration_ms", func(o *codecs.CreateTriggerOptions) { o.TwapDurationMs = optionPtr(int64(60000)) }, 60000},
		{"twap_slice_interval_ms", "twap", "slice_interval_ms", func(o *codecs.CreateTriggerOptions) { o.TwapSliceIntervalMs = optionPtr(int64(10000)) }, 10000},
		{"ladder_price_min", "ladder", "price_min_ticks", func(o *codecs.CreateTriggerOptions) { o.LadderPriceMin = &price }, 90_000_000_000},
		{"ladder_price_max", "ladder", "price_max_ticks", func(o *codecs.CreateTriggerOptions) { o.LadderPriceMax = optionPtr(models.PriceFromDecimal("110")) }, 110_000_000_000},
		{"ladder_levels", "ladder", "levels", func(o *codecs.CreateTriggerOptions) { o.LadderLevels = optionPtr(int32(4)) }, 4},
		{"ladder_distribution", "ladder", "levels", func(o *codecs.CreateTriggerOptions) { o.LadderDistribution = optionPtr("linear") }, 4},
	}
	for _, kind := range []string{"stop_loss", "take_profit", "trailing_stop", "twap", "ladder"} {
		for _, tc := range cases {
			t.Run(kind+"/"+tc.name, func(t *testing.T) {
				handler := &optionCapture{}
				service := newTriggerControlServiceWith(t, handler)
				opts := baseTriggerOptions(kind)
				tc.set(&opts)
				err := createWithOptions(service, kind, nil, "market", nil, "", "gtc", false, opts)
				handler.mu.Lock()
				defer handler.mu.Unlock()
				if kind != tc.allowed {
					var validation *sdkerrors.ValidationError
					if !errors.As(err, &validation) || !strings.Contains(err.Error(), tc.name) || !strings.Contains(err.Error(), tc.allowed) {
						t.Errorf("want named applicability ValidationError, got %v", err)
					}
					if len(handler.requests) != 0 {
						t.Fatalf("refused option sent %d requests", len(handler.requests))
					}
				} else {
					if err != nil {
						t.Fatal(err)
					}
					if len(handler.requests) != 1 {
						t.Fatalf("want 1 request, got %d", len(handler.requests))
					}
					strategy := handler.requests[0].GetTrigger().ProtoReflect().Get(handler.requests[0].GetTrigger().ProtoReflect().Descriptor().Fields().ByName(protoreflect.Name(kind))).Message()
					got := strategy.Get(strategy.Descriptor().Fields().ByName(protoreflect.Name(tc.wire))).Int()
					if got != tc.value {
						t.Fatalf("%s wire value=%d want %d", tc.name, got, tc.value)
					}
				}
			})
		}
	}
}

func baseTriggerOptions(kind string) codecs.CreateTriggerOptions {
	switch kind {
	case "trailing_stop":
		return codecs.CreateTriggerOptions{TrailingDistanceBps: optionPtr(int32(50))}
	case "twap":
		return codecs.CreateTriggerOptions{TwapDurationMs: optionPtr(int64(60000)), TwapSliceIntervalMs: optionPtr(int64(10000))}
	case "ladder":
		return codecs.CreateTriggerOptions{LadderPriceMin: optionPtr(models.PriceFromDecimal("90")), LadderPriceMax: optionPtr(models.PriceFromDecimal("110")), LadderLevels: optionPtr(int32(4))}
	default:
		return codecs.CreateTriggerOptions{}
	}
}

func createWithOptions(s *TriggersService, kind string, triggerPrice *models.PriceInput, orderType string, limit *models.PriceInput, source, tif string, post bool, opts codecs.CreateTriggerOptions) error {
	if triggerPrice == nil && (kind == "stop_loss" || kind == "take_profit") {
		triggerPrice = optionPtr(models.PriceFromDecimal("100"))
	}
	_, err := s.Create(context.Background(), nil, "BTC-USDT", kind, triggerPrice, "sell", models.QtyFromDecimal("0.01"), orderType, limit, source, tif, nil, nil, post, opts)
	return err
}

func TestTriggerExecutionApplicability(t *testing.T) {
	for _, kind := range []string{"stop_loss", "take_profit", "trailing_stop", "twap", "ladder"} {
		for _, field := range []string{"trigger_price", "trigger_price_source", "post_only", "limit_price", "tif", "max_slippage_ticks", "max_slippage_bps"} {
			for _, orderType := range []string{"market", "limit"} {
				// Limit trailing sells have no limit child selector; covered by their fixed execution.
				if kind == "trailing_stop" && orderType == "limit" {
					continue
				}
				t.Run(kind+"/"+field+"/"+orderType, func(t *testing.T) {
					h := &optionCapture{}
					service := newTriggerControlServiceWith(t, h)
					opts := baseTriggerOptions(kind)
					var triggerPrice, limit *models.PriceInput
					source, tif, post := "", "gtc", false
					if orderType == "limit" && kind != "ladder" {
						limit = optionPtr(models.PriceFromDecimal("100"))
					}
					allowed := false
					wire := ""
					switch field {
					case "trigger_price":
						triggerPrice = optionPtr(models.PriceFromDecimal("100"))
						allowed = kind == "stop_loss" || kind == "take_profit"
						wire = "trigger_price_ticks"
					case "trigger_price_source":
						source = "index"
						allowed = kind != "twap" && kind != "ladder"
						wire = "trigger_price_source"
					case "post_only":
						post = true
						allowed = kind == "ladder" || (orderType == "limit" && (kind == "stop_loss" || kind == "take_profit"))
						wire = "post_only"
					case "limit_price":
						limit = optionPtr(models.PriceFromDecimal("100"))
						allowed = orderType == "limit" && kind != "ladder"
						wire = "price_ticks"
					case "tif":
						tif = "fok"
						allowed = orderType == "limit" && (kind == "stop_loss" || kind == "take_profit")
					case "max_slippage_ticks":
						opts.MaxSlippageTicks = optionPtr(int64(5))
						allowed = kind == "trailing_stop" || (kind != "ladder" && orderType == "market")
						wire = field
					case "max_slippage_bps":
						opts.MaxSlippageBps = optionPtr(int32(5))
						allowed = kind == "trailing_stop" || (kind != "ladder" && orderType == "market")
						wire = field
					}
					err := createWithOptions(service, kind, triggerPrice, orderType, limit, source, tif, post, opts)
					h.mu.Lock()
					defer h.mu.Unlock()
					if !allowed {
						var validation *sdkerrors.ValidationError
						if !errors.As(err, &validation) || !strings.Contains(err.Error(), field) {
							t.Errorf("want %s ValidationError, got %v", field, err)
						}
						if len(h.requests) != 0 {
							t.Fatalf("refused option sent %d requests", len(h.requests))
						}
						return
					}
					if err != nil {
						t.Fatal(err)
					}
					if len(h.requests) != 1 {
						t.Fatalf("want 1 request got %d", len(h.requests))
					}
					strategy := h.requests[0].GetTrigger().ProtoReflect()
					strategy = strategy.Get(strategy.Descriptor().Fields().ByName(protoreflect.Name(kind))).Message()
					if field == "trigger_price" || field == "trigger_price_source" || kind == "ladder" || kind == "trailing_stop" {
						value := strategy.Get(strategy.Descriptor().Fields().ByName(protoreflect.Name(wire)))
						switch field {
						case "post_only":
							if !value.Bool() {
								t.Fatal("post_only lost")
							}
						case "trigger_price_source":
							if value.Enum() != 2 {
								t.Fatal("source lost")
							}
						default:
							expected := int64(5)
							if field == "trigger_price" {
								expected = 100_000_000_000
							}
							if value.Int() != expected {
								t.Fatalf("%s: got %d want %d", field, value.Int(), expected)
							}
						}
						return
					}
					if kind != "twap" {
						strategy = strategy.Get(strategy.Descriptor().Fields().ByName("child")).Message()
					}
					execution := strategy.WhichOneof(strategy.Descriptor().Oneofs().ByName("execution"))
					if field == "tif" {
						if execution.Name() != "limit_fok" {
							t.Fatal("FOK lost")
						}
						return
					}
					strategy = strategy.Get(execution).Message()
					value := strategy.Get(strategy.Descriptor().Fields().ByName(protoreflect.Name(wire)))
					if field == "post_only" {
						if !value.Bool() {
							t.Fatal("post_only lost")
						}
					} else {
						expected := int64(5)
						if field == "limit_price" {
							expected = 100_000_000_000
						}
						if value.Int() != expected {
							t.Fatalf("%s: got %d want %d", field, value.Int(), expected)
						}
					}
				})
			}
		}
	}
}
