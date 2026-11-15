package decode_test

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/Fabric-Labs/polyester-sdk-go/codecs/decode"
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	authv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/auth/v1"
	ledgerrdv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/ledger/read/v1"
	typev1 "github.com/Fabric-Labs/polyester-sdk-go/gen/polyester/type/v1"
	transferv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/transfer/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestDustQuoteDecodesExactAmountsLabelsAndPrices(t *testing.T) {
	asOf := time.Date(2026, 11, 14, 11, 59, 58, 0, time.UTC)
	quote, err := decode.DustQuoteFromProto(&transferv1.QuoteDustConversionResponse{
		QuoteId: "dq_1", ExpiresAt: timestamppb.New(asOf.Add(32 * time.Second)), TargetAssetId: 3, FeeBps: 200,
		GrossUsdE18: &typev1.U128{Lo: 5500000000000000000}, PayoutE18: &typev1.U128{Hi: 1, Lo: 7},
		Balances: []*transferv1.DustBalance{
			{AssetId: 10, AmountE18: &typev1.U128{Lo: 5000000000000000001}, Eligibility: transferv1.DustAssetEligibility_DUST_ASSET_ELIGIBILITY_ELIGIBLE, UsdValueE18: &typev1.U128{Lo: 2500000000000000000}},
			{AssetId: 11, AmountE18: &typev1.U128{Lo: 1}, Eligibility: transferv1.DustAssetEligibility_DUST_ASSET_ELIGIBILITY_NO_PRICE},
			{AssetId: 12, Eligibility: transferv1.DustAssetEligibility(99)},
		},
		Prices: []*transferv1.DustAssetPrice{{AssetId: 10, PriceTicks: 500000000, PriceScale: 9, AsOf: timestamppb.New(asOf)}, {AssetId: 3, PriceTicks: -15, PriceScale: 0}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if text, _ := quote.Payout.Format(); text != "18.446744073709551623" {
		t.Fatalf("payout = %s", text)
	}
	if text, _ := quote.Balances[0].Amount.Format(); text != "5.000000000000000001" {
		t.Fatalf("amount = %s", text)
	}
	if quote.Balances[0].Eligibility != "eligible" || quote.Balances[1].Eligibility != "no_price" || quote.Balances[2].Eligibility != "unknown(99)" {
		t.Fatalf("eligibility = %+v", quote.Balances)
	}
	if quote.Balances[0].USDValue == nil || quote.Balances[1].USDValue != nil {
		t.Fatalf("usd values = %v, %v; want nil without a price", quote.Balances[0].USDValue, quote.Balances[1].USDValue)
	}
	if quote.Prices[0].Price != "0.5" || quote.Prices[1].Price != "-15" || quote.Prices[0].AsOf == nil || !quote.Prices[0].AsOf.Equal(asOf) {
		t.Fatalf("prices = %+v", quote.Prices)
	}
	if _, err := decode.DustQuoteFromProto(&transferv1.QuoteDustConversionResponse{Prices: []*transferv1.DustAssetPrice{{PriceTicks: 1, PriceScale: 37}}}); err == nil {
		t.Fatal("a price scale above the protocol maximum decoded")
	}
}

func TestDustConversionLabelsRefusalCodes(t *testing.T) {
	list, err := decode.DustConversionsListFromProto(&transferv1.ListDustConversionsResponse{Conversions: []*transferv1.DustConversion{
		{ConversionId: "1", State: transferv1.DustConversionState_DUST_CONVERSION_STATE_REFUSED, RefusalCode: transferv1.ErrorCode_ERROR_CODE_DUST_PRICE_MOVED},
		{ConversionId: "2", State: transferv1.DustConversionState_DUST_CONVERSION_STATE_PROCESSING},
		{ConversionId: "3", State: transferv1.DustConversionState(9), RefusalCode: transferv1.ErrorCode(999)},
	}, NextPageToken: "n"})
	if err != nil {
		t.Fatal(err)
	}
	c := list.Conversions
	if c[0].State != "refused" || c[0].RefusalCode != "dust_price_moved" || c[1].State != "processing" || c[1].RefusalCode != "" ||
		c[2].State != "unknown(9)" || c[2].RefusalCode != "UNKNOWN_ERROR_CODE(999)" || list.NextPageToken != "n" {
		t.Fatalf("conversions = %+v", list)
	}
	_, err = decode.DustConversionsListFromProto(&transferv1.ListDustConversionsResponse{Conversions: []*transferv1.DustConversion{{}}})
	var contractErr *sdkerrors.ResponseContractError
	if !errors.As(err, &contractErr) {
		t.Fatalf("a conversion without its ID: %v", err)
	}
}

func TestStatementExportLabels(t *testing.T) {
	list, err := decode.StatementExportsListFromProto(&ledgerrdv1.ListStatementExportsResponse{Exports: []*ledgerrdv1.StatementExport{
		{ExportId: 1, Status: ledgerrdv1.StatementExportStatus_STATEMENT_EXPORT_STATUS_FAILED, FailureReason: ledgerrdv1.StatementExportFailureReason_STATEMENT_EXPORT_FAILURE_REASON_TOO_LARGE},
		{ExportId: 2, Status: ledgerrdv1.StatementExportStatus_STATEMENT_EXPORT_STATUS_QUEUED},
		{ExportId: 3, Status: ledgerrdv1.StatementExportStatus(8), FailureReason: ledgerrdv1.StatementExportFailureReason(4)},
	}})
	if err != nil {
		t.Fatal(err)
	}
	e := list.Exports
	if e[0].Status != "failed" || e[0].FailureReason != "too_large" || e[1].Status != "queued" || e[1].FailureReason != "" || e[1].SubaccountID != "" ||
		e[2].Status != "unknown(8)" || e[2].FailureReason != "unknown(4)" {
		t.Fatalf("exports = %+v", e)
	}
}

// Policy reads name the dust and funding-flow permissions.
func TestPolicyActionsNameConvertDustAndReadFundingFlows(t *testing.T) {
	actions := []authv1.PolicyAction{authv1.PolicyAction_CONVERT_DUST, authv1.PolicyAction_READ_FUNDING_FLOWS}
	sub := decode.SubaccountPolicyMessageFromProto(&authv1.SubaccountPolicyView{Id: 1, Actions: actions})
	api := decode.ApiPolicyMessageFromProto(&authv1.ApiPolicyView{Id: 2, Actions: actions})
	want := []string{"CONVERT_DUST", "READ_FUNDING_FLOWS"}
	if !reflect.DeepEqual(sub.Actions, want) || !reflect.DeepEqual(api.Actions, want) {
		t.Fatalf("actions = %v and %v, want %v", sub.Actions, api.Actions, want)
	}
	if authv1.PolicyAction_CONVERT_DUST != 39 || authv1.PolicyAction_READ_FUNDING_FLOWS != 40 ||
		authv1.SubaccountPermission_SUBACCOUNT_PERMISSION_CONVERT_DUST != 30 || authv1.SubaccountPermission_SUBACCOUNT_PERMISSION_READ_FUNDING_FLOWS != 31 {
		t.Fatal("dust and funding-flow permission numbers moved")
	}
}

// A verification refused for the 30-day social rules keeps its retry_after
// in the decoded social verification.
func TestSocialVerificationKeepsRetryAfter(t *testing.T) {
	at := time.Date(2026, 12, 14, 11, 20, 0, 0, time.UTC)
	data, err := decode.APIDataFromProto(&authv1.GetSocialVerificationResponse{Verification: &authv1.SocialVerification{
		ErrorCode: authv1.AuthErrorCode_AUTH_SOCIAL_CHANGE_LIMIT_REACHED, RetryAfter: timestamppb.New(at),
	}})
	if err != nil {
		t.Fatal(err)
	}
	verification, _ := data.Raw["verification"].(map[string]any)
	if verification["retry_after"] != "2026-12-14T11:20:00Z" || verification["error_code"] != "AUTH_SOCIAL_CHANGE_LIMIT_REACHED" {
		t.Fatalf("verification = %#v", verification)
	}
}
