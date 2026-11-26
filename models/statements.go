package models

import (
	"encoding/csv"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/Fabric-Labs/polyester-sdk-go/errors"
)

// Statement export type labels: the kind of file an export produces.
// Statements.CreateExport takes one of them; a type newer than this SDK
// reads as "unknown(<n>)" on StatementExport.Type.
const (
	// StatementExportTypeAccountStatement is an account statement: one row
	// per entry, account bucket and asset, with the entry's net amount, the
	// fees paid and the balance after it. ParseStatementCSV reads it.
	StatementExportTypeAccountStatement = "account_statement"
	// StatementExportTypeLedger is a ledger: one row per ledger movement,
	// each transfer leg on its own, with its transfer reason, side,
	// counterparty and the balance after it. ParseLedgerCSV reads it.
	StatementExportTypeLedger = "ledger"
	// StatementExportTypeBalances is the account's balances over the
	// period: one row per account bucket and asset, with its balance at the
	// start and at the end of the period and the period's credits and
	// debits. ParseBalancesCSV reads it. Its period must start inside the
	// account history, or the create is refused with
	// errors.CodeStatementHistoryUnavailable.
	StatementExportTypeBalances = "balances"
)

// StatementExport is a whole-period account statement, ledger or balances
// file produced in the background as one CSV file, as Statements.CreateExport,
// GetExport, CancelExport and ListExports return it. The file is the header
// row, then its rows: oldest first for an account statement or a ledger, by
// account bucket and asset for balances. Type says which parser reads it:
// ParseStatementCSV for "account_statement", ParseLedgerCSV for "ledger",
// ParseBalancesCSV for "balances".
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
	// Type is the kind of file the export produces, as requested:
	// StatementExportTypeAccountStatement, StatementExportTypeLedger or
	// StatementExportTypeBalances, or
	// "unknown(n)" for a type newer than this SDK, which reads without
	// failing. Exports created before types existed are account statements.
	Type string `json:"type"`
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
	// "unavailable" (create a new export) or "history_unavailable" (a
	// balances export whose period starts before the account history, which
	// moved later after the export was created: create a new export starting
	// at or after HistoryAvailableFrom); empty otherwise, and "unknown(n)"
	// for a reason newer than this SDK.
	FailureReason string `json:"failure_reason,omitempty"`
	// HistoryAvailableFrom is the earliest instant whose balances the account
	// history holds, set only when FailureReason is "history_unavailable".
	HistoryAvailableFrom *time.Time `json:"history_available_from,omitempty"`
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
	// Filename is the name the download saves the file as, by the export's
	// type: account-statement-<ExportID>.csv, ledger-<ExportID>.csv or
	// balances-<ExportID>.csv.
	Filename string `json:"filename"`
}

// StatementRow is one row of an account statement file
// (StatementExportTypeAccountStatement): the effect of one ledger entry on one
// account bucket in one asset. Amounts are exact decimal strings in whole units of the asset,
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
	// internal transfer linked to the entry before the export was created;
	// empty for entries outside a chain lifecycle flow, such as trades, and
	// for a statement exported before the column existed. A flow linked
	// later, which Transfers.List then shows, is not in the file: a new
	// export shows it.
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

// LedgerRow is one row of a ledger file (StatementExportTypeLedger): one
// ledger movement, one transfer leg on the account, as Transfers.List
// returns it. A trade's settlement is several rows, such as its base and
// quote legs and each fee. Amounts are exact decimal strings in whole units
// of the asset, with up to 18 fractional digits; never parse them as floats.
type LedgerRow struct {
	// Time of the transfer, in UTC with microseconds.
	Time time.Time `json:"time"`
	// TransferID identifies the transfer, an unsigned 128-bit decimal, as
	// Transfers.List returns it. With Account and Side it identifies the row.
	TransferID string `json:"transfer_id"`
	// Account is the account bucket: "funding" or "trading".
	Account string `json:"account"`
	// Asset is the asset symbol.
	Asset string `json:"asset"`
	// Type is the transfer reason, such as "trade_base", "trade_quote",
	// "maker_fee", "taker_fee", "rebate", "deposit", "withdraw",
	// "internal_transfer" or "funding_to_trading".
	Type string `json:"type"`
	// Side is "debit" when the movement takes from the account bucket,
	// "credit" when it adds to it.
	Side string `json:"side"`
	// Amount is the signed balance change: negative for a debit, positive
	// for a credit.
	Amount string `json:"amount"`
	// Counterparty is the kind of the transfer's other side:
	// "funding_account", "trading_account", "external_address",
	// "private_counterparty", "fee_account" or "system_account"; empty when
	// unknown.
	Counterparty string `json:"counterparty,omitempty"`
	// Balance is the bucket's balance in the asset after the movement: the
	// previous row's Balance of the same bucket and asset, plus Amount.
	Balance string `json:"balance"`
	// LinkID correlates related transfer legs (a trade's match ID); empty when
	// unavailable.
	LinkID string `json:"link_id,omitempty"`
	// FlowID is the public lifecycle flow ID of a transfer linked to a chain
	// lifecycle flow before the export was created; empty otherwise, as for
	// StatementRow.FlowID.
	FlowID string `json:"flow_id,omitempty"`
}

// ledgerColumns are the columns every ledger file has.
var ledgerColumns = []string{"time", "transfer_id", "account", "asset", "type", "side", "amount", "counterparty", "balance", "link_id", "flow_id"}

