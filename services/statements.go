package services

import (
	"context"
	"fmt"
	"iter"
	"strings"
	"time"

	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	"github.com/Fabric-Labs/polyester-sdk-go/codecs/decode"
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
	// 366 days long. Both are required. The statement covers the period up to
	// its CompleteThrough, which is at most EndTime.
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
	return UnaryAuth(ctx, s.transport, s.client().ExportStatement, req, decode.StatementPageFromProto)
}

// Export reads the whole statement from opts.PageToken (or its start),
// following each page's NextPageToken with the same fields, and returns the
// pages' CSV concatenated in order with their CompleteThrough. It stops at
// the first error. A page whose CompleteThrough differs from the first page's
// returns a *errors.ResponseContractError.
func (s *StatementsService) Export(ctx context.Context, opts ExportStatementOptions) (models.Statement, error) {
	var csv strings.Builder
	var out models.Statement
	first := true
	for page, err := range paginate(opts.PageToken, func(token string) ([]models.StatementPage, string, error) {
		opts.PageToken = token
		page, err := s.ExportPage(ctx, opts)
		return []models.StatementPage{page}, page.NextPageToken, err
	}) {
		if err != nil {
			return models.Statement{}, err
		}
		if first {
			out.CompleteThrough = page.CompleteThrough
			first = false
		} else if err := decode.SameCompleteThrough(out.CompleteThrough, page.CompleteThrough); err != nil {
			return models.Statement{}, err
		}
		csv.WriteString(page.CSV)
	}
	out.CSV = csv.String()
	return out, nil
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

// CreateStatementExportOptions asks for a whole-period statement file of one
// account.
type CreateStatementExportOptions struct {
	// Account and SubAccountID select the account as for Export.
	Account      AccountScope
	SubAccountID *string
	// StartTime (inclusive) and EndTime (exclusive) bound the period: EndTime
	// after StartTime and at most codecs.StatementExportMaxPeriod (366 days)
	// after it. Both are required. A period still open when the export is
	// created ends then; see models.StatementExport.CompleteThrough.
	StartTime time.Time
	EndTime   time.Time
	// AssetID keeps one asset; 0 covers every asset.
	AssetID uint32
	// IdempotencyKey is required: 1 to 64 characters of letters, digits and
	// ".", "_", ":" or "-", unique per root account. Repeating a create with
	// the same key returns the same export; the same key with other fields is
	// refused with ERROR_CODE_IDEMPOTENCY_KEY_REUSED.
	IdempotencyKey string
}

// ListStatementExportsOptions selects a page of one account's exports.
type ListStatementExportsOptions struct {
	// Account and SubAccountID select the account as for Export.
	Account      AccountScope
	SubAccountID *string
	// Limit is the page size; 0 uses the server's default of 20. At most
	// codecs.ListStatementExportsLimitMax.
	Limit int
	// PageToken continues from an earlier page's NextPageToken.
	PageToken string
}

func (s *StatementsService) exportsClient() ledgerrdv1connect.StatementExportServiceClient {
	return ledgerrdv1connect.NewStatementExportServiceClient(s.transport.HTTP, s.transport.Config.APIURL, s.transport.ConnectOptions(true)...)
}

// CreateExport asks for a whole-period statement produced in the background
// as one CSV file, or returns the export already created under the same
// idempotency key. Poll it with GetExport until its Status is "ready" or
// "failed", then download the file over REST (see models.StatementExport).
// A root account has at most 5 exports queued or processing; another create
// is refused with ERROR_CODE_STATEMENT_EXPORT_LIMIT_REACHED, which is not
// retryable until one of them finishes. An API key needs READ_STATEMENTS.
func (s *StatementsService) CreateExport(ctx context.Context, opts CreateStatementExportOptions) (models.StatementExport, error) {
	if opts.StartTime.IsZero() || opts.EndTime.IsZero() {
		return models.StatementExport{}, &errors.ValidationError{Msg: "statement export requires StartTime and EndTime"}
	}
	if !opts.EndTime.After(opts.StartTime) || opts.EndTime.Sub(opts.StartTime) > codecs.StatementExportMaxPeriod {
		return models.StatementExport{}, &errors.ValidationError{Msg: "statement export EndTime must be after StartTime and at most 366 days after it"}
	}
	req := &ledgerrdv1.CreateStatementExportRequest{
		StartTime:      timestamppb.New(opts.StartTime),
		EndTime:        timestamppb.New(opts.EndTime),
		AssetId:        opts.AssetID,
		IdempotencyKey: opts.IdempotencyKey,
	}
	if err := codecs.CheckContractString(req, "idempotency_key", opts.IdempotencyKey, "idempotency_key"); err != nil {
		return models.StatementExport{}, err
	}
	if err := s.scoped.ApplyOptionalSubaccountIDPtr(&req.SubaccountId, opts.Account, opts.SubAccountID); err != nil {
		return models.StatementExport{}, err
	}
	return UnaryAuthDecoded(ctx, s.transport, s.exportsClient().CreateStatementExport, req, decode.StatementExportFromCreate)
}

// GetExport reads one statement export by its public ID.
func (s *StatementsService) GetExport(ctx context.Context, exportID string) (models.StatementExport, error) {
	id, err := codecs.ParseAccountID(exportID, "export_id")
	if err != nil {
		return models.StatementExport{}, err
	}
	req := &ledgerrdv1.GetStatementExportRequest{ExportId: id}
	return UnaryAuthDecoded(ctx, s.transport, s.exportsClient().GetStatementExport, req, decode.StatementExportFromGet)
}

// ListExports reads one page of an account's statement exports, newest
// first.
func (s *StatementsService) ListExports(ctx context.Context, opts ListStatementExportsOptions) (models.StatementExportsList, error) {
	limit, err := PaginationLimit(opts.Limit, "limit")
	if err != nil {
		return models.StatementExportsList{}, err
	}
	if limit > codecs.ListStatementExportsLimitMax {
		return models.StatementExportsList{}, &errors.ValidationError{Msg: fmt.Sprintf("limit %d exceeds %d", limit, codecs.ListStatementExportsLimitMax)}
	}
	req := &ledgerrdv1.ListStatementExportsRequest{Limit: limit, PageToken: opts.PageToken}
	if err := s.scoped.ApplyOptionalSubaccountIDPtr(&req.SubaccountId, opts.Account, opts.SubAccountID); err != nil {
		return models.StatementExportsList{}, err
	}
	return UnaryAuthDecoded(ctx, s.transport, s.exportsClient().ListStatementExports, req, decode.StatementExportsListFromProto)
}

// Exports iterates an account's statement exports from opts.PageToken (or
// the newest), following each page's NextPageToken. It stops at the first
// error.
func (s *StatementsService) Exports(ctx context.Context, opts ListStatementExportsOptions) iter.Seq2[models.StatementExport, error] {
	return paginate(opts.PageToken, func(token string) ([]models.StatementExport, string, error) {
		page := opts
		page.PageToken = token
		list, err := s.ListExports(ctx, page)
		return list.Exports, list.NextPageToken, err
	})
}
