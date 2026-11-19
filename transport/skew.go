package transport

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"connectrpc.com/connect"
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	"github.com/Fabric-Labs/polyester-sdk-go/wire"
)

// maxErrorBodyBytes bounds how much of a failed answer's body is read. A
// longer one is unreadable and reads by its status alone.
const maxErrorBodyBytes = 64 * 1024

// restCodeForm is the contract's form for a REST problem code. A new one at 429
// is a refusal this SDK version can't name yet; anything else there isn't a code.
var restCodeForm = regexp.MustCompile(`^[A-Z][A-Z0-9_]*$`)

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
//   - a retryable admission problem, such as a 429 RATE_LIMIT_EXCEEDED, as
//     *errors.RateLimitError with the problem's rateLimit detail and the
//     response's Retry-After;
//   - a 429 REPLAY_CAPACITY_EXHAUSTED problem as *errors.RateLimitError with
//     code AUTH_REPLAY_CAPACITY_EXHAUSTED;
//   - an outcome-unknown mutation problem as *errors.APIError, including
//     codes introduced by a newer server before SDK regeneration;
//   - a problem that lists field violations as *errors.ValidationError;
//   - an answer with no recognised code as *errors.ServerError, by its status.
type timestampSkewRoundTripper struct {
	base http.RoundTripper
}

