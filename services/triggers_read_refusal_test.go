package services

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Fabric-Labs/polyester-sdk-go/auth"
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	triggersv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/triggers/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/transport"
	"google.golang.org/protobuf/proto"
)

// triggerReads calls each trigger read through its public method. Each path
// names the procedure the test server counts, so unrelated traffic to the
// listener can't change a request count.
var triggerReads = []struct {
	name string
	path string
	call func(context.Context, *TriggersService) error
	ok   proto.Message
}{
	{"List", "/triggers.v1.TriggersService/ListTriggers", func(ctx context.Context, s *TriggersService) error {
		_, err := s.List(ctx, nil, nil, nil, nil, 0, nil)
		return err
	}, &triggersv1.ListTriggersResponse{}},
	{"Get", "/triggers.v1.TriggersService/GetTrigger", func(ctx context.Context, s *TriggersService) error {
		_, err := s.Get(ctx, nil, "9", nil)
		return err
	}, &triggersv1.GetTriggerResponse{Trigger: &triggersv1.Trigger{TriggerId: 9}}},
	{"ListEvents", "/triggers.v1.TriggersService/ListTriggerEvents", func(ctx context.Context, s *TriggersService) error {
		_, err := s.ListEvents(ctx, nil, "9", nil, 0, nil, nil)
		return err
	}, &triggersv1.ListTriggerEventsResponse{}},
}

// scriptedTriggerServer answers a read's procedure with failFor failures, then
// with ok, and counts only requests to that procedure.
func scriptedTriggerServer(t *testing.T, path string, failFor int32, status int, contentType, body string, retryAfter string, ok proto.Message) (*TriggersService, *atomic.Int32) {
	t.Helper()
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != path {
			http.NotFound(w, r)
			return
		}
		if calls.Add(1) <= failFor {
			w.Header().Set("Content-Type", contentType)
			if retryAfter != "" {
				w.Header().Set("Retry-After", retryAfter)
			}
			w.WriteHeader(status)
			_, _ = w.Write([]byte(body))
			return
		}
		raw, err := proto.Marshal(ok)
		if err != nil {
			t.Error(err)
			return
		}
		w.Header().Set("Content-Type", "application/proto")
		_, _ = w.Write(raw)
	}))
	t.Cleanup(srv.Close)
	creds, err := auth.LoadCredentials("ak_test", auth.GenerateEd25519Keypair().SecretKeyHex, false)
	if err != nil {
		t.Fatal(err)
	}
	cfg := transport.DefaultConfig()
	cfg.APIURL = srv.URL
	return NewTriggersService(transport.NewFactory(cfg, creds, nil), nil, nil, nil, nil, nil, nil), &calls
}

func wantAuthRefusal(status int, code string) func(*testing.T, error) {
	return func(t *testing.T, err error) {
		t.Helper()
		var authErr *sdkerrors.AuthError
		if !errors.As(err, &authErr) || authErr.Status != status || authErr.Code != code {
			t.Fatalf("want AuthError %d %s, got %T %+v", status, code, err, err)
		}
		if sdkerrors.IsRetryable(err) || sdkerrors.MutationOutcomeUnknown(err) {
			t.Fatalf("a definite refusal is known and final: %+v", err)
		}
	}
}

