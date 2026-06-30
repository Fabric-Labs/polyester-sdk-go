package services

import (
	"context"
	"strings"

	"github.com/Fabric-Labs/polyester-sdk-go/codecs/decode"
	authv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/auth/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/gen/auth/v1/authv1connect"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
	"github.com/Fabric-Labs/polyester-sdk-go/transport"
)

type SocialVerificationService struct {
	transport *transport.Factory
}

func NewSocialVerificationService(factory *transport.Factory) *SocialVerificationService {
	return &SocialVerificationService{transport: factory}
}

func (s *SocialVerificationService) client() authv1connect.SocialVerificationServiceClient {
	return authv1connect.NewSocialVerificationServiceClient(s.transport.HTTP, s.transport.Config.APIURL, s.transport.ConnectOptions(true)...)
}

func (s *SocialVerificationService) Start(ctx context.Context, provider, method, handle string) (models.ApiData, error) {
	req := &authv1.StartSocialVerificationRequest{Provider: providerEnum(provider), Method: methodEnum(method), Handle: handle}
	return UnaryAuth(ctx, s.transport, s.client().StartSocialVerification, req, func(msg *authv1.StartSocialVerificationResponse) models.ApiData {
		return decode.APIDataFromProtoMust(msg)
	})
}

func (s *SocialVerificationService) MarkReady(ctx context.Context, provider string) (models.ApiData, error) {
	req := &authv1.SocialVerificationReadyRequest{Provider: providerEnum(provider)}
	return UnaryAuth(ctx, s.transport, s.client().SocialVerificationReady, req, func(msg *authv1.SocialVerificationReadyResponse) models.ApiData {
		return decode.APIDataFromProtoMust(msg)
	})
}

func (s *SocialVerificationService) Get(ctx context.Context, provider string) (models.ApiData, error) {
	req := &authv1.GetSocialVerificationRequest{Provider: providerEnum(provider)}
	return UnaryAuth(ctx, s.transport, s.client().GetSocialVerification, req, func(msg *authv1.GetSocialVerificationResponse) models.ApiData {
		return decode.APIDataFromProtoMust(msg)
	})
}

func providerEnum(v string) authv1.SocialProvider {
	switch strings.ToLower(v) {
	case "twitter":
		return authv1.SocialProvider_TWITTER
	case "discord":
		return authv1.SocialProvider_DISCORD
	default:
		return authv1.SocialProvider_PROVIDER_UNSPECIFIED
	}
}

func methodEnum(v string) authv1.SocialVerificationMethod {
	switch strings.ToLower(v) {
	case "profile":
		return authv1.SocialVerificationMethod_METHOD_PROFILE
	case "channel":
		return authv1.SocialVerificationMethod_METHOD_CHANNEL
	case "dm":
		return authv1.SocialVerificationMethod_METHOD_DM
	default:
		return authv1.SocialVerificationMethod_METHOD_UNSPECIFIED
	}
}
