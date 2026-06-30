package services

import (
	"context"
	"time"

	"github.com/Fabric-Labs/polyester-sdk-go/auth"
	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	"github.com/Fabric-Labs/polyester-sdk-go/codecs/decode"
	"github.com/Fabric-Labs/polyester-sdk-go/errors"
	authv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/auth/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/gen/auth/v1/authv1connect"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
	"github.com/Fabric-Labs/polyester-sdk-go/realtime"
	"github.com/Fabric-Labs/polyester-sdk-go/transport"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type ApiKeysService struct {
	transport        *transport.Factory
	scoped           ScopedSubAccount
	defaultAccountID *string
	realtime         RealtimeClient
}

func NewApiKeysService(factory *transport.Factory, defaultSubAccountID *string, realtime RealtimeClient, defaultAccountID *string) *ApiKeysService {
	return &ApiKeysService{transport: factory, scoped: ScopedSubAccount{DefaultSubAccountID: defaultSubAccountID}, realtime: realtime, defaultAccountID: defaultAccountID}
}

func (s *ApiKeysService) client() authv1connect.ApiKeyServiceClient {
	return authv1connect.NewApiKeyServiceClient(s.transport.HTTP, s.transport.Config.APIURL, s.transport.ConnectOptions(true)...)
}

func (s *ApiKeysService) List(ctx context.Context, account AccountScope, subAccountID *string) (models.ApiKeysList, error) {
	req := &authv1.ListApiKeysRequest{}
	sub, err := s.scoped.ResolveSubAccountID(subAccountID, account)
	if err != nil {
		return models.ApiKeysList{}, err
	}
	if parsed, err := codecs.ParseOptionalSubaccountID(sub); err != nil {
		return models.ApiKeysList{}, err
	} else if parsed != nil {
		req.SubaccountId = parsed
	}
	return UnaryAuth(ctx, s.transport, s.client().ListApiKeys, req, decode.ApiKeysListFromProto)
}

func (s *ApiKeysService) Get(ctx context.Context, keyID string) (*models.ApiKeySummary, error) {
	return UnaryAuth(ctx, s.transport, s.client().GetApiKey, &authv1.GetApiKeyRequest{KeyId: keyID}, decode.ApiKeyFromGetProto)
}

func (s *ApiKeysService) Create(ctx context.Context, label string, account AccountScope, subAccountID *string, icon, color string, ipWhitelist []string, publicKeyEd25519 []byte) (*models.ApiKeySummary, error) {
	req := &authv1.CreateApiKeyRequest{Label: label, Icon: icon, Color: color}
	sub, err := s.scoped.ResolveSubAccountID(subAccountID, account)
	if err != nil {
		return nil, err
	}
	if parsed, err := codecs.ParseOptionalSubaccountID(sub); err != nil {
		return nil, err
	} else if parsed != nil {
		req.SubaccountId = parsed
	}
	if len(ipWhitelist) > 0 {
		req.IpWhitelist = ipWhitelist
	}
	if len(publicKeyEd25519) > 0 {
		req.PublicKeyEd25519 = publicKeyEd25519
	}
	return UnaryAuth(ctx, s.transport, s.client().CreateApiKey, req, decode.ApiKeyFromCreateProto)
}

func (s *ApiKeysService) Update(ctx context.Context, keyID, label, icon, color string, status *string, ipWhitelist []string, expiresAt *time.Time) (*models.ApiKeySummary, error) {
	req := &authv1.UpdateApiKeyRequest{KeyId: keyID, Label: label, Icon: icon, Color: color}
	if status != nil && *status != "" {
		protoStatus, err := codecs.ApiKeyStatusFromLabel(*status)
		if err != nil {
			return nil, err
		}
		req.Status = protoStatus
	}
	if ipWhitelist != nil {
		req.IpWhitelist = &authv1.IpWhitelist{Cidrs: ipWhitelist}
	}
	if expiresAt != nil {
		if expiresAt.Location() == time.Local {
			return nil, &errors.ValidationError{Msg: "expires_at must be timezone-aware"}
		}
		req.ExpiresAt = timestamppb.New(expiresAt.UTC())
	}
	return UnaryAuth(ctx, s.transport, s.client().UpdateApiKey, req, decode.ApiKeyFromUpdateProto)
}

func (s *ApiKeysService) Delete(ctx context.Context, keyID string) error {
	_, err := UnaryAuth(ctx, s.transport, s.client().DeleteApiKey, &authv1.DeleteApiKeyRequest{KeyId: keyID}, decode.Void[authv1.DeleteApiKeyResponse])
	return err
}

func (s *ApiKeysService) GenerateKeypair() models.Ed25519Keypair {
	return auth.GenerateEd25519Keypair()
}

func (s *ApiKeysService) Subscribe(ctx context.Context, accountID any) (*realtime.Subscription[models.ApiKeySummary], error) {
	return SubscribeAccountProto(ctx, s.realtime, "private:auth:api-keys:{account_id}:proto", accountID, s.defaultAccountID, decode.ApiKeyFromBytes)
}
