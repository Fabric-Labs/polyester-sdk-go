package models

import (
	stderrors "errors"
	"testing"
	"time"

	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
)

func TestParseStatementCSVReadsColumnsByHeaderName(t *testing.T) {
	// Reordered columns, a column a later release appends, a quoted field
	// with a comma and a doubled quote, CRLF line endings.
	text := "flow_id,balance,time,asset,entry_id,type,account,amount,fee,link_id,future_column\r\n" +
		"flow_1,100,2026-10-02T14:03:11.000001Z,USDT,27419385013475392777,deposit,funding,100,0,,x\r\n" +
		",\"1,1\",2026-10-03T08:12:45.123456Z,\"B\"\"TC\",27419385013475392811,trade,trading,0.1,0,77,y\r\n"
	rows, err := ParseStatementCSV(text)
	if err != nil {
		t.Fatal(err)
	}
	want := []StatementRow{
		{Time: time.Date(2026, 10, 2, 14, 3, 11, 1000, time.UTC), EntryID: "27419385013475392777", Type: "deposit", Account: "funding", Asset: "USDT", Amount: "100", Fee: "0", Balance: "100", FlowID: "flow_1"},
		{Time: time.Date(2026, 10, 3, 8, 12, 45, 123456000, time.UTC), EntryID: "27419385013475392811", Type: "trade", Account: "trading", Asset: "B\"TC", Amount: "0.1", Fee: "0", Balance: "1,1", LinkID: "77"},
	}
	if len(rows) != len(want) {
		t.Fatalf("rows = %+v", rows)
	}
	for i := range want {
		if rows[i] != want[i] {
			t.Fatalf("row %d = %+v, want %+v", i, rows[i], want[i])
		}
	}
}

func TestParseStatementCSVWithoutFlowIDReadsAnEmptyFlowID(t *testing.T) {
	rows, err := ParseStatementCSV("time,entry_id,type,account,asset,amount,fee,balance,link_id\r\n" +
		"2026-10-03T08:12:45.123456Z,1,trade,trading,BTC,0.1,0,1.1,77\r\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].FlowID != "" || rows[0].Balance != "1.1" {
		t.Fatalf("rows = %+v", rows)
	}
}

func TestParseStatementCSVHeaderOnlyHasNoRows(t *testing.T) {
	rows, err := ParseStatementCSV("time,entry_id,type,account,asset,amount,fee,balance,link_id,flow_id\r\n")
	if err != nil || len(rows) != 0 {
		t.Fatalf("rows=%+v err=%v", rows, err)
	}
}

func TestParseStatementCSVRefusesMalformedStatements(t *testing.T) {
	for name, text := range map[string]string{
		"empty":          "",
		"missing column": "time,entry_id,type,account,asset,amount,fee,link_id\r\n",
		"short row":      "time,entry_id,type,account,asset,amount,fee,balance,link_id\r\n2026-10-03T08:12:45Z,1,trade\r\n",
		"bad time":       "time,entry_id,type,account,asset,amount,fee,balance,link_id\r\nyesterday,1,trade,trading,BTC,0.1,0,1.1,77\r\n",
		"later page":     "2026-10-03T08:12:45.123456Z,1,trade,trading,BTC,0.1,0,1.1,77,\r\n",
	} {
		_, err := ParseStatementCSV(text)
		var contract *sdkerrors.ResponseContractError
		if !stderrors.As(err, &contract) {
			t.Fatalf("%s: error = %v, want *errors.ResponseContractError", name, err)
		}
	}
}

const ledgerHeader = "time,transfer_id,account,asset,type,side,amount,counterparty,balance,link_id,flow_id\r\n"

func TestParseLedgerCSVReadsColumnsByHeaderName(t *testing.T) {
	// A trade's quote debit and its taker fee, then a deposit credit, with
	// reordered columns, a column a later release appends and CRLF endings.
	text := "side,flow_id,time,transfer_id,account,asset,type,amount,counterparty,balance,link_id,future_column\r\n" +
		"debit,,2026-10-03T08:12:45.123456Z,27419385013475392811,trading,USDT,trade_quote,-100.5,trading_account,899.5,77,x\r\n" +
		"debit,,2026-10-03T08:12:45.123456Z,27419385013475392812,trading,USDT,taker_fee,-0.1,fee_account,899.4,77,y\r\n" +
		"credit,flow_1,2026-10-04T00:00:00.000001Z,27419385013475392900,funding,BTC,deposit,0.25,,0.25,,z\r\n"
	rows, err := ParseLedgerCSV(text)
	if err != nil {
		t.Fatal(err)
	}
	trade := time.Date(2026, 10, 3, 8, 12, 45, 123456000, time.UTC)
	want := []LedgerRow{
		{Time: trade, TransferID: "27419385013475392811", Account: "trading", Asset: "USDT", Type: "trade_quote", Side: "debit", Amount: "-100.5", Counterparty: "trading_account", Balance: "899.5", LinkID: "77"},
		{Time: trade, TransferID: "27419385013475392812", Account: "trading", Asset: "USDT", Type: "taker_fee", Side: "debit", Amount: "-0.1", Counterparty: "fee_account", Balance: "899.4", LinkID: "77"},
		{Time: time.Date(2026, 10, 4, 0, 0, 0, 1000, time.UTC), TransferID: "27419385013475392900", Account: "funding", Asset: "BTC", Type: "deposit", Side: "credit", Amount: "0.25", Balance: "0.25", FlowID: "flow_1"},
	}
	if len(rows) != len(want) {
		t.Fatalf("rows = %+v", rows)
	}
	for i := range want {
		if rows[i] != want[i] {
			t.Fatalf("row %d = %+v, want %+v", i, rows[i], want[i])
		}
	}
}

