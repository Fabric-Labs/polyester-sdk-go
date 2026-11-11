package services

import (
	"context"
	"fmt"

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
	return UnaryAuth(ctx, s.transport, retryUnavailable(triggerReadRetry, s.client().ListTriggers), req, scaledRead(decode.TriggersListFromProto, s.readTriggersList, scales))
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
	return UnaryAuth(ctx, s.transport, retryUnavailable(triggerReadRetry, s.client().GetTrigger), req, scaledRead(decode.GetTriggerFromProto, s.readTrigger, scales))
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
	return UnaryAuth(ctx, s.transport, s.client().CreateTrigger, req, decode.TriggerMutationFromProto)
}

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
	return UnaryAuth(ctx, s.transport, s.client().CancelTrigger, req, decode.TriggerMutationFromCancel)
}

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
	return UnaryAuth(ctx, s.transport, s.client().PauseTrigger, req, decode.TriggerMutationFromPause)
}

// Resume resumes a paused trigger. symbol and symbolID are optional: omitted,
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
	return UnaryAuth(ctx, s.transport, s.client().ResumeTrigger, req, decode.TriggerMutationFromResume)
}

// Modify patches a trigger. symbol and symbolID are optional, as for Resume.
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
	return UnaryAuth(ctx, s.transport, s.client().ModifyTrigger, req, decode.TriggerMutationFromModify)
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
	return UnaryAuth(ctx, s.transport, retryUnavailable(triggerReadRetry, s.client().ListTriggerEvents), req, decode.TriggerEventsListFromProto)
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