func (t timestampSkewRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}
	// Each round trip answers for itself: a redirect followed to a target
	// that never answers leaves no answer behind.
	record := answerRecordOf(req.Context())
	record.clear()
	resp, err := base.RoundTrip(req)
	if err != nil || resp.StatusCode < 300 {
		return resp, err
	}
	// The answer's status and delay are fixed here, from its headers, before
	// its body is read or a redirect is decided; the call's error, whatever
	// later fails, carries them (see answerInterceptor). A ConnectRPC error
	// decoded by connect-go keeps the real status in its metadata too (its
	// code alone implies another).
	record.set(resp)
	resp.Header.Set(wire.AnswerStatusHeader, strconv.Itoa(resp.StatusCode))
	if resp.StatusCode < 400 {
		// The client's redirect policy decides; a signed request's redirect
		// is refused, and one with nothing to follow reaches connect-go.
		return resp, nil
	}
	body, readErr := io.ReadAll(io.LimitReader(resp.Body, maxErrorBodyBytes+1))
	if readErr == nil && len(body) > maxErrorBodyBytes {
		// Longer than the SDK reads: unreadable, whatever its first bytes
		// say. It is never handed back to connect-go to decode whole.
		return unreadableAnswer(resp)
	}
	// Hand the caller the whole body: the inspected prefix, then the rest.
	resp.Body = prefixedBody{Reader: io.MultiReader(bytes.NewReader(body), resp.Body), Closer: resp.Body}
	if readErr != nil {
		return resp, nil
	}
	isJSON := isJSONContentType(resp.Header.Get("Content-Type"))
	// Connect asks for gzip, so a gateway may compress an error. Classify what
	// the body says; Connect still gets the bytes as received.
	decoded, overflow := decodedErrorBody(resp.Header.Get("Content-Encoding"), body)
	if overflow {
		// Longer than the SDK reads once decoded: unreadable, decided on the
		// byte count before anything parses it.
		return unreadableAnswer(resp)
	}
	// Connect error codes are lowercase names, so this never matches one.
	var detail struct {
		Code       string `json:"code"`
		Detail     string `json:"detail"`
		Title      string `json:"title"`
		Message    string `json:"message"`
		Violations []struct {
			FieldPath string `json:"fieldPath"`
			RuleID    string `json:"ruleId"`
			Message   string `json:"message"`
		} `json:"violations"`
		Details []struct {
			Type  string `json:"type"`
			Value string `json:"value"`
		} `json:"details"`
		RateLimit       json.RawMessage `json:"rateLimit"`
		TimestampWindow json.RawMessage `json:"timestampWindow"`
	}
	if isJSON || resp.StatusCode == 504 {
		if json.Unmarshal(decoded, &detail) != nil {
			// A body Connect's decoder can't read has no code either, whatever
			// fields were decoded before the failure.
			detail.Code = ""
		}
	}
	if wire.IsPlaceholderCode(detail.Code) {
		detail.Code = ""
	}
	hasServiceDetail := false
	futureCode := ""
	// An explicit outcome-unknown code: a problem's code, a ConnectRPC
	// error's code-only message, or an order detail's code.
	outcomeCode := ""
	switch {
	case strings.HasSuffix(detail.Code, "_OUTCOME_UNKNOWN"):
		outcomeCode = wire.ErrorCodeFromREST(detail.Code)
	case strings.HasSuffix(strings.TrimSpace(detail.Message), "_OUTCOME_UNKNOWN"):
		outcomeCode = wire.ErrorCodeFromREST(strings.TrimSpace(detail.Message))
	}
	for _, item := range detail.Details {
		value, decodeErr := base64.RawStdEncoding.DecodeString(strings.TrimRight(item.Value, "="))
		if decodeErr != nil {
			continue
		}
		if wire.CarriesContractCode(item.Type, value) {
			hasServiceDetail = true
		}
		if code := wire.OutcomeUnknownCode(item.Type, value); code != "" && outcomeCode == "" {
			outcomeCode = code
		}
		if futureCode == "" {
			futureCode = wire.FutureCodeLabel(item.Type, value)
		}
	}
	// The HTTP status decides outcome certainty first. A 504 never proves a
	// refusal: the API answers it only where a write's outcome is unknown,
	// and an edge answers it for a request that may have reached the API. So
	// before any code, message or typed detail is read for a refusal, a 504
	// is an unknown outcome that safe reads retry: an *errors.APIError with
	// the outcome-unknown code the answer names, else an *errors.ServerError.
	// No body, a ConnectRPC code, a retryable rate-limit, admission or
	// authentication code, or a typed detail included, changes that.
	if resp.StatusCode == http.StatusGatewayTimeout {
		var mapped error
		switch {
		case outcomeCode != "":
			mapped = &sdkerrors.APIError{
				Msg:     firstNonEmpty(detail.Detail, detail.Title, strings.TrimSpace(detail.Message), detail.Code, outcomeCode),
				Code:    outcomeCode,
				Problem: sdkerrors.DecodeHTTPProblem(decoded, resp.StatusCode),
			}
		case detail.Code == "deadline_exceeded":
			// ConnectRPC answers its own deadline_exceeded with 504: the
			// request's deadline passed, read as the deadline it is (as
			// ConnectRPC deadline_exceeded reads), whatever its message
			// or detail says. Still an unknown outcome that reads retry.
			mapped = &sdkerrors.TransportError{
				Msg: firstNonEmpty(strings.TrimSpace(detail.Message), "deadline exceeded"),
				Err: context.DeadlineExceeded,
			}
		default:
			// A REST problem's code is uppercase; a ConnectRPC error's
			// lowercase code names no service code, so only its detail's
			// label is kept.
			restCode := detail.Code
			if restCode == strings.ToLower(restCode) {
				restCode = ""
			}
			mapped = &sdkerrors.ServerError{
				Msg:    firstNonEmpty(detail.Detail, detail.Title, detail.Message, "Gateway Timeout"),
				Code:   firstNonEmpty(futureCode, restCode),
				Status: http.StatusGatewayTimeout,
			}
		}
		return nil, failedAnswer(resp, mapped)
	}
	// Connect's own code and detail decoder remains authoritative.
	switch detail.Code {
	case "unknown", "data_loss", "canceled":
		// The contract never uses these, and they say nothing a 401 or 403
		// doesn't already say better.
		if !hasServiceDetail && (resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden) {
			code := connectCodeForStatus(resp.StatusCode)
			mapped := wire.MapConnectError(connect.NewError(code, errors.New(firstNonEmpty(detail.Message, statusLine(resp.StatusCode)))))
			sdkerrors.AttachAnswer(mapped, resp.StatusCode, wire.RetryAfterHeaderSeconds(resp.Header))
			_ = resp.Body.Close()
			return nil, connect.NewError(code, mapped)
		}
		return passToConnect(resp, isJSON, decoded)
	case "invalid_argument", "deadline_exceeded", "not_found", "already_exists", "permission_denied", "resource_exhausted", "failed_precondition", "aborted", "out_of_range", "unimplemented", "internal", "unavailable", "unauthenticated":
		return passToConnect(resp, isJSON, decoded)
	}
	recognised := wire.IsRecognisedRESTCode(detail.Code)
	msg := detail.Detail
	if msg == "" {
		msg = detail.Title
	}
	var mapped error
	switch {
	case strings.HasSuffix(detail.Code, "_OUTCOME_UNKNOWN"):
		// A timed-out mutation may still apply. Preserve the same API error
		// as a Connect ErrorDetail before considering temporary HTTP failures.
		code := wire.ErrorCodeFromREST(detail.Code)
		if msg == "" {
			msg = detail.Code
		}
		mapped = &sdkerrors.APIError{Msg: msg, Code: code}
	case isRetryableRESTAdmissionCode(detail.Code):
		connectErr := connect.NewError(connect.CodeResourceExhausted, errors.New(msg))
		for name, values := range resp.Header {
			connectErr.Meta()[name] = values
		}
		mapped = wire.MapConnectError(connectErr)
		if rate, ok := mapped.(*sdkerrors.RateLimitError); ok {
			rate.Code = "ERROR_CODE_" + strings.TrimPrefix(detail.Code, "ERROR_CODE_")
			if limit := problemRateLimit(detail.RateLimit); limit != nil {
				rate.Detail = limit
				if seconds := limit.RetryAfterSeconds(); seconds != nil {
					rate.RetryAfter = seconds
				}
			}
		}
	case detail.Code == "REPLAY_CAPACITY_EXHAUSTED" || detail.Code == sdkerrors.AuthCodeReplayCapacityExhausted:
		// Valid credentials, not processed: a retryable rate refusal that
		// takes Retry-After only when the response names one.
		// The message is set afterwards: a code-shaped message would read as
		// a service code rather than a rate refusal.
		connectErr := connect.NewError(connect.CodeResourceExhausted, errors.New("replay capacity exhausted"))
		for name, values := range resp.Header {
			connectErr.Meta()[name] = values
		}
		mapped = wire.MapConnectError(connectErr)
		if rate, ok := mapped.(*sdkerrors.RateLimitError); ok {
			rate.Code = sdkerrors.AuthCodeReplayCapacityExhausted
			rate.Msg = firstNonEmpty(msg, detail.Code)
		}
	case detail.Code == "ORDER_SEQUENCE_EXHAUSTED" || detail.Code == "ERROR_CODE_ORDER_SEQUENCE_EXHAUSTED":
		if msg == "" {
			msg = detail.Code
		}
		mapped = &sdkerrors.APIError{Msg: msg, Code: "ERROR_CODE_ORDER_SEQUENCE_EXHAUSTED"}
	case resp.StatusCode == http.StatusServiceUnavailable && strings.TrimPrefix(detail.Code, "ERROR_CODE_") == "ACCOUNT_ROUTING_INVALID":
		mapped = &sdkerrors.APIError{Msg: msg, Code: "ERROR_CODE_ACCOUNT_ROUTING_INVALID"}
	case resp.StatusCode == http.StatusServiceUnavailable && detail.Code == sdkerrors.AuthCodeTemporarilyUnavailable:
		// Authentication could not be checked, so the request changed
		// nothing: a retryable refusal, as its ConnectRPC detail reads.
		if msg == "" {
			msg = detail.Code
		}
		mapped = &sdkerrors.APIError{Msg: msg, Code: sdkerrors.AuthCodeTemporarilyUnavailable}
	case (resp.StatusCode == http.StatusBadGateway || resp.StatusCode == http.StatusServiceUnavailable) &&
		detail.Code != "" && detail.Code == strings.ToUpper(detail.Code):
		// REST problem codes are uppercase; lowercase Connect error bodies
		// remain with Connect's decoder. Preserve the REST code and status.
		if msg == "" {
			msg = detail.Code
		}
		mapped = &sdkerrors.ServerError{Msg: msg, Code: detail.Code, CodeSource: problemCodeSource(resp, recognised), Status: resp.StatusCode}
	case detail.Code == wire.TimestampSkewCode || detail.Code == sdkerrors.AuthCodeTimestampSkew:
		mapped = &sdkerrors.TimestampSkewError{
			Msg:    "API key timestamp is outside the allowed skew window",
			Status: resp.StatusCode,
			Window: problemTimestampWindow(detail.TimestampWindow),
		}
	case detail.Code == sdkerrors.AuthCodeSubaccountOwnerRoleLocked:
		mapped = &sdkerrors.SubaccountOwnerRoleLockedError{Msg: msg, Status: resp.StatusCode}
	case detail.Code == sdkerrors.AuthCodeSessionKindNotAllowed:
		mapped = &sdkerrors.SessionRequiredError{Msg: msg, Status: resp.StatusCode}
	case (resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden) &&
		(recognised || detail.Code == "PERMISSION_DENIED" || sdkerrors.AuthErrorCode(&sdkerrors.AuthError{Code: detail.Code}) != ""):
		// A contract code such as SUBACCOUNT_READ_FORBIDDEN refuses the
		// operation; keep it and the detail instead of the bare status. An
		// order refusal reads as its ConnectRPC detail names it
		// (ERROR_CODE_API_KEY_SPOT_TRADE_DENY), so both transports return the
		// same code.
		mapped = &sdkerrors.AuthError{Msg: msg, Code: wire.OrderPermissionCodeFromREST(detail.Code), Status: resp.StatusCode}
	case len(detail.Violations) > 0 && recognised:
		violations := make([]sdkerrors.FieldViolation, 0, len(detail.Violations))
		for _, violation := range detail.Violations {
			violations = append(violations, sdkerrors.FieldViolation{
				FieldPath: violation.FieldPath, RuleID: violation.RuleID, Message: violation.Message,
			})
		}
		mapped = &sdkerrors.ValidationError{
			Msg: msg, Code: detail.Code, Metadata: sdkerrors.ViolationMetadata(violations), Violations: violations,
		}
	case recognised && resp.StatusCode == http.StatusNotImplemented:
		if msg == "" {
			msg = detail.Code
		}
		mapped = &sdkerrors.NotImplementedError{Msg: msg}
	case recognised && resp.StatusCode >= 500:
		if msg == "" {
			msg = detail.Code
		}
		mapped = &sdkerrors.ServerError{Msg: msg, Code: detail.Code, CodeSource: problemCodeSource(resp, recognised), Status: resp.StatusCode}
	case (recognised && resp.StatusCode != http.StatusUnauthorized && resp.StatusCode != http.StatusForbidden) ||
		(resp.StatusCode == http.StatusTooManyRequests && restCodeForm.MatchString(detail.Code)):
		code := wire.ErrorCodeFromREST(detail.Code)
		if msg == "" {
			msg = detail.Code
		}
		mapped = &sdkerrors.APIError{Msg: msg, Code: code}
	default:
		if hasServiceDetail || leftToConnect(resp.StatusCode, isJSON, len(decoded), detail.Message) {
			return passToConnect(resp, isJSON, decoded)
		}
		// A gateway or an edge can send any of these, so none proves that a
		// mutation was refused. The code is kept for diagnosis only.
		mapped = &sdkerrors.ServerError{
			Msg:    firstNonEmpty(msg, detail.Message, detail.Code, statusLine(resp.StatusCode)),
			Code:   firstNonEmpty(detail.Code, futureCode),
			Status: resp.StatusCode,
		}
	}
	problem := sdkerrors.DecodeHTTPProblem(decoded, resp.StatusCode)
	switch e := mapped.(type) {
	case *sdkerrors.APIError:
		e.Problem = problem
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
	return nil, failedAnswer(resp, mapped)
}

