package wire

import (
	"context"
	"errors"
	"fmt"
	alertsv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/alerts/v1"
	"net/http"
	"strconv"
	"strings"

	"buf.build/gen/go/bufbuild/protovalidate/protocolbuffers/go/buf/validate"
	"connectrpc.com/connect"
	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	authv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/auth/v1"
	withdrawv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/chain/withdraw/v1"
	ledgerrdv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/ledger/read/v1"
	orderv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/orders/v1"
	polyesterapiv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/polyester/api/v1"
	ratelimitv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/polyester/ratelimit/v1"
	transferv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/transfer/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/internal/redact"
	"github.com/Fabric-Labs/polyester-sdk-go/useragent"
	"google.golang.org/protobuf/proto"
)

var routeNotFoundMessages = map[string]struct{}{
	"not found":          {},
	"404 page not found": {},
	"404 not found":      {},
}

const emptyErrorMessage = "request failed without server error details"

// AnswerStatusHeader carries a failed answer's real HTTP status into the
// metadata of the ConnectRPC error connect-go decodes from it. The transport
// sets it on every failed answer; it never leaves the client.
const AnswerStatusHeader = "Polyester-Sdk-Answer-Status"

// AnswerWireCodeHeader carries the code a failed answer's ConnectRPC error
// body names on the wire into the metadata of the error connect-go decodes
// from it: connect-go infers a code from the HTTP status when the body names
// none. The transport removes it from every failed answer and sets it only
// from a JSON body it parsed; it never leaves the client.
const AnswerWireCodeHeader = "Polyester-Sdk-Answer-Wire-Code"

// typedRefusalSource is the CodeSource of an unavailable refusal whose code
// detail carries: the detail's full name when the answer's body named the
// wire code unavailable and carries exactly one detail of detail's type, and
// empty otherwise. A code connect-go inferred from the status, or details
// that may disagree, prove nothing.
func typedRefusalSource(connectErr *connect.Error, detail proto.Message) string {
	if connectErr.Meta().Get(AnswerWireCodeHeader) != connect.CodeUnavailable.String() {
		return ""
	}
	name := string(detail.ProtoReflect().Descriptor().FullName())
	count := 0
	for _, item := range connectErr.Details() {
		if item.Type() == name {
			count++
		}
	}
	if count != 1 {
		return ""
	}
	return name
}

func retryAfterSeconds(connectErr *connect.Error) *float64 {
	return RetryAfterHeaderSeconds(connectErr.Meta())
}

// RetryAfterHeaderSeconds reads the delay an answer asks for, in seconds:
// see errors.RetryAfterHeader.
func RetryAfterHeaderSeconds(header http.Header) *float64 {
	return sdkerrors.RetryAfterHeader(header)
}

func rateLimitDetailFromProto(msg *ratelimitv1.RateLimitDetail) *sdkerrors.RateLimitDetail {
	if msg == nil {
		return nil
	}
	enumName := func(names map[int32]string, raw int32, unknownPrefix string) string {
		if name, ok := names[raw]; ok {
			return name
		}
		return fmt.Sprintf("%s(%d)", unknownPrefix, raw)
	}
	out := &sdkerrors.RateLimitDetail{
		Reason:      enumName(ratelimitv1.FailureReason_name, int32(msg.GetReason()), "UNKNOWN_FAILURE_REASON"),
		OperationID: msg.GetOperationId(),
		PolicyClass: enumName(ratelimitv1.PolicyClass_name, int32(msg.GetPolicyClass()), "UNKNOWN_POLICY_CLASS"),
		Scope:       enumName(ratelimitv1.LimiterScope_name, int32(msg.GetScope()), "UNKNOWN_LIMITER_SCOPE"),
		RefillModel: enumName(ratelimitv1.RefillModel_name, int32(msg.GetRefillModel()), "UNKNOWN_REFILL_MODEL"),
	}
	if msg.Limit != nil {
		v := msg.GetLimit()
		out.Limit = &v
	}
	if msg.Remaining != nil {
		v := msg.GetRemaining()
		out.Remaining = &v
	}
	if msg.RetryAfterMs != nil {
		v := msg.GetRetryAfterMs()
		out.RetryAfterMs = &v
	}
	if msg.PolicyVersion != nil {
		v := msg.GetPolicyVersion()
		out.PolicyVersion = &v
	}
	return out
}

