package models

import (
	"encoding/csv"
	"io"
	"strings"
	"time"

	"github.com/Fabric-Labs/polyester-sdk-go/errors"
)

// StatementPage is one page of an account statement, as
// Statements.ExportPage returns it.
type StatementPage struct {
	// CSV holds the page's rows as RFC 4180 CSV in UTF-8 with CRLF line
	// endings, kept as sent. Only the first page starts with the header row,
	// so the pages concatenated in order are the whole statement. A period
	// without movements is the header row alone.
	CSV string `json:"csv"`
	// NextPageToken continues the statement; empty on the last page. Reuse it
	// only with the same request fields.
	NextPageToken string `json:"next_page_token,omitempty"`
}

// Rows parses the page by header name; see ParseStatementCSV. Only the first
// page carries the header, so Rows of a later page fails: parse a Statement
// instead.
func (p StatementPage) Rows() ([]StatementRow, error) {
	return ParseStatementCSV(p.CSV)
}

// Statement is a whole account statement: every page's CSV concatenated in
// order, as Statements.Export returns it.
type Statement struct {
	// CSV is the complete statement: the header row, then every row, oldest
	// first.
	CSV string `json:"csv"`
}

// Rows parses the statement by header name; see ParseStatementCSV.
func (s Statement) Rows() ([]StatementRow, error) {
	return ParseStatementCSV(s.CSV)
}

// StatementRow is the effect of one ledger entry on one account bucket in
// one asset. Amounts are exact decimal strings in whole units of the asset,
// with up to 18 fractional digits; never parse them as floats.
type StatementRow struct {
	// Time of the entry, in UTC with microseconds.
	Time time.Time `json:"time"`
	// EntryID is the entry's stable identifier, an unsigned 128-bit decimal:
	// the TransferID of the entry's first transfer leg on the account, as
	// Transfers.List returns it.
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
	return &errors.ResponseContractError{Operation: "ledger.read.v1.LedgerReadService/ExportStatement", Msg: msg}
}
