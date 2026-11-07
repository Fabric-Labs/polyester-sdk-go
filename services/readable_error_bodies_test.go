package services

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Fabric-Labs/polyester-sdk-go/auth"
	"github.com/Fabric-Labs/polyester-sdk-go/catalogs"
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	orderv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/orders/v1"
	ratelimitv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/polyester/ratelimit/v1"
	triggersv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/triggers/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
	"github.com/Fabric-Labs/polyester-sdk-go/transport"
	"google.golang.org/protobuf/proto"
)

// An HTTP error with no recognised code proves nothing about a mutation. Each
// case goes through a public read and a public create against a local server.
// Only the requested procedure is counted and every expectation needs at least
// one request, so a case that never reached the server fails.

const (
	readErrorPath  = "/triggers.v1.TriggersService/GetTrigger"
	writeErrorPath = "/orders.v1.OrdersService/CreateOrder"
	jsonType       = "application/json"
	problemType    = "application/problem+json"
)

type errorBody struct{ name, contentType, body string }

var noCodeBodies = []errorBody{
	{"no-code", jsonType, `{"message":"m"}`},
	{"empty-code", jsonType, `{"code":"","message":"m"}`},
	{"null-code", jsonType, `{"code":null,"message":"m"}`},
	{"int-code", jsonType, `{"code":5,"message":"m"}`},
	{"empty-object", jsonType, `{}`},
	{"problem-no-code", problemType, `{"title":"t","detail":"d"}`},
	{"json-array", jsonType, `[1]`},
	{"broken-json", jsonType, `{broken`},
	{"unknown-detail", jsonType, `{"message":"m","details":[{"type":"x.y.Unknown","value":"AA"}]}`},
	{"lowercase-code", jsonType, `{"code":"nope","message":"m"}`},
}

var bareBodies = []errorBody{
	{"html", "text/html", "<html>oops</html>"},
	{"text", "text/plain", "oops"},
	{"empty-text", "text/plain", ""},
	{"empty-json", jsonType, ""},
}

var (
	unrecognisedBody   = errorBody{"unrecognised", problemType, `{"code":"NOPE_NOT_A_CODE","detail":"d"}`}
	connectUnknownBody = errorBody{"connect-unknown", jsonType, `{"code":"unknown","message":"m"}`}
)

// callWithErrorBody answers one public call and returns its error and the
// requests made. A read gets a success after failFor failures; failFor < 0
// never succeeds.
func callWithErrorBody(t *testing.T, op string, status int, body errorBody, failFor int32) (int32, error) {
	t.Helper()
	path := writeErrorPath
	if op == "read" {
		path = readErrorPath
	}
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != path {
			http.NotFound(w, r)
			return
		}
		if n := calls.Add(1); failFor >= 0 && n > failFor {
			w.Header().Set("Content-Type", "application/proto")
			raw, err := proto.Marshal(&triggersv1.GetTriggerResponse{Trigger: &triggersv1.Trigger{TriggerId: 9}})
			if err != nil {
				t.Error(err)
				return
			}
			_, _ = w.Write(raw)
			return
		}
		w.Header().Set("Content-Type", body.contentType)
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body.body))
	}))
	defer srv.Close()
	creds, err := auth.LoadCredentials("ak_test", auth.GenerateEd25519Keypair().SecretKeyHex, false)
	if err != nil {
		t.Fatal(err)
	}
	cfg := transport.DefaultConfig()
	cfg.APIURL = srv.URL
	factory := transport.NewFactory(cfg, creds, nil)
	if op == "read" {
		_, err = NewTriggersService(factory, nil, nil, nil, nil, nil, nil).Get(context.Background(), nil, "9", nil)
		return calls.Load(), err
	}
	cats := catalogs.NewManager()
	if hydrateErr := cats.HydrateSpotConfig(map[string]any{"assets": []any{map[string]any{"asset": "BTC", "market_data_volume_scale": float64(8)}}, "pairs": []any{map[string]any{"symbol": "BTC-USDT", "symbol_id": float64(1), "base_asset": "BTC", "base_quantity_scale": float64(8), "quote_quantity_scale": float64(6)}}}); hydrateErr != nil {
		t.Fatal(hydrateErr)
	}
	id := uint32(1)
	price := models.PriceFromDecimal("100")
	qty := models.MustQtyScaled(16000)
	_, err = NewOrdersService(factory, cats, nil, nil, nil, nil, nil).Create(context.Background(), models.CreateOrderRequest{
		SymbolID: &id, Side: "buy", OrderType: "limit", Price: &price, Qty: models.QtyFromScaled(qty),
	}, nil)
	return calls.Load(), err
}

