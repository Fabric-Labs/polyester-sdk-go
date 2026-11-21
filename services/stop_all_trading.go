package services

import (
	"context"
	"errors"
	"fmt"

	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	orderv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/orders/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

// stopAllTriggerPageLimit is ListTriggers' largest page.
const stopAllTriggerPageLimit = 1000

// stopAllLiveTriggerStatuses are the trigger statuses that can still place an
// order or hold reserved funds.
var stopAllLiveTriggerStatuses = []string{"created", "armed", "running", "paused"}

// StopAllTradingScope selects what StopAllTrading stops. SubAccountID and
// Symbol narrow both the triggers and the orders; nil means every sub-account
// the account scope reaches and every market. RequestID is the cancel-all
// request ID (its idempotency key). A blank Symbol, or a RequestID that is
// empty or padded, is refused before anything is canceled, as
// OrdersService.CancelAllWith refuses it.
type StopAllTradingScope struct {
	SubAccountID *string
	Symbol       *string
	RequestID    *string
}

// StopAllTradingTriggerFailure is a live trigger whose cancel failed: it may
// still be live. Each listed leg of an OCO whose cancel failed is one.
type StopAllTradingTriggerFailure struct {
	TriggerID string
	Err       error
}

// StopAllTradingResult reports what StopAllTrading canceled.
// CanceledTriggers are the listed triggers this call canceled, both legs of
// a canceled OCO included, with the status the cancel answered. A listed
// trigger that had already ended by its cancel is in neither list: it is
// stopped.
type StopAllTradingResult struct {
	CanceledTriggers []models.TriggerMutationResult
	FailedTriggers   []StopAllTradingTriggerFailure
	CancelAll        models.CancelAllOrdersResult
}

// StopAllTrading stops automated trading in scope: it lists every live
// trigger (created, armed, running and paused, attached legs included, page
// by page), cancels each one, then cancels all open orders. The legs of an
// OCO are canceled together, once, with CancelOco by their OrderListID, since
// canceling one leg ends the other. CancelAllOrders and the dead-man switch
// cancel open orders only, never triggers, so a trigger left live would place
// its order later; canceling the triggers first means an order a trigger
// places before its cancel is still canceled by the cancel-all.
//
// A trigger or OCO whose cancel is refused because it has already ended
// (errors.IsTriggerAlreadyEnded, or ERROR_CODE_TRIGGER_NOT_FOUND) after the
// listing named it live, for example after a fill or an expiry, is stopped:
// it is not a failure. A trigger whose cancel fails otherwise does not stop
// the others or the cancel-all: it is listed in FailedTriggers and the
// returned error joins those failures.
// A failed listing, a canceled ctx or a failed cancel-all returns at once
// with what was done so far. A listing fails, before anything is canceled,
// when ListTriggers answers with a page token it already sent in this run.
func StopAllTrading(ctx context.Context, triggers *TriggersService, orders *OrdersService, account AccountScope, scope StopAllTradingScope) (StopAllTradingResult, error) {
	var result StopAllTradingResult
	cancelAllParams := CancelAllParams{
		SubAccountID: scope.SubAccountID,
		Symbol:       scope.Symbol,
		RequestID:    scope.RequestID,
	}
	// The final cancel-all refuses a blank symbol or a request ID it would
	// not send as given: refuse it here, before any trigger is canceled.
	if err := checkCancelAllOrders(cancelAllParams); err != nil {
		return result, err
	}
	var targets []stopAllTarget
	seen := map[string]bool{}
	ocoTarget := map[string]int{}
	// Every page token sent in this run: a server that answers with any of
	// them again would page forever and the cancels would never start.
	sentTokens := map[string]bool{}
	var pageToken *string
	for {
		page, err := triggers.List(ctx, account, scope.SubAccountID, scope.Symbol, stopAllLiveTriggerStatuses, stopAllTriggerPageLimit, pageToken)
		if err != nil {
			return result, err
		}
		for _, trigger := range page.Triggers {
			if seen[trigger.TriggerID] {
				continue
			}
			seen[trigger.TriggerID] = true
			if trigger.OrderListID == "" {
				targets = append(targets, stopAllTarget{legs: []string{trigger.TriggerID}})
				continue
			}
			if i, ok := ocoTarget[trigger.OrderListID]; ok {
				targets[i].legs = append(targets[i].legs, trigger.TriggerID)
				continue
			}
			ocoTarget[trigger.OrderListID] = len(targets)
			targets = append(targets, stopAllTarget{orderListID: trigger.OrderListID, legs: []string{trigger.TriggerID}})
		}
		if page.NextPageToken == "" {
			break
		}
		if sentTokens[page.NextPageToken] {
			return result, &sdkerrors.ResponseContractError{
				Operation: "ListTriggers",
				Msg:       "next_page_token repeats an earlier page token; StopAllTrading stopped listing and nothing was canceled",
			}
		}
		next := page.NextPageToken
		sentTokens[next] = true
		pageToken = &next
	}
	var failures []error
	for _, target := range targets {
		canceled, err := target.cancel(ctx, triggers, account, scope.SubAccountID)
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return result, ctxErr
			}
			if stopAllAlreadyEnded(err) {
				continue
			}
			for _, triggerID := range target.legs {
				result.FailedTriggers = append(result.FailedTriggers, StopAllTradingTriggerFailure{TriggerID: triggerID, Err: err})
			}
			failures = append(failures, fmt.Errorf("%s: %w", target, err))
			continue
		}
		result.CanceledTriggers = append(result.CanceledTriggers, canceled...)
	}
	cancelAll, err := orders.CancelAllWith(ctx, account, cancelAllParams)
	if err != nil {
		return result, err
	}
	result.CancelAll = cancelAll
	return result, errors.Join(failures...)
}

// stopAllTarget is one cancel of StopAllTrading: a trigger, or an OCO with
// the legs the listing named.
type stopAllTarget struct {
	orderListID string
	legs        []string
}

func (t stopAllTarget) String() string {
	if t.orderListID != "" {
		return "cancel OCO " + t.orderListID
	}
	return "cancel trigger " + t.legs[0]
}

// cancel cancels the trigger, or the OCO by its OrderListID, and returns the
// listed triggers it canceled.
func (t stopAllTarget) cancel(ctx context.Context, triggers *TriggersService, account AccountScope, subAccountID *string) ([]models.TriggerMutationResult, error) {
	if t.orderListID == "" {
		canceled, err := triggers.Cancel(ctx, account, t.legs[0], subAccountID)
		if err != nil {
			return nil, err
		}
		return []models.TriggerMutationResult{canceled}, nil
	}
	canceled, err := triggers.CancelOco(ctx, account, t.orderListID, subAccountID)
	if err != nil {
		return nil, err
	}
	out := make([]models.TriggerMutationResult, 0, len(t.legs))
	for _, triggerID := range t.legs {
		out = append(out, models.TriggerMutationResult{TriggerID: triggerID, Status: canceled.Status})
	}
	return out, nil
}

// stopAllAlreadyEnded reports a cancel refused because the trigger or OCO,
// listed as live, has ended since: it is stopped.
func stopAllAlreadyEnded(err error) bool {
	if sdkerrors.IsTriggerAlreadyEnded(err) {
		return true
	}
	var apiErr *sdkerrors.APIError
	return errors.As(err, &apiErr) && apiErr.Code == orderv1.ErrorCode_ERROR_CODE_TRIGGER_NOT_FOUND.String()
}
