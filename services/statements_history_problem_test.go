package services

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/Fabric-Labs/polyester-sdk-go/auth"
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	"github.com/Fabric-Labs/polyester-sdk-go/internal/hardening"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
	"github.com/Fabric-Labs/polyester-sdk-go/transport"
)

// statementsAnswering is a statements service whose server answers every
// request with status, the problem content type and body.
func statementsAnswering(t *testing.T, status int, body string) *StatementsService {
	t.Helper()
	srv := hardening.NewTestServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/problem+json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	creds, err := auth.LoadCredentials("ak_test", auth.GenerateEd25519Keypair().SecretKeyHex.Reveal(), false)
	if err != nil {
		t.Fatal(err)
	}
	cfg := transport.DefaultConfig()
	cfg.APIURL = srv.URL
	return NewStatementsService(transport.NewFactory(cfg, creds, nil), nil)
}

func createBalancesExport(svc *StatementsService) error {
	start := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	_, err := svc.CreateExport(context.Background(), CreateStatementExportOptions{
		Type: models.StatementExportTypeBalances, StartTime: start, EndTime: start.AddDate(0, 1, 0), IdempotencyKey: "balances-2026-11",
	})
	return err
}

// The REST answer to a balances export that starts before the account
// history, in the contract's published problem shape, reads as the
// ConnectRPC refusal does: the typed code with the instant to start from,
// to the nanosecond.
func TestStatementHistoryUnavailableProblemCarriesHistoryAvailableFrom(t *testing.T) {
	svc := statementsAnswering(t, http.StatusConflict, `{"type": "https://polyester.dev/problems/statement-history-unavailable", "title": "Conflict", "status": 409,
 "detail": "statement_history_unavailable: the period starts before the account history", "code": "STATEMENT_HISTORY_UNAVAILABLE",
 "historyAvailableFrom": "2026-11-25T14:55:02.123456789Z"}`)
	err := createBalancesExport(svc)
	var apiErr *sdkerrors.APIError
	if !errors.As(err, &apiErr) || apiErr.Code != sdkerrors.CodeStatementHistoryUnavailable || !sdkerrors.IsStatementHistoryUnavailable(err) {
		t.Fatalf("err = %T %v", err, err)
	}
	want := time.Date(2026, 11, 25, 14, 55, 2, 123456789, time.UTC)
	if apiErr.HistoryAvailableFrom == nil || !apiErr.HistoryAvailableFrom.Equal(want) {
		t.Fatalf("history_available_from = %v, want %v", apiErr.HistoryAvailableFrom, want)
	}
	if sdkerrors.IsRetryable(err) {
		t.Fatal("a history refusal is not retryable as is")
	}
}

// A problem without the member, or with one that is not RFC 3339, keeps the
// typed code and leaves HistoryAvailableFrom nil; another code never sets it.
func TestStatementHistoryProblemWithoutAnInstant(t *testing.T) {
	for name, body := range map[string]string{
		"no member":  `{"status": 409, "code": "STATEMENT_HISTORY_UNAVAILABLE", "detail": "statement_history_unavailable"}`,
		"bad member": `{"status": 409, "code": "STATEMENT_HISTORY_UNAVAILABLE", "historyAvailableFrom": "yesterday"}`,
	} {
		var apiErr *sdkerrors.APIError
		err := createBalancesExport(statementsAnswering(t, http.StatusConflict, body))
		if !errors.As(err, &apiErr) || apiErr.Code != sdkerrors.CodeStatementHistoryUnavailable || apiErr.HistoryAvailableFrom != nil {
			t.Fatalf("%s: err = %T %#v", name, err, err)
		}
	}
	var apiErr *sdkerrors.APIError
	err := createBalancesExport(statementsAnswering(t, http.StatusConflict,
		`{"status": 409, "code": "IDEMPOTENCY_KEY_REUSED", "historyAvailableFrom": "2026-11-25T14:55:02Z"}`))
	if !errors.As(err, &apiErr) || apiErr.HistoryAvailableFrom != nil || sdkerrors.IsStatementHistoryUnavailable(err) {
		t.Fatalf("other code: err = %T %#v", err, err)
	}
}

// historyAvailableFrom is an optional extension of the problem: a member of
// any other JSON type never breaks reading the problem, so the typed code
// and the other fields are kept, and only a valid RFC 3339 string sets
// HistoryAvailableFrom. The same holds for a problem of another code that
// carries the member.
func TestStatementHistoryProblemToleratesNonStringInstants(t *testing.T) {
	for _, member := range []string{`1795618502`, `{"seconds": 1795618502}`, `null`, `["2026-11-25T14:55:02Z"]`, `true`} {
		for _, code := range []string{"STATEMENT_HISTORY_UNAVAILABLE", "IDEMPOTENCY_KEY_REUSED"} {
			body := `{"type": "https://polyester.dev/problems/x", "title": "Conflict", "status": 409,
 "detail": "refused", "code": "` + code + `", "historyAvailableFrom": ` + member + `}`
			err := createBalancesExport(statementsAnswering(t, http.StatusConflict, body))
			var apiErr *sdkerrors.APIError
			if !errors.As(err, &apiErr) {
				t.Fatalf("%s with %s: err = %T %v, want *errors.APIError", code, member, err, err)
			}
			if apiErr.Code != "ERROR_CODE_"+code || apiErr.Msg != "refused" || apiErr.HistoryAvailableFrom != nil {
				t.Fatalf("%s with %s: err = %#v", code, member, apiErr)
			}
			if apiErr.Problem == nil || apiErr.Problem.Code != code || apiErr.Problem.Status != http.StatusConflict {
				t.Fatalf("%s with %s: problem = %#v", code, member, apiErr.Problem)
			}
		}
	}
}