func rateLimitError(msg string, code string, detail *sdkerrors.RateLimitDetail, headerRetry *float64) *sdkerrors.RateLimitError {
	retryAfter := headerRetry
	if seconds := detail.RetryAfterSeconds(); seconds != nil {
		retryAfter = seconds
	}
	return &sdkerrors.RateLimitError{
		Msg:        msg,
		Code:       code,
		RetryAfter: retryAfter,
		Detail:     detail,
	}
}

// MapConnectError converts Connect errors into SDK error types.
//
// Every level of the error it returns is redacted (see redact.Text): an
// answer or a transport error can quote a URL with credentials, such as a
// proxy's, and the SDK error copies its text. Each level keeps its type, so
// errors.Is and errors.As still find it.
func MapConnectError(err error) error {
	mapped := mapConnectError(err)
	// A ConnectRPC error decoded from an answer keeps the answer's headers;
	// its metadata, the status ConnectRPC answers its code with and the delay
	// it asked for, is captured here once for whatever error it maps to. A
	// REST answer's was captured by the transport, and an error with no
	// answer behind it, such as a refused connection, has no headers.
	var connectErr *connect.Error
	if errors.As(err, &connectErr) && len(connectErr.Meta()) > 0 {
		// The status is the answer's own, recorded by the transport; it is
		// never inferred from the code.
		status, _ := strconv.Atoi(connectErr.Meta().Get(AnswerStatusHeader))
		sdkerrors.AttachAnswer(mapped, status, retryAfterSeconds(connectErr))
	}
	redactChain(mapped)
	return mapped
}

