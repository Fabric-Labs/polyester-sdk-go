package services

import (
	"context"
	"fmt"
	"strings"

	"github.com/Fabric-Labs/polyester-sdk-go/catalogs"
	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	"github.com/Fabric-Labs/polyester-sdk-go/codecs/decode"
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	triggersv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/triggers/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/gen/triggers/v1/triggersv1connect"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
	"github.com/Fabric-Labs/polyester-sdk-go/realtime"
	"github.com/Fabric-Labs/polyester-sdk-go/transport"
)

type TriggersService struct {
	transport            *transport.Factory
	catalogs             *catalogs.Manager
	scoped               ScopedSubAccount
	defaultAccountID     *string
	realtime             RealtimeClient
	catalogHydrationDone <-chan struct{}
	catalogLastError     func() error
}

func NewTriggersService(
	factory *transport.Factory,
	cats *catalogs.Manager,
	defaultSubAccountID *string,
	realtime RealtimeClient,
	defaultAccountID *string,
	catalogHydrationDone <-chan struct{},
	catalogLastError func() error,
) *TriggersService {
	return &TriggersService{
		transport:            factory,
		catalogs:             cats,
		scoped:               ScopedSubAccount{DefaultSubAccountID: defaultSubAccountID},
		realtime:             realtime,
		defaultAccountID:     defaultAccountID,
		catalogHydrationDone: catalogHydrationDone,
		catalogLastError:     catalogLastError,
	}
}

