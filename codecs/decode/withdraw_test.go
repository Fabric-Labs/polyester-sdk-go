package decode

import (
	"testing"

	chainwithdrawv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/chain/withdraw/v1"
)

func TestWithdrawDestinationValidationCodeLabels(t *testing.T) {
	cases := []struct {
		code     chainwithdrawv1.WithdrawDestinationValidationCode
		expected string
	}{
		{chainwithdrawv1.WithdrawDestinationValidationCode_RESULT_UNSPECIFIED, "unspecified"},
		{chainwithdrawv1.WithdrawDestinationValidationCode_VALID, "valid"},
		{chainwithdrawv1.WithdrawDestinationValidationCode_INVALID_ADDRESS, "invalid_address"},
		{chainwithdrawv1.WithdrawDestinationValidationCode_UNSUPPORTED_CHAIN, "unsupported_chain"},
		{chainwithdrawv1.WithdrawDestinationValidationCode_POLYESTER_SMART_ACCOUNT, "polyester_smart_account"},
		{chainwithdrawv1.WithdrawDestinationValidationCode_TOKEN_CONTRACT, "token_contract"},
		{chainwithdrawv1.WithdrawDestinationValidationCode_DENYLISTED_ADDRESS, "denylisted_address"},
		{chainwithdrawv1.WithdrawDestinationValidationCode(99), "unknown_code_99"},
	}
	for _, tc := range cases {
		got := withdrawDestinationValidationCodeLabel(tc.code)
		if got != tc.expected {
			t.Fatalf("code=%v got %q want %q", tc.code, got, tc.expected)
		}
	}
}

func TestWithdrawDestinationValidationFromProto(t *testing.T) {
	msg := &chainwithdrawv1.ValidateWithdrawDestinationResponse{
		Valid:                       true,
		Code:                        chainwithdrawv1.WithdrawDestinationValidationCode_VALID,
		Message:                     "ok",
		CanonicalDestinationAddress: "0xabc",
	}
	got := WithdrawDestinationValidationFromProto(msg)
	if !got.Valid || got.Code != "valid" || got.Message != "ok" || got.CanonicalDestinationAddress != "0xabc" {
		t.Fatalf("unexpected validation result: %+v", got)
	}

	denied := WithdrawDestinationValidationFromProto(&chainwithdrawv1.ValidateWithdrawDestinationResponse{
		Valid:   false,
		Code:    chainwithdrawv1.WithdrawDestinationValidationCode_DENYLISTED_ADDRESS,
		Message: "blocked",
	})
	if denied.Valid || denied.Code != "denylisted_address" || denied.Message != "blocked" {
		t.Fatalf("unexpected denylist result: %+v", denied)
	}
}