// ParseLedgerCSV parses a ledger file, header row first, reading columns by
// header name: columns a later release appends are ignored. It returns an
// *errors.ResponseContractError when the header lacks a ledger column (an
// account statement lacks transfer_id and side) or a row is not valid CSV or
// has a malformed time. An empty ledger (header only) returns no rows.
func ParseLedgerCSV(text string) ([]LedgerRow, error) {
	reader := csv.NewReader(strings.NewReader(text))
	header, err := reader.Read()
	if err == io.EOF {
		return nil, ledgerError("ledger has no header row")
	}
	if err != nil {
		return nil, ledgerError("ledger header: " + err.Error())
	}
	index := make(map[string]int, len(header))
	for i, name := range header {
		if _, dup := index[name]; !dup {
			index[name] = i
		}
	}
	for _, name := range ledgerColumns {
		if _, ok := index[name]; !ok {
			return nil, ledgerError("ledger header lacks the " + name + " column")
		}
	}
	var rows []LedgerRow
	for {
		record, err := reader.Read()
		if err == io.EOF {
			return rows, nil
		}
		if err != nil {
			return nil, ledgerError("ledger row: " + err.Error())
		}
		at, err := time.Parse(time.RFC3339Nano, record[index["time"]])
		if err != nil {
			return nil, ledgerError("ledger row time " + record[index["time"]] + " is not RFC 3339")
		}
		rows = append(rows, LedgerRow{
			Time:         at.UTC(),
			TransferID:   record[index["transfer_id"]],
			Account:      record[index["account"]],
			Asset:        record[index["asset"]],
			Type:         record[index["type"]],
			Side:         record[index["side"]],
			Amount:       record[index["amount"]],
			Counterparty: record[index["counterparty"]],
			Balance:      record[index["balance"]],
			LinkID:       record[index["link_id"]],
			FlowID:       record[index["flow_id"]],
		})
	}
}

func ledgerError(msg string) error {
	return &errors.ResponseContractError{Operation: "ledger CSV", Msg: msg}
}

// BalancesRow is one row of a balances file (StatementExportTypeBalances):
// one account bucket in one asset over the export's period. Amounts are
// exact decimal strings in whole units of the asset, with up to 18
// fractional digits; never parse them as floats. Every row reconciles:
// ClosingBalance equals OpeningBalance plus Credits minus Debits.
type BalancesRow struct {
	// Account is the account bucket: "funding" or "trading".
	Account string `json:"account"`
	// Asset is the asset symbol.
	Asset string `json:"asset"`
	// OpeningBalance is the bucket's balance in the asset at the export's
	// StartTime.
	OpeningBalance string `json:"opening_balance"`
	// Credits is the total the period's movements added to the bucket, the
	// sum of the ledger file's credit rows of that bucket and asset; "0" when
	// none.
	Credits string `json:"credits"`
	// Debits is the total the period's movements took from the bucket, as a
	// positive amount: the sum of the ledger file's debit rows of that bucket
	// and asset; "0" when none.
	Debits string `json:"debits"`
	// ClosingBalance is the bucket's balance in the asset at the export's
	// CompleteThrough.
	ClosingBalance string `json:"closing_balance"`
	// Movements counts the period's ledger movements on the bucket in the
	// asset.
	Movements uint64 `json:"movements"`
}

// balancesColumns are the columns every balances file has.
var balancesColumns = []string{"account", "asset", "opening_balance", "credits", "debits", "closing_balance", "movements"}

// ParseBalancesCSV parses a balances file, header row first, reading columns
// by header name: columns a later release appends are ignored. Rows come
// sorted by account, then asset, and a bucket and asset with a zero balance
// at both ends and no movement has no row, so a header-only file returns no
// rows. It returns an *errors.ResponseContractError when the header lacks a
// balances column or a row is not valid CSV or has a malformed movements
// count.
func ParseBalancesCSV(text string) ([]BalancesRow, error) {
	reader := csv.NewReader(strings.NewReader(text))
	header, err := reader.Read()
	if err == io.EOF {
		return nil, balancesError("balances file has no header row")
	}
	if err != nil {
		return nil, balancesError("balances header: " + err.Error())
	}
	index := make(map[string]int, len(header))
	for i, name := range header {
		if _, dup := index[name]; !dup {
			index[name] = i
		}
	}
	for _, name := range balancesColumns {
		if _, ok := index[name]; !ok {
			return nil, balancesError("balances header lacks the " + name + " column")
		}
	}
	var rows []BalancesRow
	for {
		record, err := reader.Read()
		if err == io.EOF {
			return rows, nil
		}
		if err != nil {
			return nil, balancesError("balances row: " + err.Error())
		}
		movements, err := strconv.ParseUint(record[index["movements"]], 10, 64)
		if err != nil {
			return nil, balancesError("balances row movements " + record[index["movements"]] + " is not a count")
		}
		rows = append(rows, BalancesRow{
			Account:        record[index["account"]],
			Asset:          record[index["asset"]],
			OpeningBalance: record[index["opening_balance"]],
			Credits:        record[index["credits"]],
			Debits:         record[index["debits"]],
			ClosingBalance: record[index["closing_balance"]],
			Movements:      movements,
		})
	}
}

func balancesError(msg string) error {
	return &errors.ResponseContractError{Operation: "balances CSV", Msg: msg}
}
