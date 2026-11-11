package services

import (
	"context"

	"github.com/Fabric-Labs/polyester-sdk-go/codecs/decode"
	chaindepositv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/chain/deposit/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/gen/chain/deposit/v1/chaindepositv1connect"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
	"github.com/Fabric-Labs/polyester-sdk-go/transport"
)

type DepositService struct {
	transport *transport.Factory
	scoped    ScopedSubAccount
}

func NewDepositService(factory *transport.Factory, defaultSubAccountID *string) *DepositService {
	return &DepositService{transport: factory, scoped: ScopedSubAccount{DefaultSubAccountID: defaultSubAccountID}}
}

func (s *DepositService) client() chaindepositv1connect.DepositAddressServiceClient {
	return chaindepositv1connect.NewDepositAddressServiceClient(s.transport.HTTP, s.transport.Config.APIURL, s.transport.ConnectOptions(true)...)
}

// ListAddresses lists the account's deposit addresses on chainID. An API key
// needs READ_DEPOSIT_ADDRESSES on its policy; a key without a policy is
// refused with permission_denied (AUTH_API_KEY_ACCESS_DENIED).
func (s *DepositService) ListAddresses(ctx context.Context, chainID uint32, account AccountScope, subAccountID *string) (models.DepositAddressesList, error) {
	req := &chaindepositv1.ListDepositAddressesRequest{ChainId: chainID}
	if err := s.scoped.ApplyOptionalSubaccountID(&req.SubaccountId, account, subAccountID); err != nil {
		return models.DepositAddressesList{}, err
	}
	return UnaryAuth(ctx, s.transport, s.client().ListDepositAddresses, req, decode.DepositAddressesListFromProto)
}

// CreateAddress creates a deposit address on chainID. An API key needs
// CREATE_DEPOSIT_ADDRESS on its policy.
func (s *DepositService) CreateAddress(ctx context.Context, chainID uint32, account AccountScope, subAccountID *string) (models.DepositAddress, error) {
	req := &chaindepositv1.CreateDepositAddressRequest{ChainId: chainID}
	if err := s.scoped.ApplyOptionalSubaccountID(&req.SubaccountId, account, subAccountID); err != nil {
		return models.DepositAddress{}, err
	}
	result, err := UnaryAuthDecoded(ctx, s.transport, s.client().CreateDepositAddress, req, decode.CreateDepositAddressFromProto)
	if err != nil {
		return models.DepositAddress{}, err
	}
	return result, nil
}
