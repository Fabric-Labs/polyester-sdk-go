package codecs

import (
	"fmt"
	"testing"
	"time"

	chainlifecyclev1 "github.com/Fabric-Labs/polyester-sdk-go/gen/chain/lifecycle/v1"
	ledgerrdv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/ledger/read/v1"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// The page, batch and period limits are the published contract's: its
// validation rules, and for the statement export period the CEL rule rebuilt
// from the SDK's constant.
func TestContract1945423cLimitsAreTheContracts(t *testing.T) {
	if BatchInternalTransferLegsMin != 1 || BatchInternalTransferLegsMax != 10 {
		t.Fatalf("batch legs = %d..%d; want 1..10", BatchInternalTransferLegsMin, BatchInternalTransferLegsMax)
	}
	if ListAccountFlowsLimitMax != 500 || ListDustConversionsLimitMax != 100 || ListStatementExportsLimitMax != 100 {
		t.Fatalf("page maximums = %d, %d, %d; want 500, 100, 100", ListAccountFlowsLimitMax, ListDustConversionsLimitMax, ListStatementExportsLimitMax)
	}
	for _, field := range []protoreflect.Name{"polyester_chain_ids", "zipped_asset_ids", "unified_asset_ids"} {
		if _, high := contractRepeatedItems(&chainlifecyclev1.ListAccountFlowsRequest{}, field); high != ListAccountFlowsFilterIDsMax || high != 100 {
			t.Fatalf("%s holds at most %d IDs; want the shared 100", field, high)
		}
	}
	want := fmt.Sprintf("this.end_time > this.start_time && this.end_time - this.start_time <= duration('%dh')", int(StatementExportMaxPeriod/time.Hour))
	period := messageCEL(&ledgerrdv1.CreateStatementExportRequest{})
	if len(period) != 1 || period[0] != [2]string{"create_statement_export.period_valid", want} {
		t.Fatalf("CreateStatementExportRequest CEL = %v, want %q", period, want)
	}
	if StatementExportMaxPeriod != 366*24*time.Hour {
		t.Fatalf("statement export period = %v; want 366 days", StatementExportMaxPeriod)
	}
}

func TestCheckContractStringReadsTheContractsRules(t *testing.T) {
	msg := &ledgerrdv1.CreateStatementExportRequest{}
	for value, ok := range map[string]bool{
		"statement-2026":    true,
		"a.b_c:d-e":         true,
		"":                  false,
		"with space":        false,
		"slash/not-allowed": false,
		"énergie":           false,
	} {
		err := CheckContractString(msg, "idempotency_key", value, "idempotency_key")
		if (err == nil) != ok {
			t.Fatalf("%q: err = %v, want ok=%v", value, err, ok)
		}
	}
}
