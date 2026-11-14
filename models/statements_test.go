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
	rows, err := Statement{CSV: "time,entry_id,type,account,asset,amount,fee,balance,link_id,flow_id\r\n"}.Rows()
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
