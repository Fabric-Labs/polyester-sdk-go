package transport

import (
	"context"
	"encoding/base64"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"connectrpc.com/connect"
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	authv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/auth/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/gen/auth/v1/authv1connect"
	changelogv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/changelog/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/gen/changelog/v1/changelogv1connect"
	ratelimitv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/polyester/ratelimit/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/hardening"
	"google.golang.org/protobuf/proto"
)

const requestTooLargeProblem = `{"type":"https://polyester.dev/problems/request-too-large","title":"Request Entity Too Large","status":413,"detail":"request too large","code":"REQUEST_TOO_LARGE"}`

// requestTooLargeServer refuses every request with the uniform 413 problem
// before reading its body, as the API does for an oversized one. chunked
// streams the problem without a Content-Length.
func requestTooLargeServer(t *testing.T, chunked bool) string {
	t.Helper()
	srv := hardening.NewTestServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/problem+json")
		if !chunked {
			w.Header().Set("Content-Length", strconv.Itoa(len(requestTooLargeProblem)))
		}
		w.WriteHeader(http.StatusRequestEntityTooLarge)
		if chunked {
			_, _ = io.WriteString(w, requestTooLargeProblem[:20])
			w.(http.Flusher).Flush()
			_, _ = io.WriteString(w, requestTooLargeProblem[20:])
			return
		}
		_, _ = io.WriteString(w, requestTooLargeProblem)
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

func assertRequestTooLarge(t *testing.T, err error) {
	t.Helper()
	var api *sdkerrors.APIError
	if !errors.As(err, &api) || api.Code != sdkerrors.CodeRequestTooLarge {
		t.Fatalf("error = %T %v, want *errors.APIError with code REQUEST_TOO_LARGE", err, err)
	}
	if !sdkerrors.IsRequestTooLarge(err) {
		t.Fatal("IsRequestTooLarge = false")
	}
	if api.Problem == nil || api.Problem.Status != 413 || api.Problem.Code != "REQUEST_TOO_LARGE" ||
		api.Problem.Type != "https://polyester.dev/problems/request-too-large" || api.Problem.Detail != "request too large" {
		t.Fatalf("problem = %+v", api.Problem)
	}
	// The request was refused before it was read: nothing changed, and the
	// same body is refused again.
	if sdkerrors.IsRetryable(err) || sdkerrors.MutationOutcomeUnknown(err) {
		t.Fatalf("retryable=%v outcome unknown=%v, want both false", sdkerrors.IsRetryable(err), sdkerrors.MutationOutcomeUnknown(err))
	}
	var rate *sdkerrors.RateLimitError
	if errors.As(err, &rate) {
		t.Fatal("a request size refusal is not a rate limit")
	}
}

func TestRequestTooLargeProblemIsATypedRefusal(t *testing.T) {
	for _, chunked := range []bool{false, true} {
		t.Run("chunked="+strconv.FormatBool(chunked), func(t *testing.T) {
			url := requestTooLargeServer(t, chunked)
			cfg := DefaultConfig()
			cfg.APIURL = url
			factory := NewFactory(cfg, nil, nil)
			client := authv1connect.NewAuthServiceClient(factory.HTTP, url, factory.ConnectOptions(false)...)
			_, err := client.Me(context.Background(), connect.NewRequest(&authv1.MeRequest{}))
			assertRequestTooLarge(t, MapError(err))
		})
	}
}

// An oversized request is refused before the server reads its body; the
// refusal, not the interrupted upload, is what the caller sees.
func TestOversizedRequestRefusedBeforeItsBodyIsRead(t *testing.T) {
	for _, chunked := range []bool{false, true} {
		t.Run("chunked="+strconv.FormatBool(chunked), func(t *testing.T) {
			url := requestTooLargeServer(t, chunked)
			cfg := DefaultConfig()
			cfg.APIURL = url
			factory := NewFactory(cfg, nil, nil)
			client := changelogv1connect.NewChangelogServiceClient(factory.HTTP, url, factory.ConnectOptions(false)...)
			_, err := client.GetPost(context.Background(), connect.NewRequest(&changelogv1.GetPostRequest{Slug: strings.Repeat("x", 4<<20)}))
			assertRequestTooLarge(t, MapError(err))
		})
	}
}

// A REST rate-limit problem is one typed rate-limit error whichever side
// refused, with its rate_limit detail and the server's retry time.
func TestRateLimitProblemCarriesItsDetail(t *testing.T) {
	u64 := func(v uint64) *uint64 { return &v }
	f64 := func(v float64) *float64 { return &v }
	for _, tc := range []struct {
		name    string
		headers map[string]string
		body    string
		detail  sdkerrors.RateLimitDetail
		retry   *float64
	}{
		{
			name:    "service quota",
			headers: map[string]string{"Retry-After": "2", "RateLimit-Limit": "30", "RateLimit-Remaining": "0", "RateLimit-Reset": "2"},
			body:    `{"code":"RATE_LIMIT_EXCEEDED","detail":"Rate limit exceeded","rate_limit":{"reason":"QUOTA_EXCEEDED","limit":"30","remaining":"0","retry_after_ms":"1409","policy_version":"5","operation_id":"auth.wallet_challenge","policy_class":"AUTH_PUBLIC","scope":"AUTH_SUBJECT","refill_model":"CONTINUOUS"},"status":429,"title":"Rate limit exceeded","type":"https://polyester.dev/problems/rate-limit-exceeded"}`,
			detail: sdkerrors.RateLimitDetail{
				Reason: "QUOTA_EXCEEDED", Limit: u64(30), Remaining: u64(0), RetryAfterMs: u64(1409), PolicyVersion: u64(5),
				OperationID: "auth.wallet_challenge", PolicyClass: "AUTH_PUBLIC", Scope: "AUTH_SUBJECT", RefillModel: "CONTINUOUS",
			},
			retry: f64(1.409),
		},
		{
			name:    "gateway window",
			headers: map[string]string{"Retry-After": "59", "X-RateLimit-Limit": "30", "X-RateLimit-Remaining": "0", "X-RateLimit-Reset": "59"},
			body:    `{"code":"RATE_LIMIT_EXCEEDED","detail":"Rate limit exceeded","rate_limit":{"reason":"QUOTA_EXCEEDED","scope":"CLIENT_IP"},"status":429,"title":"Rate limit exceeded","type":"https://polyester.dev/problems/rate-limit-exceeded"}`,
			detail:  sdkerrors.RateLimitDetail{Reason: "QUOTA_EXCEEDED", Scope: "CLIENT_IP"},
			retry:   f64(59),
		},
		{
			name:   "gateway concurrency",
			body:   `{"code":"RATE_LIMIT_EXCEEDED","detail":"Rate limit exceeded","rate_limit":{"reason":"QUOTA_EXCEEDED","scope":"CLIENT_IP"},"status":429,"title":"Rate limit exceeded","type":"https://polyester.dev/problems/rate-limit-exceeded"}`,
			detail: sdkerrors.RateLimitDetail{Reason: "QUOTA_EXCEEDED", Scope: "CLIENT_IP"},
		},
		{
			name:   "numbers and a newer enum value",
			body:   `{"code":"RATE_LIMIT_EXCEEDED","rate_limit":{"reason":"SOME_NEW_REASON","limit":30,"retry_after_ms":250}}`,
			detail: sdkerrors.RateLimitDetail{Reason: "SOME_NEW_REASON", Limit: u64(30), RetryAfterMs: u64(250)},
			retry:  f64(0.25),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := callMeAgainst(t, http.StatusTooManyRequests, "application/problem+json", tc.body, tc.headers)
			var rate *sdkerrors.RateLimitError
			if !errors.As(err, &rate) {
				t.Fatalf("error = %T %v, want *errors.RateLimitError", err, err)
			}
			if rate.Code != "ERROR_CODE_RATE_LIMIT_EXCEEDED" {
				t.Fatalf("code = %q", rate.Code)
			}
			if rate.Detail == nil {
				t.Fatal("rate_limit detail dropped")
			}
			if got, want := *rate.Detail, tc.detail; !rateLimitDetailEqual(got, want) {
				t.Fatalf("detail = %+v, want %+v", got, want)
			}
			if (rate.RetryAfter == nil) != (tc.retry == nil) || (tc.retry != nil && *rate.RetryAfter != *tc.retry) {
				t.Fatalf("retry after = %v, want %v", rate.RetryAfter, tc.retry)
			}
			if !sdkerrors.IsRetryable(err) || sdkerrors.MutationOutcomeUnknown(err) {
				t.Fatalf("retryable=%v outcome unknown=%v", sdkerrors.IsRetryable(err), sdkerrors.MutationOutcomeUnknown(err))
			}
		})
	}
}