func assertErrorFlags(t *testing.T, err error, retryable, unknown bool) {
	t.Helper()
	if sdkerrors.IsRetryable(err) != retryable || sdkerrors.MutationOutcomeUnknown(err) != unknown {
		t.Fatalf("%T %v: retryable=%v unknown=%v, want %v %v", err, err, sdkerrors.IsRetryable(err), sdkerrors.MutationOutcomeUnknown(err), retryable, unknown)
	}
}

func eachOp(t *testing.T, name string, f func(t *testing.T, op string)) {
	t.Helper()
	for _, op := range []string{"read", "write"} {
		t.Run(name+"/"+op, func(t *testing.T) { f(t, op) })
	}
}

func TestNoRecognisedCodeIsAServerErrorWithOutcomeUnknown(t *testing.T) {
	bodies := append(append(append([]errorBody{}, noCodeBodies...), bareBodies...), unrecognisedBody, connectUnknownBody,
		errorBody{"connect-data-loss", jsonType, `{"code":"data_loss","message":"m"}`})
	for _, status := range []int{400, 402, 405, 409, 410, 412, 413, 418, 422, 431, 500, 501, 505, 520, 599} {
		for _, body := range bodies {
			eachOp(t, fmt.Sprintf("%d/%s", status, body.name), func(t *testing.T, op string) {
				calls, err := callWithErrorBody(t, op, status, body, -1)
				var server *sdkerrors.ServerError
				if !errors.As(err, &server) {
					t.Fatalf("got %T %v", err, err)
				}
				assertErrorFlags(t, err, false, true)
				// Not unavailable, so a read doesn't retry it either.
				if calls != 1 {
					t.Fatalf("calls=%d", calls)
				}
				if body.name == "unrecognised" && (server.Code != "NOPE_NOT_A_CODE" || server.Status != status) {
					t.Fatalf("code or status lost: %+v", server)
				}
			})
		}
	}
}

func TestCanceledConnectCodeStaysAnUnknownOutcome(t *testing.T) {
	eachOp(t, "500", func(t *testing.T, op string) {
		calls, err := callWithErrorBody(t, op, 500, errorBody{"canceled", jsonType, `{"code":"canceled","message":"m"}`}, -1)
		assertErrorFlags(t, err, false, true)
		if calls != 1 {
			t.Fatalf("calls=%d", calls)
		}
	})
}

func TestJSON404WithoutACodeIsNotAMissingRoute(t *testing.T) {
	for _, body := range append(append([]errorBody{}, noCodeBodies...), unrecognisedBody) {
		eachOp(t, body.name, func(t *testing.T, op string) {
			calls, err := callWithErrorBody(t, op, 404, body, -1)
			var server *sdkerrors.ServerError
			if !errors.As(err, &server) || server.Status != 404 {
				t.Fatalf("got %T %+v", err, err)
			}
			assertErrorFlags(t, err, false, true)
			if calls != 1 {
				t.Fatalf("calls=%d", calls)
			}
		})
	}
}

func TestBare404StaysAMissingRouteWithOutcomeUnknown(t *testing.T) {
	bodies := append(append([]errorBody{}, bareBodies...),
		errorBody{"route-message", jsonType, `{"message":"404 page not found"}`},
		errorBody{"route-phrase", jsonType, `{"message":"Not Found"}`})
	for _, body := range bodies {
		eachOp(t, body.name, func(t *testing.T, op string) {
			calls, err := callWithErrorBody(t, op, 404, body, -1)
			var route *sdkerrors.RouteNotFoundError
			if !errors.As(err, &route) {
				t.Fatalf("got %T %v", err, err)
			}
			assertErrorFlags(t, err, false, true)
			if calls != 1 {
				t.Fatalf("calls=%d", calls)
			}
		})
	}
}

