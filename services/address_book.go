package services

import (
	"context"

	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	"github.com/Fabric-Labs/polyester-sdk-go/codecs/decode"
	authv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/auth/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/gen/auth/v1/authv1connect"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
	"github.com/Fabric-Labs/polyester-sdk-go/realtime"
	"github.com/Fabric-Labs/polyester-sdk-go/transport"
)

type AddressBookService struct {
	transport        *transport.Factory
	scoped           ScopedSubAccount
	defaultAccountID *string
	realtime         RealtimeClient
}

func NewAddressBookService(factory *transport.Factory, defaultSubAccountID *string, realtime RealtimeClient, defaultAccountID *string) *AddressBookService {
	return &AddressBookService{transport: factory, scoped: ScopedSubAccount{DefaultSubAccountID: defaultSubAccountID}, realtime: realtime, defaultAccountID: defaultAccountID}
}

func (s *AddressBookService) client() authv1connect.AddressBookServiceClient {
	return authv1connect.NewAddressBookServiceClient(s.transport.HTTP, s.transport.Config.APIURL, s.transport.ConnectOptions(true)...)
}

func (s *AddressBookService) ListBooks(ctx context.Context) (models.AddressBooksList, error) {
	return UnaryAuth(ctx, s.transport, s.client().ListAddressBooks, &authv1.ListAddressBooksRequest{}, decode.ListBooksFromProto)
}

func (s *AddressBookService) ListEntries(ctx context.Context, account AccountScope, subAccountID, kind *string, limit int, pageToken *string) (models.AddressBookEntriesList, error) {
	parsedLimit, err := PaginationLimitOrDefault(limit, 50, "limit")
	if err != nil {
		return models.AddressBookEntriesList{}, err
	}
	req := &authv1.ListAddressBookEntriesRequest{Limit: parsedLimit}
	if err := s.scoped.ApplyOptionalSubaccountID(&req.SubaccountId, account, subAccountID); err != nil {
		return models.AddressBookEntriesList{}, err
	}
	if kind != nil && *kind != "" {
		req.Kind = codecs.AddressBookEntryKindFromLabel(*kind)
	}
	if pageToken != nil {
		req.PageToken = *pageToken
	}
	return UnaryAuth(ctx, s.transport, s.client().ListAddressBookEntries, req, decode.ListEntriesFromProto)
}

func (s *AddressBookService) ListTransferCounterparties(ctx context.Context, account AccountScope, subAccountID *string, direction, kind *string, limit int) (models.TransferCounterpartiesList, error) {
	parsedLimit, err := PaginationLimitOrDefault(limit, 50, "limit")
	if err != nil {
		return models.TransferCounterpartiesList{}, err
	}
	req := &authv1.ListTransferCounterpartiesRequest{Limit: parsedLimit}
	if err := s.scoped.ApplyOptionalSubaccountID(&req.SubaccountId, account, subAccountID); err != nil {
		return models.TransferCounterpartiesList{}, err
	}
	if direction != nil && *direction != "" {
		req.Direction = codecs.TransferCounterpartyDirectionFromLabel(*direction)
	}
	if kind != nil && *kind != "" {
		req.Kind = codecs.AddressBookEntryKindFromLabel(*kind)
	}
	return UnaryAuth(ctx, s.transport, s.client().ListTransferCounterparties, req, decode.ListCounterpartiesFromProto)
}

func (s *AddressBookService) ListTransferDestinations(ctx context.Context, account AccountScope, subAccountID *string, kind string, limit int, pageToken *string) (models.AddressBookTransferDestinationsList, error) {
	parsedLimit, err := PaginationLimitOrDefault(limit, 50, "limit")
	if err != nil {
		return models.AddressBookTransferDestinationsList{}, err
	}
	if kind == "" {
		kind = "internal_account"
	}
	req := &authv1.ListTransferDestinationsRequest{
		Kind:  codecs.AddressBookEntryKindFromLabel(kind),
		Limit: parsedLimit,
	}
	if err := s.scoped.ApplyOptionalSubaccountID(&req.SubaccountId, account, subAccountID); err != nil {
		return models.AddressBookTransferDestinationsList{}, err
	}
	if pageToken != nil {
		req.PageToken = *pageToken
	}
	return UnaryAuth(ctx, s.transport, s.client().ListTransferDestinations, req, decode.ListDestinationsFromProto)
}

func (s *AddressBookService) ListInternalTransferWhitelistEntries(ctx context.Context, account AccountScope, subAccountID *string, limit int, pageToken *string) (models.InternalTransferWhitelistEntriesList, error) {
	parsedLimit, err := PaginationLimitOrDefault(limit, 50, "limit")
	if err != nil {
		return models.InternalTransferWhitelistEntriesList{}, err
	}
	req := &authv1.ListInternalTransferWhitelistEntriesRequest{Limit: parsedLimit}
	if err := s.scoped.ApplyOptionalSubaccountID(&req.SubaccountId, account, subAccountID); err != nil {
		return models.InternalTransferWhitelistEntriesList{}, err
	}
	if pageToken != nil {
		req.PageToken = *pageToken
	}
	return UnaryAuth(ctx, s.transport, s.client().ListInternalTransferWhitelistEntries, req, decode.ListInternalWhitelistFromProto)
}

func (s *AddressBookService) GetWithdrawWhitelistView(ctx context.Context, account AccountScope, subAccountID *string) (models.WithdrawWhitelistView, error) {
	req := &authv1.GetWithdrawWhitelistViewRequest{}
	if err := s.scoped.ApplyOptionalSubaccountID(&req.SubaccountId, account, subAccountID); err != nil {
		return models.WithdrawWhitelistView{}, err
	}
	return UnaryAuth(ctx, s.transport, s.client().GetWithdrawWhitelistView, req, decode.WithdrawWhitelistViewFromGetProto)
}

func (s *AddressBookService) GetView(ctx context.Context, account AccountScope, subAccountID *string, limit int) (models.AddressBookView, error) {
	parsedLimit, err := PaginationLimitOrDefault(limit, 50, "limit")
	if err != nil {
		return models.AddressBookView{}, err
	}
	req := &authv1.GetAddressBookViewRequest{Limit: parsedLimit}
	if err := s.scoped.ApplyOptionalSubaccountID(&req.SubaccountId, account, subAccountID); err != nil {
		return models.AddressBookView{}, err
	}
	return UnaryAuth(ctx, s.transport, s.client().GetAddressBookView, req, decode.AddressBookViewFromProto)
}

func (s *AddressBookService) Subscribe(ctx context.Context, accountID any) (*realtime.Subscription[models.AddressBookViewInvalidation], error) {
	return SubscribeAccountProto(ctx, s.realtime, "private:auth:address-books:{account_id}:proto", accountID, s.defaultAccountID, decode.AddressBookInvalidationFromBytes)
}

func (s *AddressBookService) SubscribeViewInvalidations(ctx context.Context, rootAccountPublicID any) (*realtime.Subscription[models.AddressBookViewInvalidation], error) {
	return s.Subscribe(ctx, rootAccountPublicID)
}