// A definite refusal in a REST problem body returns at once with its typed
// reason, as the same refusal does in a Connect error body. Only the envelope
// differs, so each pair must make one request.
func TestTriggerReadsReturnDefiniteRefusalsAtOnce(t *testing.T) {
	const problem = "application/problem+json"
	for _, tc := range []struct {
		name, contentType, body string
		status                  int
		retryAfter              string
		check                   func(*testing.T, error)
	}{
		{"problem 401 IP_NOT_ALLOWED", problem, `{"code":"IP_NOT_ALLOWED","detail":"ip refused","status":401}`, 401, "", wantAuthRefusal(401, "IP_NOT_ALLOWED")},
		{"problem 403 PERMISSION_DENIED", problem, `{"code":"PERMISSION_DENIED","detail":"denied","status":403}`, 403, "", wantAuthRefusal(403, "PERMISSION_DENIED")},
		{"connect 401 control", "application/json", `{"code":"unauthenticated","message":"ip refused"}`, 401, "", wantAuthRefusal(401, "unauthenticated")},
		{"connect 403 control", "application/json", `{"code":"permission_denied","message":"denied"}`, 403, "", wantAuthRefusal(403, "permission_denied")},
		{"problem 400 violations", problem, `{"code":"INVALID_REQUEST","detail":"bad","violations":[{"fieldPath":"limit","ruleId":"gte","message":"too small"}]}`, 400, "", func(t *testing.T, err error) {
			var validation *sdkerrors.ValidationError
			if !errors.As(err, &validation) || validation.Code != "INVALID_REQUEST" ||
				len(validation.Violations) != 1 || validation.Violations[0].FieldPath != "limit" {
				t.Fatalf("got %T %+v", err, err)
			}
		}},
		{"problem 400 code only", problem, `{"code":"BAD_PRICE","detail":"bad"}`, 400, "", func(t *testing.T, err error) {
			var apiErr *sdkerrors.APIError
			if !errors.As(err, &apiErr) || apiErr.Code != "ERROR_CODE_BAD_PRICE" || apiErr.Problem == nil || apiErr.Problem.Status != 400 {
				t.Fatalf("got %T %+v", err, err)
			}
		}},
		{"problem 412 owner role", problem, `{"code":"AUTH_SUBACCOUNT_OWNER_ROLE_LOCKED","detail":"locked"}`, 412, "", func(t *testing.T, err error) {
			if !sdkerrors.IsSubaccountOwnerRoleLocked(err) {
				t.Fatalf("got %T %+v", err, err)
			}
		}},
		{"problem 403 session kind", problem, `{"code":"AUTH_SESSION_KIND_NOT_ALLOWED","detail":"session"}`, 403, "", func(t *testing.T, err error) {
			var session *sdkerrors.SessionRequiredError
			if !errors.As(err, &session) {
				t.Fatalf("got %T %+v", err, err)
			}
		}},
		{"problem 401 timestamp skew", problem, `{"code":"TIMESTAMP_SKEW"}`, 401, "", func(t *testing.T, err error) {
			var skew *sdkerrors.TimestampSkewError
			if !errors.As(err, &skew) {
				t.Fatalf("got %T %+v", err, err)
			}
		}},
		{"problem 429 keeps its delay", problem, `{"code":"RATE_LIMIT_EXCEEDED","detail":"slow down"}`, 429, "3", func(t *testing.T, err error) {
			var rate *sdkerrors.RateLimitError
			if !errors.As(err, &rate) || rate.Code != "ERROR_CODE_RATE_LIMIT_EXCEEDED" || rate.RetryAfter == nil || *rate.RetryAfter != 3 {
				t.Fatalf("got %T %+v", err, err)
			}
		}},
		{"problem 503 routing invalid", problem, `{"code":"ACCOUNT_ROUTING_INVALID","detail":"routing"}`, 503, "", func(t *testing.T, err error) {
			var apiErr *sdkerrors.APIError
			if !errors.As(err, &apiErr) || apiErr.Code != "ERROR_CODE_ACCOUNT_ROUTING_INVALID" {
				t.Fatalf("got %T %+v", err, err)
			}
		}},
	} {
		for _, read := range triggerReads {
			t.Run(tc.name+"/"+read.name, func(t *testing.T) {
				service, calls := scriptedTriggerServer(t, read.path, 1<<30, tc.status, tc.contentType, tc.body, tc.retryAfter, read.ok)
				err := read.call(context.Background(), service)
				if err == nil {
					t.Fatal("the refusal was not reported")
				}
				if got := calls.Load(); got != 1 {
					t.Fatalf("requests=%d, want 1: %+v", got, err)
				}
				tc.check(t, err)
			})
		}
	}
}

// Real unavailability keeps its bounded retry: two failures, then success, is
// exactly three requests and no error, whatever the envelope. A request count
// of one here would mean the failure was classified as a refusal.
func TestTriggerReadsStillRetryRealUnavailability(t *testing.T) {
	for _, tc := range []struct {
		name, contentType, body string
		status                  int
	}{
		{"connect unavailable", "application/json", `{"code":"unavailable","message":"catching up"}`, 503},
		{"problem 503 UNAVAILABLE", "application/problem+json", `{"code":"UNAVAILABLE","detail":"catching up"}`, 503},
		{"problem 502", "application/problem+json", `{"code":"BAD_GATEWAY","detail":"upstream"}`, 502},
		{"problem 504", "application/problem+json", `{"code":"GATEWAY_TIMEOUT","detail":"upstream"}`, 504},
		{"html 503", "text/html", "Service Unavailable", 503},
	} {
		for _, read := range triggerReads {
			t.Run(tc.name+"/"+read.name, func(t *testing.T) {
				service, calls := scriptedTriggerServer(t, read.path, 2, tc.status, tc.contentType, tc.body, "", read.ok)
				if err := read.call(context.Background(), service); err != nil {
					t.Fatalf("requests=%d err=%v", calls.Load(), err)
				}
				if got := calls.Load(); got != 3 {
					t.Fatalf("requests=%d, want 3", got)
				}
			})
		}
	}
}

// When unavailability outlasts the budget, the read stops and reports it as a
// server failure rather than retrying without limit.
func TestTriggerReadRetryOfRealUnavailabilityStaysBounded(t *testing.T) {
	read := triggerReads[1]
	service, calls := scriptedTriggerServer(t, read.path, 1<<30, 503, "application/problem+json", `{"code":"UNAVAILABLE","detail":"catching up"}`, "", read.ok)
	started := time.Now()
	err := read.call(context.Background(), service)
	var serverErr *sdkerrors.ServerError
	if !errors.As(err, &serverErr) || !strings.Contains(serverErr.Code, "UNAVAILABLE") {
		t.Fatalf("got %T %+v", err, err)
	}
	if got := calls.Load(); got < 2 || got > 12 {
		t.Fatalf("requests=%d, want a bounded retry", got)
	}
	if elapsed := time.Since(started); elapsed > 5*time.Second {
		t.Fatalf("took %s, past the read budget", elapsed)
	}
}