func TestStatus401And403StayDefiniteAuthRefusals(t *testing.T) {
	bodies := append(append(append([]errorBody{}, noCodeBodies...), bareBodies...), unrecognisedBody, connectUnknownBody)
	for _, status := range []int{401, 403} {
		for _, body := range bodies {
			eachOp(t, fmt.Sprintf("%d/%s", status, body.name), func(t *testing.T, op string) {
				calls, err := callWithErrorBody(t, op, status, body, -1)
				var authErr *sdkerrors.AuthError
				if !errors.As(err, &authErr) || authErr.Status != status {
					t.Fatalf("got %T %+v", err, err)
				}
				assertErrorFlags(t, err, false, false)
				if calls != 1 {
					t.Fatalf("calls=%d", calls)
				}
			})
		}
	}
}

func TestUnavailableStatusesStayUnknownSendACreateOnceAndLetReadsRetry(t *testing.T) {
	bodies := append(append([]errorBody{}, noCodeBodies...), bareBodies...)
	for _, status := range []int{429, 502, 503, 504} {
		for _, body := range bodies {
			t.Run(fmt.Sprintf("%d/%s", status, body.name), func(t *testing.T) {
				calls, err := callWithErrorBody(t, "write", status, body, -1)
				var server *sdkerrors.ServerError
				if !errors.As(err, &server) {
					t.Fatalf("got %T %v", err, err)
				}
				assertErrorFlags(t, err, false, true)
				if calls != 1 {
					t.Fatalf("create sent %d times", calls)
				}
				calls, err = callWithErrorBody(t, "read", status, body, 2)
				if err != nil || calls != 3 {
					t.Fatalf("read: err=%v calls=%d, want a success on the third request", err, calls)
				}
			})
		}
	}
}

func TestWellFormedUnrecognised429StaysAKnownRefusal(t *testing.T) {
	for _, code := range []string{"FUTURE_CAPACITY", "NOPE_NOT_A_CODE"} {
		body := errorBody{code, problemType, fmt.Sprintf(`{"code":%q,"detail":"d"}`, code)}
		eachOp(t, code, func(t *testing.T, op string) {
			calls, err := callWithErrorBody(t, op, 429, body, -1)
			var api *sdkerrors.APIError
			if !errors.As(err, &api) || api.Code != code {
				t.Fatalf("got %T %+v", err, err)
			}
			assertErrorFlags(t, err, false, false)
			if calls != 1 {
				t.Fatalf("calls=%d", calls)
			}
		})
	}
}

func TestTriggerNotFoundKeepsItsTypedError(t *testing.T) {
	for _, tc := range []struct {
		body errorBody
		code string
	}{
		{errorBody{"rest", problemType, `{"code":"TRIGGER_NOT_FOUND","detail":"d"}`}, "ERROR_CODE_TRIGGER_NOT_FOUND"},
		{errorBody{"connect", jsonType, `{"code":"not_found","message":"m"}`}, "not_found"},
	} {
		eachOp(t, tc.body.name, func(t *testing.T, op string) {
			calls, err := callWithErrorBody(t, op, 404, tc.body, -1)
			var api *sdkerrors.APIError
			if !errors.As(err, &api) || api.Code != tc.code {
				t.Fatalf("got %T %+v", err, err)
			}
			assertErrorFlags(t, err, false, false)
			if calls != 1 {
				t.Fatalf("calls=%d", calls)
			}
		})
	}
}