func TestParseLedgerCSVHeaderOnlyHasNoRows(t *testing.T) {
	rows, err := ParseLedgerCSV(ledgerHeader)
	if err != nil || len(rows) != 0 {
		t.Fatalf("rows=%+v err=%v", rows, err)
	}
}

func TestParseLedgerCSVRefusesMalformedLedgers(t *testing.T) {
	for name, text := range map[string]string{
		"empty":             "",
		"missing column":    "time,transfer_id,account,asset,type,side,amount,counterparty,balance,link_id\r\n",
		"account statement": "time,entry_id,type,account,asset,amount,fee,balance,link_id,flow_id\r\n",
		"short row":         ledgerHeader + "2026-10-03T08:12:45Z,1,trading\r\n",
		"bad time":          ledgerHeader + "yesterday,1,trading,BTC,deposit,credit,1,,1,,\r\n",
	} {
		_, err := ParseLedgerCSV(text)
		var contract *sdkerrors.ResponseContractError
		if !stderrors.As(err, &contract) {
			t.Fatalf("%s: error = %v, want *errors.ResponseContractError", name, err)
		}
	}
}

func TestParseStatementCSVRefusesALedger(t *testing.T) {
	_, err := ParseStatementCSV(ledgerHeader)
	var contract *sdkerrors.ResponseContractError
	if !stderrors.As(err, &contract) {
		t.Fatalf("error = %v, want *errors.ResponseContractError", err)
	}
}

const balancesHeader = "account,asset,opening_balance,credits,debits,closing_balance,movements\r\n"

func TestParseBalancesCSVReadsColumnsByHeaderName(t *testing.T) {
	// Reordered columns, a column a later release appends, CRLF endings.
	text := "movements,asset,account,closing_balance,debits,credits,opening_balance,future_column\r\n" +
		"3,BTC,funding,0.75,0.25,1,0,x\r\n" +
		"12,USDT,trading,899.4,100.6,0,1000,y\r\n"
	rows, err := ParseBalancesCSV(text)
	if err != nil {
		t.Fatal(err)
	}
	want := []BalancesRow{
		{Account: "funding", Asset: "BTC", OpeningBalance: "0", Credits: "1", Debits: "0.25", ClosingBalance: "0.75", Movements: 3},
		{Account: "trading", Asset: "USDT", OpeningBalance: "1000", Credits: "0", Debits: "100.6", ClosingBalance: "899.4", Movements: 12},
	}
	if len(rows) != len(want) {
		t.Fatalf("rows = %+v", rows)
	}
	for i := range want {
		if rows[i] != want[i] {
			t.Fatalf("row %d = %+v, want %+v", i, rows[i], want[i])
		}
	}
}

func TestParseBalancesCSVHeaderOnlyHasNoRows(t *testing.T) {
	rows, err := ParseBalancesCSV(balancesHeader)
	if err != nil || len(rows) != 0 {
		t.Fatalf("rows=%+v err=%v", rows, err)
	}
}

func TestParseBalancesCSVRefusesMalformedFiles(t *testing.T) {
	for name, text := range map[string]string{
		"empty":           "",
		"missing column":  "account,asset,opening_balance,credits,debits,closing_balance\r\n",
		"ledger":          ledgerHeader,
		"short row":       balancesHeader + "funding,BTC\r\n",
		"bad movements":   balancesHeader + "funding,BTC,0,1,0,1,three\r\n",
		"signed movement": balancesHeader + "funding,BTC,0,1,0,1,-1\r\n",
	} {
		_, err := ParseBalancesCSV(text)
		var contract *sdkerrors.ResponseContractError
		if !stderrors.As(err, &contract) {
			t.Fatalf("%s: error = %v, want *errors.ResponseContractError", name, err)
		}
	}
}