// passToConnect hands a failed answer to connect-go, which decodes a
// ConnectRPC error body and keeps the answer's headers on the error, the
// transport's AnswerStatusHeader included. An answer it cannot decode (an
// error content type other than application/json, or JSON that does not
// parse) it reports by status alone and without the answer's headers, so the
// SDK reads that one here, exactly as connect-go would, with the answer
// attached.
func passToConnect(resp *http.Response, _ bool, decoded []byte) (*http.Response, error) {
	if connectWillDecode(resp.Header.Get("Content-Type"), resp.Header.Get("Content-Encoding"), decoded) {
		return resp, nil
	}
	code := connectCodeForUndecodable(resp.StatusCode)
	mapped := wire.MapConnectError(connect.NewError(code, errors.New(resp.Status)))
	sdkerrors.AttachAnswer(mapped, resp.StatusCode, wire.RetryAfterHeaderSeconds(resp.Header))
	_ = resp.Body.Close()
	// The same code connect-go reports, so retries choose as before.
	return nil, connect.NewError(code, mapped)
}

// unreadableAnswer reports a failed answer whose body is longer than
// maxErrorBodyBytes: it reads by status alone, as an answer connect-go cannot
// decode does, with the status and delay its headers gave. A 504 is an
// unknown outcome, as every 504 is.
func unreadableAnswer(resp *http.Response) (*http.Response, error) {
	_ = resp.Body.Close()
	if resp.StatusCode == http.StatusGatewayTimeout {
		return nil, failedAnswer(resp, &sdkerrors.ServerError{Msg: "Gateway Timeout", Status: http.StatusGatewayTimeout})
	}
	return passToConnect(resp, false, nil)
}

