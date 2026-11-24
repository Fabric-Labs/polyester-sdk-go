package models

import (
	"encoding/csv"
	"io"
	"strings"
	"time"

	"github.com/Fabric-Labs/polyester-sdk-go/errors"
)

// StatementExport is a whole-period account statement produced in the
// background as one CSV file, as Statements.CreateExport, GetExport,
// CancelExport and ListExports return it. The file is the header row, then
// every row oldest first; ParseStatementCSV reads it.
//
// Download a "ready" file within 7 days with an authenticated REST
// GET /v1/statements/exports/{ExportID}/file (text/csv; it supports Range
// requests with If-Range), which has no ConnectRPC method, or from the
// credential-free URL Statements.GetExportDownloadURL returns. An "expired"
// file is gone: create a new export.
type StatementExport struct {
	// ExportID is the export's public ID.
	ExportID string `json:"export_id"`
	// SubaccountID is the public ID of the sub-account the statement covers;
	// empty for a root account.
	SubaccountID string `json:"subaccount_id,omitempty"`
	// StartTime (inclusive) and EndTime (exclusive) are the requested period.
	StartTime *time.Time `json:"start_time,omitempty"`
	EndTime   *time.Time `json:"end_time,omitempty"`
	// AssetID is the one asset the statement covers; 0 for every asset.
	AssetID        uint32 `json:"asset_id,omitempty"`
	IdempotencyKey string `json:"idempotency_key,omitempty"`
	// Status is "queued" (waiting for the account history to cover the
	// period and for the account's other exports), "processing", "ready"
	// (downloadable until ExpiresAt), "failed" (FailureReason says why),
	// "expired" or "canceled" (by Statements.CancelExport; its partial file
	// is deleted); "unknown(n)" for a status newer than this SDK. Every
	// status but "queued" and "processing" is final; see IsFinal.
	Status string `json:"status"`
	// CompleteThrough is the end of what the file covers, exclusive: the
	// earlier of EndTime and the time the export was created.
	CompleteThrough *time.Time `json:"complete_through,omitempty"`
	// Movements and Bytes count the ledger movements and bytes written so far;
	// the file's totals once ready.
	Movements uint64     `json:"movements"`
	Bytes     uint64     `json:"bytes"`
	CreatedAt *time.Time `json:"created_at,omitempty"`
	// ReadyAt is set once the file is ready, and ExpiresAt, 7 days later,
	// with it.
	ReadyAt   *time.Time `json:"ready_at,omitempty"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	// FailureReason is why a failed export failed: "too_large" (the period
	// holds more than 4,000,000 ledger movements of the account, counting
	// every asset's even when the export keeps one: split the period) or
	// "unavailable" (create a new export); empty otherwise, and "unknown(n)"
	// for a reason newer than this SDK.
	FailureReason string `json:"failure_reason,omitempty"`
}

// IsFinal reports whether the export's Status can no longer change: every
// status but "queued" and "processing", including one newer than this SDK.
func (e StatementExport) IsFinal() bool {
	return e.Status != "queued" && e.Status != "processing"
}

// StatementExportsList is one page of an account's exports, newest first.
type StatementExportsList struct {
	Exports       []StatementExport `json:"exports"`
	NextPageToken string            `json:"next_page_token,omitempty"`
	// InProgressExports counts the root account's exports "queued" or
	// "processing" across all its sub-accounts, whichever account the list
	// selects; MaxInProgressExports is the most it may have at a time. While
	// they are equal, Statements.CreateExport is refused with
	// ERROR_CODE_STATEMENT_EXPORT_LIMIT_REACHED.
	InProgressExports    uint32 `json:"in_progress_exports"`
	MaxInProgressExports uint32 `json:"max_in_progress_exports"`
}

// StatementExportDownloadURL is a credential-free URL of a "ready" statement
// file, as Statements.GetExportDownloadURL returns it.
type StatementExportDownloadURL struct {
	// URL downloads the file without credentials until ExpiresAt. Anyone who
	// holds it can download the file until then, so it is a Secret: printing
	// or encoding it writes [REDACTED]. Call URL.Reveal to hand it to a
	// browser, and never log, store or share it.
	URL Secret `json:"url"`
	// ExpiresAt is when URL stops working: 60 seconds after it was signed, or
	// the file's own expiry if sooner.
	ExpiresAt time.Time `json:"expires_at"`
	// Filename is the name the download saves the file as,
	// statement-<ExportID>.csv.
	Filename string `json:"filename"`
}

// StatementRow is the effect of one ledger entry on one account bucket in
// one asset. Amounts are exact decimal strings in whole units of the asset,
// with up to 18 fractional digits; never parse them as floats.
type StatementRow struct {
	// Time of the entry, in UTC with microseconds.
	Time time.Time `json:"time"`
	// EntryID identifies the entry, an unsigned 128-bit decimal: the
	// TransferID of the entry's first transfer leg on the account within the
	// export's period, as Transfers.List returns it. It is the same whichever
	// asset the export keeps. A period boundary can fall between the legs of
	// one entry: each export then holds its own part of it, under the
	// TransferID of that part's first leg, so do not join rows of different
	// periods on EntryID alone.
	EntryID string `json:"entry_id"`
	// Type is "trade" for a trade's settlement and its fees; otherwise the
	// entry's transfer reason, such as "deposit", "withdraw",
	// "internal_transfer", "funding_to_trading" or "system_adjustment".
	Type string `json:"type"`
	// Account is the account bucket: "funding" or "trading".
	Account string `json:"account"`
	// Asset is the asset symbol.
	Asset string `json:"asset"`
	// Amount is the signed balance change of the entry, excluding Fee. Fees
	// the account receives are included here.
	Amount string `json:"amount"`
	// Fee is the fee the account paid in the asset for the entry; "0" when
	// none.
	Fee string `json:"fee"`
	// Balance is the bucket's balance in the asset after the entry: the
	// previous row's Balance of the same bucket and asset, plus Amount, minus
	// Fee.
	Balance string `json:"balance"`
	// LinkID correlates related transfer legs (a trade's match ID); empty when
	// unavailable.
	LinkID string `json:"link_id,omitempty"`
	// FlowID is the public lifecycle flow ID of a deposit, withdrawal or
	// internal transfer, as Transfers.List returns it; empty for entries
	// outside a chain lifecycle flow, such as trades, and for a statement
	// exported before the column existed.
	FlowID string `json:"flow_id,omitempty"`
}

// statementColumns are the columns every statement has; flow_id is optional.
var statementColumns = []string{"time", "entry_id", "type", "account", "asset", "amount", "fee", "balance", "link_id"}

// ParseStatementCSV parses an account statement, header row first, reading
// columns by header name: columns a later release appends are ignored, and a
// statement without flow_id reads an empty FlowID. It returns an
// *errors.ResponseContractError when the header lacks a required column or a
// row is not valid CSV or has a malformed time. An empty statement (header
// only) returns no rows.
func ParseStatementCSV(text string) ([]StatementRow, error) {
	reader := csv.NewReader(strings.NewReader(text))
	header, err := reader.Read()
	if err == io.EOF {
		return nil, statementError("statement has no header row")
	}
	if err != nil {
		return nil, statementError("statement header: " + err.Error())
	}
	index := make(map[string]int, len(header))
	for i, name := range header {
		if _, dup := index[name]; !dup {
			index[name] = i
		}
	}
	for _, name := range statementColumns {
		if _, ok := index[name]; !ok {
			return nil, statementError("statement header lacks the " + name + " column")
		}
	}
	flow, hasFlow := index["flow_id"]
	var rows []StatementRow
	for {
		record, err := reader.Read()
		if err == io.EOF {
			return rows, nil
		}
		if err != nil {
			return nil, statementError("statement row: " + err.Error())
		}
		at, err := time.Parse(time.RFC3339Nano, record[index["time"]])
		if err != nil {
			return nil, statementError("statement row time " + record[index["time"]] + " is not RFC 3339")
		}
		row := StatementRow{
			Time:    at.UTC(),
			EntryID: record[index["entry_id"]],
			Type:    record[index["type"]],
			Account: record[index["account"]],
			Asset:   record[index["asset"]],
			Amount:  record[index["amount"]],
			Fee:     record[index["fee"]],
			Balance: record[index["balance"]],
			LinkID:  record[index["link_id"]],
		}
		if hasFlow {
			row.FlowID = record[flow]
		}
		rows = append(rows, row)
	}
}

func statementError(msg string) error {
	return &errors.ResponseContractError{Operation: "statement CSV", Msg: msg}
}
