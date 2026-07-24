package services

import (
	"context"

	"github.com/Fabric-Labs/polyester-sdk-go/codecs/decode"
	authv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/auth/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/gen/auth/v1/authv1connect"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
	"github.com/Fabric-Labs/polyester-sdk-go/transport"
)

type AuthService struct {
	transport *transport.Factory
	realtime  RealtimeClient
	Profile   *ProfileService
}

func NewAuthService(factory *transport.Factory, realtime RealtimeClient) *AuthService {
	return &AuthService{transport: factory, realtime: realtime, Profile: NewProfileService(realtime)}
}

func (s *AuthService) client() authv1connect.AuthServiceClient {
	return authv1connect.NewAuthServiceClient(s.transport.HTTP, s.transport.Config.APIURL, s.transport.ConnectOptions(true)...)
}

func (s *AuthService) Me(ctx context.Context) (models.MeResult, error) {
	return UnaryAuth(ctx, s.transport, s.client().Me, &authv1.MeRequest{}, decode.MeFromProto)
}
