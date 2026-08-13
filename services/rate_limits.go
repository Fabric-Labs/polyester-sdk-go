package services

import (
	"context"

	"github.com/Fabric-Labs/polyester-sdk-go/codecs/decode"
	ratelimitv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/ratelimit/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/gen/ratelimit/v1/ratelimitv1connect"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
	"github.com/Fabric-Labs/polyester-sdk-go/transport"
)

type RateLimitService struct {
	transport *transport.Factory
	scoped    ScopedSubAccount
}

func NewRateLimitService(factory *transport.Factory, defaultSubAccountID *string) *RateLimitService {
	return &RateLimitService{transport: factory, scoped: ScopedSubAccount{DefaultSubAccountID: defaultSubAccountID}}
}

func (s *RateLimitService) client() ratelimitv1connect.RateLimitServiceClient {
	return ratelimitv1connect.NewRateLimitServiceClient(s.transport.HTTP, s.transport.Config.APIURL, s.transport.ConnectOptions(false)...)
}

func (s *RateLimitService) writeClient() ratelimitv1connect.RateLimitServiceClient {
	return ratelimitv1connect.NewRateLimitServiceClient(s.transport.HTTP, s.transport.Config.APIURL, s.transport.ConnectOptions(true)...)
}

func (s *RateLimitService) GetRateLimitConfig(ctx context.Context) (models.RateLimitConfig, error) {
	return UnaryPublic(ctx, s.transport, s.client().GetRateLimitConfig, &ratelimitv1.GetRateLimitConfigRequest{}, decode.RateLimitConfigFromProto)
}

func (s *RateLimitService) GetTradingRateLimits(ctx context.Context, account AccountScope, subAccountID *string) (models.TradingRateLimits, error) {
	req := &ratelimitv1.GetTradingRateLimitsRequest{}
	if err := s.scoped.ApplyOptionalSubaccountIDPtr(&req.SubaccountId, account, subAccountID); err != nil {
		return models.TradingRateLimits{}, err
	}
	return UnaryAuth(ctx, s.transport, s.writeClient().GetTradingRateLimits, req, decode.TradingRateLimitsFromProto)
}
