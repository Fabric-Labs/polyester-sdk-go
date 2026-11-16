package wire

import (
	"errors"
	"testing"
	"time"

	"connectrpc.com/connect"
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	authv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/auth/v1"
	withdrawv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/chain/withdraw/v1"
	ledgerrdv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/ledger/read/v1"
	orderv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/orders/v1"
	ratelimitv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/polyester/ratelimit/v1"
	transferv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/transfer/v1"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func connectErrWith(code connect.Code, detail proto.Message) *connect.Error {
	connectErr := connect.NewError(code, errors.New("refused"))
	d, err := connect.NewErrorDetail(detail)
	if err != nil {
		panic(err)
	}
	connectErr.AddDetail(d)
	return connectErr
}

// A withdrawal refused because its route's withdrawals are suspended is
// final for that request: nothing was reserved, it is not retryable and its
// outcome is known.
func TestWithdrawRouteSuspendedIsFinalAndNotRetryable(t *testing.T) {
	mapped := MapConnectError(connectErrWith(connect.CodeFailedPrecondition, &withdrawv1.ErrorDetail{Code: withdrawv1.ErrorCode_ERROR_CODE_ROUTE_SUSPENDED}))
	var apiErr *sdkerrors.APIError
	if !errors.As(mapped, &apiErr) || apiErr.Code != sdkerrors.CodeRouteSuspended || !sdkerrors.IsRouteSuspended(mapped) {
		t.Fatalf("mapped = %T %#v", mapped, mapped)
	}
	if sdkerrors.IsRetryable(mapped) || sdkerrors.MutationOutcomeUnknown(mapped) {
		t.Fatalf("retryable=%v unknown=%v", sdkerrors.IsRetryable(mapped), sdkerrors.MutationOutcomeUnknown(mapped))
	}
	if !IsRecognisedRESTCode("ROUTE_SUSPENDED") || ErrorCodeFromREST("ROUTE_SUSPENDED") != sdkerrors.CodeRouteSuspended {
		t.Fatal("REST ROUTE_SUSPENDED is not recognised as the withdraw code")
	}
	if sdkerrors.IsRouteSuspended(MapConnectError(connectErrWith(connect.CodeFailedPrecondition, &withdrawv1.ErrorDetail{Code: withdrawv1.ErrorCode_ERROR_CODE_EXTERNAL_DESTINATION_COOLING_DOWN}))) {
		t.Fatal("a cooling-down destination reads as a suspended route")
	}
}

// Dust and statement export refusals: only a busy dust conversion is
// retryable. The 6-hour window, the daily limit and the export limit last
// too long for an automatic retry, though they are resource_exhausted.
func TestDustAndStatementExportRefusalsRetryClassification(t *testing.T) {
	for code, retryable := range map[transferv1.ErrorCode]bool{
		transferv1.ErrorCode_ERROR_CODE_DUST_BUSY:               true,
		transferv1.ErrorCode_ERROR_CODE_DUST_WINDOW_ACTIVE:      false,
		transferv1.ErrorCode_ERROR_CODE_DUST_DAILY_LIMIT:        false,
		transferv1.ErrorCode_ERROR_CODE_DUST_QUOTE_EXPIRED:      false,
		transferv1.ErrorCode_ERROR_CODE_DUST_PRICE_MOVED:        false,
		transferv1.ErrorCode_ERROR_CODE_DUST_BALANCE_CHANGED:    false,
		transferv1.ErrorCode_ERROR_CODE_DUST_NOT_ELIGIBLE:       false,
		transferv1.ErrorCode_ERROR_CODE_DUST_CONVERSION_FAILED:  false,
		transferv1.ErrorCode_ERROR_CODE_DUST_TARGET_UNAVAILABLE: false,
	} {
		status := connect.CodeFailedPrecondition
		if code == transferv1.ErrorCode_ERROR_CODE_DUST_BUSY || code == transferv1.ErrorCode_ERROR_CODE_DUST_WINDOW_ACTIVE || code == transferv1.ErrorCode_ERROR_CODE_DUST_DAILY_LIMIT {
			status = connect.CodeResourceExhausted
		}
		mapped := MapConnectError(connectErrWith(status, &transferv1.ErrorDetail{Code: code}))
		var apiErr *sdkerrors.APIError
		if !errors.As(mapped, &apiErr) || apiErr.Code != code.String() {
			t.Fatalf("%s: mapped = %T %#v", code, mapped, mapped)
		}
		if sdkerrors.IsRetryable(mapped) != retryable || sdkerrors.MutationOutcomeUnknown(mapped) {
			t.Fatalf("%s: retryable=%v unknown=%v, want retryable=%v", code, sdkerrors.IsRetryable(mapped), sdkerrors.MutationOutcomeUnknown(mapped), retryable)
		}
	}
	for _, code := range []ledgerrdv1.ErrorCode{
		ledgerrdv1.ErrorCode_ERROR_CODE_STATEMENT_EXPORT_LIMIT_REACHED,
		ledgerrdv1.ErrorCode_ERROR_CODE_IDEMPOTENCY_KEY_REUSED,
		ledgerrdv1.ErrorCode_ERROR_CODE_STATEMENT_EXPORT_NOT_READY,
		ledgerrdv1.ErrorCode_ERROR_CODE_STATEMENT_EXPORT_EXPIRED,
		ledgerrdv1.ErrorCode_ERROR_CODE_STATEMENT_EXPORT_FAILED,
	} {
		mapped := MapConnectError(connectErrWith(connect.CodeResourceExhausted, &ledgerrdv1.ErrorDetail{Code: code}))
		var apiErr *sdkerrors.APIError
		if !errors.As(mapped, &apiErr) || apiErr.Code != code.String() || sdkerrors.IsRetryable(mapped) {
			t.Fatalf("%s: mapped = %T %#v retryable=%v", code, mapped, mapped, sdkerrors.IsRetryable(mapped))
		}
	}
}

