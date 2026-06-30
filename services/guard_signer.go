package services

import (
	"context"

	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	"github.com/Fabric-Labs/polyester-sdk-go/codecs/decode"
	guardv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/chain/guard/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/gen/chain/guard/v1/chainguardv1connect"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
	"github.com/Fabric-Labs/polyester-sdk-go/transport"
)

type GuardSignerService struct {
	transport *transport.Factory
	scoped    ScopedSubAccount
}

func NewGuardSignerService(factory *transport.Factory, defaultSubAccountID *string) *GuardSignerService {
	return &GuardSignerService{transport: factory, scoped: ScopedSubAccount{DefaultSubAccountID: defaultSubAccountID}}
}

func (s *GuardSignerService) client() chainguardv1connect.GuardSignerServiceClient {
	return chainguardv1connect.NewGuardSignerServiceClient(s.transport.HTTP, s.transport.Config.APIURL, s.transport.ConnectOptions(true)...)
}

func (s *GuardSignerService) CreateWallet(ctx context.Context, account AccountScope, subAccountID *string) (models.CreateGuardSignerWalletResult, error) {
	req := &guardv1.CreateGuardSignerWalletRequest{}
	if err := s.scoped.ApplyOptionalSubaccountID(&req.SubaccountId, account, subAccountID); err != nil {
		return models.CreateGuardSignerWalletResult{}, err
	}
	return UnaryAuth(ctx, s.transport, s.client().CreateGuardSignerWallet, req, decode.CreateWalletFromProto)
}

func (s *GuardSignerService) GetStatus(ctx context.Context, account AccountScope, subAccountID *string) (*models.GuardSignerStatus, error) {
	req := &guardv1.GetGuardSignerStatusRequest{}
	if err := s.scoped.ApplyOptionalSubaccountID(&req.SubaccountId, account, subAccountID); err != nil {
		return nil, err
	}
	return UnaryAuth(ctx, s.transport, s.client().GetGuardSignerStatus, req, decode.StatusFromProto)
}

func (s *GuardSignerService) SignProtectedAction(ctx context.Context, action string, account AccountScope, subAccountID *string, externalPolychainChainID *int, externalAddresses, internalAddresses []string, whitelistRequired *bool) (*models.GuardApproval, error) {
	req := &guardv1.SignProtectedActionRequest{
		Action: codecs.ProtectedActionFromLabel(action),
		Args: codecs.ProtectedActionArgsToProto(
			externalPolychainChainID,
			externalAddresses,
			internalAddresses,
			whitelistRequired,
		),
	}
	if err := s.scoped.ApplyOptionalSubaccountID(&req.SubaccountId, account, subAccountID); err != nil {
		return nil, err
	}
	return UnaryAuth(ctx, s.transport, s.client().SignProtectedAction, req, decode.SignProtectedActionFromProto)
}

func (s *GuardSignerService) BatchSignProtectedActions(ctx context.Context, actions []map[string]any, account AccountScope, subAccountID *string) (models.BatchSignProtectedActionsResult, error) {
	items := make([]*guardv1.BatchSignProtectedActionItem, 0, len(actions))
	for _, item := range actions {
		parsed, err := codecs.BatchProtectedActionFromMap(item)
		if err != nil {
			return models.BatchSignProtectedActionsResult{}, err
		}
		items = append(items, codecs.BatchProtectedActionItemToProto(parsed))
	}
	req := &guardv1.BatchSignProtectedActionsRequest{Actions: items}
	if err := s.scoped.ApplyOptionalSubaccountID(&req.SubaccountId, account, subAccountID); err != nil {
		return models.BatchSignProtectedActionsResult{}, err
	}
	return UnaryAuth(ctx, s.transport, s.client().BatchSignProtectedActions, req, decode.BatchSignFromProto)
}

func (s *GuardSignerService) RotateWallet(ctx context.Context, account AccountScope, subAccountID *string) (models.RotateGuardSignerWalletResult, error) {
	req := &guardv1.RotateGuardSignerWalletRequest{}
	if err := s.scoped.ApplyOptionalSubaccountID(&req.SubaccountId, account, subAccountID); err != nil {
		return models.RotateGuardSignerWalletResult{}, err
	}
	return UnaryAuth(ctx, s.transport, s.client().RotateGuardSignerWallet, req, decode.RotateWalletFromProto)
}

func (s *GuardSignerService) ExportWallet(ctx context.Context, account AccountScope, subAccountID *string) (models.ExportGuardSignerWalletResult, error) {
	req := &guardv1.ExportGuardSignerWalletRequest{}
	if err := s.scoped.ApplyOptionalSubaccountID(&req.SubaccountId, account, subAccountID); err != nil {
		return models.ExportGuardSignerWalletResult{}, err
	}
	return UnaryAuth(ctx, s.transport, s.client().ExportGuardSignerWallet, req, decode.ExportWalletFromProto)
}
