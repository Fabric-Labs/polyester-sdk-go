package services

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

func createLadder(s *TriggersService, orderType string) error {
	opts := baseTriggerOptions("ladder")
	_, err := s.Create(context.Background(), nil, "BTC-USDT", "ladder", nil, "buy", models.QtyFromDecimal("0.0002"), orderType, nil, "", "", nil, nil, true, opts)
	return err
}

// POLY-9423: a ladder places only limit GTC orders, so an explicit market
// (or any other non-limit) order type is refused before sending, never
// remapped to limit. A ladder without an order type, or with limit, is sent.
func TestPoly9423LadderRefusesNonLimitOrderType(t *testing.T) {
	for _, orderType := range []string{"market", "MARKET", "stop"} {
		t.Run(orderType, func(t *testing.T) {
			h := &optionCapture{}
			err := createLadder(newTriggerControlServiceWith(t, h), orderType)
			var validation *sdkerrors.ValidationError
			if !errors.As(err, &validation) || !strings.Contains(validation.Msg, "order_type") {
				t.Errorf("ladder order_type=%q: want order_type ValidationError, got %v", orderType, err)
			}
			h.mu.Lock()
			defer h.mu.Unlock()
			if len(h.requests) != 0 {
				t.Fatalf("ladder order_type=%q sent %d requests", orderType, len(h.requests))
			}
		})
	}
	for _, orderType := range []string{"", "limit", "Limit"} {
		t.Run("allowed/"+orderType, func(t *testing.T) {
			h := &optionCapture{}
			if err := createLadder(newTriggerControlServiceWith(t, h), orderType); err != nil {
				t.Fatalf("ladder order_type=%q: %v", orderType, err)
			}
			h.mu.Lock()
			defer h.mu.Unlock()
			if len(h.requests) != 1 || h.requests[0].GetTrigger().GetLadder() == nil || !h.requests[0].GetTrigger().GetLadder().GetPostOnly() {
				t.Fatalf("ladder order_type=%q: sent %+v", orderType, h.requests)
			}
		})
	}
}

// POLY-9426: modify takes 0 to 10000 bps as ModifyTriggerRequest does; the
// server decides 10000 by side and trigger type (a buy stop-loss or
// take-profit market child takes it). 10001 and negatives are refused before
// sending.
func TestPoly9426ModifyAcceptsTenThousandBps(t *testing.T) {
	for _, bps := range []int32{0, 1, 9999, 10_000} {
		h := &triggerControlHandler{}
		svc := newTriggerControlService(t, h)
		value := bps
		if _, err := svc.Modify(context.Background(), nil, codecs.FormatID(9), nil, nil, nil, codecs.ModifyTriggerOptions{MaxSlippageBps: &value}); err != nil {
			t.Errorf("modify max_slippage_bps=%d: %v", bps, err)
			continue
		}
		h.mu.Lock()
		if len(h.modify) != 1 || h.modify[0].GetMaxSlippage() == nil || h.modify[0].GetMaxSlippageBps() != bps {
			t.Errorf("modify max_slippage_bps=%d sent %+v", bps, h.modify)
		}
		h.mu.Unlock()
	}
	for _, bps := range []int32{10_001, -1} {
		h := &triggerControlHandler{}
		svc := newTriggerControlService(t, h)
		value := bps
		_, err := svc.Modify(context.Background(), nil, codecs.FormatID(9), nil, nil, nil, codecs.ModifyTriggerOptions{MaxSlippageBps: &value})
		var validation *sdkerrors.ValidationError
		if !errors.As(err, &validation) || !strings.Contains(validation.Msg, "between 0 and 10000") {
			t.Errorf("modify max_slippage_bps=%d: want ValidationError naming 0 to 10000, got %v", bps, err)
		}
		h.mu.Lock()
		if len(h.modify) != 0 {
			t.Errorf("modify max_slippage_bps=%d sent %d requests", bps, len(h.modify))
		}
		h.mu.Unlock()
	}
}
