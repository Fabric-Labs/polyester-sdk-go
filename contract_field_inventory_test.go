package polyester

import (
	"bufio"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"

	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"

	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	_ "github.com/Fabric-Labs/polyester-sdk-go/gen/auth/v1"
	_ "github.com/Fabric-Labs/polyester-sdk-go/gen/chain/lifecycle/v1"
	_ "github.com/Fabric-Labs/polyester-sdk-go/gen/ledger/read/v1"
	_ "github.com/Fabric-Labs/polyester-sdk-go/gen/marketdata/v1"
	_ "github.com/Fabric-Labs/polyester-sdk-go/gen/orderbook/v1"
	_ "github.com/Fabric-Labs/polyester-sdk-go/gen/orders/v1"
	_ "github.com/Fabric-Labs/polyester-sdk-go/gen/pnl/v1"
	_ "github.com/Fabric-Labs/polyester-sdk-go/gen/transfer/v1"
	_ "github.com/Fabric-Labs/polyester-sdk-go/gen/triggers/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
	"github.com/Fabric-Labs/polyester-sdk-go/services"
)

// contractFieldMessages are the request and reply messages public contracts
// 1945423c, 5c2119f and 0.93.5 added or changed, and the messages nested in their new
// replies, the lifecycle flow views every flow read and update decodes, and the
// balance read, profit and loss, trigger cancel-all, activity actor, item and
// details, index price candle and internal transfer batch abandon messages.
var contractFieldMessages = []protoreflect.FullName{
	"auth.v1.AuthErrorDetail",
	"chain.lifecycle.v1.FlowSummaryView", "chain.lifecycle.v1.FlowTxMatchView", "chain.lifecycle.v1.AssetIds",
	"chain.lifecycle.v1.RequestFee", "chain.lifecycle.v1.FlowSummaryProgressView", "chain.lifecycle.v1.FlowTimelineItemView",
	"auth.v1.ListAccountActivityRequest", "auth.v1.ListAccountActivityResponse", "auth.v1.ActivityEvent", "auth.v1.ActivityActor",
	"auth.v1.ActivityItem", "auth.v1.ActivityDetails", "auth.v1.ActivityTextChange",
	"auth.v1.ActivityBoolChange", "auth.v1.ActivityUintChange", "auth.v1.ActivityTimeChange",
	"auth.v1.ActivityIdChange", "auth.v1.ActivityActionsChange", "auth.v1.ActivityListChange",
	"auth.v1.ActivityIdListChange", "auth.v1.ActivityApiKeyStatusChange", "auth.v1.ActivityApiKeyPurposeChange",
	"auth.v1.ActivitySubaccountStatusChange", "auth.v1.ActivitySubaccountRoleChange",
	"auth.v1.ActivityBrokerRoleChange", "auth.v1.ActivityInviteStatusChange", "auth.v1.ActivityAccountDetails",
	"auth.v1.ActivitySessionDetails", "auth.v1.ActivityApiKeyDetails", "auth.v1.ActivityPolicyDetails",
	"auth.v1.ActivitySubaccountDetails", "auth.v1.ActivityMemberDetails", "auth.v1.ActivityInviteDetails",
	"auth.v1.ActivityDestinationDetails", "auth.v1.ActivityBrokerAllowlistDetails",
	"auth.v1.ActivitySecurityDetails",
	"marketdata.v1.GetIndexPriceCandlesRequest", "marketdata.v1.GetIndexPriceCandlesResponse",
	"chain.lifecycle.v1.GetAccountFlowRequest", "chain.lifecycle.v1.GetAccountFlowResponse",
	"chain.lifecycle.v1.ListAccountFlowsRequest", "chain.lifecycle.v1.ListAccountFlowsResponse",
	"ledger.read.v1.BalanceSeries", "ledger.read.v1.GetBalanceHistoryResponse",
	"ledger.read.v1.CreateStatementExportRequest", "ledger.read.v1.CreateStatementExportResponse",
	"ledger.read.v1.GetStatementExportRequest", "ledger.read.v1.GetStatementExportResponse",
	"ledger.read.v1.ListStatementExportsRequest", "ledger.read.v1.ListStatementExportsResponse", "ledger.read.v1.StatementExport",
	"ledger.read.v1.CancelStatementExportRequest", "ledger.read.v1.CancelStatementExportResponse",
	"ledger.read.v1.GetStatementExportDownloadUrlRequest", "ledger.read.v1.GetStatementExportDownloadUrlResponse",
	"orderbook.v1.GetOrderBookRequest", "orderbook.v1.GetOrderBookResponse",
	"orders.v1.ErrorDetail",
	"transfer.v1.BatchCreateInternalTransfersRequest", "transfer.v1.InternalTransferBatchLeg",
	"transfer.v1.BatchCreateInternalTransfersResponse", "transfer.v1.InternalTransferBatchLegResult", "transfer.v1.ResolvedDestination",
	"transfer.v1.GetInternalTransferBatchRequest", "transfer.v1.GetInternalTransferBatchResponse",
	"transfer.v1.AbandonInternalTransferBatchRequest", "transfer.v1.AbandonInternalTransferBatchResponse",
	"transfer.v1.ListDustBalancesRequest", "transfer.v1.ListDustBalancesResponse",
	"transfer.v1.QuoteDustConversionRequest", "transfer.v1.QuoteDustConversionResponse",
	"transfer.v1.CreateDustConversionRequest", "transfer.v1.CreateDustConversionResponse",
	"transfer.v1.GetDustConversionRequest", "transfer.v1.GetDustConversionResponse",
	"transfer.v1.ListDustConversionsRequest", "transfer.v1.ListDustConversionsResponse",
	"transfer.v1.DustBalance", "transfer.v1.DustAssetPrice", "transfer.v1.DustSource", "transfer.v1.DustConversion",
	"ledger.read.v1.GetBalancesResponse", "ledger.read.v1.AssetBalance", "ledger.read.v1.TransferRow",
	"pnl.v1.GetProfitAndLossRequest", "pnl.v1.GetProfitAndLossResponse",
	"pnl.v1.ProductProfitAndLoss", "pnl.v1.MarketProfitAndLoss", "pnl.v1.HoldingProfitAndLoss",
	"pnl.v1.ListDailyProfitAndLossRequest", "pnl.v1.ListDailyProfitAndLossResponse", "pnl.v1.DailyProfitAndLoss",
	"pnl.v1.GetTradingStatisticsRequest", "pnl.v1.GetTradingStatisticsResponse",
	"triggers.v1.CancelAllTriggersRequest", "triggers.v1.CancelAllTriggersResponse",
}