// Every published REST code is sent once and keeps its meaning.
func TestEveryContractCodeKeepsItsMeaningOnACreate(t *testing.T) {
	raw, err := os.ReadFile("../transport/testdata/rest_error_codes.json")
	if err != nil {
		t.Fatal(err)
	}
	var catalog struct {
		Codes []struct {
			Code    string `json:"code"`
			Status  int    `json:"status"`
			Connect string `json:"connect"`
		} `json:"codes"`
	}
	if err = json.Unmarshal(raw, &catalog); err != nil {
		t.Fatal(err)
	}
	if len(catalog.Codes) == 0 {
		t.Fatal("empty catalog")
	}
	for _, entry := range catalog.Codes {
		t.Run(fmt.Sprintf("%d/%s/%s", entry.Status, entry.Code, entry.Connect), func(t *testing.T) {
			body := errorBody{entry.Code, problemType, fmt.Sprintf(`{"code":%q,"detail":"d"}`, entry.Code)}
			calls, err := callWithErrorBody(t, "write", entry.Status, body, -1)
			if calls != 1 {
				t.Fatalf("calls=%d", calls)
			}
			var api *sdkerrors.APIError
			if errors.As(err, &api) && api.Code == "unknown" {
				t.Fatalf("code lost: %+v", api)
			}
			var server *sdkerrors.ServerError
			switch {
			case entry.Status == 501:
				var notImplemented *sdkerrors.NotImplementedError
				if !errors.As(err, &notImplemented) {
					t.Fatalf("got %T %v", err, err)
				}
				assertErrorFlags(t, err, false, false)
			case entry.Status == 500:
				if !errors.As(err, &server) || server.Status != 500 || !strings.HasSuffix(server.Code, entry.Code) {
					t.Fatalf("got %T %+v", err, err)
				}
				assertErrorFlags(t, err, false, true)
			case entry.Status == 504:
				assertErrorFlags(t, err, false, true)
			case entry.Status < 500:
				var route *sdkerrors.RouteNotFoundError
				if errors.As(err, &server) || errors.As(err, &route) || sdkerrors.MutationOutcomeUnknown(err) {
					t.Fatalf("got %T %+v", err, err)
				}
			}
		})
	}
}

var placeholderCodes = []string{"UNSPECIFIED", "ERROR_CODE_UNSPECIFIED", "AUTH_UNSPECIFIED"}

var serviceDetailTypeNames = []string{
	"orders.v1.ErrorDetail",
	"auth.v1.AuthErrorDetail",
	"chain.withdraw.v1.ErrorDetail",
	"transfer.v1.ErrorDetail",
	"ledger.read.v1.ErrorDetail",
	"polyester.ratelimit.v1.RateLimitDetail",
}

// detailBody is a JSON error body with one service detail of these raw bytes.
func detailBody(name, detailType string, value []byte, code string) errorBody {
	body := map[string]any{
		"message": "m",
		"details": []map[string]string{{"type": detailType, "value": base64.StdEncoding.EncodeToString(value)}},
	}
	if code != "" {
		body["code"] = code
	}
	raw, _ := json.Marshal(body)
	return errorBody{name, jsonType, string(raw)}
}

func wantServerError(t *testing.T, err error, calls int32) {
	t.Helper()
	var server *sdkerrors.ServerError
	if !errors.As(err, &server) {
		t.Fatalf("got %T %+v", err, err)
	}
	assertErrorFlags(t, err, false, true)
	if calls != 1 {
		t.Fatalf("calls=%d", calls)
	}
}

var noCodeStatuses = []int{400, 402, 404, 405, 409, 410, 412, 413, 418, 422, 431, 500, 501, 505, 520, 599}

func TestPlaceholderCodeIsNoCode(t *testing.T) {
	for _, code := range placeholderCodes {
		body := errorBody{code, problemType, fmt.Sprintf(`{"code":%q,"detail":"d"}`, code)}
		for _, status := range noCodeStatuses {
			eachOp(t, fmt.Sprintf("%s/%d", code, status), func(t *testing.T, op string) {
				calls, err := callWithErrorBody(t, op, status, body, -1)
				wantServerError(t, err, calls)
				var server *sdkerrors.ServerError
				if errors.As(err, &server) && server.Code != "" {
					t.Fatalf("placeholder kept as a code: %+v", server)
				}
			})
		}
		for _, status := range []int{401, 403} {
			t.Run(fmt.Sprintf("%s/%d", code, status), func(t *testing.T) {
				calls, err := callWithErrorBody(t, "write", status, body, -1)
				var authErr *sdkerrors.AuthError
				if !errors.As(err, &authErr) || authErr.Status != status {
					t.Fatalf("got %T %+v", err, err)
				}
				assertErrorFlags(t, err, false, false)
				if calls != 1 {
					t.Fatalf("calls=%d", calls)
				}
			})
		}
		for _, status := range []int{429, 502, 503, 504} {
			t.Run(fmt.Sprintf("%s/%d", code, status), func(t *testing.T) {
				calls, err := callWithErrorBody(t, "write", status, body, -1)
				wantServerError(t, err, calls)
				calls, err = callWithErrorBody(t, "read", status, body, 2)
				if err != nil || calls != 3 {
					t.Fatalf("read: err=%v calls=%d", err, calls)
				}
			})
		}
	}
}

