package services

import (
	"context"
	"iter"
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

// StatementsService exports account statements: an account's balance
// movements for a period, with fees and the balance after each entry, as one
// CSV file produced in the background.
// An API key needs READ_STATEMENTS on its policy (READ_BALANCES and
// READ_INTERNAL_TRANSFERS do not grant it).
type StatementsService struct {
	transport *transport.Factory
	scoped    ScopedSubAccount
}

func NewStatementsService(factory *transport.Factory, defaultSubAccountID *string) *StatementsService {
	return &StatementsService{transport: factory, scoped: ScopedSubAccount{DefaultSubAccountID: defaultSubAccountID}}
}

// CreateStatementExportOptions asks for a whole-period statement file of one
// account.
type CreateStatementExportOptions struct {
	// Account and SubAccountID select the account as for Transfers.List:
	// pass one or neither. With neither, the client's DefaultSubAccountID is
	// used, and without one the statement covers the account the credentials
	// authenticate (a sub-account-scoped key's own sub-account).
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
	// Account and SubAccountID select the account as for CreateExport.
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
	return UnaryAuthDecoded(ctx, s.transport, s.exportsClient().GetStatementExport, req, echoed("GetStatementExport", "export_id", codecs.FormatID(id), func(msg *ledgerrdv1.GetStatementExportResponse) (string, bool) {
		return codecs.FormatID(msg.GetExport().GetExportId()), msg.GetExport() != nil
	}, decode.StatementExportFromGet))
}

// CancelExport stops a "queued" or "processing" export: it becomes
// "canceled" at once, no longer counts toward the root account's exports in
// progress, and its partial file is deleted. Canceling a "canceled" export
// again returns it unchanged; a "ready", "failed" or "expired" one is refused
// with ERROR_CODE_STATEMENT_EXPORT_NOT_CANCELABLE. An API key needs
// READ_STATEMENTS.
func (s *StatementsService) CancelExport(ctx context.Context, exportID string) (models.StatementExport, error) {
	id, err := codecs.ParseAccountID(exportID, "export_id")
	if err != nil {
		return models.StatementExport{}, err
	}
	req := &ledgerrdv1.CancelStatementExportRequest{ExportId: id}
	return UnaryAuthDecoded(ctx, s.transport, s.exportsClient().CancelStatementExport, req, func(msg *ledgerrdv1.CancelStatementExportResponse) (models.StatementExport, error) {
		return decode.StatementExportFromCancel(msg, id)
	})
}

// GetExportDownloadURL returns a URL of a "ready" export's file that needs no
// credentials, valid for 60 seconds from signing (or until the file expires,
// if sooner), so a browser can save the file straight to disk. Ask for it
// right before downloading; each call returns a new URL. The URL is a
// models.Secret: never log, store or share it. Before the file is ready the
// call is refused with ERROR_CODE_STATEMENT_EXPORT_NOT_READY, after a
// failure with ERROR_CODE_STATEMENT_EXPORT_FAILED, for a canceled export with
// ERROR_CODE_STATEMENT_EXPORT_CANCELED, and once the file has expired (or has
// less than 5 seconds left) with ERROR_CODE_STATEMENT_EXPORT_EXPIRED. An API
// key needs READ_STATEMENTS.
func (s *StatementsService) GetExportDownloadURL(ctx context.Context, exportID string) (models.StatementExportDownloadURL, error) {
	id, err := codecs.ParseAccountID(exportID, "export_id")
	if err != nil {
		return models.StatementExportDownloadURL{}, err
	}
	req := &ledgerrdv1.GetStatementExportDownloadUrlRequest{ExportId: id}
	return UnaryAuthDecoded(ctx, s.transport, s.exportsClient().GetStatementExportDownloadUrl, req, decode.StatementExportDownloadURLFromProto)
}

// ListExports reads one page of an account's statement exports, newest
// first, with the root account's exports in progress and its limit.
func (s *StatementsService) ListExports(ctx context.Context, opts ListStatementExportsOptions) (models.StatementExportsList, error) {
	limit, err := PaginationLimit(opts.Limit, "limit")
	if err != nil {
		return models.StatementExportsList{}, err
	}
	req := &ledgerrdv1.ListStatementExportsRequest{Limit: limit, PageToken: opts.PageToken}
	if err := codecs.CheckPageLimit(req); err != nil {
		return models.StatementExportsList{}, err
	}
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
