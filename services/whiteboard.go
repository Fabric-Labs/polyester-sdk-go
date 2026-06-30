package services

import (
	"context"

	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	collabv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/collab/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/gen/collab/v1/collabv1connect"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
	"github.com/Fabric-Labs/polyester-sdk-go/transport"
	"github.com/Fabric-Labs/polyester-sdk-go/wire"
)

type WhiteboardService struct {
	transport *transport.Factory
}

func NewWhiteboardService(factory *transport.Factory) *WhiteboardService {
	return &WhiteboardService{transport: factory}
}

func (s *WhiteboardService) client() collabv1connect.WhiteboardServiceClient {
	return collabv1connect.NewWhiteboardServiceClient(s.transport.HTTP, s.transport.Config.APIURL, s.transport.ConnectOptions(true)...)
}

func (s *WhiteboardService) Create(ctx context.Context, title, audience, defaultRole string, aclEntries []map[string]any, initialSnapshot map[string]any) (models.ApiData, error) {
	if audience == "" {
		audience = "private"
	}
	if defaultRole == "" {
		defaultRole = "viewer"
	}
	req := &collabv1.CreateBoardRequest{
		Title:       title,
		Audience:    codecs.BoardAudienceFromLabel(audience),
		DefaultRole: codecs.BoardRoleFromLabel(defaultRole),
	}
	entries, err := boardACLEntriesFromMaps(aclEntries)
	if err != nil {
		return models.ApiData{}, err
	}
	req.AclEntries = entries
	if initialSnapshot != nil {
		st, err := wire.StructFromMap(initialSnapshot)
		if err != nil {
			return models.ApiData{}, err
		}
		req.InitialSnapshot = st
	}
	return UnaryAuth(ctx, s.transport, s.client().CreateBoard, req, func(msg *collabv1.CreateBoardResponse) models.ApiData { return apiData(msg) })
}

func (s *WhiteboardService) Get(ctx context.Context, boardID string) (models.ApiData, error) {
	return UnaryAuth(ctx, s.transport, s.client().GetBoard, &collabv1.GetBoardRequest{BoardId: boardID}, func(msg *collabv1.GetBoardResponse) models.ApiData { return apiData(msg) })
}

func (s *WhiteboardService) List(ctx context.Context, includeArchived bool, limit int, pageToken string) (models.ApiData, error) {
	req := &collabv1.ListBoardsRequest{IncludeArchived: includeArchived, Limit: uint32(limit), PageToken: pageToken}
	return UnaryAuth(ctx, s.transport, s.client().ListBoards, req, func(msg *collabv1.ListBoardsResponse) models.ApiData { return apiData(msg) })
}

func (s *WhiteboardService) Update(ctx context.Context, boardID, title, audience, defaultRole string, initialSnapshot map[string]any) (models.ApiData, error) {
	req := &collabv1.UpdateBoardRequest{BoardId: boardID}
	if title != "" {
		req.Title = &title
	}
	if audience != "" {
		v := codecs.BoardAudienceFromLabel(audience)
		req.Audience = &v
	}
	if defaultRole != "" {
		v := codecs.BoardRoleFromLabel(defaultRole)
		req.DefaultRole = &v
	}
	if initialSnapshot != nil {
		st, err := wire.StructFromMap(initialSnapshot)
		if err != nil {
			return models.ApiData{}, err
		}
		req.InitialSnapshot = st
	}
	return UnaryAuth(ctx, s.transport, s.client().UpdateBoard, req, func(msg *collabv1.UpdateBoardResponse) models.ApiData { return apiData(msg) })
}

func (s *WhiteboardService) UpdateACL(ctx context.Context, boardID string, aclEntries []map[string]any) (models.ApiData, error) {
	entries, err := boardACLEntriesFromMaps(aclEntries)
	if err != nil {
		return models.ApiData{}, err
	}
	req := &collabv1.UpdateBoardAclRequest{BoardId: boardID, AclEntries: entries}
	return UnaryAuth(ctx, s.transport, s.client().UpdateBoardAcl, req, func(msg *collabv1.UpdateBoardAclResponse) models.ApiData { return apiData(msg) })
}

func (s *WhiteboardService) Archive(ctx context.Context, boardID string, archived bool) (models.ApiData, error) {
	req := &collabv1.ArchiveBoardRequest{BoardId: boardID, Archived: archived}
	return UnaryAuth(ctx, s.transport, s.client().ArchiveBoard, req, func(msg *collabv1.ArchiveBoardResponse) models.ApiData { return apiData(msg) })
}

func (s *WhiteboardService) MintJoinToken(ctx context.Context, boardID string) (models.ApiData, error) {
	return UnaryAuth(ctx, s.transport, s.client().MintJoinToken, &collabv1.MintJoinTokenRequest{BoardId: boardID}, func(msg *collabv1.MintJoinTokenResponse) models.ApiData { return apiData(msg) })
}

func boardACLEntriesFromMaps(values []map[string]any) ([]*collabv1.BoardAclEntry, error) {
	if len(values) == 0 {
		return nil, nil
	}
	out := make([]*collabv1.BoardAclEntry, 0, len(values))
	for _, value := range values {
		msg := &collabv1.BoardAclEntry{}
		if err := wire.MessageFromMap(msg, value); err != nil {
			return nil, err
		}
		out = append(out, msg)
	}
	return out, nil
}