func mapConnectError(err error) error {
	if err == nil {
		return nil
	}
	// An SDK error mapped from an answer is kept as it is, with the answer's
	// metadata, never rebuilt.
	var mappedTransport *sdkerrors.TransportError
	if errors.As(err, &mappedTransport) {
		return mappedTransport
	}
	// The caller cancelled or ran out of time: keep that answer reachable.
	if errors.Is(err, context.Canceled) {
		return &sdkerrors.TransportError{Msg: redact.Message(err.Error(), err), Err: context.Canceled}
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return &sdkerrors.TransportError{Msg: redact.Message(err.Error(), err), Err: context.DeadlineExceeded}
	}
	// Connect may wrap an interceptor-returned SDK error before it reaches the
	// service layer. Preserve client-side signing/authentication failures
	// instead of reclassifying them as transport or unknown API failures.
	var authErr *sdkerrors.AuthError
	if errors.As(err, &authErr) {
		return authErr
	}
	// The HTTP client refused to follow a redirect of the signed request.
	var redirectErr *sdkerrors.RedirectRefusedError
	if errors.As(err, &redirectErr) {
		return redirectErr
	}
	var rateLimitErr *sdkerrors.RateLimitError
	if errors.As(err, &rateLimitErr) {
		return rateLimitErr
	}
	var serverErr *sdkerrors.ServerError
	if errors.As(err, &serverErr) {
		return serverErr
	}
	var skewErr *sdkerrors.TimestampSkewError
	if errors.As(err, &skewErr) {
		return skewErr
	}
	var notImplementedErr *sdkerrors.NotImplementedError
	if errors.As(err, &notImplementedErr) {
		return notImplementedErr
	}
	var routeErr *sdkerrors.RouteNotFoundError
	if errors.As(err, &routeErr) {
		return routeErr
	}
	var ownerLockedErr *sdkerrors.SubaccountOwnerRoleLockedError
	if errors.As(err, &ownerLockedErr) {
		return ownerLockedErr
	}
	var sessionErr *sdkerrors.SessionRequiredError
	if errors.As(err, &sessionErr) {
		return sessionErr
	}
	var problemErr *sdkerrors.ValidationError
	if errors.As(err, &problemErr) {
		return problemErr
	}
	var apiErr *sdkerrors.APIError
	if errors.As(err, &apiErr) {
		return apiErr
	}
	var connectErr *connect.Error
	if !errors.As(err, &connectErr) {
		return &sdkerrors.TransportError{Msg: redact.Message(err.Error(), err)}
	}
	headerRetry := retryAfterSeconds(connectErr)
	if connectErr.Message() == TimestampSkewCode {
		return &sdkerrors.TimestampSkewError{
			Msg: "API key timestamp is outside the allowed skew window",
		}
	}
	// A failed HTTP exchange arrives as a *url.Error, whose text quotes a URL:
	// for a redirect, the server-supplied Location.
	msg := redact.Message(connectErr.Message(), connectErr)
	if strings.TrimSpace(msg) == "" {
		msg = emptyErrorMessage
	}
	// Business outcomes take precedence over standalone backoff details.
	var outcome *orderv1.ErrorDetail
	for _, detail := range connectErr.Details() {
		value, err := detail.Value()
		if err != nil {
			continue
		}
		if order, ok := value.(*orderv1.ErrorDetail); ok {
			if strings.HasSuffix(order.GetCode().String(), "_OUTCOME_UNKNOWN") {
				outcome = order
				break
			}
			if outcome == nil && isFinalResourceCode(order.GetCode().String()) {
				outcome = order
			}
		}
	}
	if strings.HasSuffix(strings.TrimSpace(msg), "_OUTCOME_UNKNOWN") && (outcome == nil || !strings.HasSuffix(outcome.GetCode().String(), "_OUTCOME_UNKNOWN")) {
		return &sdkerrors.APIError{Msg: msg, Code: orderErrorCodeFromName(strings.TrimSpace(msg))}
	}
	if outcome != nil {
		mapped := &sdkerrors.APIError{Msg: msg, Code: outcome.GetCode().String(), LadderLevelCause: ladderLevelCause(outcome)}
		if outcome.ExistingOrderId != nil {
			mapped.ExistingOrderID = codecs.FormatUint64ID(outcome.GetExistingOrderId())
		}
		return mapped
	}
	// A detail with no code says nothing about the outcome, but its field
	// violations still name what to fix on a recognised Connect code.
	var placeholderViolations []sdkerrors.FieldViolation
	futureCode := ""
	for _, detail := range connectErr.Details() {
		value, valueErr := detail.Value()
		if valueErr != nil {
			continue
		}
		if isPlaceholderDetail(value) {
			if orderDetail, ok := value.(*orderv1.ErrorDetail); ok {
				for _, violation := range orderDetail.GetViolations() {
					placeholderViolations = append(placeholderViolations, sdkerrors.FieldViolation{
						FieldPath: violation.GetFieldPath(),
						RuleID:    violation.GetRuleId(),
						Message:   violation.GetMessage(),
					})
				}
			}
			continue
		}
		// A code this SDK can't name stands only behind a recognised refusal.
		if isServerFailureCode(connectErr.Code()) && hasFutureCode(value) {
			if futureCode == "" {
				futureCode = futureCodeLabel(value)
			}
			continue
		}
		if requestDetail, ok := value.(*polyesterapiv1.RequestErrorDetail); ok && requestDetail.GetCode() == polyesterapiv1.RequestErrorCode_REQUEST_TOO_LARGE {
			// A request over the API's size limit, refused before it was
			// read: the same body is refused again, so it is no rate refusal
			// to wait out. Any other code reads as the plain refusal. The
			// contract sets the limit on every such refusal: one without it
			// is malformed, not a size refusal to act on.
			if requestDetail.GetLimitBytes() == 0 {
				return &sdkerrors.ResponseContractError{Msg: "a REQUEST_TOO_LARGE refusal without its limit_bytes"}
			}
			return &sdkerrors.APIError{Msg: msg, Code: sdkerrors.CodeRequestTooLarge, LimitBytes: requestDetail.GetLimitBytes()}
		}
		if authDetail, ok := value.(*authv1.AuthErrorDetail); ok {
			codeName := authDetail.GetCode().String()
			detailMsg := authDetail.GetMessage()
			if detailMsg == "" {
				detailMsg = msg
			}
			switch authDetail.GetCode() {
			case authv1.AuthErrorCode_AUTH_SUBACCOUNT_OWNER_ROLE_LOCKED:
				return &sdkerrors.SubaccountOwnerRoleLockedError{Msg: detailMsg}
			case authv1.AuthErrorCode_AUTH_SESSION_KIND_NOT_ALLOWED:
				return &sdkerrors.SessionRequiredError{Msg: detailMsg}
			case authv1.AuthErrorCode_AUTH_TIMESTAMP_SKEW:
				return &sdkerrors.TimestampSkewError{Msg: detailMsg, Window: timestampWindowFromProto(authDetail.GetTimestampWindow())}
			case authv1.AuthErrorCode_AUTH_REPLAY_CAPACITY_EXHAUSTED:
				// Valid credentials, not processed: a retryable rate refusal.
				return rateLimitError(detailMsg, codeName, nil, headerRetry)
			}
			if apiKeyRefusalCodes[authDetail.GetCode()] {
				return &sdkerrors.AuthError{Msg: detailMsg, Code: codeName}
			}
			apiErr := &sdkerrors.APIError{Msg: detailMsg, Code: codeName}
			if authDetail.RetryAfter != nil {
				retryAt := authDetail.GetRetryAfter().AsTime()
				apiErr.RetryAt = &retryAt
			}
			return apiErr
		}
		if rl, ok := value.(*ratelimitv1.RateLimitDetail); ok {
			return rateLimitError(msg, "", rateLimitDetailFromProto(rl), headerRetry)
		}
		if orderDetail, ok := value.(*orderv1.ErrorDetail); ok {
			unknown := strings.HasSuffix(orderDetail.GetCode().String(), "_OUTCOME_UNKNOWN")
			rl := rateLimitDetailFromProto(orderDetail.GetRateLimit())
			cause := ladderLevelCause(orderDetail)
			if !unknown && (rl != nil || isRetryableAdmissionCode(orderDetail.GetCode().String())) {
				rateErr := rateLimitError(msg, orderDetail.GetCode().String(), rl, headerRetry)
				rateErr.LadderLevelCause = cause
				return rateErr
			}
			code := orderDetail.GetCode().String()
			// Backoff hints and unknown mutation outcomes take precedence.
			if connectErr.Code() == connect.CodeUnavailable && !strings.HasSuffix(code, "_OUTCOME_UNKNOWN") && code != "ERROR_CODE_ACCOUNT_ROUTING_INVALID" {
				// An admission refusal reads from its code, whatever status
				// the answer had.
				return &sdkerrors.ServerError{Msg: msg, Code: code, AdmissionRefused: code == "ERROR_CODE_UNAVAILABLE" || isSpecificAdmissionRefusal(code)}
			}
			if len(orderDetail.GetViolations()) > 0 && !unknown {
				violations := make([]sdkerrors.FieldViolation, 0, len(orderDetail.GetViolations()))
				for _, violation := range orderDetail.GetViolations() {
					violations = append(violations, sdkerrors.FieldViolation{
						FieldPath: violation.GetFieldPath(),
						RuleID:    violation.GetRuleId(),
						Message:   violation.GetMessage(),
					})
				}
				return &sdkerrors.ValidationError{
					Msg: msg, Code: code, Metadata: sdkerrors.ViolationMetadata(violations), Violations: violations, LadderLevelCause: cause,
				}
			}
			if isAuthLikeErrorCode(code) {
				return &sdkerrors.AuthError{Msg: msg, Code: code}
			}
			apiErr := &sdkerrors.APIError{Msg: msg, Code: code, LadderLevelCause: cause}
			if orderDetail.ExistingOrderId != nil {
				apiErr.ExistingOrderID = codecs.FormatUint64ID(orderDetail.GetExistingOrderId())
			}
			return apiErr
		}
		if requestViolations, ok := value.(*validate.Violations); ok && len(requestViolations.GetViolations()) > 0 {
			violations := make([]sdkerrors.FieldViolation, 0, len(requestViolations.GetViolations()))
			for _, violation := range requestViolations.GetViolations() {
				violations = append(violations, sdkerrors.FieldViolation{
					FieldPath: protovalidateFieldPath(violation.GetField()),
					RuleID:    violation.GetRuleId(),
					Message:   violation.GetMessage(),
				})
			}
			return &sdkerrors.ValidationError{
				Msg: msg, Code: connectErr.Code().String(), Metadata: sdkerrors.ViolationMetadata(violations), Violations: violations,
			}
		}
		if withdrawDetail, ok := value.(*withdrawv1.ErrorDetail); ok {
			code := withdrawDetail.GetCode().String()
			if code == withdrawv1.ErrorCode_ERROR_CODE_RATE_LIMIT_EXCEEDED.String() {
				return rateLimitError(msg, code, nil, headerRetry)
			}
			if connectErr.Code() == connect.CodeUnavailable {
				return &sdkerrors.ServerError{Msg: msg, Code: code, CodeSource: typedRefusalSource(connectErr, withdrawDetail)}
			}
			if isAuthLikeErrorCode(code) {
				return &sdkerrors.AuthError{Msg: msg, Code: code}
			}
			apiErr := &sdkerrors.APIError{Msg: msg, Code: code, FieldPath: withdrawDetail.GetFieldPath()}
			if floor := withdrawDetail.GetMinFloorE18(); floor != nil {
				apiErr.MinFloorE18 = codecs.U128ToStr(floor.GetHi(), floor.GetLo())
			}
			if withdrawDetail.PayableFrom != nil {
				payable := withdrawDetail.GetPayableFrom().AsTime()
				apiErr.PayableFrom = &payable
			}
			return apiErr
		}
		if alertDetail, ok := value.(*alertsv1.ErrorDetail); ok {
			code := alertDetail.GetCode().String()
			switch alertDetail.GetCode() {
			case alertsv1.ErrorCode_PRICE_UNAVAILABLE:
				// No current price to record: nothing was created or changed.
				return &sdkerrors.ServerError{Msg: msg, Code: code, AdmissionRefused: true}
			case alertsv1.ErrorCode_SERVICE_UNAVAILABLE:
				return &sdkerrors.ServerError{Msg: msg, Code: code, CodeSource: typedRefusalSource(connectErr, alertDetail)}
			case alertsv1.ErrorCode_SUBACCOUNT_KEY_REFUSED:
				return &sdkerrors.AuthError{Msg: msg, Code: code}
			}
			return &sdkerrors.APIError{Msg: msg, Code: code}
		}
		if ledgerDetail, ok := value.(*ledgerrdv1.ErrorDetail); ok {
			code := ledgerDetail.GetCode().String()
			if connectErr.Code() == connect.CodeUnavailable {
				return &sdkerrors.ServerError{Msg: msg, Code: code, CodeSource: typedRefusalSource(connectErr, ledgerDetail)}
			}
			if isAuthLikeErrorCode(code) {
				return &sdkerrors.AuthError{Msg: msg, Code: code}
			}
			return &sdkerrors.APIError{Msg: msg, Code: code}
		}
		if transferDetail, ok := value.(*transferv1.ErrorDetail); ok {
			code := transferDetail.GetCode().String()
			if code == transferv1.ErrorCode_ERROR_CODE_RATE_LIMIT_EXCEEDED.String() {
				return rateLimitError(msg, code, nil, headerRetry)
			}
			if connectErr.Code() == connect.CodeUnavailable {
				return &sdkerrors.ServerError{Msg: msg, Code: code, CodeSource: typedRefusalSource(connectErr, transferDetail)}
			}
			if isAuthLikeErrorCode(code) {
				return &sdkerrors.AuthError{Msg: msg, Code: code}
			}
			return &sdkerrors.APIError{Msg: msg, Code: code}
		}
	}
	if useragent.IsCloudflareBrowserBan(msg) {
		return &sdkerrors.TransportError{Msg: useragent.Cloudflare1010Message()}
	}
	if strings.HasSuffix(msg, "_OUTCOME_UNKNOWN") {
		return &sdkerrors.APIError{Msg: msg, Code: orderErrorCodeFromName(msg)}
	}
	messageCode := strings.TrimSpace(msg)
	if IsPlaceholderCode(messageCode) {
		messageCode = ""
	}
	namedCode := orderErrorCodeFromName(messageCode)
	if connectErr.Code() == connect.CodeUnavailable && namedCode == "ERROR_CODE_ACCOUNT_ROUTING_INVALID" {
		return &sdkerrors.APIError{Msg: msg, Code: namedCode}
	}
	if connectErr.Code() == connect.CodeUnavailable && isSpecificAdmissionRefusal(namedCode) {
		return &sdkerrors.ServerError{Msg: msg, Code: namedCode, AdmissionRefused: true}
	}
	if connectErr.Code() == connect.CodeResourceExhausted {
		if isRetryableAdmissionCode(namedCode) {
			return rateLimitError(msg, namedCode, nil, headerRetry)
		}
		// Only the typed RequestErrorDetail or the REST problem is the size
		// refusal: a message that reads REQUEST_TOO_LARGE stays a rate refusal.
		if _, known := orderv1.ErrorCode_value[namedCode]; known || (isServiceCodeMessage(messageCode) && messageCode != sdkerrors.CodeRequestTooLarge) {
			return &sdkerrors.APIError{Msg: msg, Code: namedCode}
		}
	}

	switch connectErr.Code() {
	case connect.CodeUnauthenticated:
		return &sdkerrors.AuthError{Msg: msg, Code: "unauthenticated"}
	case connect.CodePermissionDenied:
		return &sdkerrors.AuthError{Msg: msg, Code: "permission_denied"}
	case connect.CodeUnavailable, connect.CodeInternal, connect.CodeUnknown, connect.CodeDataLoss:
		// The contract never uses unknown or data_loss, and neither says a
		// request was refused.
		return &sdkerrors.ServerError{Msg: msg, Code: futureCode}
	case connect.CodeResourceExhausted:
		code := orderErrorCodeFromName(messageCode)
		if _, known := orderv1.ErrorCode_value[code]; !known {
			code = ""
		}
		return rateLimitError(msg, code, nil, headerRetry)
	case connect.CodeCanceled, connect.CodeDeadlineExceeded:
		return &sdkerrors.TransportError{Msg: msg}
	case connect.CodeInvalidArgument, connect.CodeFailedPrecondition, connect.CodeOutOfRange:
		return &sdkerrors.ValidationError{
			Msg: msg, Code: connectErr.Code().String(), Metadata: sdkerrors.ViolationMetadata(placeholderViolations), Violations: placeholderViolations,
		}
	case connect.CodeUnimplemented:
		if _, ok := routeNotFoundMessages[strings.ToLower(strings.TrimSpace(msg))]; ok {
			return &sdkerrors.RouteNotFoundError{}
		}
		return &sdkerrors.NotImplementedError{Msg: msg}
	}
	return &sdkerrors.APIError{Msg: msg, Code: connectErr.Code().String()}
}

