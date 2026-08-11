package services

import (
	"context"

	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	"github.com/Fabric-Labs/polyester-sdk-go/codecs/decode"
	ledgerrdv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/ledger/read/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/gen/ledger/read/v1/ledgerrdv1connect"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
	"github.com/Fabric-Labs/polyester-sdk-go/realtime"
	"github.com/Fabric-Labs/polyester-sdk-go/transport"
)

type TransfersService struct {
	transport        *transport.Factory
	scoped           ScopedSubAccount
	defaultAccountID *string
	realtime         RealtimeClient
}

func NewTransfersService(factory *transport.Factory, defaultSubAccountID *string, realtime RealtimeClient, defaultAccountID *string) *TransfersService {
	return &TransfersService{transport: factory, scoped: ScopedSubAccount{DefaultSubAccountID: defaultSubAccountID}, realtime: realtime, defaultAccountID: defaultAccountID}
}

func (s *TransfersService) client() ledgerrdv1connect.LedgerReadServiceClient {
	return ledgerrdv1connect.NewLedgerReadServiceClient(s.transport.HTTP, s.transport.Config.APIURL, s.transport.ConnectOptions(true)...)
}

func (s *TransfersService) List(ctx context.Context, account AccountScope, subAccountID *string, limit int, reversed bool, since *int64) (models.TransfersList, error) {
	parsedLimit, err := PaginationLimit(limit, "limit")
	if err != nil {
		return models.TransfersList{}, err
	}
	req := &ledgerrdv1.ListTransfersRequest{Limit: parsedLimit, Reversed: reversed}
	sub, err := s.scoped.ResolveSubAccountID(subAccountID, account)
	if err != nil {
		return models.TransfersList{}, err
	}
	if parsed, err := codecs.ParseOptionalSubaccountID(sub); err != nil {
		return models.TransfersList{}, err
	} else if parsed != nil {
		req.SubaccountId = parsed
	}
	if since != nil {
		req.TsMinUs = uint64(*since)
	}
	return UnaryAuth(ctx, s.transport, s.client().ListTransfers, req, decode.TransfersListFromProto)
}

func (s *TransfersService) Subscribe(ctx context.Context, accountID any) (*realtime.Subscription[models.LedgerTransfer], error) {
	return SubscribeAccountProto(ctx, s.realtime, "private:ledger:transfers:{account_id}:proto", accountID, s.defaultAccountID, decode.LedgerTransferFromBytes)
}
