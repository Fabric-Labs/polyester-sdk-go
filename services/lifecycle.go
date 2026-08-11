package services

import (
	"context"
	"fmt"
	"strings"

	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	"github.com/Fabric-Labs/polyester-sdk-go/codecs/decode"
	"github.com/Fabric-Labs/polyester-sdk-go/errors"
	lifecyclev1 "github.com/Fabric-Labs/polyester-sdk-go/gen/chain/lifecycle/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/gen/chain/lifecycle/v1/chainlifecyclev1connect"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
	"github.com/Fabric-Labs/polyester-sdk-go/realtime"
	"github.com/Fabric-Labs/polyester-sdk-go/transport"
)

type LifecycleService struct {
	transport *transport.Factory
	realtime  RealtimeClient
}

func NewLifecycleService(factory *transport.Factory, realtime RealtimeClient) *LifecycleService {
	return &LifecycleService{transport: factory, realtime: realtime}
}

func (s *LifecycleService) client() chainlifecyclev1connect.LifecycleReadServiceClient {
	return chainlifecyclev1connect.NewLifecycleReadServiceClient(s.transport.HTTP, s.transport.Config.APIURL, s.transport.ConnectOptions(false)...)
}

func (s *LifecycleService) ListFlows(ctx context.Context, limit int, pageToken *string, scope *string, ownerAccountID *string, smartAccountAddress *string, reversed bool) (models.LifecycleFlowsList, error) {
	parsedLimit, err := PaginationLimitOrDefault(limit, 50, "limit")
	if err != nil {
		return models.LifecycleFlowsList{}, err
	}
	req := &lifecyclev1.ListFlowsRequest{Limit: parsedLimit}
	if reversed {
		req.Sort = lifecyclev1.Sort_SORT_OLDEST
	} else {
		req.Sort = lifecyclev1.Sort_SORT_NEWEST
	}
	if pageToken != nil {
		req.PageToken = *pageToken
	}
	if scope != nil {
		name := strings.ToUpper(*scope)
		if !strings.HasPrefix(name, "LIST_") {
			name = "LIST_" + name
		}
		if v, ok := lifecyclev1.ListScope_value[name]; ok {
			req.Scope = lifecyclev1.ListScope(v)
		} else {
			return models.LifecycleFlowsList{}, &errors.ValidationError{Msg: "scope must be one of 'all', 'open_only', or 'terminal_only'"}
		}
	}
	if ownerAccountID != nil {
		id, err := codecs.IDToInt(*ownerAccountID, "owner_account_id")
		if err != nil {
			return models.LifecycleFlowsList{}, err
		}
		req.AccountSelector = &lifecyclev1.ListFlowsRequest_OwnerAccountId{OwnerAccountId: id}
	}
	if smartAccountAddress != nil {
		req.AccountSelector = &lifecyclev1.ListFlowsRequest_SmartAccountAddress{SmartAccountAddress: *smartAccountAddress}
	}
	return UnaryPublic(ctx, s.transport, s.client().ListFlows, req, decode.FlowsListFromProto)
}

func (s *LifecycleService) GetFlow(ctx context.Context, intentID, flowID *string) (models.LifecycleFlowSummary, error) {
	resolved := flowID
	if resolved == nil || *resolved == "" {
		resolved = intentID
	}
	if resolved == nil || *resolved == "" {
		return models.LifecycleFlowSummary{}, &errors.ValidationError{Msg: "flow_id or intent_id is required"}
	}
	req := &lifecyclev1.GetFlowByIdRequest{FlowId: *resolved}
	return UnaryPublicDecoded(ctx, s.transport, s.client().GetFlowById, req, decode.FlowFromGetResponse)
}

// GetFlowByTx returns all matches in the first response page for a transaction.
// A chain transaction can contain multiple bundled lifecycle flows. Use
// ListFlowsByTx to follow NextPageToken when the result spans multiple pages.
func (s *LifecycleService) GetFlowByTx(ctx context.Context, txHash, lookupKind string, limit int) (models.LifecycleFlowsList, error) {
	parsedLimit, err := PaginationLimitOrDefault(limit, 50, "limit")
	if err != nil {
		return models.LifecycleFlowsList{}, err
	}
	req := &lifecyclev1.ListFlowsByTxRequest{
		TxHash:     txHash,
		LookupKind: codecs.TxLookupKindFromLabel(lookupKind),
		Limit:      parsedLimit,
	}
	return UnaryPublicDecoded(ctx, s.transport, s.client().ListFlowsByTx, req, decode.FlowFromGetByTxResponse)
}

func (s *LifecycleService) ListFlowsByTx(ctx context.Context, txHash, lookupKind string, limit int, pageToken *string) (models.LifecycleFlowsList, error) {
	parsedLimit, err := PaginationLimitOrDefault(limit, 50, "limit")
	if err != nil {
		return models.LifecycleFlowsList{}, err
	}
	req := &lifecyclev1.ListFlowsByTxRequest{
		TxHash:     txHash,
		LookupKind: codecs.TxLookupKindFromLabel(lookupKind),
		Limit:      parsedLimit,
	}
	if pageToken != nil {
		req.PageToken = *pageToken
	}
	return UnaryPublic(ctx, s.transport, s.client().ListFlowsByTx, req, decode.FlowsByTxListFromProto)
}

func (s *LifecycleService) SubscribeOpenFlows(ctx context.Context, accountID any) (*realtime.Subscription[models.LifecycleFlowSummary], error) {
	if id := stringish(accountID); id != "" {
		return SubscribeAccountProto(ctx, s.realtime, "private:chain:lifecycle:flows:{account_id}:proto", accountID, nil, decode.FlowSummaryFromBytes)
	}
	return SubscribePublicProto(ctx, s.realtime, "public:chain:lifecycle:flows:proto", decode.FlowSummaryFromBytes)
}

func (s *LifecycleService) SubscribeFlowDetail(ctx context.Context, flowID string) (*realtime.Subscription[models.LifecycleFlowSummary], error) {
	channel := fmt.Sprintf("public:chain:lifecycle:flow:%s:proto", flowID)
	return SubscribePublicProto(ctx, s.realtime, channel, decode.FlowDetailFromBytes)
}
