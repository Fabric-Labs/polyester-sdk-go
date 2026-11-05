package transport

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	orderv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/orders/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/wire"
)

// maxErrorBodyBytes bounds how much of an error body is inspected for a
// gateway code.
const maxErrorBodyBytes = 64 * 1024

// timestampSkewRoundTripper reports gateway answers whose JSON (or problem
// JSON) body is not a Connect error, so Connect would discard their code and
// surface a bare HTTP status:
//   - an API-key {"code":"TIMESTAMP_SKEW"} rejection as
//     *errors.TimestampSkewError;
//   - a 412 {"code":"AUTH_SUBACCOUNT_OWNER_ROLE_LOCKED"} problem as
//     *errors.SubaccountOwnerRoleLockedError;
//   - a 403 {"code":"AUTH_SESSION_KIND_NOT_ALLOWED"} problem as
//     *errors.SessionRequiredError;
//   - an API-key refusal ({"code":"SIGNATURE_INVALID"}, "IP_NOT_ALLOWED",
//     "UNAUTHENTICATED", ...) or a {"code":"PERMISSION_DENIED"} problem as
//     *errors.AuthError with the problem's code and detail;
//   - a problem that lists field violations as *errors.ValidationError.
type timestampSkewRoundTripper struct {
	base http.RoundTripper
}

func (t timestampSkewRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}
	resp, err := base.RoundTrip(req)
	if err != nil || resp.StatusCode < 400 || !isJSONContentType(resp.Header.Get("Content-Type")) {
		return resp, err
	}
	body, readErr := io.ReadAll(io.LimitReader(resp.Body, maxErrorBodyBytes))
	// Hand the caller the whole body: the inspected prefix, then the rest.
	resp.Body = prefixedBody{Reader: io.MultiReader(bytes.NewReader(body), resp.Body), Closer: resp.Body}
	if readErr != nil {
		return resp, nil
	}
	// Connect error codes are lowercase names, so this never matches one.
	var detail struct {
		Code       string `json:"code"`
		Detail     string `json:"detail"`
		Title      string `json:"title"`
		Violations []struct {
			FieldPath string `json:"fieldPath"`
			RuleID    string `json:"ruleId"`
			Message   string `json:"message"`
		} `json:"violations"`
	}
	if json.Unmarshal(body, &detail) != nil {
		return resp, nil
	}
	msg := detail.Detail
	if msg == "" {
		msg = detail.Title
	}
	var mapped error
	switch {
	case strings.HasSuffix(detail.Code, "_OUTCOME_UNKNOWN"):
		// A timed-out mutation may still apply. Preserve the same API error
		// as a Connect ErrorDetail before considering temporary HTTP failures.
		code := detail.Code
		if _, ok := orderv1.ErrorCode_value["ERROR_CODE_"+code]; ok {
			code = "ERROR_CODE_" + code
		}
		if msg == "" {
			msg = detail.Code
		}
		mapped = &sdkerrors.APIError{Msg: msg, Code: code}
	case (resp.StatusCode == http.StatusBadGateway || resp.StatusCode == http.StatusServiceUnavailable) &&
		detail.Code != "" && detail.Code == strings.ToUpper(detail.Code):
		// REST problem codes are uppercase; lowercase Connect error bodies
		// remain with Connect's decoder. Preserve the REST code and status.
		if msg == "" {
			msg = detail.Code
		}
		mapped = &sdkerrors.ServerError{Msg: msg, Code: detail.Code, Status: resp.StatusCode}
	case detail.Code == wire.TimestampSkewCode || detail.Code == sdkerrors.AuthCodeTimestampSkew:
		mapped = &sdkerrors.TimestampSkewError{
			Msg:    "API key timestamp is outside the allowed skew window",
			Status: resp.StatusCode,
		}
	case detail.Code == sdkerrors.AuthCodeSubaccountOwnerRoleLocked:
		mapped = &sdkerrors.SubaccountOwnerRoleLockedError{Msg: msg, Status: resp.StatusCode}
	case detail.Code == sdkerrors.AuthCodeSessionKindNotAllowed:
		mapped = &sdkerrors.SessionRequiredError{Msg: msg, Status: resp.StatusCode}
	case (resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden) &&
		(detail.Code == "PERMISSION_DENIED" || sdkerrors.AuthErrorCode(&sdkerrors.AuthError{Code: detail.Code}) != ""):
		mapped = &sdkerrors.AuthError{Msg: msg, Code: detail.Code, Status: resp.StatusCode}
	case len(detail.Violations) > 0 && detail.Code != "":
		violations := make([]sdkerrors.FieldViolation, 0, len(detail.Violations))
		for _, violation := range detail.Violations {
			violations = append(violations, sdkerrors.FieldViolation{
				FieldPath: violation.FieldPath, RuleID: violation.RuleID, Message: violation.Message,
			})
		}
		mapped = &sdkerrors.ValidationError{
			Msg: msg, Code: detail.Code, Metadata: sdkerrors.ViolationMetadata(violations), Violations: violations,
		}
	default:
		return resp, nil
	}
	problem := sdkerrors.DecodeHTTPProblem(body, resp.StatusCode)
	switch e := mapped.(type) {
	case *sdkerrors.AuthError:
		e.Problem = problem
	case *sdkerrors.TimestampSkewError:
		e.Problem = problem
	case *sdkerrors.SessionRequiredError:
		e.Problem = problem
	case *sdkerrors.SubaccountOwnerRoleLockedError:
		e.Problem = problem
	case *sdkerrors.ValidationError:
		e.Problem = problem
	}
	_ = resp.Body.Close()
	return nil, mapped
}

type prefixedBody struct {
	io.Reader
	io.Closer
}

func isJSONContentType(value string) bool {
	value = strings.ToLower(value)
	return strings.HasPrefix(value, "application/json") || strings.HasPrefix(value, "application/problem+json")
}

func wrapTimestampSkew(client *http.Client) *http.Client {
	if _, ok := client.Transport.(timestampSkewRoundTripper); ok {
		return client
	}
	wrapped := *client
	wrapped.Transport = timestampSkewRoundTripper{base: client.Transport}
	return &wrapped
}
