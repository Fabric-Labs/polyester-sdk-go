package services

import (
	"context"

	"github.com/Fabric-Labs/polyester-sdk-go/codecs/decode"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
	"github.com/Fabric-Labs/polyester-sdk-go/realtime"
)

// ProfileService exposes public identity realtime updates.
// Unary profile RPCs require JWT/session auth and are not wrapped in this API-key SDK.
type ProfileService struct {
	realtime RealtimeClient
}

func NewProfileService(realtime RealtimeClient) *ProfileService {
	return &ProfileService{realtime: realtime}
}

func (s *ProfileService) SubscribeIdentity(ctx context.Context) (*realtime.Subscription[models.AccountIdentity], error) {
	return SubscribePublicProto(ctx, s.realtime, "public:identity:updates:proto", decode.AccountIdentityFromBytes)
}
