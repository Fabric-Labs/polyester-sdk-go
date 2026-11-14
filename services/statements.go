package services

import (
	"context"
	"strings"
	"time"

	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	"github.com/Fabric-Labs/polyester-sdk-go/errors"
	ledgerrdv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/ledger/read/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/gen/ledger/read/v1/ledgerrdv1connect"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
	"github.com/Fabric-Labs/polyester-sdk-go/transport"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// ExportStatementOptions selects the account, period and asset of an
// account statement.
type ExportStatementOptions struct {
	// Account and SubAccountID select the account as for Transfers.List: pass
	// one or neither. With neither, the client's DefaultSubAccountID is used,
	// and without one the statement covers the account the credentials
	// authenticate (a sub-account-scoped key's own sub-account).
	Account      AccountScope
	SubAccountID *string
	// StartTime (inclusive) and EndTime (exclusive) bound the period, at most
	// 366 days long. Both are required. Movements recorded after the request
	// are not included.
	StartTime time.Time
	EndTime   time.Time
	// AssetID keeps one asset, by its numeric ID (the LedgerID of the spot and
	// funding configs); 0 covers every asset.
	AssetID uint32
	// PageToken continues from an earlier page's NextPageToken. Reuse it only
	// with the same fields.
	PageToken string
}

// StatementsService exports account statements: an account's balance
// movements for a period, with fees and the balance after each entry, as CSV.
// An API key needs READ_STATEMENTS on its policy (READ_BALANCES and
// READ_INTERNAL_TRANSFERS do not grant it).
type StatementsService struct {
	transport *transport.Factory
	scoped    ScopedSubAccount
}

func NewStatementsService(factory *transport.Factory, defaultSubAccountID *string) *StatementsService {
	return &StatementsService{transport: factory, scoped: ScopedSubAccount{DefaultSubAccountID: defaultSubAccountID}}
}

func (s *StatementsService) client() ledgerrdv1connect.LedgerReadServiceClient {
	return ledgerrdv1connect.NewLedgerReadServiceClient(s.transport.HTTP, s.transport.Config.APIURL, s.transport.ConnectOptions(true)...)
}

// ExportPage reads one page of the statement: at most 2000 ledger
// movements, entries never split, oldest first. Only the first page starts
// with the header row. Continue with the page's NextPageToken in
// opts.PageToken until it is empty, or use Export.
func (s *StatementsService) ExportPage(ctx context.Context, opts ExportStatementOptions) (models.StatementPage, error) {
	req, err := s.request(opts)
	if err != nil {
		return models.StatementPage{}, err
	}
	return UnaryAuth(ctx, s.transport, s.client().ExportStatement, req, func(msg *ledgerrdv1.ExportStatementResponse) models.StatementPage {
		return models.StatementPage{CSV: msg.GetCsv(), NextPageToken: msg.GetNextPageToken()}
	})
}

// Export reads the whole statement from opts.PageToken (or its start),
// following each page's NextPageToken with the same fields, and returns the
// pages' CSV concatenated in order. It stops at the first error.
func (s *StatementsService) Export(ctx context.Context, opts ExportStatementOptions) (models.Statement, error) {
	var csv strings.Builder
	for page, err := range paginate(opts.PageToken, func(token string) ([]string, string, error) {
		opts.PageToken = token
		page, err := s.ExportPage(ctx, opts)
		return []string{page.CSV}, page.NextPageToken, err
	}) {
		if err != nil {
			return models.Statement{}, err
		}
		csv.WriteString(page)
	}
	return models.Statement{CSV: csv.String()}, nil
}

func (s *StatementsService) request(opts ExportStatementOptions) (*ledgerrdv1.ExportStatementRequest, error) {
	if opts.StartTime.IsZero() || opts.EndTime.IsZero() {
		return nil, &errors.ValidationError{Msg: "statement export requires StartTime and EndTime"}
	}
	req := &ledgerrdv1.ExportStatementRequest{
		StartTime: timestamppb.New(opts.StartTime),
		EndTime:   timestamppb.New(opts.EndTime),
		AssetId:   opts.AssetID,
		PageToken: opts.PageToken,
	}
	sub, err := s.scoped.ResolveSubAccountID(opts.SubAccountID, opts.Account)
	if err != nil {
		return nil, err
	}
	parsed, err := codecs.ParseOptionalSubaccountID(sub)
	if err != nil {
		return nil, err
	}
	req.SubaccountId = parsed
	return req, nil
}
