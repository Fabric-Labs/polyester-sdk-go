package services

import (
	"context"

	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	polychartv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/polychart/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/gen/polychart/v1/polychartv1connect"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
	"github.com/Fabric-Labs/polyester-sdk-go/transport"
	"github.com/Fabric-Labs/polyester-sdk-go/wire"
)

type PolychartService struct {
	transport *transport.Factory
}

func NewPolychartService(factory *transport.Factory) *PolychartService {
	return &PolychartService{transport: factory}
}

func (s *PolychartService) client() polychartv1connect.PolychartServiceClient {
	return polychartv1connect.NewPolychartServiceClient(s.transport.HTTP, s.transport.Config.APIURL, s.transport.ConnectOptions(true)...)
}

func (s *PolychartService) GetMarketLayers(ctx context.Context, engineSymbolID uint32) (models.ApiData, error) {
	req := &polychartv1.GetMarketLayersRequest{EngineSymbolId: engineSymbolID}
	return UnaryAuth(ctx, s.transport, s.client().GetMarketLayers, req, func(msg *polychartv1.GetMarketLayersResponse) models.ApiData { return apiData(msg) })
}

func (s *PolychartService) ListInboxMarketLayers(ctx context.Context, engineSymbolID uint32, limit int, pageToken string) (models.ApiData, error) {
	req := &polychartv1.ListInboxMarketLayersRequest{EngineSymbolId: engineSymbolID, Limit: uint32(limit), PageToken: pageToken}
	return UnaryAuth(ctx, s.transport, s.client().ListInboxMarketLayers, req, func(msg *polychartv1.ListInboxMarketLayersResponse) models.ApiData { return apiData(msg) })
}

func (s *PolychartService) GetLayerSnapshot(ctx context.Context, layer map[string]any) (models.ApiData, error) {
	ref, err := layerRefFromMap(layer)
	if err != nil {
		return models.ApiData{}, err
	}
	return UnaryAuth(ctx, s.transport, s.client().GetLayerSnapshot, &polychartv1.GetLayerSnapshotRequest{Layer: ref}, func(msg *polychartv1.GetLayerSnapshotResponse) models.ApiData { return apiData(msg) })
}

func (s *PolychartService) GetLayerSubscribeTokens(ctx context.Context, layers []map[string]any) (models.ApiData, error) {
	refs, err := layerRefsFromMaps(layers)
	if err != nil {
		return models.ApiData{}, err
	}
	req := &polychartv1.GetLayerSubscribeTokensRequest{Layers: refs}
	return UnaryAuth(ctx, s.transport, s.client().GetLayerSubscribeTokens, req, func(msg *polychartv1.GetLayerSubscribeTokensResponse) models.ApiData { return apiData(msg) })
}

func (s *PolychartService) ResolveLayerShareToken(ctx context.Context, token string) (models.ApiData, error) {
	return UnaryAuth(ctx, s.transport, s.client().ResolveLayerShareToken, &polychartv1.ResolveLayerShareTokenRequest{Token: token}, func(msg *polychartv1.ResolveLayerShareTokenResponse) models.ApiData { return apiData(msg) })
}

func (s *PolychartService) CreateLayerShareLink(ctx context.Context, layer map[string]any, perms int, expiresAtMs int64) (models.ApiData, error) {
	ref, err := layerRefFromMap(layer)
	if err != nil {
		return models.ApiData{}, err
	}
	req := &polychartv1.CreateLayerShareLinkRequest{Layer: ref, Perms: uint32(perms), ExpiresAtMs: uint64(expiresAtMs)}
	return UnaryAuth(ctx, s.transport, s.client().CreateLayerShareLink, req, func(msg *polychartv1.CreateLayerShareLinkResponse) models.ApiData { return apiData(msg) })
}

func (s *PolychartService) RevokeLayerShareLink(ctx context.Context, token string) (models.ApiData, error) {
	return UnaryAuth(ctx, s.transport, s.client().RevokeLayerShareLink, &polychartv1.RevokeLayerShareLinkRequest{Token: token}, func(msg *polychartv1.RevokeLayerShareLinkResponse) models.ApiData { return apiData(msg) })
}

func (s *PolychartService) ListOwnerPublishedLayers(ctx context.Context, ownerID string, engineSymbolID uint32, limit int, pageToken string) (models.ApiData, error) {
	id, err := codecs.IDToInt(ownerID, "owner_id")
	if err != nil {
		return models.ApiData{}, err
	}
	req := &polychartv1.ListOwnerPublishedLayersRequest{OwnerId: id, Limit: uint32(limit), PageToken: pageToken}
	if engineSymbolID != 0 {
		req.EngineSymbolId = &engineSymbolID
	}
	return UnaryAuth(ctx, s.transport, s.client().ListOwnerPublishedLayers, req, func(msg *polychartv1.ListOwnerPublishedLayersResponse) models.ApiData { return apiData(msg) })
}

