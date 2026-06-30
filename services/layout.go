package services

import (
	"context"

	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	"github.com/Fabric-Labs/polyester-sdk-go/codecs/decode"
	layoutv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/layout/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/gen/layout/v1/layoutv1connect"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
	"github.com/Fabric-Labs/polyester-sdk-go/transport"
	"github.com/Fabric-Labs/polyester-sdk-go/wire"
)

type LayoutService struct {
	transport *transport.Factory
}

func NewLayoutService(factory *transport.Factory) *LayoutService {
	return &LayoutService{transport: factory}
}

func (s *LayoutService) client() layoutv1connect.LayoutServiceClient {
	return layoutv1connect.NewLayoutServiceClient(s.transport.HTTP, s.transport.Config.APIURL, s.transport.ConnectOptions(true)...)
}

func (s *LayoutService) GetLayouts(ctx context.Context, limit int, pageToken string) (models.ApiData, error) {
	req := &layoutv1.GetLayoutsRequest{Limit: uint32(limit), PageToken: pageToken}
	return UnaryAuth(ctx, s.transport, s.client().GetLayouts, req, func(msg *layoutv1.GetLayoutsResponse) models.ApiData {
		return decode.APIDataFromProtoMust(msg)
	})
}

func (s *LayoutService) GetLayout(ctx context.Context, layoutID string) (models.ApiData, error) {
	id, err := codecs.IDToInt(layoutID, "layout_id")
	if err != nil {
		return models.ApiData{}, err
	}
	return UnaryAuth(ctx, s.transport, s.client().GetLayout, &layoutv1.GetLayoutRequest{LayoutId: id}, func(msg *layoutv1.GetLayoutResponse) models.ApiData {
		return decode.APIDataFromProtoMust(msg)
	})
}

func (s *LayoutService) UpsertLayout(ctx context.Context, layout map[string]any) (models.ApiData, error) {
	msg := &layoutv1.Layout{}
	if err := wire.MessageFromMap(msg, layout); err != nil {
		return models.ApiData{}, err
	}
	return UnaryAuth(ctx, s.transport, s.client().UpsertLayout, &layoutv1.UpsertLayoutRequest{Layout: msg}, func(msg *layoutv1.UpsertLayoutResponse) models.ApiData {
		return decode.APIDataFromProtoMust(msg)
	})
}

func (s *LayoutService) DeleteLayout(ctx context.Context, layoutID string) (models.ApiData, error) {
	id, err := codecs.IDToInt(layoutID, "layout_id")
	if err != nil {
		return models.ApiData{}, err
	}
	return UnaryAuth(ctx, s.transport, s.client().DeleteLayout, &layoutv1.DeleteLayoutRequest{LayoutId: id}, func(msg *layoutv1.DeleteLayoutResponse) models.ApiData {
		return decode.APIDataFromProtoMust(msg)
	})
}

func (s *LayoutService) ResolveLayoutShareToken(ctx context.Context, token string) (models.ApiData, error) {
	return UnaryAuth(ctx, s.transport, s.client().ResolveLayoutShareToken, &layoutv1.ResolveLayoutShareTokenRequest{Token: token}, func(msg *layoutv1.ResolveLayoutShareTokenResponse) models.ApiData {
		return decode.APIDataFromProtoMust(msg)
	})
}

func (s *LayoutService) CreateLayoutShareLink(ctx context.Context, layoutID string, expiresAtMs int64) (models.ApiData, error) {
	id, err := codecs.IDToInt(layoutID, "layout_id")
	if err != nil {
		return models.ApiData{}, err
	}
	return UnaryAuth(ctx, s.transport, s.client().CreateLayoutShareLink, &layoutv1.CreateLayoutShareLinkRequest{LayoutId: id, ExpiresAtMs: uint64(expiresAtMs)}, func(msg *layoutv1.CreateLayoutShareLinkResponse) models.ApiData {
		return decode.APIDataFromProtoMust(msg)
	})
}

func (s *LayoutService) RevokeLayoutShareLink(ctx context.Context, token string) (models.ApiData, error) {
	return UnaryAuth(ctx, s.transport, s.client().RevokeLayoutShareLink, &layoutv1.RevokeLayoutShareLinkRequest{Token: token}, func(msg *layoutv1.RevokeLayoutShareLinkResponse) models.ApiData {
		return decode.APIDataFromProtoMust(msg)
	})
}

func (s *LayoutService) ListOwnerPublishedLayouts(ctx context.Context, ownerID string, limit int, pageToken string) (models.ApiData, error) {
	id, err := codecs.IDToInt(ownerID, "owner_id")
	if err != nil {
		return models.ApiData{}, err
	}
	req := &layoutv1.ListOwnerPublishedLayoutsRequest{OwnerId: id, Limit: uint32(limit), PageToken: pageToken}
	return UnaryAuth(ctx, s.transport, s.client().ListOwnerPublishedLayouts, req, func(msg *layoutv1.ListOwnerPublishedLayoutsResponse) models.ApiData {
		return decode.APIDataFromProtoMust(msg)
	})
}

