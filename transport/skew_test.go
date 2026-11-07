package transport

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"connectrpc.com/connect"
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	authv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/auth/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/gen/auth/v1/authv1connect"
	"github.com/Fabric-Labs/polyester-sdk-go/hardening"
)

func callMeAgainst(t *testing.T, status int, contentType, body string, headers ...map[string]string) error {
	t.Helper()
	srv := hardening.NewTestServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", contentType)
		for _, values := range headers {
			for k, v := range values {
				w.Header().Set(k, v)
			}
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	cfg := DefaultConfig()
	cfg.APIURL = srv.URL
	factory := NewFactory(cfg, nil, nil)
	client := authv1connect.NewAuthServiceClient(factory.HTTP, cfg.APIURL, factory.ConnectOptions(false)...)
	_, err := client.Me(context.Background(), connect.NewRequest(&authv1.MeRequest{}))
	if err == nil {
		t.Fatal("expected an error")
	}
	return MapError(err)
}

func TestTimestampSkewProblemMapsToRetryableSkewError(t *testing.T) {
	err := callMeAgainst(t, http.StatusUnauthorized, "application/problem+json",
		`{"title":"Unauthorized","status":401,"detail":"timestamp outside window","code":"TIMESTAMP_SKEW"}`)
	var skew *sdkerrors.TimestampSkewError
	if !errors.As(err, &skew) || skew.Status != http.StatusUnauthorized {
		t.Fatalf("err=%T %v", err, err)
	}
	if !sdkerrors.IsRetryable(err) {
		t.Fatal("TIMESTAMP_SKEW is retryable")
	}
}

func TestOtherAuthProblemStaysAuthError(t *testing.T) {
	err := callMeAgainst(t, http.StatusUnauthorized, "application/problem+json",
		`{"title":"Unauthorized","status":401,"code":"INVALID_SIGNATURE"}`)
	var skew *sdkerrors.TimestampSkewError
	var authErr *sdkerrors.AuthError
	if errors.As(err, &skew) || !errors.As(err, &authErr) {
		t.Fatalf("err=%T %v", err, err)
	}
}

func TestConnectTimestampSkewMessageMapsToSkewError(t *testing.T) {
	err := callMeAgainst(t, http.StatusUnauthorized, "application/json",
		`{"code":"unauthenticated","message":"TIMESTAMP_SKEW"}`)
	var skew *sdkerrors.TimestampSkewError
	if !errors.As(err, &skew) {
		t.Fatalf("err=%T %v", err, err)
	}
}

func TestBareHTTP501ProvesNothingAboutAMutation(t *testing.T) {
	err := callMeAgainst(t, http.StatusNotImplemented, "text/plain", "not implemented")
	var server *sdkerrors.ServerError
	if !errors.As(err, &server) || server.Status != http.StatusNotImplemented || sdkerrors.IsRetryable(err) || !sdkerrors.MutationOutcomeUnknown(err) {
		t.Fatalf("err=%T %v", err, err)
	}
}

func TestGatewayTimestampSkewJSONMapsToSkewError(t *testing.T) {
	// The API gateway's shape, observed on devnet: a plain JSON body whose
	// code is not a Connect code.
	err := callMeAgainst(t, http.StatusUnauthorized, "application/json", `{"code":"TIMESTAMP_SKEW"}`)
	var skew *sdkerrors.TimestampSkewError
	if !errors.As(err, &skew) || skew.Status != http.StatusUnauthorized || !sdkerrors.IsRetryable(err) {
		t.Fatalf("err=%T %v", err, err)
	}
}

func TestLargeJSONErrorBodyReachesConnectIntact(t *testing.T) {
	// A Connect error whose body exceeds the inspected prefix must still
	// decode with its full message.
	message := strings.Repeat("x", maxErrorBodyBytes+1024)
	err := callMeAgainst(t, http.StatusBadRequest, "application/json",
		`{"code":"invalid_argument","message":"`+message+`"}`)
	var validation *sdkerrors.ValidationError
	if !errors.As(err, &validation) || validation.Msg != message {
		t.Fatalf("err=%T len=%d", err, len(err.Error()))
	}
}

func TestOwnerRoleLockedProblemMapsToTypedError(t *testing.T) {
	err := callMeAgainst(t, http.StatusPreconditionFailed, "application/problem+json",
		`{"status":412,"code":"AUTH_SUBACCOUNT_OWNER_ROLE_LOCKED","title":"Subaccount Owner Role Locked"}`)
	var locked *sdkerrors.SubaccountOwnerRoleLockedError
	if !errors.As(err, &locked) || locked.Status != http.StatusPreconditionFailed {
		t.Fatalf("err=%T %v", err, err)
	}
	if sdkerrors.AuthErrorCode(err) != sdkerrors.AuthCodeSubaccountOwnerRoleLocked || sdkerrors.IsRetryable(err) {
		t.Fatalf("err=%v", err)
	}
}

// REST problem bodies name the auth reason without the AUTH_ prefix;
// AuthErrorCode reads them as the auth.v1 code Connect answers.
func TestAuthProblemCodesReadAsAuthErrorCodes(t *testing.T) {
	for body, want := range map[string]string{
		`{"status":401,"code":"SIGNATURE_INVALID","detail":"API key signature is invalid"}`: sdkerrors.AuthCodeSignatureInvalid,
		`{"status":401,"code":"IP_NOT_ALLOWED","detail":"API key signature is invalid"}`:    sdkerrors.AuthCodeIPNotAllowed,
		`{"status":401,"code":"UNAUTHENTICATED","detail":"API key signature is invalid"}`:   sdkerrors.AuthCodeAuthenticationRequired,
	} {
		err := callMeAgainst(t, http.StatusUnauthorized, "application/problem+json", body)
		var authErr *sdkerrors.AuthError
		if !errors.As(err, &authErr) || authErr.Status != http.StatusUnauthorized || authErr.Msg != "API key signature is invalid" {
			t.Fatalf("%s: err=%T %v", want, err, err)
		}
		if sdkerrors.AuthErrorCode(err) != want || sdkerrors.IsRetryable(err) {
			t.Fatalf("%s: code=%q", want, sdkerrors.AuthErrorCode(err))
		}
	}
}

func TestSessionKindProblemMapsToSessionRequired(t *testing.T) {
	err := callMeAgainst(t, http.StatusForbidden, "application/problem+json",
		`{"type":"https://polyester.dev/problems/auth-session-kind-not-allowed","title":"Interactive Session Required","status":403,"detail":"interactive session required","code":"AUTH_SESSION_KIND_NOT_ALLOWED"}`)
	var required *sdkerrors.SessionRequiredError
	if !errors.As(err, &required) || required.Status != http.StatusForbidden || required.Msg != "interactive session required" {
		t.Fatalf("err=%T %v", err, err)
	}
	if sdkerrors.AuthErrorCode(err) != sdkerrors.AuthCodeSessionKindNotAllowed || sdkerrors.IsRetryable(err) {
		t.Fatalf("code=%q", sdkerrors.AuthErrorCode(err))
	}
}

func TestPermissionDeniedProblemKeepsItsDetail(t *testing.T) {
	err := callMeAgainst(t, http.StatusForbidden, "application/problem+json",
		`{"status":403,"code":"PERMISSION_DENIED","detail":"api key policy does not allow EXTERNAL_WITHDRAW"}`)
	var authErr *sdkerrors.AuthError
	if !errors.As(err, &authErr) || authErr.Error() != "api key policy does not allow EXTERNAL_WITHDRAW" ||
		authErr.Code != "PERMISSION_DENIED" || authErr.Status != http.StatusForbidden {
		t.Fatalf("err=%T %v", err, err)
	}
}

func TestProblemViolationsSurfaceOnValidationError(t *testing.T) {
	err := callMeAgainst(t, http.StatusBadRequest, "application/problem+json",
		`{"status":400,"code":"QTY_STEP_SIZE","detail":"Quantity must align with the step size.",`+
			`"violations":[{"fieldPath":"order.baseQty","ruleId":"step_size","message":"quantity does not align with the pair step size"}]}`)
	var validationErr *sdkerrors.ValidationError
	if !errors.As(err, &validationErr) || validationErr.Code != "QTY_STEP_SIZE" || validationErr.Msg != "Quantity must align with the step size." {
		t.Fatalf("err=%T %v", err, err)
	}
	got := sdkerrors.Violations(err)
	if len(got) != 1 || got[0].FieldPath != "order.baseQty" || got[0].RuleID != "step_size" {
		t.Fatalf("violations=%#v", got)
	}
}

func TestGatewayAuthProblemPreservesMetadata(t *testing.T) {
	err := callMeAgainst(t, 401, "application/problem+json", `{"code":"UNAUTHENTICATED","status":401,"detail":"","type":"https://polyester.dev/problems/unauthenticated"}`)
	var authErr *sdkerrors.AuthError
	if !errors.As(err, &authErr) || authErr.Problem == nil {
		t.Fatalf("problem missing: %T %v", err, err)
	}
	problem := authErr.Problem
	if problem.Code != "UNAUTHENTICATED" || problem.Status != 401 || problem.Detail != "" || problem.Type != "https://polyester.dev/problems/unauthenticated" {
		t.Fatalf("problem=%+v", problem)
	}
}

func TestGatewayProblemTypeDerivedFromCode(t *testing.T) {
	for _, suppliedType := range []string{"", "https://polyester.dev/problems/AUTH_SESSION_KIND_NOT_ALLOWED"} {
		body := map[string]any{"code": "AUTH_SESSION_KIND_NOT_ALLOWED", "detail": "", "status": 403}
		if suppliedType != "" {
			body["type"] = suppliedType
		}
		encoded, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		err = callMeAgainst(t, 403, "application/problem+json", string(encoded))
		var required *sdkerrors.SessionRequiredError
		if !errors.As(err, &required) || required.Problem == nil {
			t.Fatalf("problem missing: %T %v", err, err)
		}
		problem := required.Problem
		if problem.Code != "AUTH_SESSION_KIND_NOT_ALLOWED" || problem.Status != 403 || problem.Detail != "" || problem.Type != "https://polyester.dev/problems/auth-session-kind-not-allowed" {
			t.Fatalf("supplied type %q: problem=%+v", suppliedType, problem)
		}
	}
}

func gzipped(t *testing.T, body string) string {
	t.Helper()
	var buf bytes.Buffer
	writer := gzip.NewWriter(&buf)
	if _, err := writer.Write([]byte(body)); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

// Compression must not hide a code or invent one: each body classifies the same
// with and without it.
func TestCompressedErrorBodiesKeepTheirClassification(t *testing.T) {
	const problem = "application/problem+json"
	for _, tc := range []struct {
		name, contentType, body string
		status                  int
		check                   func(error) bool
	}{
		{"connect refusal", "application/json", `{"code":"invalid_argument","message":"m"}`, 400, func(err error) bool {
			var target *sdkerrors.ValidationError
			return errors.As(err, &target) && target.Code == "invalid_argument"
		}},
		{"connect not found", "application/json", `{"code":"not_found","message":"m"}`, 404, func(err error) bool {
			var target *sdkerrors.APIError
			return errors.As(err, &target) && target.Code == "not_found"
		}},
		{"rest refusal", problem, `{"code":"BAD_PRICE","detail":"d"}`, 400, func(err error) bool {
			var target *sdkerrors.APIError
			return errors.As(err, &target) && target.Code == "ERROR_CODE_BAD_PRICE"
		}},
		{"gateway skew", "application/json", `{"code":"TIMESTAMP_SKEW"}`, 401, func(err error) bool {
			var target *sdkerrors.TimestampSkewError
			return errors.As(err, &target)
		}},
		{"no code", "application/json", `{"message":"m"}`, 500, func(err error) bool {
			var target *sdkerrors.ServerError
			return errors.As(err, &target) && target.Msg == "m" && sdkerrors.MutationOutcomeUnknown(err)
		}},
		{"unrecognised", problem, `{"code":"NOPE_NOT_A_CODE"}`, 500, func(err error) bool {
			var target *sdkerrors.ServerError
			return errors.As(err, &target) && target.Code == "NOPE_NOT_A_CODE" && sdkerrors.MutationOutcomeUnknown(err)
		}},
		{"bare 404", "text/plain", "404 page not found", 404, func(err error) bool {
			var target *sdkerrors.RouteNotFoundError
			return errors.As(err, &target)
		}},
	} {
		for _, encoding := range []string{"identity", "gzip"} {
			t.Run(tc.name+"/"+encoding, func(t *testing.T) {
				body := tc.body
				if encoding == "gzip" {
					body = gzipped(t, tc.body)
				}
				err := callMeAgainst(t, tc.status, tc.contentType, body, map[string]string{"Content-Encoding": encoding})
				if !tc.check(err) {
					t.Fatalf("got %T %+v", err, err)
				}
			})
		}
	}
}

func TestLongCompressedConnectErrorReachesConnectIntact(t *testing.T) {
	// Random hex barely compresses, so the compressed body also passes the
	// inspected prefix.
	random := make([]byte, maxErrorBodyBytes)
	if _, err := rand.Read(random); err != nil {
		t.Fatal(err)
	}
	message := hex.EncodeToString(random)
	body := gzipped(t, `{"code":"invalid_argument","message":"`+message+`"}`)
	if len(body) <= maxErrorBodyBytes {
		t.Fatalf("fixture is %d bytes and must pass the inspected prefix", len(body))
	}
	err := callMeAgainst(t, 400, "application/json", body, map[string]string{"Content-Encoding": "gzip"})
	var validation *sdkerrors.ValidationError
	if !errors.As(err, &validation) || validation.Msg != message {
		t.Fatalf("got %T", err)
	}
}

// An encoding this client can't decode leaves no readable error. Whatever the
// body says, only a 401 or 403 is known, on its status alone, and a 404 reads as
// bare with an unknown outcome.
func TestUndecodableEncodingProvesNothing(t *testing.T) {
	for _, encoding := range []string{"br", "zstd", "deflate", "bogus"} {
		for _, body := range []string{`{"code":"BAD_PRICE"}`, `{"code":"invalid_argument","message":"m"}`, "corrupt"} {
			for _, status := range []int{400, 404, 409, 412, 422, 429, 500, 501, 502, 503, 504, 520} {
				t.Run(fmt.Sprintf("%s/%d/%s", encoding, status, body), func(t *testing.T) {
					err := callMeAgainst(t, status, "application/json", body, map[string]string{"Content-Encoding": encoding})
					var server *sdkerrors.ServerError
					var route *sdkerrors.RouteNotFoundError
					if !errors.As(err, &server) && !errors.As(err, &route) {
						t.Fatalf("got %T %+v", err, err)
					}
					if sdkerrors.IsRetryable(err) || !sdkerrors.MutationOutcomeUnknown(err) {
						t.Fatalf("got %T %+v", err, err)
					}
				})
			}
			for _, status := range []int{401, 403} {
				t.Run(fmt.Sprintf("%s/%d/%s", encoding, status, body), func(t *testing.T) {
					err := callMeAgainst(t, status, "application/json", body, map[string]string{"Content-Encoding": encoding})
					if sdkerrors.IsRetryable(err) {
						t.Fatalf("got %T %+v", err, err)
					}
					var authErr *sdkerrors.AuthError
					if !errors.As(err, &authErr) && !sdkerrors.MutationOutcomeUnknown(err) {
						t.Fatalf("got %T %+v", err, err)
					}
				})
			}
		}
	}
}

func TestCorruptGzipBodyProvesNothing(t *testing.T) {
	err := callMeAgainst(t, 400, "application/json", "not gzip", map[string]string{"Content-Encoding": "gzip"})
	var server *sdkerrors.ServerError
	if !errors.As(err, &server) || !sdkerrors.MutationOutcomeUnknown(err) {
		t.Fatalf("got %T %+v", err, err)
	}
}
