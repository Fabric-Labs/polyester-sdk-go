package services

import (
	"context"

	"github.com/Fabric-Labs/polyester-sdk-go/codecs/decode"
	authv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/auth/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/gen/auth/v1/authv1connect"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
	"github.com/Fabric-Labs/polyester-sdk-go/realtime"
	"github.com/Fabric-Labs/polyester-sdk-go/transport"
)

type ProfileService struct {
	transport *transport.Factory
	realtime  RealtimeClient
}

func NewProfileService(factory *transport.Factory, realtime RealtimeClient) *ProfileService {
	return &ProfileService{transport: factory, realtime: realtime}
}

func (s *ProfileService) client() authv1connect.ProfileServiceClient {
	return authv1connect.NewProfileServiceClient(s.transport.HTTP, s.transport.Config.APIURL, s.transport.ConnectOptions(true)...)
}

func (s *ProfileService) Get(ctx context.Context) (models.UserProfile, error) {
	return UnaryAuth(ctx, s.transport, s.client().GetProfile, &authv1.GetProfileRequest{}, decode.ProfileFromProto)
}

func (s *ProfileService) Update(ctx context.Context, username, bio, website, twitter, avatarURL string) (models.UserProfile, error) {
	req := &authv1.UserProfilePatch{
		Username:  stringPtr(username),
		Bio:       stringPtr(bio),
		Website:   stringPtr(website),
		Twitter:   stringPtr(twitter),
		AvatarUrl: stringPtr(avatarURL),
	}
	return UnaryAuth(ctx, s.transport, s.client().UpdateProfile, req, decode.ProfileFromProto)
}

func (s *ProfileService) GetUsernameHistory(ctx context.Context) (models.UsernameHistoryList, error) {
	return UnaryAuth(ctx, s.transport, s.client().GetUsernameHistory, &authv1.GetUsernameHistoryRequest{}, decode.UsernameHistoryFromProto)
}

func (s *ProfileService) SubscribeIdentity(ctx context.Context) (*realtime.Subscription[models.AccountIdentity], error) {
	return SubscribePublicProto(ctx, s.realtime, "public:identity:updates:proto", decode.AccountIdentityFromBytes)
}