// contractFieldTypes resolves the SDK types an inventory line names, so a
// covered field must name a field that exists.
var contractFieldTypes = map[string]reflect.Type{
	"models.BalanceHistory":                 reflect.TypeFor[models.BalanceHistory](),
	"models.BalanceHistorySeries":           reflect.TypeFor[models.BalanceHistorySeries](),
	"models.DustAssetPrice":                 reflect.TypeFor[models.DustAssetPrice](),
	"models.DustBalance":                    reflect.TypeFor[models.DustBalance](),
	"models.DustBalances":                   reflect.TypeFor[models.DustBalances](),
	"models.DustConversion":                 reflect.TypeFor[models.DustConversion](),
	"models.DustConversionsList":            reflect.TypeFor[models.DustConversionsList](),
	"models.DustQuote":                      reflect.TypeFor[models.DustQuote](),
	"models.DustSource":                     reflect.TypeFor[models.DustSource](),
	"models.InternalTransferBatchLeg":       reflect.TypeFor[models.InternalTransferBatchLeg](),
	"models.OrderbookData":                  reflect.TypeFor[models.OrderbookData](),
	"models.InternalTransferBatchLegResult": reflect.TypeFor[models.InternalTransferBatchLegResult](),
	"models.InternalTransferBatchResult":    reflect.TypeFor[models.InternalTransferBatchResult](),
	"models.LifecycleFlowsList":             reflect.TypeFor[models.LifecycleFlowsList](),
	"models.LifecycleFlowSummary":           reflect.TypeFor[models.LifecycleFlowSummary](),
	"models.LifecycleRequestFee":            reflect.TypeFor[models.LifecycleRequestFee](),
	"models.LifecycleFlowProgress":          reflect.TypeFor[models.LifecycleFlowProgress](),
	"models.LifecycleFlowTimelineItem":      reflect.TypeFor[models.LifecycleFlowTimelineItem](),
	"models.ResolvedDestination":            reflect.TypeFor[models.ResolvedDestination](),
	"models.StatementExport":                reflect.TypeFor[models.StatementExport](),
	"models.StatementExportsList":           reflect.TypeFor[models.StatementExportsList](),
	"models.StatementExportDownloadURL":     reflect.TypeFor[models.StatementExportDownloadURL](),
	"errors.APIError":                       reflect.TypeFor[sdkerrors.APIError](),
	"errors.ValidationError":                reflect.TypeFor[sdkerrors.ValidationError](),
	"errors.RateLimitError":                 reflect.TypeFor[sdkerrors.RateLimitError](),
	"errors.TimestampSkewError":             reflect.TypeFor[sdkerrors.TimestampSkewError](),
	"models.OrderErrorDetail":               reflect.TypeFor[models.OrderErrorDetail](),
	"models.SubAccountActivityEvent":        reflect.TypeFor[models.SubAccountActivityEvent](),
	"models.SubAccountActivityActor":        reflect.TypeFor[models.SubAccountActivityActor](),
	"models.ActivityItem":                   reflect.TypeFor[models.ActivityItem](),
	"models.ActivityDetails":                reflect.TypeFor[models.ActivityDetails](),
	"models.ActivityAccountDetails":         reflect.TypeFor[models.ActivityAccountDetails](),
	"models.ActivitySessionDetails":         reflect.TypeFor[models.ActivitySessionDetails](),
	"models.ActivityAPIKeyDetails":          reflect.TypeFor[models.ActivityAPIKeyDetails](),
	"models.ActivityPolicyDetails":          reflect.TypeFor[models.ActivityPolicyDetails](),
	"models.ActivitySubaccountDetails":      reflect.TypeFor[models.ActivitySubaccountDetails](),
	"models.ActivityMemberDetails":          reflect.TypeFor[models.ActivityMemberDetails](),
	"models.ActivityInviteDetails":          reflect.TypeFor[models.ActivityInviteDetails](),
	"models.ActivityDestinationDetails":     reflect.TypeFor[models.ActivityDestinationDetails](),
	"models.ActivityBrokerAllowlistDetails": reflect.TypeFor[models.ActivityBrokerAllowlistDetails](),
	"models.ActivitySecurityDetails":        reflect.TypeFor[models.ActivitySecurityDetails](),
	"models.ActivityChange":                 reflect.TypeFor[models.ActivityChange[string]](),
	"models.ActivityListChange":             reflect.TypeFor[models.ActivityListChange[string]](),
	"models.IndexPriceCandlesResult":        reflect.TypeFor[models.IndexPriceCandlesResult](),
	"models.IndexPriceCandle":               reflect.TypeFor[models.IndexPriceCandle](),
	"models.SubAccountActivityList":         reflect.TypeFor[models.SubAccountActivityList](),
	"models.ListAccountActivityOptions":     reflect.TypeFor[models.ListAccountActivityOptions](),
	"errors.LadderLevelCause":               reflect.TypeFor[sdkerrors.LadderLevelCause](),
	"ListAccountFlowsParams":                reflect.TypeFor[services.ListAccountFlowsParams](),
	"CreateStatementExportOptions":          reflect.TypeFor[services.CreateStatementExportOptions](),
	"ListStatementExportsOptions":           reflect.TypeFor[services.ListStatementExportsOptions](),
	"ListDustConversionsOptions":            reflect.TypeFor[services.ListDustConversionsOptions](),
	"models.BalancesList":                   reflect.TypeFor[models.BalancesList](),
	"models.AssetBalance":                   reflect.TypeFor[models.AssetBalance](),
	"models.LedgerTransfer":                 reflect.TypeFor[models.LedgerTransfer](),
	"models.ProfitAndLoss":                  reflect.TypeFor[models.ProfitAndLoss](),
	"models.ProductProfitAndLoss":           reflect.TypeFor[models.ProductProfitAndLoss](),
	"models.MarketProfitAndLoss":            reflect.TypeFor[models.MarketProfitAndLoss](),
	"models.HoldingProfitAndLoss":           reflect.TypeFor[models.HoldingProfitAndLoss](),
	"models.DailyProfitAndLoss":             reflect.TypeFor[models.DailyProfitAndLoss](),
	"models.DailyProfitAndLossList":         reflect.TypeFor[models.DailyProfitAndLossList](),
	"models.TradingStatistics":              reflect.TypeFor[models.TradingStatistics](),
	"models.CancelAllTriggersResult":        reflect.TypeFor[models.CancelAllTriggersResult](),
	"ProfitAndLossParams":                   reflect.TypeFor[services.ProfitAndLossParams](),
	"ListDailyProfitAndLossParams":          reflect.TypeFor[services.ListDailyProfitAndLossParams](),
	"CancelAllTriggersParams":               reflect.TypeFor[services.CancelAllTriggersParams](),
}

