package transport

import (
	"context"
	"net"
	"testing"

	"connectrpc.com/connect"
	authv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/auth/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/gen/auth/v1/authv1connect"

	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
)

// round4Answers are failed answers whose error must carry the answer's
// Retry-After: a server deadline at 504, an auth refusal, a permission
// refusal, and bodies that cannot be read, at 503 and 504.
var round4Answers = []struct {
	name        string
	status      int
	contentType string
	body        string
}{
	{"deadline 504", 504, "application/json", `{"code":"deadline_exceeded","message":"context deadline exceeded"}`},
	{"auth 503", 503, "application/problem+json", `{"code":"AUTH_TEMPORARILY_UNAVAILABLE","status":503}`},
	{"permission denied 403", 403, "application/problem+json", `{"code":"PERMISSION_DENIED","status":403,"detail":"denied"}`},
	{"malformed message 503", 503, "application/json", `{"code":"unavailable","message":5}`},
	{"malformed message 504", 504, "application/json", `{"code":"unavailable","message":5}`},
	{"malformed detail 503", 503, "application/json", `{"code":"unavailable","message":"m","details":"nope"}`},
	{"malformed detail 504", 504, "application/json", `{"code":"unavailable","message":"m","details":[{"type":"orders.v1.ErrorDetail","value":"!!"}]}`},
	{"broken json 503", 503, "application/json", `{broken`},
}

// Every error that came from an answer carries the answer's Retry-After,
// whatever error type it maps to.
func TestRound4EveryAnswerCarriesItsRetryAfter(t *testing.T) {
	for _, tc := range round4Answers {
		err := callMeAgainst(t, tc.status, tc.contentType, tc.body, map[string]string{"Retry-After": "30"})
		if got := sdkerrors.RetryAfter(err); got == nil || *got != 30 {
			t.Errorf("%s: %T %v: retry after %v, want 30", tc.name, err, err, got)
		}
	}
}

// Every error that came from an answer carries the answer's status too; an
// error with no answer behind it, a refused connection, carries neither.
func TestRound4AnswerStatusAndNoAnswer(t *testing.T) {
	for _, tc := range round4Answers {
		err := callMeAgainst(t, tc.status, tc.contentType, tc.body, map[string]string{"Retry-After": "30"})
		if got := sdkerrors.AnswerStatus(err); got != tc.status {
			t.Errorf("%s: %T status %d, want %d", tc.name, err, got, tc.status)
		}
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := listener.Addr().String()
	_ = listener.Close()
	cfg := DefaultConfig()
	cfg.APIURL = "http://" + addr
	factory := NewFactory(cfg, nil, nil)
	client := authv1connect.NewAuthServiceClient(factory.HTTP, cfg.APIURL, factory.ConnectOptions(false)...)
	_, callErr := client.Me(context.Background(), connect.NewRequest(&authv1.MeRequest{}))
	mapped := MapError(callErr)
	if sdkerrors.AnswerStatus(mapped) != 0 || sdkerrors.RetryAfter(mapped) != nil {
		t.Fatalf("refused connection %T %v carries an answer", mapped, mapped)
	}
}
