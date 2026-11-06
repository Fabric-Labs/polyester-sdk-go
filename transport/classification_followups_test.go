package transport

import (
	"connectrpc.com/connect"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	authv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/auth/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/gen/auth/v1/authv1connect"
	"net/http"
	"net/http/httptest"
	"testing"

	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	orderv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/orders/v1"
	"google.golang.org/protobuf/proto"
)

func TestAdmissionClassification(t *testing.T) {
	for _, tc := range []struct {
		name      string
		status    int
		retryable bool
	}{
		{"OVERLOADED", 503, true}, {"SYSTEM_RATE_LIMITED", 429, true}, {"ORDER_SEQUENCE_EXHAUSTED", 429, false},
	} {
		for _, shape := range []string{"rest", "connect", "connect_detail"} {
			t.Run(tc.name+"/"+shape, func(t *testing.T) {
				body := map[string]any{"code": tc.name, "detail": "admission refused"}
				status := tc.status
				if shape != "rest" {
					status = 429
					body = map[string]any{"code": "resource_exhausted", "message": tc.name}
				}
				if shape == "connect_detail" {
					data, err := proto.Marshal(&orderv1.ErrorDetail{Code: orderv1.ErrorCode(orderv1.ErrorCode_value["ERROR_CODE_"+tc.name])})
					if err != nil {
						t.Fatal(err)
					}
					body["details"] = []map[string]string{{"type": "orders.v1.ErrorDetail", "value": base64.StdEncoding.EncodeToString(data)}}
				}
				raw, err := json.Marshal(body)
				if err != nil {
					t.Fatal(err)
				}
				mapped := callMeAgainst(t, status, "application/json", string(raw))
				if sdkerrors.IsRetryable(mapped) != tc.retryable || sdkerrors.MutationOutcomeUnknown(mapped) {
					t.Fatalf("error=%T %v retryable=%v unknown=%v", mapped, mapped, sdkerrors.IsRetryable(mapped), sdkerrors.MutationOutcomeUnknown(mapped))
				}
			})
		}
	}
}

func TestFutureConnectOutcomeByName(t *testing.T) {
	for _, code := range []string{"unavailable", "deadline_exceeded"} {
		err := callMeAgainst(t, 504, "application/json", fmt.Sprintf(`{"code":%q,"message":"TWAP_CREATE_OUTCOME_UNKNOWN"}`, code))
		if sdkerrors.IsRetryable(err) || !sdkerrors.MutationOutcomeUnknown(err) {
			t.Errorf("error=%T %v retryable=%v unknown=%v", err, err, sdkerrors.IsRetryable(err), sdkerrors.MutationOutcomeUnknown(err))
		}
	}
}

func TestRESTAPIErrorKeepsProblem(t *testing.T) {
	err := callMeAgainst(t, 504, "application/problem+json", `{"code":"ORDER_CREATE_OUTCOME_UNKNOWN","status":400,"detail":"read back first","type":"https://example.com/wrong"}`)
	api, ok := err.(*sdkerrors.APIError)
	if !ok {
		t.Fatalf("error=%T %v", err, err)
	}
	p := api.Problem
	if p == nil {
		t.Fatal("APIError discarded REST problem")
	}
	if p.Status != 504 || p.Code != "ORDER_CREATE_OUTCOME_UNKNOWN" || p.Detail != "read back first" || p.Type != sdkerrors.ProblemType(p.Code) {
		t.Fatalf("problem=%+v", p)
	}
}

func TestRESTAdmissionKeepsRetryDelay(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/problem+json")
		w.Header().Set("Retry-After", "1.25")
		w.WriteHeader(429)
		_, _ = w.Write([]byte(`{"code":"SYSTEM_RATE_LIMITED"}`))
	}))
	defer srv.Close()
	cfg := DefaultConfig()
	cfg.APIURL = srv.URL
	factory := NewFactory(cfg, nil, nil)
	client := authv1connect.NewAuthServiceClient(factory.HTTP, srv.URL, factory.ConnectOptions(false)...)
	_, err := client.Me(context.Background(), connect.NewRequest(&authv1.MeRequest{}))
	mapped := MapError(err)
	var rate *sdkerrors.RateLimitError
	if !errors.As(mapped, &rate) || rate.RetryAfter == nil || *rate.RetryAfter != 1.25 {
		t.Fatalf("rate delay lost: %T %+v", mapped, mapped)
	}
}
