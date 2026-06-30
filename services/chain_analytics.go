package services

import (
	"context"

	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	"github.com/Fabric-Labs/polyester-sdk-go/codecs/decode"
	"github.com/Fabric-Labs/polyester-sdk-go/errors"
	analyticsv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/chain/analytics/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/gen/chain/analytics/v1/chainanalyticsv1connect"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
	"github.com/Fabric-Labs/polyester-sdk-go/transport"
)

type ChainAnalyticsService struct {
	transport *transport.Factory
}

func NewChainAnalyticsService(factory *transport.Factory) *ChainAnalyticsService {
	return &ChainAnalyticsService{transport: factory}
}

func (s *ChainAnalyticsService) client() chainanalyticsv1connect.ChainAnalyticsServiceClient {
	return chainanalyticsv1connect.NewChainAnalyticsServiceClient(s.transport.HTTP, s.transport.Config.APIURL, s.transport.ConnectOptions(false)...)
}

func (s *ChainAnalyticsService) GetZippedAssetSupply(ctx context.Context, zippedAssetID uint32, rangeKey, bucket string, startTsSec, endTsSec uint32) (models.ApiData, error) {
	rng, err := codecs.ResolveAnalyticsRange(rangeKey)
	if err != nil {
		return models.ApiData{}, err
	}
	req := &analyticsv1.GetZippedAssetSupplyRequest{
		ZippedAssetId: zippedAssetID,
		Range:         rng,
		Bucket:        bucket,
		StartTsSec:    startTsSec,
		EndTsSec:      endTsSec,
	}
	return UnaryPublic(ctx, s.transport, s.client().GetZippedAssetSupply, req, func(msg *analyticsv1.GetZippedAssetSupplyResponse) models.ApiData {
		return decode.APIDataFromProtoMust(msg)
	})
}

func (s *ChainAnalyticsService) GetZippedAssetSupplyGroup(ctx context.Context, groupID, rangeKey, bucket string, startTsSec, endTsSec uint32) (models.ApiData, error) {
	rng, err := codecs.ResolveAnalyticsRange(rangeKey)
	if err != nil {
		return models.ApiData{}, err
	}
	req := &analyticsv1.GetZippedAssetSupplyGroupRequest{
		GroupId:    groupID,
		Range:      rng,
		Bucket:     bucket,
		StartTsSec: startTsSec,
		EndTsSec:   endTsSec,
	}
	return UnaryPublic(ctx, s.transport, s.client().GetZippedAssetSupplyGroup, req, func(msg *analyticsv1.GetZippedAssetSupplyGroupResponse) models.ApiData {
		return decode.APIDataFromProtoMust(msg)
	})
}

func (s *ChainAnalyticsService) GetUnifiedAssetBalances(ctx context.Context, assetID uint32, rangeKey, bucket string, startTsSec, endTsSec uint32) (models.ApiData, error) {
	if assetID == 0 {
		return models.ApiData{}, &errors.ValidationError{Msg: "asset_id must be positive"}
	}
	rng, err := codecs.ResolveAnalyticsRange(rangeKey)
	if err != nil {
		return models.ApiData{}, err
	}
	req := &analyticsv1.GetUnifiedAssetBalancesRequest{
		AssetId:    assetID,
		Range:      rng,
		Bucket:     bucket,
		StartTsSec: startTsSec,
		EndTsSec:   endTsSec,
	}
	return UnaryPublic(ctx, s.transport, s.client().GetUnifiedAssetBalances, req, func(msg *analyticsv1.GetUnifiedAssetBalancesResponse) models.ApiData {
		return decode.APIDataFromProtoMust(msg)
	})
}
