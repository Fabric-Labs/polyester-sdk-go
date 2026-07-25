package services

import (
	"context"

	"github.com/Fabric-Labs/polyester-sdk-go/catalogs"
	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	"github.com/Fabric-Labs/polyester-sdk-go/codecs/decode"
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
}

func NewTriggersService(factory *transport.Factory, cats *catalogs.Manager, defaultSubAccountID *string, realtime RealtimeClient, defaultAccountID *string, catalogHydrationDone <-chan struct{}) *TriggersService {
	return &TriggersService{
		transport:            factory,
		catalogs:             cats,
		scoped:               ScopedSubAccount{DefaultSubAccountID: defaultSubAccountID},
		realtime:             realtime,
		defaultAccountID:     defaultAccountID,
		catalogHydrationDone: catalogHydrationDone,
	}
}

func (s *TriggersService) ensureCatalogs(ctx context.Context) error {
	if s.catalogHydrationDone == nil {
		return nil
	}
	select {
	case <-s.catalogHydrationDone:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *TriggersService) client() triggersv1connect.TriggersServiceClient {
	return triggersv1connect.NewTriggersServiceClient(s.transport.HTTP, s.transport.Config.APIURL, s.transport.ConnectOptions(true)...)
}

func (s *TriggersService) List(ctx context.Context, account AccountScope, subAccountID, symbol *string, status []string, limit int, pageToken *string) (models.TriggersList, error) {
	req := &triggersv1.ListTriggersRequest{Limit: uint32(limit)}
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
	if symbol != nil {
		req.Symbol = *symbol
	}
	for _, label := range status {
		st, err := decode.TriggerStatusFromLabel(label)
		if err != nil {
			return models.TriggersList{}, err
		}
		req.Status = append(req.Status, st)
	}
	return UnaryAuth(ctx, s.transport, s.client().ListTriggers, req, decode.TriggersListFromProto)
}

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
	return UnaryAuth(ctx, s.transport, s.client().GetTrigger, req, decode.GetTriggerFromProto)
}

func (s *TriggersService) Create(ctx context.Context, account AccountScope, symbol, triggerType string, triggerPrice *models.PriceInput, side string, qty models.QtyInput, orderType string, limitPrice *models.PriceInput, triggerPriceSource, tif string, subAccountID *string, clientTriggerID *string, postOnly bool, opts codecs.CreateTriggerOptions) (models.TriggerMutationResult, error) {
	if err := s.ensureCatalogs(ctx); err != nil {
		return models.TriggerMutationResult{}, err
	}
	sub, err := s.scoped.ResolveSubAccountID(subAccountID, account)
	if err != nil {
		return models.TriggerMutationResult{}, err
	}
	scale, err := codecs.QuantityScaleForSymbol(s.catalogs, &symbol)
	if err != nil {
		return models.TriggerMutationResult{}, err
	}
	req, err := codecs.CreateTriggerToProto(symbol, triggerType, triggerPrice, side, qty, orderType, limitPrice, triggerPriceSource, tif, sub, clientTriggerID, postOnly, scale, opts)
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

func (s *TriggersService) Resume(ctx context.Context, account AccountScope, triggerID string, subAccountID *string) (models.TriggerMutationResult, error) {
	id, err := codecs.IDToInt(triggerID, "trigger_id")
	if err != nil {
		return models.TriggerMutationResult{}, err
	}
	req := &triggersv1.ResumeTriggerRequest{TriggerId: id}
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

func (s *TriggersService) Modify(ctx context.Context, account AccountScope, triggerID string, subAccountID *string, opts codecs.ModifyTriggerOptions) (models.TriggerMutationResult, error) {
	sub, err := s.scoped.ResolveSubAccountID(subAccountID, account)
	if err != nil {
		return models.TriggerMutationResult{}, err
	}
	req, err := codecs.ModifyTriggerToProto(triggerID, sub, opts)
	if err != nil {
		return models.TriggerMutationResult{}, err
	}
	return UnaryAuth(ctx, s.transport, s.client().ModifyTrigger, req, decode.TriggerMutationFromModify)
}

func (s *TriggersService) ListEvents(ctx context.Context, account AccountScope, triggerID string, subAccountID *string, limit int, pageToken *string) (models.TriggerEventsList, error) {
	id, err := codecs.IDToInt(triggerID, "trigger_id")
	if err != nil {
		return models.TriggerEventsList{}, err
	}
	req := &triggersv1.ListTriggerEventsRequest{TriggerId: id, Limit: uint32(limit)}
	sub, err := s.scoped.ResolveSubAccountID(subAccountID, account)
	if err != nil {
		return models.TriggerEventsList{}, err
	}
	if parsed, err := codecs.ParseOptionalSubaccountID(sub); err != nil {
		return models.TriggerEventsList{}, err
	} else if parsed != nil {
		req.SubaccountId = parsed
	}
	if pageToken != nil && *pageToken != "" {
		req.PageToken = *pageToken
	}
	return UnaryAuth(ctx, s.transport, s.client().ListTriggerEvents, req, decode.TriggerEventsListFromProto)
}

func (s *TriggersService) Subscribe(ctx context.Context, accountID any) (*realtime.Subscription[models.Trigger], error) {
	return SubscribeAccountProto(ctx, s.realtime, "private:spot:triggers:{account_id}:proto", accountID, s.defaultAccountID, decode.TriggerFromBytes)
}

func (s *TriggersService) SubscribeEvents(ctx context.Context, accountID any) (*realtime.Subscription[models.TriggerEvent], error) {
	return SubscribeAccountProto(ctx, s.realtime, "private:spot:triggers:events:{account_id}:proto", accountID, s.defaultAccountID, decode.TriggerEventFromBytes)
}