// connectWillDecode reports whether connect-go decodes an error answer and
// keeps its headers: an exact application/json content type, an encoding it
// reads, and JSON that parses as its wire error (a string code and message,
// well-formed details). It is lenient with the code's value, as connect-go
// is: a missing or unknown one reads by status.
func connectWillDecode(contentType, encoding string, decoded []byte) bool {
	if contentType != "application/json" && contentType != "application/json; charset=utf-8" {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(encoding)) {
	case "", "identity", "gzip":
	default:
		return false
	}
	var wireErr struct {
		Code    string `json:"code"`
		Message string `json:"message"`
		Details []struct {
			Type  string          `json:"type"`
			Value string          `json:"value"`
			Debug json.RawMessage `json:"debug"`
		} `json:"details"`
	}
	if decoded == nil || json.Unmarshal(decoded, &wireErr) != nil {
		return false
	}
	for _, item := range wireErr.Details {
		if _, err := base64.RawStdEncoding.DecodeString(strings.TrimRight(item.Value, "=")); err != nil {
			return false
		}
	}
	return true
}

// connectCodeForUndecodable is connect-go's code for an answer it cannot
// decode, by HTTP status.
func connectCodeForUndecodable(status int) connect.Code {
	switch status {
	case 400:
		return connect.CodeInternal
	case 401:
		return connect.CodeUnauthenticated
	case 403:
		return connect.CodePermissionDenied
	case 404:
		return connect.CodeUnimplemented
	case 429, 502, 503, 504:
		return connect.CodeUnavailable
	}
	return connect.CodeUnknown
}

