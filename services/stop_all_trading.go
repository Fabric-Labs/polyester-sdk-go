package services

import (
	"context"
	"errors"
	"fmt"

	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
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
// request ID (its idempotency key).
type StopAllTradingScope struct {
	SubAccountID *string
	Symbol       *string
	RequestID    *string
}

// StopAllTradingTriggerFailure is a live trigger whose cancel failed.
type StopAllTradingTriggerFailure struct {
	TriggerID string
	Err       error
}

// StopAllTradingResult reports what StopAllTrading canceled.
type StopAllTradingResult struct {
	CanceledTriggers []models.TriggerMutationResult
	FailedTriggers   []StopAllTradingTriggerFailure
	CancelAll        models.CancelAllOrdersResult
}

// StopAllTrading stops automated trading in scope: it lists every live
// trigger (created, armed, running and paused, attached legs included, page
// by page), cancels each one, then cancels all open orders. CancelAllOrders
// and the dead-man switch cancel open orders only, never triggers, so a
// trigger left live would place its order later; canceling the triggers
// first means an order a trigger places before its cancel is still canceled
// by the cancel-all.
//
// A trigger whose cancel fails does not stop the others or the cancel-all: it
// is listed in FailedTriggers and the returned error joins those failures.
// A failed listing, a canceled ctx or a failed cancel-all returns at once
// with what was done so far. A listing fails, before anything is canceled,
// when ListTriggers answers with a page token it already sent in this run.
func StopAllTrading(ctx context.Context, triggers *TriggersService, orders *OrdersService, account AccountScope, scope StopAllTradingScope) (StopAllTradingResult, error) {
	var result StopAllTradingResult
	var triggerIDs []string
	seen := map[string]bool{}
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
			if !seen[trigger.TriggerID] {
				seen[trigger.TriggerID] = true
				triggerIDs = append(triggerIDs, trigger.TriggerID)
			}
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
	for _, triggerID := range triggerIDs {
		canceled, err := triggers.Cancel(ctx, account, triggerID, scope.SubAccountID)
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return result, ctxErr
			}
			result.FailedTriggers = append(result.FailedTriggers, StopAllTradingTriggerFailure{TriggerID: triggerID, Err: err})
			failures = append(failures, fmt.Errorf("cancel trigger %s: %w", triggerID, err))
			continue
		}
		result.CanceledTriggers = append(result.CanceledTriggers, canceled)
	}
	cancelAll, err := orders.CancelAllWith(ctx, account, CancelAllParams{
		SubAccountID: scope.SubAccountID,
		Symbol:       scope.Symbol,
		RequestID:    scope.RequestID,
	})
	if err != nil {
		return result, err
	}
	result.CancelAll = cancelAll
	return result, errors.Join(failures...)
}