func rateLimitDetailEqual(a, b sdkerrors.RateLimitDetail) bool {
	eq := func(x, y *uint64) bool { return (x == nil) == (y == nil) && (x == nil || *x == *y) }
	return a.Reason == b.Reason && a.OperationID == b.OperationID && a.PolicyClass == b.PolicyClass &&
		a.Scope == b.Scope && a.RefillModel == b.RefillModel &&
		eq(a.Limit, b.Limit) && eq(a.Remaining, b.Remaining) && eq(a.RetryAfterMs, b.RetryAfterMs) && eq(a.PolicyVersion, b.PolicyVersion)
}

// AUTH_RESOURCE_EXHAUSTED is a capacity refusal that waiting does not
// resolve, not a rate limit.
func TestAuthResourceExhaustedProblemIsNotARateLimit(t *testing.T) {
	err := callMeAgainst(t, http.StatusTooManyRequests, "application/problem+json",
		`{"code":"AUTH_RESOURCE_EXHAUSTED","detail":"no new account can be created","status":429,"title":"Resource Exhausted","type":"https://polyester.dev/problems/auth-resource-exhausted"}`)
	var rate *sdkerrors.RateLimitError
	if errors.As(err, &rate) {
		t.Fatalf("error = %+v, want no rate limit", rate)
	}
	var api *sdkerrors.APIError
	if !errors.As(err, &api) || api.Code != "AUTH_RESOURCE_EXHAUSTED" || sdkerrors.IsRetryable(err) || sdkerrors.MutationOutcomeUnknown(err) {
		t.Fatalf("error = %T %+v", err, err)
	}
}

