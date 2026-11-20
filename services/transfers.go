package services

import (
	"context"
	"math"

	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	"github.com/Fabric-Labs/polyester-sdk-go/codecs/decode"
	"github.com/Fabric-Labs/polyester-sdk-go/errors"
	ledgerrdv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/ledger/read/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/gen/ledger/read/v1/ledgerrdv1connect"
	ledgerv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/ledger/v1"
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

// ListTransfersParams selects a page of ledger transfers for ListWith.
type ListTransfersParams struct {
	// Account and SubAccountID select the account as for List.
	Account      AccountScope
	SubAccountID *string
	// Limit is the page size; 0 uses the server's default.
	Limit int
	// Reversed lists newest first; false lists oldest first.
	Reversed bool
	// Since lists transfers at or after this time, in UTC epoch microseconds;
	// nil sets no lower bound.
	Since *int64
	// TransferCode lists only transfers with this reason, a
	// ledger.v1.TransferCode value, the same number a row carries in
	// models.LedgerTransfer.TransferType (for example 1000 for DEPOSIT).
	// 0 lists every reason.
	TransferCode int
	// PageToken continues from an earlier page's NextPageToken. A token is
	// bound to the filters it was returned for: send it with the same
	// account, TransferCode, Since and Reversed. "" reads the first page.
	PageToken string
}

// List reads a page. Pass its opaque NextPageToken as the final argument to
// continue with the same account, filters and direction. An empty token ends
// paging. Use ListWith to list one transfer reason.
func (s *TransfersService) List(ctx context.Context, account AccountScope, subAccountID *string, limit int, reversed bool, since *int64, pageToken ...string) (models.TransfersList, error) {
	if len(pageToken) > 1 {
		return models.TransfersList{}, &errors.ValidationError{Msg: "at most one page token is allowed"}
	}
	params := ListTransfersParams{Account: account, SubAccountID: subAccountID, Limit: limit, Reversed: reversed, Since: since}
	if len(pageToken) == 1 {
		params.PageToken = pageToken[0]
	}
	return s.ListWith(ctx, params)
}

// ListWith reads a page of ledger transfers selected by params. An empty
// NextPageToken ends paging.
func (s *TransfersService) ListWith(ctx context.Context, params ListTransfersParams) (models.TransfersList, error) {
	parsedLimit, err := PaginationLimit(params.Limit, "limit")
	if err != nil {
		return models.TransfersList{}, err
	}
	req := &ledgerrdv1.ListTransfersRequest{Limit: parsedLimit, Reversed: params.Reversed, PageToken: params.PageToken}
	if err := codecs.CheckPageLimit(req); err != nil {
		return models.TransfersList{}, err
	}
	if params.TransferCode < 0 || params.TransferCode > math.MaxInt32 {
		return models.TransfersList{}, &errors.ValidationError{Msg: "transfer_code must be a ledger.v1.TransferCode value, or 0 for every reason"}
	}
	req.TransferCode = ledgerv1.TransferCode(params.TransferCode)
	sub, err := s.scoped.ResolveSubAccountID(params.SubAccountID, params.Account)
	if err != nil {
		return models.TransfersList{}, err
	}
	if parsed, err := codecs.ParseOptionalSubaccountID(sub); err != nil {
		return models.TransfersList{}, err
	} else if parsed != nil {
		req.SubaccountId = parsed
	}
	if params.Since != nil {
		if *params.Since < 0 {
			return models.TransfersList{}, &errors.ValidationError{Msg: "since must be non-negative (UTC epoch microseconds)"}
		}
		req.TsMinUs = uint64(*params.Since)
	}
	return UnaryAuthDecoded(ctx, s.transport, s.client().ListTransfers, req, decode.TransfersListFromProto)
}

func (s *TransfersService) Subscribe(ctx context.Context, accountID any) (*realtime.Subscription[models.LedgerTransfer], error) {
	return SubscribeAccountProtoBatch(ctx, s.realtime, "private:ledger:transfers:{account_id}:proto", accountID, s.defaultAccountID, decode.LedgerTransferBatchFromBytes)
}