// failedAnswer closes a failed answer the SDK mapped itself and hands mapped
// to connect-go, with the delay the answer's Retry-After asked for: the one
// place a REST answer's delay is captured.
func failedAnswer(resp *http.Response, mapped error) error {
	sdkerrors.AttachAnswer(mapped, resp.StatusCode, wire.RetryAfterHeaderSeconds(resp.Header))
	_ = resp.Body.Close()
	return roundTripError(mapped, resp.StatusCode)
}

// roundTripError hands a mapped answer to connect-go, which reports a plain
// RoundTripper error as unavailable and keeps a *connect.Error's own code.
// Read retry selects on that code, so a definite refusal carries the code of
// its HTTP status and returns at once. Unavailability or an unknown outcome
// stays plain: reads may retry those from read context. Any other server error
// is internal, which reads don't retry. A refusal that changed nothing because
// authentication was briefly unavailable carries unavailable, as its ConnectRPC
// answer does, so reads retry it too. Either way wire.MapConnectError returns
// the typed error inside.
func roundTripError(mapped error, status int) error {
	var apiErr *sdkerrors.APIError
	if errors.As(mapped, &apiErr) && apiErr.Code == sdkerrors.AuthCodeTemporarilyUnavailable {
		return connect.NewError(connect.CodeUnavailable, mapped)
	}
	var serverErr *sdkerrors.ServerError
	if errors.As(mapped, &serverErr) {
		if isUnavailableStatus(serverErr.Status) {
			return mapped
		}
		return connect.NewError(connect.CodeInternal, mapped)
	}
	if sdkerrors.MutationOutcomeUnknown(mapped) {
		return mapped
	}
	return connect.NewError(connectCodeForStatus(status), mapped)
}