// timestampWindowFromProto decodes a timestamp-skew refusal's window; nil
// when the refusal names none.
func timestampWindowFromProto(msg *authv1.TimestampWindow) *sdkerrors.TimestampWindow {
	if msg == nil {
		return nil
	}
	return &sdkerrors.TimestampWindow{ServerTimeMs: msg.GetServerTimeMs(), RecvWindowMs: msg.GetRecvWindowMs(), MaxFutureMs: msg.GetMaxFutureMs()}
}

// ladderLevelCause reads why a ladder order can't be placed from an order
// ErrorDetail: its cause_code in the form of the detail's code, and its
// level_index with presence kept, since 0 is the first order.
func ladderLevelCause(detail *orderv1.ErrorDetail) sdkerrors.LadderLevelCause {
	var cause sdkerrors.LadderLevelCause
	if detail.GetCauseCode() != orderv1.ErrorCode_ERROR_CODE_UNSPECIFIED {
		cause.CauseCode = detail.GetCauseCode().String()
	}
	if detail.LevelIndex != nil {
		index := detail.GetLevelIndex()
		cause.LevelIndex = &index
	}
	return cause
}

// isServerFailureCode reports a Connect code that says the server failed, not
// that it refused the request.
func isServerFailureCode(code connect.Code) bool {
	switch code {
	case connect.CodeInternal, connect.CodeUnknown, connect.CodeDataLoss, connect.CodeCanceled, connect.CodeUnavailable, connect.CodeDeadlineExceeded:
		return true
	}
	return false
}

