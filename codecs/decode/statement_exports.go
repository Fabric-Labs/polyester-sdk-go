package decode

import (
	"fmt"
	"time"

	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	ledgerrdv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/ledger/read/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

func statementExport(op string, msg *ledgerrdv1.StatementExport) (models.StatementExport, error) {
	if msg == nil {
		return models.StatementExport{}, &sdkerrors.ResponseContractError{Operation: op, Msg: "missing export"}
	}
	if err := requireID(op, "export_id", msg.GetExportId()); err != nil {
		return models.StatementExport{}, err
	}
	out := models.StatementExport{
		ExportID:        codecs.FormatID(msg.GetExportId()),
		StartTime:       timestampTime(msg.GetStartTime()),
		EndTime:         timestampTime(msg.GetEndTime()),
		AssetID:         msg.GetAssetId(),
		IdempotencyKey:  msg.GetIdempotencyKey(),
		Status:          enumLabel(msg.GetStatus(), "STATEMENT_EXPORT_STATUS_"),
		CompleteThrough: timestampTime(msg.GetCompleteThrough()),
		Movements:       msg.GetMovements(),
		Bytes:           msg.GetBytes(),
		CreatedAt:       timestampTime(msg.GetCreatedAt()),
		ReadyAt:         timestampTime(msg.GetReadyAt()),
		ExpiresAt:       timestampTime(msg.GetExpiresAt()),
		FailureReason:   enumLabel(msg.GetFailureReason(), "STATEMENT_EXPORT_FAILURE_REASON_"),
	}
	if msg.SubaccountId != nil {
		if err := requireID(op, "subaccount_id", msg.GetSubaccountId()); err != nil {
			return models.StatementExport{}, err
		}
		out.SubaccountID = codecs.FormatID(msg.GetSubaccountId())
	}
	return out, nil
}

// StatementExportFromCreate decodes a CreateStatementExport reply. A reply
// without its export or export ID returns a *errors.ResponseContractError:
// the export may have been created, so repeat the request with the same
// idempotency key to read it.
func StatementExportFromCreate(msg *ledgerrdv1.CreateStatementExportResponse) (models.StatementExport, error) {
	return statementExport("CreateStatementExport", msg.GetExport())
}

// StatementExportFromGet decodes a GetStatementExport reply. A reply without
// its export or export ID returns a *errors.ResponseContractError.
func StatementExportFromGet(msg *ledgerrdv1.GetStatementExportResponse) (models.StatementExport, error) {
	return statementExport("GetStatementExport", msg.GetExport())
}

// StatementExportsListFromProto decodes one page of exports. An export
// without its ID returns a *errors.ResponseContractError.
func StatementExportsListFromProto(msg *ledgerrdv1.ListStatementExportsResponse) (models.StatementExportsList, error) {
	out := make([]models.StatementExport, 0, len(msg.GetExports()))
	for _, e := range msg.GetExports() {
		export, err := statementExport("ListStatementExports", e)
		if err != nil {
			return models.StatementExportsList{}, err
		}
		out = append(out, export)
	}
	return models.StatementExportsList{Exports: out, NextPageToken: msg.GetNextPageToken()}, nil
}

// StatementPageFromProto decodes one ExportStatement page.
func StatementPageFromProto(msg *ledgerrdv1.ExportStatementResponse) models.StatementPage {
	return models.StatementPage{CSV: msg.GetCsv(), NextPageToken: msg.GetNextPageToken(), CompleteThrough: timestampTime(msg.GetCompleteThrough())}
}

// SameCompleteThrough reports a *errors.ResponseContractError when a later
// page of one statement names a different complete_through than its first:
// the contract gives every page of one statement the same value.
func SameCompleteThrough(first, page *time.Time) error {
	if (first == nil) != (page == nil) || (first != nil && !first.Equal(*page)) {
		return &sdkerrors.ResponseContractError{Operation: "ExportStatement", Msg: fmt.Sprintf("complete_through changed between pages: %v then %v", first, page)}
	}
	return nil
}