func (s *TriggersService) ensureCatalogs(ctx context.Context) error {
	if s.catalogHydrationDone == nil {
		return awaitCatalogs(ctx, s.catalogs, true)
	}
	select {
	case <-s.catalogHydrationDone:
		if s.catalogLastError != nil {
			return hydrationFailure(s.catalogLastError())
		}
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *TriggersService) client() triggersv1connect.TriggersServiceClient {
	return triggersv1connect.NewTriggersServiceClient(s.transport.HTTP, s.transport.Config.APIURL, s.transport.ConnectOptions(true)...)
}

// List reads triggers. It retries UNAVAILABLE with the same request for
// about 2 s (25 ms, doubling, capped at 500 ms): trigger reads answer it until
// they reflect every answered trigger change.
//
// One optional models.ListTriggersOptions filters by trigger type and by
// parent order; an invalid filter is refused before sending.
func (s *TriggersService) List(ctx context.Context, account AccountScope, subAccountID, symbol *string, status []string, limit int, pageToken *string, opts ...models.ListTriggersOptions) (models.TriggersList, error) {
	if len(opts) > 1 {
		return models.TriggersList{}, &sdkerrors.ValidationError{Msg: "triggers.List accepts at most one ListTriggersOptions value"}
	}
	parsedLimit, err := PaginationLimit(limit, "limit")
	if err != nil {
		return models.TriggersList{}, err
	}
	req := &triggersv1.ListTriggersRequest{Limit: parsedLimit}
	if err := codecs.CheckPageLimit(req); err != nil {
		return models.TriggersList{}, err
	}
	if len(opts) == 1 {
		if err := applyListTriggersOptions(req, opts[0]); err != nil {
			return models.TriggersList{}, err
		}
	}
	if pageToken != nil && *pageToken != "" {
		req.PageToken = *pageToken
	}
	sub, err := s.scoped.ResolveSubAccountID(subAccountID, account)
	if err != nil {
		return models.TriggersList{}, err
	}
	if parsed, err := codecs.ParseOptionalSubaccountID(sub); err != nil {
		return models.TriggersList{}, err
	} else if parsed != nil {
		req.SubaccountId = parsed
	}
	if namesSymbol(symbol) {
		if err := s.ensureCatalogs(ctx); err != nil {
			return models.TriggersList{}, err
		}
	}
	resolvedSymbolID, err := ResolveOptionalSymbolID(s.catalogs, symbol, nil, "triggers.list")
	if err != nil {
		return models.TriggersList{}, err
	}
	req.SymbolId = resolvedSymbolID
	for _, label := range status {
		st, err := decode.TriggerStatusFromLabel(label)
		if err != nil {
			return models.TriggersList{}, err
		}
		req.Status = append(req.Status, st)
	}
	scales, err := readQuantityScales(ctx, s.catalogs)
	if err != nil {
		return models.TriggersList{}, err
	}
	return UnaryAuthDecoded(ctx, s.transport, retryUnavailable(triggerReadRetry, s.client().ListTriggers), req, scaledRead(decode.TriggersListFromProto, s.readTriggersList, scales))
}

// Get reads one trigger. Like List, it retries a catching-up UNAVAILABLE
// with the same request for about 2 s, so a read after an answered Modify,
// Pause or Resume returns the trigger as changed.
func (s *TriggersService) Get(ctx context.Context, account AccountScope, triggerID string, subAccountID *string) (*models.Trigger, error) {
	id, err := codecs.IDToInt(triggerID, "trigger_id")
	if err != nil {
		return nil, err
	}
	req := &triggersv1.GetTriggerRequest{TriggerId: id}
	sub, err := s.scoped.ResolveSubAccountID(subAccountID, account)
	if err != nil {
		return nil, err
	}
	if parsed, err := codecs.ParseOptionalSubaccountID(sub); err != nil {
		return nil, err
	} else if parsed != nil {
		req.SubaccountId = parsed
	}
	scales, err := readQuantityScales(ctx, s.catalogs)
	if err != nil {
		return nil, err
	}
	return UnaryAuthDecoded(ctx, s.transport, retryUnavailable(triggerReadRetry, s.client().GetTrigger), req, scaledRead(decode.GetTriggerFromProto, s.readTrigger, scales))
}

func (s *TriggersService) Create(ctx context.Context, account AccountScope, symbol, triggerType string, triggerPrice *models.PriceInput, side string, qty models.QtyInput, orderType string, limitPrice *models.PriceInput, triggerPriceSource, tif string, subAccountID *string, clientTriggerID *string, postOnly bool, opts codecs.CreateTriggerOptions) (models.TriggerMutationResult, error) {
	if err := s.ensureCatalogs(ctx); err != nil {
		return models.TriggerMutationResult{}, err
	}
	sub, err := s.scoped.ResolveSubAccountID(subAccountID, account)
	if err != nil {
		return models.TriggerMutationResult{}, err
	}
	// With hydration on, the catalogs are loaded by now: an unknown symbol is
	// refused as unknown, not for its scale. Without it the scale comes first.
	hydrated := s.catalogs != nil && s.catalogs.Hydration() != nil
	var symbolID uint32
	if hydrated {
		if symbolID, err = ResolveSymbolID(s.catalogs, &symbol, nil, "triggers.create"); err != nil {
			return models.TriggerMutationResult{}, err
		}
	}
	scale, err := codecs.QuantityScaleForSymbol(s.catalogs, &symbol)
	if err != nil {
		return models.TriggerMutationResult{}, err
	}
	if !hydrated {
		if symbolID, err = ResolveSymbolID(s.catalogs, &symbol, nil, "triggers.create"); err != nil {
			return models.TriggerMutationResult{}, err
		}
	}
	req, err := codecs.CreateTriggerToProto(symbolID, symbol, triggerType, triggerPrice, side, qty, orderType, limitPrice, triggerPriceSource, tif, sub, clientTriggerID, postOnly, scale, opts)
	if err != nil {
		return models.TriggerMutationResult{}, err
	}
	return UnaryAuthDecoded(ctx, s.transport, s.client().CreateTrigger, req, decode.TriggerMutationFromProto)
}

// Cancel cancels a trigger. A trigger that has already ended (completed,
// canceled or failed) is refused with ERROR_CODE_TRIGGER_STATUS_INVALID for at
// least 60 seconds after it ended, a repeat of a cancel that succeeded
// included; errors.IsTriggerAlreadyEnded reports it, and it means "already
// done": read the trigger for how it ended. Later the same cancel may answer
// ERROR_CODE_TRIGGER_NOT_FOUND, which Get still resolves for a trigger that
// ended; treat it as done too for a trigger you know you created. Pause,
// Resume and Modify of an ended trigger, a ladder or TWAP included, answer the
// same way. Trigger IDs are opaque, account-scoped public IDs: use one only as
// the API gave it, for the account that owns it.
func (s *TriggersService) Cancel(ctx context.Context, account AccountScope, triggerID string, subAccountID *string) (models.TriggerMutationResult, error) {
	id, err := codecs.IDToInt(triggerID, "trigger_id")
	if err != nil {
		return models.TriggerMutationResult{}, err
	}
	req := &triggersv1.CancelTriggerRequest{TriggerId: id}
	sub, err := s.scoped.ResolveSubAccountID(subAccountID, account)
	if err != nil {
		return models.TriggerMutationResult{}, err
	}
	if parsed, err := codecs.ParseOptionalSubaccountID(sub); err != nil {
		return models.TriggerMutationResult{}, err
	} else if parsed != nil {
		req.SubaccountId = parsed
	}
	return UnaryAuthDecoded(ctx, s.transport, s.client().CancelTrigger, req, decode.TriggerMutationFromCancel)
}

// CreateOco creates a one-cancels-the-other order list: a take-profit and a
// stop-loss over a quantity the account already holds, reserved once for both
// legs. When one leg's order trades the whole quantity, the other is
// canceled; a partly filled order's rest stays protected by the other leg.
// Cancel All Orders and the dead-man switch do not cancel OCOs; an OCO leg's
// working order they cancel cancels the OCO.
//
// The result names the OCO's OrderListID, which both legs carry as
// Trigger.OrderListID, and each leg's trigger ID. A create whose outcome is
// unknown can be sent again with the same in.ClientListID and intent: it
// returns the same OCO (see models.CreateOcoInput).
func (s *TriggersService) CreateOco(ctx context.Context, account AccountScope, in models.CreateOcoInput, subAccountID *string) (models.CreateOcoResult, error) {
	if err := s.ensureCatalogs(ctx); err != nil {
		return models.CreateOcoResult{}, err
	}
	sub, err := s.scoped.ResolveSubAccountID(subAccountID, account)
	if err != nil {
		return models.CreateOcoResult{}, err
	}
	// As in Create: with hydration on, an unknown symbol is refused as
	// unknown, not for its scale.
	hydrated := s.catalogs != nil && s.catalogs.Hydration() != nil
	var symbolID uint32
	if hydrated {
		if symbolID, err = ResolveSymbolID(s.catalogs, &in.Symbol, nil, "triggers.create_oco"); err != nil {
			return models.CreateOcoResult{}, err
		}
	}
	scale, err := codecs.QuantityScaleForSymbol(s.catalogs, &in.Symbol)
	if err != nil {
		return models.CreateOcoResult{}, err
	}
	if !hydrated {
		if symbolID, err = ResolveSymbolID(s.catalogs, &in.Symbol, nil, "triggers.create_oco"); err != nil {
			return models.CreateOcoResult{}, err
		}
	}
	req, err := codecs.CreateOcoToProto(symbolID, in, sub, scale)
	if err != nil {
		return models.CreateOcoResult{}, err
	}
	return UnaryAuthDecoded(ctx, s.transport, s.client().CreateOco, req, decode.CreateOcoFromProto)
}

// CancelOco cancels an OCO by the OrderListID CreateOco returned: both legs
// end canceled and the reserved quantity is released. Canceling either leg
// with Cancel does the same. An ID that is not an OCO of the account, such as
// a trigger ID or a leg's ID, is refused with TRIGGER_NOT_FOUND and nothing
// is canceled. An OCO that has already ended is refused with
// ERROR_CODE_TRIGGER_STATUS_INVALID (errors.IsTriggerAlreadyEnded).
func (s *TriggersService) CancelOco(ctx context.Context, account AccountScope, orderListID string, subAccountID *string) (models.CancelOcoResult, error) {
	id, err := codecs.IDToInt(orderListID, "order_list_id")
	if err != nil {
		return models.CancelOcoResult{}, err
	}
	if id == 0 {
		return models.CancelOcoResult{}, &sdkerrors.ValidationError{Msg: "order_list_id must be a non-zero OCO ID"}
	}
	req := &triggersv1.CancelOcoRequest{OrderListId: id}
	sub, err := s.scoped.ResolveSubAccountID(subAccountID, account)
	if err != nil {
		return models.CancelOcoResult{}, err
	}
	if req.SubaccountId, err = codecs.ParseOptionalSubaccountID(sub); err != nil {
		return models.CancelOcoResult{}, err
	}
	return UnaryAuthDecoded(ctx, s.transport, s.client().CancelOco, req, decode.CancelOcoFromProto)
}

// BatchCancel cancels 1 to codecs.MaxBatchCancelTriggers distinct triggers
// in one request, each as Cancel cancels it: a TWAP's or ladder's open child
// orders are canceled with it. Results come back per trigger, in request
// order (see models.BatchCancelTriggersResultItem). Each trigger counts one
// against the account's cancel rate limit, as a Cancel does.
//
// requestID identifies the request for 2 minutes; one is generated when it
// is nil or empty. A request whose outcome is unknown can be sent again
// unchanged, with the same requestID and trigger IDs: it returns the
// original results and cancels nothing again. The same requestID with other
// trigger IDs rejects every item with CONFLICT_IDEMPOTENCY_KEY_REUSE.
func (s *TriggersService) BatchCancel(ctx context.Context, account AccountScope, triggerIDs []string, subAccountID *string, requestID *string) (models.BatchCancelTriggersResult, error) {
	sub, err := s.scoped.ResolveSubAccountID(subAccountID, account)
	if err != nil {
		return models.BatchCancelTriggersResult{}, err
	}
	req, err := codecs.BatchCancelTriggersToProto(triggerIDs, sub, requestID)
	if err != nil {
		return models.BatchCancelTriggersResult{}, err
	}
	return UnaryAuthDecoded(ctx, s.transport, s.client().BatchCancelTriggers, req, decode.BatchCancelTriggersFromProto)
}

// CancelAllTriggersParams selects the triggers Triggers.CancelAll cancels.
// Every filter is optional; together they narrow the match.
type CancelAllTriggersParams struct {
	SubAccountID *string
	// Symbol, Symbols or SymbolIDs (at most one of them) select the pairs,
	// at most codecs.MaxCancelAllTriggerSymbols; none matches every pair. A
	// supplied symbol must name a pair: a blank Symbol, or a blank entry in
	// Symbols, is refused rather than read as every pair. Leave Symbol nil
	// and Symbols empty to cancel on every pair.
	Symbol    *string
	Symbols   []string
	SymbolIDs []uint32
	// Side ("buy" or "sell") matches triggers whose orders trade on that
	// side: a sell stop-loss matches "sell". Nil matches both sides.
	Side *string
	// TriggerTypes lists up to codecs.MaxCancelAllTriggerTypes types
	// (stop_loss, take_profit, trailing_stop, twap, ladder), as List reports
	// them; empty matches every type.
	TriggerTypes []string
	// DryRun counts the matching triggers without canceling any.
	DryRun bool
	// RequestID is the idempotency key, 1 to 64 characters of
	// [A-Za-z0-9._:/-]. Generate it once per user action and keep it: a
	// retry needs the same key and criteria. When nil or empty a key is
	// generated, but it is not returned, so such a request cannot be
	// retried safely; supply your own whenever a retry may follow.
	RequestID *string
}

// CancelAll cancels, in one request, every trigger of the account that
// matches params, and no order (CancelAllTriggers). What it cancels is fixed
// when its request ID is first accepted: triggers created later keep
// running. Each matching trigger ends canceled with cancel reason
// "cancel_all" and releases its reserved funds, and none places an order
// after the response. A TWAP's working slice, a ladder's open levels and an
// attached leg's working exit are canceled with their trigger; an order a
// stop already placed stays open. An order's attached take-profit, stop-loss
// and trailing legs end together, as do an OCO's two legs: when one matches,
// all are canceled and counted.
//
// The result lists no trigger IDs: read the triggers again afterwards. Every
// request counts 10 units of the cancel rate limit, a dry run and a retry
// included. A retry with the same RequestID and criteria within two minutes
// returns the original count and cancels nothing again; the same RequestID
// with other criteria is refused with CONFLICT_IDEMPOTENCY_KEY_REUSE. After
// an unknown outcome, retry with the same RequestID, so supply one. Request IDs are a
// separate key space from Orders.CancelAll.
func (s *TriggersService) CancelAll(ctx context.Context, account AccountScope, params CancelAllTriggersParams) (models.CancelAllTriggersResult, error) {
	// A blank symbol filter would resolve to no pair and so match every pair.
	if params.Symbol != nil && strings.TrimSpace(*params.Symbol) == "" {
		return models.CancelAllTriggersResult{}, &sdkerrors.ValidationError{Msg: "triggers.cancel_all symbol must not be blank; leave it nil to match every pair"}
	}
	for i, symbol := range params.Symbols {
		if strings.TrimSpace(symbol) == "" {
			return models.CancelAllTriggersResult{}, &sdkerrors.ValidationError{Msg: fmt.Sprintf("triggers.cancel_all symbols[%d] must not be blank; leave symbols empty to match every pair", i)}
		}
	}
	sub, err := s.scoped.ResolveSubAccountID(params.SubAccountID, account)
	if err != nil {
		return models.CancelAllTriggersResult{}, err
	}
	if namesSymbol(params.Symbol, params.Symbols...) {
		if err := s.ensureCatalogs(ctx); err != nil {
			return models.CancelAllTriggersResult{}, err
		}
	}
	symbolIDs, err := ResolveCancelAllSymbolIDs(s.catalogs, params.Symbol, params.Symbols, params.SymbolIDs, "triggers.cancel_all")
	if err != nil {
		return models.CancelAllTriggersResult{}, err
	}
	req, err := codecs.CancelAllTriggersToProto(sub, symbolIDs, params.Side, params.TriggerTypes, params.DryRun, params.RequestID)
	if err != nil {
		return models.CancelAllTriggersResult{}, err
	}
	return UnaryAuthDecoded(ctx, s.transport, s.client().CancelAllTriggers, req, decode.CancelAllTriggersFromProto)
}

// Pause pauses a trigger. An ended trigger is refused with
// ERROR_CODE_TRIGGER_STATUS_INVALID (errors.IsTriggerAlreadyEnded), as for
// Cancel.
func (s *TriggersService) Pause(ctx context.Context, account AccountScope, triggerID string, subAccountID *string) (models.TriggerMutationResult, error) {
	id, err := codecs.IDToInt(triggerID, "trigger_id")
	if err != nil {
		return models.TriggerMutationResult{}, err
	}
	req := &triggersv1.PauseTriggerRequest{TriggerId: id}
	sub, err := s.scoped.ResolveSubAccountID(subAccountID, account)
	if err != nil {
		return models.TriggerMutationResult{}, err
	}
	if parsed, err := codecs.ParseOptionalSubaccountID(sub); err != nil {
		return models.TriggerMutationResult{}, err
	} else if parsed != nil {
		req.SubaccountId = parsed
	}
	return UnaryAuthDecoded(ctx, s.transport, s.client().PauseTrigger, req, decode.TriggerMutationFromPause)
}

// Resume resumes a paused trigger; an ended one is refused with
// ERROR_CODE_TRIGGER_STATUS_INVALID, as for Cancel. symbol and symbolID are optional: omitted,
// the request targets the trigger's own market; when set, it must be that
// market. An API key limited to specific markets must pass one.
func (s *TriggersService) Resume(ctx context.Context, account AccountScope, triggerID string, subAccountID, symbol *string, symbolID *uint32) (models.TriggerMutationResult, error) {
	id, err := codecs.IDToInt(triggerID, "trigger_id")
	if err != nil {
		return models.TriggerMutationResult{}, err
	}
	if namesSymbol(symbol) {
		if err := s.ensureCatalogs(ctx); err != nil {
			return models.TriggerMutationResult{}, err
		}
	}
	resolvedSymbolID, err := ResolveOptionalSymbolID(s.catalogs, symbol, symbolID, "triggers.resume")
	if err != nil {
		return models.TriggerMutationResult{}, err
	}
	req := &triggersv1.ResumeTriggerRequest{TriggerId: id, SymbolId: resolvedSymbolID}
	sub, err := s.scoped.ResolveSubAccountID(subAccountID, account)
	if err != nil {
		return models.TriggerMutationResult{}, err
	}
	if parsed, err := codecs.ParseOptionalSubaccountID(sub); err != nil {
		return models.TriggerMutationResult{}, err
	} else if parsed != nil {
		req.SubaccountId = parsed
	}
	return UnaryAuthDecoded(ctx, s.transport, s.client().ResumeTrigger, req, decode.TriggerMutationFromResume)
}

// Modify patches a trigger. An ended trigger is refused with
// ERROR_CODE_TRIGGER_STATUS_INVALID, as for Cancel; a running ladder or TWAP,
// which supports only cancel, with ERROR_CODE_TRIGGER_NOT_MODIFIABLE. symbol
// and symbolID are optional, as for Resume.
// When either is given, a price bound to another market is refused.
func (s *TriggersService) Modify(ctx context.Context, account AccountScope, triggerID string, subAccountID, symbol *string, symbolID *uint32, opts codecs.ModifyTriggerOptions) (models.TriggerMutationResult, error) {
	sub, err := s.scoped.ResolveSubAccountID(subAccountID, account)
	if err != nil {
		return models.TriggerMutationResult{}, err
	}
	if namesSymbol(symbol) {
		if err := s.ensureCatalogs(ctx); err != nil {
			return models.TriggerMutationResult{}, err
		}
	}
	resolvedSymbolID, err := ResolveOptionalSymbolID(s.catalogs, symbol, symbolID, "triggers.modify")
	if err != nil {
		return models.TriggerMutationResult{}, err
	}
	// A bound price is checked against the named market; one that can't be
	// named can't be checked, so the price is refused.
	market := ""
	if bound := boundPriceSymbol(opts.TriggerPrice, opts.ActivationPrice); resolvedSymbolID != 0 && bound != "" {
		if s.catalogs != nil {
			if err := s.ensureCatalogs(ctx); err != nil {
				return models.TriggerMutationResult{}, err
			}
			if canonical := s.catalogs.SymbolForSymbolID(resolvedSymbolID); canonical != nil {
				market = *canonical
			}
		}
		if market == "" {
			return models.TriggerMutationResult{}, &sdkerrors.ValidationError{Msg: fmt.Sprintf(
				"triggers.modify cannot check a price bound to %s against symbol_id %d: the market is not in the catalogs; hydrate catalogs or pass an unbound price",
				bound, resolvedSymbolID,
			)}
		}
	}
	req, err := codecs.ModifyTriggerToProto(triggerID, resolvedSymbolID, market, sub, opts)
	if err != nil {
		return models.TriggerMutationResult{}, err
	}
	return UnaryAuthDecoded(ctx, s.transport, s.client().ModifyTrigger, req, decode.TriggerMutationFromModify)
}

func (s *TriggersService) ListEvents(ctx context.Context, account AccountScope, triggerID string, subAccountID *string, limit int, eventType *string, pageToken *string) (models.TriggerEventsList, error) {
	id, err := codecs.IDToInt(triggerID, "trigger_id")
	if err != nil {
		return models.TriggerEventsList{}, err
	}
	parsedLimit, err := PaginationLimit(limit, "limit")
	if err != nil {
		return models.TriggerEventsList{}, err
	}
	req := &triggersv1.ListTriggerEventsRequest{TriggerId: id, Limit: parsedLimit}
	if err := codecs.CheckPageLimit(req); err != nil {
		return models.TriggerEventsList{}, err
	}
	sub, err := s.scoped.ResolveSubAccountID(subAccountID, account)
	if err != nil {
		return models.TriggerEventsList{}, err
	}
	if parsed, err := codecs.ParseOptionalSubaccountID(sub); err != nil {
		return models.TriggerEventsList{}, err
	} else if parsed != nil {
		req.SubaccountId = parsed
	}
	if eventType != nil && *eventType != "" {
		parsed, err := decode.TriggerEventTypeFromLabel(*eventType)
		if err != nil {
			return models.TriggerEventsList{}, err
		}
		req.EventType = parsed
	}
	if pageToken != nil && *pageToken != "" {
		req.PageToken = *pageToken
	}
	return UnaryAuthDecoded(ctx, s.transport, retryUnavailable(triggerReadRetry, s.client().ListTriggerEvents), req, decode.TriggerEventsListFromProto)
}

func (s *TriggersService) Subscribe(ctx context.Context, accountID any) (*realtime.Subscription[models.Trigger], error) {
	return SubscribeAccountProto(ctx, s.realtime, "private:spot:triggers:{account_id}:proto", accountID, s.defaultAccountID, s.decodeStreamTrigger)
}

// decodeStreamTrigger decodes one trigger stream publication with the
// catalogs loaded when it arrives.
func (s *TriggersService) decodeStreamTrigger(payload []byte) (models.Trigger, error) {
	trigger, err := decode.TriggerFromBytes(payload)
	return s.nameTrigger(decode.ScaleTrigger(trigger, streamQuantityScales(s.catalogs))), err
}

// applyListTriggersOptions sets ListTriggers' trigger type and parent order
// filters.
func applyListTriggersOptions(req *triggersv1.ListTriggersRequest, opts models.ListTriggersOptions) error {
	if opts.TriggerType != "" {
		triggerType, err := codecs.TriggerTypeFromLabel(opts.TriggerType)
		if err != nil {
			return err
		}
		req.TriggerType = triggerType
	}
	if opts.ParentOrderID != nil {
		id, err := codecs.IDToInt(*opts.ParentOrderID, "parent_order_id")
		if err != nil {
			return err
		}
		if id == 0 {
			return &sdkerrors.ValidationError{Msg: "parent_order_id must be a non-zero order ID"}
		}
		req.ParentOrderId = &id
	}
	if opts.OrderListID != nil {
		id, err := codecs.IDToInt(*opts.OrderListID, "order_list_id")
		if err != nil {
			return err
		}
		if id == 0 {
			return &sdkerrors.ValidationError{Msg: "order_list_id must be a non-zero OCO ID"}
		}
		req.OrderListId = &id
	}
	if opts.ClientListID != nil {
		id, err := codecs.ValidateClientListID(*opts.ClientListID)
		if err != nil {
			return err
		}
		req.ClientListId = &id
	}
	return nil
}

// nameTrigger sets a trigger's Symbol from the loaded catalogs; it stays
// empty when they do not know the pair.
func (s *TriggersService) nameTrigger(trigger models.Trigger) models.Trigger {
	if trigger.Symbol == "" && s.catalogs != nil {
		if symbol := s.catalogs.SymbolForSymbolID(trigger.SymbolID); symbol != nil {
			trigger.Symbol = *symbol
		}
	}
	return trigger
}

func (s *TriggersService) SubscribeEvents(ctx context.Context, accountID any) (*realtime.Subscription[models.TriggerEvent], error) {
	return SubscribeAccountProtoBatch(ctx, s.realtime, "private:spot:triggers:events:{account_id}:proto", accountID, s.defaultAccountID, decode.TriggerEventBatchFromBytes)
}

// boundPriceSymbol returns the market the first bound price is bound to, or
// "" when no price carries a market binding.
func boundPriceSymbol(prices ...*models.PriceInput) string {
	for _, price := range prices {
		if price == nil {
			continue
		}
		if ticks, ok := price.TicksValue(); ok && ticks.Symbol() != "" {
			return ticks.Symbol()
		}
	}
	return ""
}

// readTriggersList attaches the pair's base quantity scale and name to each
// trigger.
func (s *TriggersService) readTriggersList(list models.TriggersList, scales decode.BaseQuantityScales) models.TriggersList {
	for i := range list.Triggers {
		list.Triggers[i] = s.nameTrigger(decode.ScaleTrigger(list.Triggers[i], scales))
	}
	return list
}

// readTrigger attaches the pair's base quantity scale and name to a trigger
// read.
func (s *TriggersService) readTrigger(trigger *models.Trigger, scales decode.BaseQuantityScales) *models.Trigger {
	if trigger == nil {
		return nil
	}
	read := s.nameTrigger(decode.ScaleTrigger(*trigger, scales))
	return &read
}