// apiKeyRefusalCodes are the auth.v1 reasons an API key's credentials were
// refused, other than AUTH_TIMESTAMP_SKEW (a TimestampSkewError). They map to
// *AuthError, as the bare unauthenticated answer that preceded them did.
var apiKeyRefusalCodes = map[authv1.AuthErrorCode]bool{
	authv1.AuthErrorCode_AUTH_SIGNATURE_INVALID:    true,
	authv1.AuthErrorCode_AUTH_MISSING_HEADERS:      true,
	authv1.AuthErrorCode_AUTH_KEY_EXPIRED:          true,
	authv1.AuthErrorCode_AUTH_KEY_DISABLED:         true,
	authv1.AuthErrorCode_AUTH_IP_NOT_ALLOWED:       true,
	authv1.AuthErrorCode_AUTH_API_KEY_NOT_ACCEPTED: true,
	authv1.AuthErrorCode_AUTH_RECV_WINDOW_INVALID:  true,
	authv1.AuthErrorCode_AUTH_REPLAY_DETECTED:      true,
}

// protovalidateFieldPath renders a buf.validate field path the way order
// ErrorDetail violations name fields: "items[2].limit_gtc.price_ticks".
func protovalidateFieldPath(path *validate.FieldPath) string {
	var b strings.Builder
	for _, element := range path.GetElements() {
		if b.Len() > 0 {
			b.WriteByte('.')
		}
		b.WriteString(element.GetFieldName())
		switch subscript := element.GetSubscript().(type) {
		case *validate.FieldPathElement_Index:
			fmt.Fprintf(&b, "[%d]", subscript.Index)
		case *validate.FieldPathElement_BoolKey:
			fmt.Fprintf(&b, "[%t]", subscript.BoolKey)
		case *validate.FieldPathElement_IntKey:
			fmt.Fprintf(&b, "[%d]", subscript.IntKey)
		case *validate.FieldPathElement_UintKey:
			fmt.Fprintf(&b, "[%d]", subscript.UintKey)
		case *validate.FieldPathElement_StringKey:
			fmt.Fprintf(&b, "[%q]", subscript.StringKey)
		}
	}
	return b.String()
}