func (s *PolychartService) PublishLayer(ctx context.Context, layer map[string]any, title, description string, tags []string) (models.ApiData, error) {
	ref, err := layerRefFromMap(layer)
	if err != nil {
		return models.ApiData{}, err
	}
	req := &polychartv1.PublishLayerRequest{Layer: ref, Title: title, Description: description}
	if len(tags) > 0 {
		req.Tags = append([]string(nil), tags...)
	}
	return UnaryAuth(ctx, s.transport, s.client().PublishLayer, req, func(msg *polychartv1.PublishLayerResponse) models.ApiData { return apiData(msg) })
}

func (s *PolychartService) UnpublishLayer(ctx context.Context, layer map[string]any) (models.ApiData, error) {
	ref, err := layerRefFromMap(layer)
	if err != nil {
		return models.ApiData{}, err
	}
	return UnaryAuth(ctx, s.transport, s.client().UnpublishLayer, &polychartv1.UnpublishLayerRequest{Layer: ref}, func(msg *polychartv1.UnpublishLayerResponse) models.ApiData { return apiData(msg) })
}

func (s *PolychartService) UpsertLayer(ctx context.Context, layer map[string]any, expectedRevision int) (models.ApiData, error) {
	msg := &polychartv1.Layer{}
	if err := wire.MessageFromMap(msg, layer); err != nil {
		return models.ApiData{}, err
	}
	req := &polychartv1.UpsertLayerRequest{Layer: msg, ExpectedRevision: uint64(expectedRevision)}
	return UnaryAuth(ctx, s.transport, s.client().UpsertLayer, req, func(msg *polychartv1.UpsertLayerResponse) models.ApiData { return apiData(msg) })
}

func (s *PolychartService) DeleteLayer(ctx context.Context, layer map[string]any, expectedRevision int) (models.ApiData, error) {
	ref, err := layerRefFromMap(layer)
	if err != nil {
		return models.ApiData{}, err
	}
	req := &polychartv1.DeleteLayerRequest{Layer: ref, ExpectedRevision: uint64(expectedRevision)}
	return UnaryAuth(ctx, s.transport, s.client().DeleteLayer, req, func(msg *polychartv1.DeleteLayerResponse) models.ApiData { return apiData(msg) })
}

func (s *PolychartService) UpsertDrawing(ctx context.Context, drawing map[string]any, expectedLayerRevision int) (models.ApiData, error) {
	msg := &polychartv1.Drawing{}
	if err := wire.MessageFromMap(msg, drawing); err != nil {
		return models.ApiData{}, err
	}
	req := &polychartv1.UpsertDrawingRequest{Drawing: msg, ExpectedLayerRevision: uint64(expectedLayerRevision)}
	return UnaryAuth(ctx, s.transport, s.client().UpsertDrawing, req, func(msg *polychartv1.UpsertDrawingResponse) models.ApiData { return apiData(msg) })
}

func (s *PolychartService) DeleteDrawing(ctx context.Context, drawing, layer map[string]any, expectedLayerRevision int) (models.ApiData, error) {
	drawingRef := &polychartv1.DrawingRef{}
	if err := wire.MessageFromMap(drawingRef, drawing); err != nil {
		return models.ApiData{}, err
	}
	layerRef, err := layerRefFromMap(layer)
	if err != nil {
		return models.ApiData{}, err
	}
	req := &polychartv1.DeleteDrawingRequest{Drawing: drawingRef, Layer: layerRef, ExpectedLayerRevision: uint64(expectedLayerRevision)}
	return UnaryAuth(ctx, s.transport, s.client().DeleteDrawing, req, func(msg *polychartv1.DeleteDrawingResponse) models.ApiData { return apiData(msg) })
}

func (s *PolychartService) SetLayerSubscriptions(ctx context.Context, engineSymbolID uint32, subscriptions []map[string]any) (models.ApiData, error) {
	items := make([]*polychartv1.LayerSubscription, 0, len(subscriptions))
	for _, item := range subscriptions {
		msg := &polychartv1.LayerSubscription{}
		if err := wire.MessageFromMap(msg, item); err != nil {
			return models.ApiData{}, err
		}
		items = append(items, msg)
	}
	req := &polychartv1.SetLayerSubscriptionsRequest{EngineSymbolId: engineSymbolID, Subscriptions: items}
	return UnaryAuth(ctx, s.transport, s.client().SetLayerSubscriptions, req, func(msg *polychartv1.SetLayerSubscriptionsResponse) models.ApiData { return apiData(msg) })
}

func layerRefFromMap(value map[string]any) (*polychartv1.LayerRef, error) {
	msg := &polychartv1.LayerRef{}
	return msg, wire.MessageFromMap(msg, value)
}

func layerRefsFromMaps(values []map[string]any) ([]*polychartv1.LayerRef, error) {
	out := make([]*polychartv1.LayerRef, 0, len(values))
	for _, value := range values {
		ref, err := layerRefFromMap(value)
		if err != nil {
			return nil, err
		}
		out = append(out, ref)
	}
	return out, nil
}
