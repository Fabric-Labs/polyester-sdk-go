package decode

import (
	"testing"
	"time"

	ledgerrdv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/ledger/read/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// An export's type reads as its label, and a type newer than this SDK reads
// as unknown(n) instead of failing the read, since more types will come.
func TestStatementExportTypeReadsKnownAndUnknownTypes(t *testing.T) {
	for value, want := range map[ledgerrdv1.StatementExportType]string{
		ledgerrdv1.StatementExportType_ACCOUNT_STATEMENT:                 models.StatementExportTypeAccountStatement,
		ledgerrdv1.StatementExportType_LEDGER:                            models.StatementExportTypeLedger,
		ledgerrdv1.StatementExportType_BALANCES:                          models.StatementExportTypeBalances,
		ledgerrdv1.StatementExportType_STATEMENT_EXPORT_TYPE_UNSPECIFIED: "",
		ledgerrdv1.StatementExportType(7):                                "unknown(7)",
	} {
		export, err := StatementExportFromGet(&ledgerrdv1.GetStatementExportResponse{Export: &ledgerrdv1.StatementExport{ExportId: 77, Type: value}})
		if err != nil {
			t.Fatalf("type %d: %v", value, err)
		}
		if export.Type != want {
			t.Fatalf("type %d reads %q, want %q", value, export.Type, want)
		}
	}
}

// A balances export that failed because the account history moved later
// names where the history starts; another export leaves it nil.
func TestStatementExportReadsHistoryAvailableFrom(t *testing.T) {
	from := time.Date(2026, 11, 25, 14, 55, 2, 123456789, time.UTC)
	export, err := StatementExportFromGet(&ledgerrdv1.GetStatementExportResponse{Export: &ledgerrdv1.StatementExport{
		ExportId:             77,
		Type:                 ledgerrdv1.StatementExportType_BALANCES,
		Status:               ledgerrdv1.StatementExportStatus_STATEMENT_EXPORT_STATUS_FAILED,
		FailureReason:        ledgerrdv1.StatementExportFailureReason_STATEMENT_EXPORT_FAILURE_REASON_HISTORY_UNAVAILABLE,
		HistoryAvailableFrom: timestamppb.New(from),
	}})
	if err != nil {
		t.Fatal(err)
	}
	if export.FailureReason != "history_unavailable" || export.HistoryAvailableFrom == nil || !export.HistoryAvailableFrom.Equal(from) {
		t.Fatalf("export = %+v", export)
	}
	export, err = StatementExportFromGet(&ledgerrdv1.GetStatementExportResponse{Export: &ledgerrdv1.StatementExport{ExportId: 77}})
	if err != nil || export.HistoryAvailableFrom != nil {
		t.Fatalf("export = %+v, err = %v", export, err)
	}
}
