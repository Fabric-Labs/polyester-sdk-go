package transport

import (
	"encoding/base64"
	"encoding/json"
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	orderv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/orders/v1"
	ratelimitv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/polyester/ratelimit/v1"
	"google.golang.org/protobuf/proto"
	"os"
	"testing"
)

func TestRetryContractCatalog(t *testing.T) {
	var catalog struct {
		BadRequests []struct {
			Catalog, Code string
			Retryable     bool
		} `json:"bad_requests"`
		Resources []struct {
			Code      string
			Status    int
			Retryable bool
		}
	}
	raw, err := os.ReadFile("testdata/retry_contract.json")
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(raw, &catalog); err != nil {
		t.Fatal(err)
	}
	for _, tc := range catalog.BadRequests {
		t.Run("400/"+tc.Catalog+"/"+tc.Code, func(t *testing.T) {
			body, _ := json.Marshal(map[string]string{"code": tc.Code, "detail": "refused"})
			e := callMeAgainst(t, 400, "application/problem+json", string(body))
			if sdkerrors.IsRetryable(e) != tc.Retryable || sdkerrors.MutationOutcomeUnknown(e) {
				t.Fatalf("%T %v retryable=%v unknown=%v", e, e, sdkerrors.IsRetryable(e), sdkerrors.MutationOutcomeUnknown(e))
			}
		})
	}
	for _, tc := range catalog.Resources {
		for _, shape := range []string{"rest", "message", "detail"} {
			t.Run(tc.Code+"/"+shape, func(t *testing.T) {
				body := map[string]any{"code": tc.Code, "detail": "refused"}
				if shape != "rest" {
					body = map[string]any{"code": "resource_exhausted", "message": tc.Code}
				}
				if shape == "detail" {
					data, _ := proto.Marshal(&orderv1.ErrorDetail{Code: orderv1.ErrorCode(orderv1.ErrorCode_value["ERROR_CODE_"+tc.Code])})
					body["details"] = []map[string]string{{"type": "orders.v1.ErrorDetail", "value": base64.StdEncoding.EncodeToString(data)}}
				}
				raw, _ := json.Marshal(body)
				e := callMeAgainst(t, tc.Status, "application/json", string(raw))
				if sdkerrors.IsRetryable(e) != tc.Retryable || sdkerrors.MutationOutcomeUnknown(e) {
					t.Fatalf("%T %v retryable=%v unknown=%v", e, e, sdkerrors.IsRetryable(e), sdkerrors.MutationOutcomeUnknown(e))
				}
			})
		}
	}
}

func TestRetryContractUnavailableRefusal(t *testing.T) {
	for _, name := range []string{"UNAVAILABLE", "FEE_SCHEDULE_UNAVAILABLE", "POLICY_UNAVAILABLE", "RATE_LIMIT_POLICY_UNAVAILABLE"} {
		for _, shape := range []string{"rest", "detail"} {
			t.Run(name+"/"+shape, func(t *testing.T) {
				body := map[string]any{"code": name, "detail": "not admitted"}
				if shape == "detail" {
					data, _ := proto.Marshal(&orderv1.ErrorDetail{Code: orderv1.ErrorCode(orderv1.ErrorCode_value["ERROR_CODE_"+name])})
					body = map[string]any{"code": "unavailable", "message": "not admitted", "details": []map[string]string{{"type": "orders.v1.ErrorDetail", "value": base64.StdEncoding.EncodeToString(data)}}}
				}
				raw, _ := json.Marshal(body)
				e := callMeAgainst(t, 503, "application/json", string(raw))
				if !sdkerrors.IsRetryable(e) || sdkerrors.MutationOutcomeUnknown(e) {
					t.Fatalf("%T %v retryable=%v unknown=%v", e, e, sdkerrors.IsRetryable(e), sdkerrors.MutationOutcomeUnknown(e))
				}
			})
		}
	}
}

func TestRetryContractAmbiguous(t *testing.T) {
	for _, tc := range []struct {
		status     int
		body, kind string
	}{{504, `{"code":"deadline_exceeded"}`, "application/json"}, {503, `{"code":"unavailable"}`, "application/json"}, {502, "", "text/plain"}, {503, "<html>unavailable</html>", "text/html"}} {
		t.Run(tc.body, func(t *testing.T) {
			e := callMeAgainst(t, tc.status, tc.kind, tc.body)
			if !sdkerrors.MutationOutcomeUnknown(e) || sdkerrors.IsRetryable(e) {
				t.Fatalf("%T %v retryable=%v unknown=%v", e, e, sdkerrors.IsRetryable(e), sdkerrors.MutationOutcomeUnknown(e))
			}
		})
	}
}

func TestRESTRefusalProblemOnOtherStatuses(t *testing.T) {
	for _, status := range []int{400, 409, 412, 422} {
		e := callMeAgainst(t, status, "application/problem+json", `{"code":"ORDER_ID_CONFLICT","detail":"refused","status":500}`)
		api, ok := e.(*sdkerrors.APIError)
		if !ok || api.Problem == nil || api.Problem.Status != status || api.Problem.Detail != "refused" {
			t.Fatalf("status=%d error=%T %+v", status, e, e)
		}
	}
}

func TestUnavailableMessageDoesNotProveRefusal(t *testing.T) {
	e := callMeAgainst(t, 503, "application/json", `{"code":"unavailable","message":"UNAVAILABLE"}`)
	if sdkerrors.IsRetryable(e) || !sdkerrors.MutationOutcomeUnknown(e) {
		t.Fatalf("%T %v", e, e)
	}
}

func TestUnknownOutcomePrecedesBackoff(t *testing.T) {
	delay := uint64(1250)
	data, err := proto.Marshal(&orderv1.ErrorDetail{Code: orderv1.ErrorCode_ERROR_CODE_ORDER_CREATE_OUTCOME_UNKNOWN, RateLimit: &ratelimitv1.RateLimitDetail{RetryAfterMs: &delay}})
	if err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(map[string]any{"code": "unavailable", "message": "read back", "details": []map[string]string{{"type": "orders.v1.ErrorDetail", "value": base64.StdEncoding.EncodeToString(data)}}})
	e := callMeAgainst(t, 503, "application/json", string(body))
	if sdkerrors.IsRetryable(e) || !sdkerrors.MutationOutcomeUnknown(e) {
		t.Fatalf("%T %v", e, e)
	}
}

func TestFutureResourceCodeIsNotBlindlyRetryable(t *testing.T) {
	e := callMeAgainst(t, 429, "application/json", `{"code":"resource_exhausted","message":"FUTURE_PERMANENT_CAPACITY"}`)
	api, ok := e.(*sdkerrors.APIError)
	if !ok || api.Code != "FUTURE_PERMANENT_CAPACITY" || sdkerrors.IsRetryable(e) || sdkerrors.MutationOutcomeUnknown(e) {
		t.Fatalf("%T %+v", e, e)
	}
}