func connectCodeForStatus(status int) connect.Code {
	switch status {
	case http.StatusBadRequest, http.StatusUnprocessableEntity, http.StatusRequestEntityTooLarge:
		return connect.CodeInvalidArgument
	case http.StatusUnauthorized:
		return connect.CodeUnauthenticated
	case http.StatusForbidden:
		return connect.CodePermissionDenied
	case http.StatusNotFound:
		return connect.CodeNotFound
	case http.StatusConflict:
		return connect.CodeAborted
	case http.StatusTooManyRequests:
		return connect.CodeResourceExhausted
	case http.StatusNotImplemented:
		return connect.CodeUnimplemented
	}
	return connect.CodeFailedPrecondition
}

func isUnavailableStatus(status int) bool {
	switch status {
	case http.StatusTooManyRequests, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return true
	}
	return false
}

// leftToConnect reports an answer with no recognised code that Connect already
// reads correctly by status: a 401 or 403, an unavailable status, or a bare 404,
// which it names unimplemented and the error mapping names a missing route.
func leftToConnect(status int, isJSON bool, bodyLen int, message string) bool {
	switch {
	case status == http.StatusUnauthorized || status == http.StatusForbidden || isUnavailableStatus(status):
		return true
	case status == http.StatusNotFound:
		return !isJSON || bodyLen == 0 || wire.IsRouteNotFoundMessage(message)
	}
	return false
}