// A ladder refused for one of its orders names that order's code and index;
// index 0 is the first order, not an absent one.
func TestLadderLevelRejectedCarriesCauseCodeAndLevelIndex(t *testing.T) {
	zero := uint32(0)
	refused := MapConnectError(connectErrWith(connect.CodeInvalidArgument, &orderv1.ErrorDetail{
		Code: orderv1.ErrorCode_ERROR_CODE_LADDER_LEVEL_REJECTED, CauseCode: orderv1.ErrorCode_ERROR_CODE_MAX_OPEN_ORDERS_PER_PAIR, LevelIndex: &zero,
		Violations: []*orderv1.FieldViolation{{FieldPath: "trigger.strategy.ladder.levels[0]", RuleId: "MAX_OPEN_ORDERS_PER_PAIR"}},
	}))
	var validation *sdkerrors.ValidationError
	if !errors.As(refused, &validation) || validation.Code != "ERROR_CODE_LADDER_LEVEL_REJECTED" {
		t.Fatalf("mapped = %T %#v", refused, refused)
	}
	cause, ok := sdkerrors.LadderCause(refused)
	if !ok || cause.CauseCode != "ERROR_CODE_MAX_OPEN_ORDERS_PER_PAIR" || cause.LevelIndex == nil || *cause.LevelIndex != 0 {
		t.Fatalf("cause = %+v ok=%v", cause, ok)
	}
	two := uint32(2)
	limited := MapConnectError(connectErrWith(connect.CodeResourceExhausted, &orderv1.ErrorDetail{
		Code: orderv1.ErrorCode_ERROR_CODE_LADDER_LEVEL_REJECTED, CauseCode: orderv1.ErrorCode_ERROR_CODE_RATE_LIMIT_EXCEEDED, LevelIndex: &two,
		RateLimit: &ratelimitv1.RateLimitDetail{RetryAfterMs: proto.Uint64(250)},
	}))
	var rateErr *sdkerrors.RateLimitError
	if !errors.As(limited, &rateErr) || rateErr.CauseCode != "ERROR_CODE_RATE_LIMIT_EXCEEDED" || rateErr.LevelIndex == nil || *rateErr.LevelIndex != 2 {
		t.Fatalf("rate-limited ladder = %T %#v", limited, limited)
	}
	plain := MapConnectError(connectErrWith(connect.CodeFailedPrecondition, &orderv1.ErrorDetail{Code: orderv1.ErrorCode_ERROR_CODE_INSUFFICIENT_FUNDS}))
	if cause, ok := sdkerrors.LadderCause(plain); ok {
		t.Fatalf("a plain refusal carries a ladder cause: %+v", cause)
	}
}

// A social account change refused for its 30-day rules says when the same
// request may succeed.
func TestSocialCooldownCarriesRetryAt(t *testing.T) {
	at := time.Date(2026, 12, 14, 11, 20, 0, 0, time.UTC)
	for _, code := range []authv1.AuthErrorCode{authv1.AuthErrorCode_AUTH_SOCIAL_ACCOUNT_COOLDOWN, authv1.AuthErrorCode_AUTH_SOCIAL_CHANGE_LIMIT_REACHED} {
		mapped := MapConnectError(connectErrWith(connect.CodeFailedPrecondition, &authv1.AuthErrorDetail{Code: code, Message: "wait", RetryAfter: timestamppb.New(at)}))
		var apiErr *sdkerrors.APIError
		if !errors.As(mapped, &apiErr) || apiErr.Code != code.String() || apiErr.RetryAt == nil || !apiErr.RetryAt.Equal(at) || sdkerrors.IsRetryable(mapped) {
			t.Fatalf("%s: mapped = %T %#v", code, mapped, mapped)
		}
	}
	mapped := MapConnectError(connectErrWith(connect.CodeFailedPrecondition, &authv1.AuthErrorDetail{Code: authv1.AuthErrorCode_AUTH_SOCIAL_CHANGE_LIMIT_REACHED}))
	var apiErr *sdkerrors.APIError
	if !errors.As(mapped, &apiErr) || apiErr.RetryAt != nil {
		t.Fatalf("without retry_after: %#v", mapped)
	}
}

// An order or trigger refused for funds answers failed_precondition, where it
// answered invalid_argument: the same account-state refusal either way, with
// its code, final and with a known outcome.
func TestInsufficientFundsIsAnAccountStateRefusal(t *testing.T) {
	for _, code := range []connect.Code{connect.CodeFailedPrecondition, connect.CodeInvalidArgument} {
		mapped := MapConnectError(connectErrWith(code, &orderv1.ErrorDetail{Code: orderv1.ErrorCode_ERROR_CODE_INSUFFICIENT_FUNDS}))
		var apiErr *sdkerrors.APIError
		if !errors.As(mapped, &apiErr) || apiErr.Code != "ERROR_CODE_INSUFFICIENT_FUNDS" {
			t.Fatalf("%v: mapped = %T %#v", code, mapped, mapped)
		}
		if sdkerrors.IsRetryable(mapped) || sdkerrors.MutationOutcomeUnknown(mapped) {
			t.Fatalf("%v: retryable=%v unknown=%v", code, sdkerrors.IsRetryable(mapped), sdkerrors.MutationOutcomeUnknown(mapped))
		}
	}
}
