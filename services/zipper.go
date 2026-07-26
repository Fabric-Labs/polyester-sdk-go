package services

import (
	"context"

	"github.com/Fabric-Labs/polyester-sdk-go/catalogs"
	"github.com/Fabric-Labs/polyester-sdk-go/codecs/decode"
	"github.com/Fabric-Labs/polyester-sdk-go/errors"
	zipperv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/chain/zipper/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/gen/chain/zipper/v1/chainzipperv1connect"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
	"github.com/Fabric-Labs/polyester-sdk-go/realtime"
	"github.com/Fabric-Labs/polyester-sdk-go/transport"
)

type ZipperService struct {
	transport *transport.Factory
	catalogs  *catalogs.Manager
	realtime  RealtimeClient
}

func NewZipperService(factory *transport.Factory, cats *catalogs.Manager, realtime RealtimeClient) *ZipperService {
	return &ZipperService{transport: factory, catalogs: cats, realtime: realtime}
}

func (s *ZipperService) client() chainzipperv1connect.ZipperServiceClient {
	return chainzipperv1connect.NewZipperServiceClient(s.transport.HTTP, s.transport.Config.APIURL, s.transport.ConnectOptions(false)...)
}

func (s *ZipperService) GetDepositWithdrawConfig(ctx context.Context) (models.DepositWithdrawConfig, error) {
	return UnaryPublic(ctx, s.transport, s.client().GetDepositWithdrawConfig, &zipperv1.GetDepositWithdrawConfigRequest{}, decode.DepositWithdrawConfigFromProto)
}

func (s *ZipperService) SubscribeZippedAssetSupply(ctx context.Context, patchCatalog bool) (*realtime.Subscription[models.ZippedAssetSupplyBatch], error) {
	decodeFn := func(payload []byte) (models.ZippedAssetSupplyBatch, error) {
		scaleFn := func(id uint32) (int, bool) {
			if s.catalogs == nil {
				return 0, false
			}
			return s.catalogs.QuantityScaleForZippedAssetID(id)
		}
		batch, err := decode.ZippedAssetSupplyBatchFromBytes(payload, scaleFn)
		if err != nil {
			return batch, err
		}
		if patchCatalog && s.catalogs != nil {
			s.catalogs.PatchZipperSupply(batch.Updates)
		}
		return batch, nil
	}
	if s.catalogs == nil {
		return nil, &errors.ValidationError{Msg: "zipped asset supply decoding requires hydrated catalogs"}
	}
	return SubscribePublicProto(ctx, s.realtime, "public:chain:zipped-asset:supply:proto", decodeFn)
}