func TestDetailWithNoCodeIsNoCode(t *testing.T) {
	for _, detailType := range serviceDetailTypeNames {
		for name, value := range map[string][]byte{"empty": nil, "undecodable": {0}} {
			body := detailBody(name, detailType, value, "")
			for _, status := range noCodeStatuses {
				eachOp(t, fmt.Sprintf("%s/%s/%d", detailType, name, status), func(t *testing.T, op string) {
					calls, err := callWithErrorBody(t, op, status, body, -1)
					wantServerError(t, err, calls)
				})
			}
			for _, status := range []int{401, 403} {
				t.Run(fmt.Sprintf("%s/%s/%d", detailType, name, status), func(t *testing.T) {
					calls, err := callWithErrorBody(t, "write", status, body, -1)
					var authErr *sdkerrors.AuthError
					if !errors.As(err, &authErr) {
						t.Fatalf("got %T %+v", err, err)
					}
					assertErrorFlags(t, err, false, false)
					if calls != 1 {
						t.Fatalf("calls=%d", calls)
					}
				})
			}
			for _, status := range []int{429, 502, 503, 504} {
				t.Run(fmt.Sprintf("%s/%s/%d", detailType, name, status), func(t *testing.T) {
					calls, err := callWithErrorBody(t, "write", status, body, -1)
					wantServerError(t, err, calls)
				})
			}
		}
	}
}

func TestEmptyDetailDoesNotTurnAServerFailureIntoARefusal(t *testing.T) {
	for _, detailType := range serviceDetailTypeNames {
		for _, tc := range []struct {
			status int
			code   string
		}{{500, "internal"}, {500, "unknown"}, {500, "data_loss"}, {503, "unavailable"}} {
			t.Run(fmt.Sprintf("%s/%s", detailType, tc.code), func(t *testing.T) {
				calls, err := callWithErrorBody(t, "write", tc.status, detailBody("d", detailType, nil, tc.code), -1)
				wantServerError(t, err, calls)
			})
		}
	}
}

func TestRecognisedConnectCodeStandsWithoutItsEmptyDetail(t *testing.T) {
	for _, detailType := range serviceDetailTypeNames {
		for _, tc := range []struct {
			status int
			code   string
		}{{400, "invalid_argument"}, {404, "not_found"}, {409, "already_exists"}} {
			t.Run(fmt.Sprintf("%s/%s", detailType, tc.code), func(t *testing.T) {
				calls, err := callWithErrorBody(t, "write", tc.status, detailBody("d", detailType, nil, tc.code), -1)
				var server *sdkerrors.ServerError
				var rate *sdkerrors.RateLimitError
				if errors.As(err, &server) || errors.As(err, &rate) {
					t.Fatalf("got %T %+v", err, err)
				}
				if code := errorCodeOf(err); code != tc.code {
					t.Fatalf("code=%q: %T %+v", code, err, err)
				}
				assertErrorFlags(t, err, false, false)
				if calls != 1 {
					t.Fatalf("calls=%d", calls)
				}
			})
		}
	}
}

func errorCodeOf(err error) string {
	var api *sdkerrors.APIError
	if errors.As(err, &api) {
		return api.Code
	}
	var validation *sdkerrors.ValidationError
	if errors.As(err, &validation) {
		return validation.Code
	}
	return ""
}

