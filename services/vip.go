package services

import (
	"context"

	"github.com/Fabric-Labs/polyester-sdk-go/codecs/decode"
	vipv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/vip/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/gen/vip/v1/vipv1connect"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
	"github.com/Fabric-Labs/polyester-sdk-go/transport"
)

type VIPService struct {
	transport *transport.Factory
}

func NewVIPService(factory *transport.Factory) *VIPService {
	return &VIPService{transport: factory}
}

func (s *VIPService) client() vipv1connect.VIPServiceClient {
	return vipv1connect.NewVIPServiceClient(s.transport.HTTP, s.transport.Config.APIURL, s.transport.ConnectOptions(false)...)
}

func (s *VIPService) writeClient() vipv1connect.VIPServiceClient {
	return vipv1connect.NewVIPServiceClient(s.transport.HTTP, s.transport.Config.APIURL, s.transport.ConnectOptions(true)...)
}

func (s *VIPService) ListVIPTiers(ctx context.Context) (models.VIPTiersList, error) {
	return UnaryPublic(ctx, s.transport, s.client().ListVIPTiers, &vipv1.ListVIPTiersRequest{}, decode.VIPTiersListFromProto)
}

func (s *VIPService) GetVIPStatus(ctx context.Context) (models.VIPStatus, error) {
	return UnaryAuth(ctx, s.transport, s.writeClient().GetVIPStatus, &vipv1.GetVIPStatusRequest{}, decode.VIPStatusFromProto)
}