// TimestampSkewCode is the API-key authentication code for a signature
// timestamp outside the server's skew window.
const TimestampSkewCode = "TIMESTAMP_SKEW"

func isAuthLikeErrorCode(code string) bool {
	return strings.Contains(code, "UNAUTHENTICATED") ||
		strings.Contains(code, "PERMISSION") ||
		strings.Contains(code, "API_KEY") ||
		strings.Contains(code, "WALLET_BINDING") ||
		strings.Contains(code, "POLICY")
}

// orderErrorCodeFromName normalizes known enum names and preserves future names.
func orderErrorCodeFromName(code string) string {
	if _, known := orderv1.ErrorCode_value["ERROR_CODE_"+code]; known {
		return "ERROR_CODE_" + code
	}
	return code
}

// Unrecognized code-only messages must not inherit generic quota retryability.
func isServiceCodeMessage(msg string) bool {
	if msg == "" || msg[0] < 'A' || msg[0] > 'Z' {
		return false
	}
	for _, r := range msg[1:] {
		switch {
		case r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_':
		default:
			return false
		}
	}
	return true
}

func isRetryableAdmissionCode(code string) bool {
	switch strings.TrimPrefix(code, "ERROR_CODE_") {
	case "OVERLOADED", "SYSTEM_RATE_LIMITED", "RATE_LIMIT_EXCEEDED", "PRICE_LEVELS_EXHAUSTED", "CLIENT_ORDER_IDS_EXHAUSTED", "CLIENT_ORDER_ID_CAPACITY_REACHED", "MARKET_LADDER_LIMIT_EXCEEDED", "MARKET_TRIGGER_LIMIT_EXCEEDED", "MARKET_TWAP_LIMIT_EXCEEDED", "NEW_ACCOUNT_CAPACITY_REACHED", "REQUEST_RECEIPTS_EXHAUSTED", "RESERVATION_CAPACITY_REACHED", "TRIGGER_CAPACITY_REACHED", "TRIGGER_CONTROL_SHARE_EXHAUSTED", "TWAP_CAPACITY_REACHED":
		return true
	default:
		return false
	}
}

func isSpecificAdmissionRefusal(code string) bool {
	switch strings.TrimPrefix(code, "ERROR_CODE_") {
	case "FEE_SCHEDULE_UNAVAILABLE", "POLICY_UNAVAILABLE", "RATE_LIMIT_POLICY_UNAVAILABLE", "PAIR_BACKLOGGED", "SETTLEMENT_BACKLOGGED", "ORDER_MODIFY_IN_PROGRESS", "ORDER_PLACEMENT_PENDING", "TRIGGER_CONTROLS_BUSY", "TRIGGER_CONTROL_IN_PROGRESS", "QUICKSWAP_ACCOUNT_OWNER_UNKNOWN":
		return true
	default:
		return false
	}
}

func isFinalResourceCode(code string) bool {
	switch strings.TrimPrefix(code, "ERROR_CODE_") {
	case "ORDER_SEQUENCE_EXHAUSTED", "TRIGGER_FIRE_LIMIT_REACHED", "QUICKSWAP_CAPACITY_REACHED", "ACCOUNT_RESERVATION_LIMIT_REACHED", "TRIGGER_LIMIT_EXCEEDED":
		return true
	default:
		return false
	}
}