// The gateway's own Connect-shaped refusal has no details: fall back to
// Retry-After when present, and give no server hint when absent.
func TestGatewayConnectRefusalFallsBackToRetryAfter(t *testing.T) {
	body := `{"code":"resource_exhausted","message":"rate limit exceeded, retry later"}`
	for _, tc := range []struct {
		name    string
		headers map[string]string
		want    *float64
	}{
		{"window", map[string]string{"Retry-After": "59", "X-RateLimit-Limit": "30", "X-RateLimit-Remaining": "0", "X-RateLimit-Reset": "59"}, func() *float64 { v := 59.0; return &v }()},
		{"concurrency", nil, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := callMeAgainst(t, http.StatusTooManyRequests, "application/json", body, tc.headers)
			var rate *sdkerrors.RateLimitError
			if !errors.As(err, &rate) || rate.Detail != nil {
				t.Fatalf("error = %T %+v", err, err)
			}
			if (rate.RetryAfter == nil) != (tc.want == nil) || (tc.want != nil && *rate.RetryAfter != *tc.want) {
				t.Fatalf("retry after = %v, want %v", rate.RetryAfter, tc.want)
			}
			if !sdkerrors.IsRetryable(err) {
				t.Fatal("a rate limit is retryable")
			}
		})
	}
}

// A service's ConnectRPC refusal carries RateLimitDetail as typed retry
// information, ahead of the headers.
func TestConnectRateLimitDetailIsTypedRetryInformation(t *testing.T) {
	limit, retry := uint64(30), uint64(1409)
	data, err := proto.Marshal(&ratelimitv1.RateLimitDetail{
		Reason: ratelimitv1.FailureReason_QUOTA_EXCEEDED, Limit: &limit, RetryAfterMs: &retry,
		OperationId: "auth.wallet_challenge", PolicyClass: ratelimitv1.PolicyClass_AUTH_PUBLIC, Scope: ratelimitv1.LimiterScope_AUTH_SUBJECT,
	})
	if err != nil {
		t.Fatal(err)
	}
	body := `{"code":"resource_exhausted","message":"rate limit exceeded","details":[{"type":"polyester.ratelimit.v1.RateLimitDetail","value":"` + base64.RawStdEncoding.EncodeToString(data) + `"}]}`
	mapped := callMeAgainst(t, http.StatusTooManyRequests, "application/json", body, map[string]string{"Retry-After": "2"})
	var rate *sdkerrors.RateLimitError
	if !errors.As(mapped, &rate) || rate.Detail == nil || rate.Detail.Scope != "AUTH_SUBJECT" || rate.Detail.OperationID != "auth.wallet_challenge" {
		t.Fatalf("error = %T %+v", mapped, mapped)
	}
	if rate.RetryAfter == nil || *rate.RetryAfter != 1.409 {
		t.Fatalf("retry after = %v, want 1.409", rate.RetryAfter)
	}
}

// AUTH_WALLET_ALREADY_LINKED is a terminal login refusal on both protocols.
func TestWalletAlreadyLinkedIsATerminalAuthRefusal(t *testing.T) {
	const message = "this wallet is already linked to another account; log in with the account address it already uses"
	data, err := proto.Marshal(&authv1.AuthErrorDetail{Code: authv1.AuthErrorCode_AUTH_WALLET_ALREADY_LINKED, Message: message})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, contentType, body string
	}{
		{"connect", "application/json", `{"code":"already_exists","message":"` + message + `","details":[{"type":"auth.v1.AuthErrorDetail","value":"` + base64.RawStdEncoding.EncodeToString(data) + `"}]}`},
		{"rest", "application/problem+json", `{"status":409,"code":"AUTH_WALLET_ALREADY_LINKED","title":"Wallet Already Linked","type":"https://polyester.dev/problems/auth-wallet-already-linked","detail":"` + message + `"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mapped := callMeAgainst(t, http.StatusConflict, tc.contentType, tc.body)
			if got := sdkerrors.AuthErrorCode(mapped); got != sdkerrors.AuthCodeWalletAlreadyLinked {
				t.Fatalf("AuthErrorCode = %q (%T %v)", got, mapped, mapped)
			}
			if !sdkerrors.IsWalletAlreadyLinked(mapped) {
				t.Fatal("IsWalletAlreadyLinked = false")
			}
			if mapped.Error() != message {
				t.Fatalf("message = %q", mapped.Error())
			}
			if sdkerrors.IsRetryable(mapped) || sdkerrors.MutationOutcomeUnknown(mapped) {
				t.Fatalf("retryable=%v outcome unknown=%v, want a terminal refusal", sdkerrors.IsRetryable(mapped), sdkerrors.MutationOutcomeUnknown(mapped))
			}
		})
	}
}