func statusLine(status int) string {
	return strings.TrimSpace(strconv.Itoa(status) + " " + http.StatusText(status))
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

// decodedErrorBody reads a failed response's body as its Content-Encoding says,
// up to maxErrorBodyBytes. overflow reports a body longer than that once
// decoded, decided on the byte count alone. An encoding this client can't
// decode leaves no readable error, so it reads as empty.
func decodedErrorBody(encoding string, body []byte) (decoded []byte, overflow bool) {
	switch strings.ToLower(strings.TrimSpace(encoding)) {
	case "", "identity":
		return body, false
	case "gzip":
		reader, err := gzip.NewReader(bytes.NewReader(body))
		if err != nil {
			return nil, false
		}
		decoded, err := io.ReadAll(io.LimitReader(reader, maxErrorBodyBytes+1))
		if len(decoded) > maxErrorBodyBytes {
			return nil, true
		}
		if err != nil {
			return nil, false
		}
		return decoded, false
	}
	return nil, false
}

type prefixedBody struct {
	io.Reader
	io.Closer
}

// problemCodeSource is errors.CodeSourceProblem for an answer that is a
// contract REST problem, a body with exactly one Content-Type field naming
// one media type, exactly application/problem+json (any case, parameters
// allowed), and a code the contract names. It is empty for any other body:
// a gateway's JSON, a Content-Type that does not parse, several Content-Type
// fields, or one field combining several media types.
func problemCodeSource(resp *http.Response, recognised bool) string {
	if !recognised {
		return ""
	}
	fields := resp.Header.Values("Content-Type")
	if len(fields) != 1 || strings.Contains(fields[0], ",") {
		return ""
	}
	mediaType, _, err := mime.ParseMediaType(fields[0])
	if err != nil || mediaType != "application/problem+json" {
		return ""
	}
	return sdkerrors.CodeSourceProblem
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

func isRetryableRESTAdmissionCode(code string) bool {
	switch strings.TrimPrefix(code, "ERROR_CODE_") {
	case "OVERLOADED", "SYSTEM_RATE_LIMITED", "RATE_LIMIT_EXCEEDED", "PRICE_LEVELS_EXHAUSTED", "CLIENT_ORDER_IDS_EXHAUSTED", "CLIENT_ORDER_ID_CAPACITY_REACHED", "MARKET_LADDER_LIMIT_EXCEEDED", "MARKET_TRIGGER_LIMIT_EXCEEDED", "MARKET_TWAP_LIMIT_EXCEEDED", "NEW_ACCOUNT_CAPACITY_REACHED", "REQUEST_RECEIPTS_EXHAUSTED", "RESERVATION_CAPACITY_REACHED", "TRIGGER_CAPACITY_REACHED", "TRIGGER_CONTROL_SHARE_EXHAUSTED", "TWAP_CAPACITY_REACHED":
		return true
	default:
		return false
	}
}

// problemRateLimit reads the rateLimit object of a REST rate-limit problem,
// whose fields are camelCase. Every field is optional: the gateway's own
// refusal sends only reason and scope. Integers arrive as JSON strings (problem
// bodies) or numbers; enum names are kept as sent, including ones newer than
// this SDK. Anything that isn't an object reads as no detail, and a field of the
// wrong type reads as absent.
// problemTimestampWindow reads a TIMESTAMP_SKEW problem's timestampWindow
// member, whose serverTimeMs is a decimal string; nil when the problem has
// none or it does not parse exactly.
func problemTimestampWindow(raw json.RawMessage) *sdkerrors.TimestampWindow {
	if len(raw) == 0 {
		return nil
	}
	var window struct {
		ServerTimeMs json.Number `json:"serverTimeMs"`
		RecvWindowMs json.Number `json:"recvWindowMs"`
		MaxFutureMs  json.Number `json:"maxFutureMs"`
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if decoder.Decode(&window) != nil {
		return nil
	}
	server, err := strconv.ParseInt(window.ServerTimeMs.String(), 10, 64)
	if err != nil {
		return nil
	}
	recv, err := strconv.ParseUint(window.RecvWindowMs.String(), 10, 32)
	if err != nil {
		return nil
	}
	future, err := strconv.ParseUint(window.MaxFutureMs.String(), 10, 32)
	if err != nil {
		return nil
	}
	return &sdkerrors.TimestampWindow{ServerTimeMs: server, RecvWindowMs: uint32(recv), MaxFutureMs: uint32(future)}
}

func problemRateLimit(raw json.RawMessage) *sdkerrors.RateLimitDetail {
	var fields map[string]json.RawMessage
	if len(raw) == 0 || json.Unmarshal(raw, &fields) != nil || fields == nil {
		return nil
	}
	text := func(name string) string {
		var value string
		_ = json.Unmarshal(fields[name], &value)
		return value
	}
	integer := func(name string) *uint64 {
		value := fields[name]
		var quoted string
		if json.Unmarshal(value, &quoted) == nil {
			value = json.RawMessage(quoted)
		}
		parsed, err := strconv.ParseUint(strings.TrimSpace(string(value)), 10, 64)
		if err != nil {
			return nil
		}
		return &parsed
	}
	return &sdkerrors.RateLimitDetail{
		Reason:        text("reason"),
		Limit:         integer("limit"),
		Remaining:     integer("remaining"),
		RetryAfterMs:  integer("retryAfterMs"),
		PolicyVersion: integer("policyVersion"),
		OperationID:   text("operationId"),
		PolicyClass:   text("policyClass"),
		Scope:         text("scope"),
		RefillModel:   text("refillModel"),
	}
}
