package decode

import (
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
	return models.StatementExportsList{
		Exports:              out,
		NextPageToken:        msg.GetNextPageToken(),
		InProgressExports:    msg.GetInProgressExports(),
		MaxInProgressExports: msg.GetMaxInProgressExports(),
	}, nil
}

// StatementExportFromCancel decodes a CancelStatementExport reply: the
// export, canceled. A reply without its export or export ID, or for another
// export than exportID, returns a *errors.ResponseContractError.
func StatementExportFromCancel(msg *ledgerrdv1.CancelStatementExportResponse, exportID uint64) (models.StatementExport, error) {
	out, err := statementExport("CancelStatementExport", msg.GetExport())
	if err != nil {
		return out, err
	}
	if msg.GetExport().GetExportId() != exportID {
		return models.StatementExport{}, &sdkerrors.ResponseContractError{Operation: "CancelStatementExport", Msg: "reply names another export"}
	}
	return out, nil
}

// StatementExportDownloadURLFromProto decodes a
// GetStatementExportDownloadUrl reply. A reply without its URL or expiry
// returns a *errors.ResponseContractError that never repeats the URL.
func StatementExportDownloadURLFromProto(msg *ledgerrdv1.GetStatementExportDownloadUrlResponse) (models.StatementExportDownloadURL, error) {
	const op = "GetStatementExportDownloadUrl"
	if msg.GetUrl() == "" {
		return models.StatementExportDownloadURL{}, &sdkerrors.ResponseContractError{Operation: op, Msg: "missing url"}
	}
	expires := timestampTime(msg.GetExpiresAt())
	if expires == nil {
		return models.StatementExportDownloadURL{}, &sdkerrors.ResponseContractError{Operation: op, Msg: "missing expires_at"}
	}
	return models.StatementExportDownloadURL{URL: models.NewSecret(msg.GetUrl()), ExpiresAt: *expires, Filename: msg.GetFilename()}, nil
}