func (s *LayoutService) PublishLayout(ctx context.Context, layoutID, title, description string, tags []string, isListed bool, changelog string) (models.ApiData, error) {
	id, err := codecs.IDToInt(layoutID, "layout_id")
	if err != nil {
		return models.ApiData{}, err
	}
	req := &layoutv1.PublishLayoutRequest{
		LayoutId: id, Title: title, Description: description, IsListed: isListed, Changelog: changelog,
	}
	if len(tags) > 0 {
		req.Tags = append([]string(nil), tags...)
	}
	return UnaryAuth(ctx, s.transport, s.client().PublishLayout, req, func(msg *layoutv1.PublishLayoutResponse) models.ApiData {
		return decode.APIDataFromProtoMust(msg)
	})
}

func (s *LayoutService) UnpublishLayout(ctx context.Context, templateID string) (models.ApiData, error) {
	id, err := codecs.IDToInt(templateID, "template_id")
	if err != nil {
		return models.ApiData{}, err
	}
	return UnaryAuth(ctx, s.transport, s.client().UnpublishLayout, &layoutv1.UnpublishLayoutRequest{TemplateId: id}, func(msg *layoutv1.UnpublishLayoutResponse) models.ApiData {
		return decode.APIDataFromProtoMust(msg)
	})
}

func (s *LayoutService) ListLayoutTemplateVersions(ctx context.Context, ownerID, templateID string, limit int, pageToken string) (models.ApiData, error) {
	owner, err := codecs.IDToInt(ownerID, "owner_id")
	if err != nil {
		return models.ApiData{}, err
	}
	template, err := codecs.IDToInt(templateID, "template_id")
	if err != nil {
		return models.ApiData{}, err
	}
	req := &layoutv1.ListLayoutTemplateVersionsRequest{
		OwnerId: owner, TemplateId: template, Limit: uint32(limit), PageToken: pageToken,
	}
	return UnaryAuth(ctx, s.transport, s.client().ListLayoutTemplateVersions, req, func(msg *layoutv1.ListLayoutTemplateVersionsResponse) models.ApiData {
		return decode.APIDataFromProtoMust(msg)
	})
}

func (s *LayoutService) GetLayoutTemplateVersion(ctx context.Context, ownerID, templateID string, version int) (models.ApiData, error) {
	owner, err := codecs.IDToInt(ownerID, "owner_id")
	if err != nil {
		return models.ApiData{}, err
	}
	template, err := codecs.IDToInt(templateID, "template_id")
	if err != nil {
		return models.ApiData{}, err
	}
	req := &layoutv1.GetLayoutTemplateVersionRequest{OwnerId: owner, TemplateId: template, Version: uint32(version)}
	return UnaryAuth(ctx, s.transport, s.client().GetLayoutTemplateVersion, req, func(msg *layoutv1.GetLayoutTemplateVersionResponse) models.ApiData {
		return decode.APIDataFromProtoMust(msg)
	})
}

func (s *LayoutService) SetLayoutTemplateSubscription(ctx context.Context, ownerID, templateID string, trackLatest bool, pinnedVersion int) (models.ApiData, error) {
	owner, err := codecs.IDToInt(ownerID, "owner_id")
	if err != nil {
		return models.ApiData{}, err
	}
	template, err := codecs.IDToInt(templateID, "template_id")
	if err != nil {
		return models.ApiData{}, err
	}
	req := &layoutv1.SetLayoutTemplateSubscriptionRequest{
		OwnerId: owner, TemplateId: template, TrackLatest: trackLatest,
	}
	if pinnedVersion != 0 {
		v := uint32(pinnedVersion)
		req.PinnedVersion = &v
	}
	return UnaryAuth(ctx, s.transport, s.client().SetLayoutTemplateSubscription, req, func(msg *layoutv1.SetLayoutTemplateSubscriptionResponse) models.ApiData {
		return decode.APIDataFromProtoMust(msg)
	})
}

func (s *LayoutService) DeleteLayoutTemplateSubscription(ctx context.Context, ownerID, templateID string) (models.ApiData, error) {
	owner, err := codecs.IDToInt(ownerID, "owner_id")
	if err != nil {
		return models.ApiData{}, err
	}
	template, err := codecs.IDToInt(templateID, "template_id")
	if err != nil {
		return models.ApiData{}, err
	}
	req := &layoutv1.DeleteLayoutTemplateSubscriptionRequest{OwnerId: owner, TemplateId: template}
	return UnaryAuth(ctx, s.transport, s.client().DeleteLayoutTemplateSubscription, req, func(msg *layoutv1.DeleteLayoutTemplateSubscriptionResponse) models.ApiData {
		return decode.APIDataFromProtoMust(msg)
	})
}

func (s *LayoutService) ListMyLayoutTemplateSubscriptions(ctx context.Context, limit int, pageToken string) (models.ApiData, error) {
	req := &layoutv1.ListMyLayoutTemplateSubscriptionsRequest{Limit: uint32(limit), PageToken: pageToken}
	return UnaryAuth(ctx, s.transport, s.client().ListMyLayoutTemplateSubscriptions, req, func(msg *layoutv1.ListMyLayoutTemplateSubscriptionsResponse) models.ApiData {
		return decode.APIDataFromProtoMust(msg)
	})
}