func TestPlaceholderDoesNotNameAResourceRefusal(t *testing.T) {
	for _, body := range []errorBody{
		detailBody("detail", "orders.v1.ErrorDetail", nil, "resource_exhausted"),
		{"message-prefixed", jsonType, `{"code":"resource_exhausted","message":"ERROR_CODE_UNSPECIFIED"}`},
		{"message-short", jsonType, `{"code":"resource_exhausted","message":"UNSPECIFIED"}`},
	} {
		t.Run(body.name, func(t *testing.T) {
			calls, err := callWithErrorBody(t, "write", 429, body, -1)
			var rate *sdkerrors.RateLimitError
			if !errors.As(err, &rate) || rate.Code != "" {
				t.Fatalf("got %T %+v", err, err)
			}
			if calls != 1 {
				t.Fatalf("calls=%d", calls)
			}
		})
	}
}

func TestRealTypedDetailIsStillACodeControl(t *testing.T) {
	raw, err := proto.Marshal(&orderv1.ErrorDetail{Code: orderv1.ErrorCode_ERROR_CODE_BAD_PRICE})
	if err != nil {
		t.Fatal(err)
	}
	for _, status := range []int{400, 500} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			calls, callErr := callWithErrorBody(t, "write", status, detailBody("real", "orders.v1.ErrorDetail", raw, ""), -1)
			if code := errorCodeOf(callErr); code != "ERROR_CODE_BAD_PRICE" {
				t.Fatalf("code=%q: %T %+v", code, callErr, callErr)
			}
			assertErrorFlags(t, callErr, false, false)
			if calls != 1 {
				t.Fatalf("calls=%d", calls)
			}
		})
	}
}

func TestConnectRefusalCodeAt500StaysKnownControl(t *testing.T) {
	calls, err := callWithErrorBody(t, "write", 500, errorBody{"connect", jsonType, `{"code":"invalid_argument","message":"m"}`}, -1)
	if code := errorCodeOf(err); code != "invalid_argument" {
		t.Fatalf("code=%q: %T %+v", code, err, err)
	}
	assertErrorFlags(t, err, false, false)
	if calls != 1 {
		t.Fatalf("calls=%d", calls)
	}
}

func TestBackoffDetailWithoutACodeStaysARateLimitControl(t *testing.T) {
	delay := uint64(1250)
	backoff := &ratelimitv1.RateLimitDetail{RetryAfterMs: &delay}
	nested, err := proto.Marshal(&orderv1.ErrorDetail{RateLimit: backoff})
	if err != nil {
		t.Fatal(err)
	}
	standalone, err := proto.Marshal(backoff)
	if err != nil {
		t.Fatal(err)
	}
	for _, body := range []errorBody{
		detailBody("nested", "orders.v1.ErrorDetail", nested, "resource_exhausted"),
		detailBody("standalone", "polyester.ratelimit.v1.RateLimitDetail", standalone, "resource_exhausted"),
	} {
		t.Run(body.name, func(t *testing.T) {
			calls, callErr := callWithErrorBody(t, "write", 429, body, -1)
			var rate *sdkerrors.RateLimitError
			if !errors.As(callErr, &rate) || rate.RetryAfter == nil || *rate.RetryAfter != 1.25 {
				t.Fatalf("got %T %+v", callErr, callErr)
			}
			assertErrorFlags(t, callErr, true, false)
			if calls != 1 {
				t.Fatalf("calls=%d", calls)
			}
		})
	}
}

func TestPlaceholderDetailKeepsFieldViolationsOnARecognisedCode(t *testing.T) {
	raw, err := proto.Marshal(&orderv1.ErrorDetail{Violations: []*orderv1.FieldViolation{{FieldPath: "x", RuleId: "r", Message: "v"}}})
	if err != nil {
		t.Fatal(err)
	}
	calls, callErr := callWithErrorBody(t, "write", 400, detailBody("violations", "orders.v1.ErrorDetail", raw, "invalid_argument"), -1)
	var validation *sdkerrors.ValidationError
	if !errors.As(callErr, &validation) || validation.Code != "invalid_argument" || len(validation.Violations) != 1 || validation.Violations[0].FieldPath != "x" {
		t.Fatalf("got %T %+v", callErr, callErr)
	}
	assertErrorFlags(t, callErr, false, false)
	if calls != 1 {
		t.Fatalf("calls=%d", calls)
	}
	calls, callErr = callWithErrorBody(t, "write", 500, detailBody("violations", "orders.v1.ErrorDetail", raw, ""), -1)
	wantServerError(t, callErr, calls)
}
