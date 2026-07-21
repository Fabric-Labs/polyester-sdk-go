package services

import (
	"context"
	"strings"

	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	"github.com/Fabric-Labs/polyester-sdk-go/codecs/decode"
	"github.com/Fabric-Labs/polyester-sdk-go/errors"
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
	if limit <= 0 {
		limit = 50
	}
	req := &authv1.ListAddressBookEntriesRequest{Limit: uint32(limit)}
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

func (s *AddressBookService) CreateEntry(ctx context.Context, label string, account AccountScope, subAccountID *string, note, kind string, polychainChainID *uint32, address, smartAccountAddress *string, tagIDs []string, newTags []models.AddressBookTagInput) (models.AddressBookEntry, error) {
	req := &authv1.CreateAddressBookEntryRequest{Label: label, Note: note}
	if err := s.scoped.ApplyOptionalSubaccountID(&req.SubaccountId, account, subAccountID); err != nil {
		return models.AddressBookEntry{}, err
	}
	entryKind := strings.ToLower(kind)
	switch entryKind {
	case "external", "external_chain":
		if polychainChainID == nil || address == nil || *address == "" {
			return models.AddressBookEntry{}, &errors.ValidationError{Msg: "external entries require polychain_chain_id and address"}
		}
		req.Entry = codecs.CreateEntryExternalToProto(*polychainChainID, *address)
	case "internal", "internal_account":
		if smartAccountAddress == nil || *smartAccountAddress == "" {
			return models.AddressBookEntry{}, &errors.ValidationError{Msg: "internal entries require smart_account_address"}
		}
		req.Entry = codecs.CreateEntryInternalToProto(*smartAccountAddress)
	default:
		return models.AddressBookEntry{}, &errors.ValidationError{Msg: "kind must be 'external' or 'internal'"}
	}
	ids, err := parseTagIDs(tagIDs)
	if err != nil {
		return models.AddressBookEntry{}, err
	}
	req.TagIds = ids
	for _, tag := range newTags {
		req.NewTags = append(req.NewTags, codecs.AddressBookTagInputToProto(tag.Name, tag.Color))
	}
	return UnaryAuth(ctx, s.transport, s.client().CreateAddressBookEntry, req, decode.EntryFromCreateProto)
}

func (s *AddressBookService) UpdateEntry(ctx context.Context, addressBookEntryID string, patch codecs.AddressBookEntryPatch) (models.AddressBookEntry, error) {
	id, err := codecs.IDToInt(addressBookEntryID, "address_book_entry_id")
	if err != nil {
		return models.AddressBookEntry{}, err
	}
	req, err := codecs.BuildUpdateAddressBookEntryRequest(id, patch)
	if err != nil {
		return models.AddressBookEntry{}, err
	}
	return UnaryAuth(ctx, s.transport, s.client().UpdateAddressBookEntry, req, decode.EntryFromUpdateProto)
}

func (s *AddressBookService) DeleteEntry(ctx context.Context, addressBookEntryID string) error {
	id, err := codecs.IDToInt(addressBookEntryID, "address_book_entry_id")
	if err != nil {
		return err
	}
	_, err = UnaryAuth(ctx, s.transport, s.client().DeleteAddressBookEntry, &authv1.DeleteAddressBookEntryRequest{AddressBookEntryId: id}, decode.Void[authv1.DeleteAddressBookEntryResponse])
	return err
}

func (s *AddressBookService) CopyEntry(ctx context.Context, addressBookEntryID string, account AccountScope, targetSubAccountID *string) (models.AddressBookEntry, error) {
	id, err := codecs.IDToInt(addressBookEntryID, "address_book_entry_id")
	if err != nil {
		return models.AddressBookEntry{}, err
	}
	req := &authv1.CopyAddressBookEntryRequest{AddressBookEntryId: id}
	if err := s.scoped.ApplyOptionalSubaccountID(&req.TargetSubaccountId, account, targetSubAccountID); err != nil {
		return models.AddressBookEntry{}, err
	}
	return UnaryAuth(ctx, s.transport, s.client().CopyAddressBookEntry, req, decode.EntryFromCopyProto)
}

func (s *AddressBookService) CreateTag(ctx context.Context, name string, account AccountScope, subAccountID *string, color string) (models.AddressBookTag, error) {
	req := &authv1.CreateAddressBookTagRequest{Name: name, Color: color}
	if err := s.scoped.ApplyOptionalSubaccountID(&req.SubaccountId, account, subAccountID); err != nil {
		return models.AddressBookTag{}, err
	}
	return UnaryAuth(ctx, s.transport, s.client().CreateAddressBookTag, req, decode.TagFromCreateProto)
}

func (s *AddressBookService) UpdateTag(ctx context.Context, tagID string, name, color *string) (models.AddressBookTag, error) {
	id, err := codecs.IDToInt(tagID, "tag_id")
	if err != nil {
		return models.AddressBookTag{}, err
	}
	req := &authv1.UpdateAddressBookTagRequest{TagId: id, Name: name, Color: color}
	return UnaryAuth(ctx, s.transport, s.client().UpdateAddressBookTag, req, decode.TagFromUpdateProto)
}

func (s *AddressBookService) DeleteTag(ctx context.Context, tagID string) error {
	id, err := codecs.IDToInt(tagID, "tag_id")
	if err != nil {
		return err
	}
	_, err = UnaryAuth(ctx, s.transport, s.client().DeleteAddressBookTag, &authv1.DeleteAddressBookTagRequest{TagId: id}, decode.Void[authv1.DeleteAddressBookTagResponse])
	return err
}

func (s *AddressBookService) ListTransferCounterparties(ctx context.Context, account AccountScope, subAccountID *string, direction, kind *string, limit int) (models.TransferCounterpartiesList, error) {
	if limit <= 0 {
		limit = 50
	}
	req := &authv1.ListTransferCounterpartiesRequest{Limit: uint32(limit)}
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
	if limit <= 0 {
		limit = 50
	}
	if kind == "" {
		kind = "internal_account"
	}
	req := &authv1.ListTransferDestinationsRequest{
		Kind:  codecs.AddressBookEntryKindFromLabel(kind),
		Limit: uint32(limit),
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
	if limit <= 0 {
		limit = 50
	}
	req := &authv1.ListInternalTransferWhitelistEntriesRequest{Limit: uint32(limit)}
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
	if limit <= 0 {
		limit = 50
	}
	req := &authv1.GetAddressBookViewRequest{Limit: uint32(limit)}
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

func parseTagIDs(tagIDs []string) ([]uint64, error) {
	if len(tagIDs) == 0 {
		return nil, nil
	}
	out := make([]uint64, 0, len(tagIDs))
	for _, item := range tagIDs {
		id, err := codecs.IDToInt(item, "tag_id")
		if err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, nil
}