// Every field of the contract's new request and reply messages is listed in
// testdata/contract_field_inventory.tsv as covered (naming the SDK parameter
// or model field that carries it) or omitted with a reason. A field the
// contract adds, or a covered name the SDK no longer has, fails here.
func TestContractFieldInventoryCoversEveryNewField(t *testing.T) {
	f, err := os.Open("testdata/contract_field_inventory.tsv")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	listed := map[string]bool{}
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) != 3 || (parts[1] != "covered" && parts[1] != "omitted") || parts[2] == "" {
			t.Fatalf("malformed inventory line %q", line)
		}
		listed[parts[0]] = true
		if parts[1] == "covered" {
			checkCoveredName(t, parts[0], parts[2])
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	var unlisted, gone []string
	present := map[string]bool{}
	for _, name := range contractFieldMessages {
		desc, err := protoregistry.GlobalFiles.FindDescriptorByName(name)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		fields := desc.(protoreflect.MessageDescriptor).Fields()
		for i := range fields.Len() {
			key := string(name) + "." + string(fields.Get(i).Name())
			present[key] = true
			if !listed[key] {
				unlisted = append(unlisted, key)
			}
		}
	}
	for key := range listed {
		if !present[key] {
			gone = append(gone, key)
		}
	}
	sort.Strings(unlisted)
	sort.Strings(gone)
	if len(unlisted) > 0 || len(gone) > 0 {
		t.Fatalf("contract field inventory out of date:\nunlisted: %v\nlisted but not in the contract: %v", unlisted, gone)
	}
}

// checkCoveredName checks each comma-separated "Type.Field" (or a bare
// "Field" of the previous type) against the SDK types it knows. Other forms,
// such as a method's parameter or a whole reply model, are taken as written.
func checkCoveredName(t *testing.T, key, name string) {
	t.Helper()
	var typ reflect.Type
	var typeName string
	for _, part := range strings.Split(name, ",") {
		word, _, _ := strings.Cut(strings.TrimSpace(part), " ")
		if word == "" {
			continue
		}
		field := word
		if dot := strings.LastIndex(word, "."); dot >= 0 {
			if _, whole := contractFieldTypes[word]; whole {
				typ = nil
				continue
			}
			typeName, field = word[:dot], word[dot+1:]
			typ = contractFieldTypes[typeName]
		}
		if typ == nil {
			continue
		}
		if _, ok := typ.FieldByName(field); !ok {
			t.Errorf("%s: %s has no field %s", key, typeName, field)
		}
	}
}
