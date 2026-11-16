package transport

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"connectrpc.com/connect"
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	authv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/auth/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/gen/auth/v1/authv1connect"
	"github.com/Fabric-Labs/polyester-sdk-go/hardening"
)

// refusalBodies are ConnectRPC refusals that decode to a known, definite
// error when the SDK reads their body.
var refusalBodies = []struct {
	name, head string
}{
	{"rate limit", `{"code":"resource_exhausted","message":"RATE_LIMIT_EXCEEDED"`},
	{"unauthenticated", `{"code":"unauthenticated","message":"bad signature"`},
	{"permission denied", `{"code":"permission_denied","message":"missing permission"`},
}

// meAnswered calls Auth.Me through the generated client against a 503 with
// Retry-After 30 whose JSON body is head, padding spaces and "}", sent with a
// Content-Length or chunked.
func meAnswered(t *testing.T, head string, padding int, chunked bool) error {
	t.Helper()
	body := head + strings.Repeat(" ", padding) + "}"
	srv := hardening.NewTestServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Retry-After", "30")
		if !chunked {
			w.Header().Set("Content-Length", strconv.Itoa(len(body)))
		}
		w.WriteHeader(http.StatusServiceUnavailable)
		if chunked {
			_, _ = w.Write([]byte(head))
			w.(http.Flusher).Flush()
			_, _ = w.Write([]byte(body[len(head):]))
			return
		}
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

// A failed body longer than the SDK reads is unreadable: it maps by its
// status alone, with the answer's status and delay, and is never decoded,
// so an oversized refusal at 503 is a server failure with an unknown
// outcome, not the known refusal its first bytes name.
func TestOversizedFailedBodyReadsByStatus(t *testing.T) {
	for _, refusal := range refusalBodies {
		for _, chunked := range []bool{false, true} {
			name := refusal.name + " by length"
			if chunked {
				name = refusal.name + " chunked"
			}
			t.Run(name, func(t *testing.T) {
				err := meAnswered(t, refusal.head, 128*1024, chunked)
				var serverErr *sdkerrors.ServerError
				if !errors.As(err, &serverErr) {
					t.Fatalf("%T %v, want *errors.ServerError", err, err)
				}
				if got := sdkerrors.AnswerStatus(err); got != http.StatusServiceUnavailable {
					t.Errorf("status %d, want 503", got)
				}
				if got := sdkerrors.RetryAfter(err); got == nil || *got != 30 {
					t.Errorf("retry after %v, want 30", got)
				}
				if !sdkerrors.MutationOutcomeUnknown(err) || sdkerrors.IsRetryable(err) {
					t.Errorf("unknown=%v retryable=%v, want an unknown outcome that is not retryable",
						sdkerrors.MutationOutcomeUnknown(err), sdkerrors.IsRetryable(err))
				}
			})
		}
	}
}

// The same refusals below the limit still decode to what they say.
func TestFailedBodyBelowTheLimitDecodes(t *testing.T) {
	for _, refusal := range refusalBodies {
		for _, chunked := range []bool{false, true} {
			err := meAnswered(t, refusal.head, 1024, chunked)
			var rate *sdkerrors.RateLimitError
			var auth *sdkerrors.AuthError
			switch refusal.name {
			case "rate limit":
				if !errors.As(err, &rate) || !sdkerrors.IsRetryable(err) || sdkerrors.MutationOutcomeUnknown(err) {
					t.Errorf("%s chunked=%v: %T %v, want a retryable known rate refusal", refusal.name, chunked, err, err)
				}
			default:
				if !errors.As(err, &auth) || sdkerrors.MutationOutcomeUnknown(err) {
					t.Errorf("%s chunked=%v: %T %v, want a known auth refusal", refusal.name, chunked, err, err)
				}
			}
			if got := sdkerrors.AnswerStatus(err); got != http.StatusServiceUnavailable {
				t.Errorf("%s chunked=%v: status %d, want 503", refusal.name, chunked, got)
			}
			if got := sdkerrors.RetryAfter(err); got == nil || *got != 30 {
				t.Errorf("%s chunked=%v: retry after %v, want 30", refusal.name, chunked, got)
			}
		}
	}
}

// meAnsweredWith calls Auth.Me through the generated client against a 503
// with Retry-After 30 whose body is body, with the given Content-Encoding.
func meAnsweredWith(t *testing.T, body, encoding string) error {
	t.Helper()
	srv := hardening.NewTestServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Retry-After", "30")
		if encoding != "" {
			w.Header().Set("Content-Encoding", encoding)
		}
		w.Header().Set("Content-Length", strconv.Itoa(len(body)))
		w.WriteHeader(http.StatusServiceUnavailable)
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

// The limit is decided on the byte count of the body as decoded, before any
// of it is parsed: a complete, valid error object followed by padding outside
// it is unreadable once the decoded body is one byte over the limit, raw or
// gzip alike, however small it is on the wire. At exactly the limit it still
// decodes to the refusal it names.
func TestBodyLimitIsDecidedOnDecodedBytes(t *testing.T) {
	for _, refusal := range refusalBodies {
		object := refusal.head + "}"
		for _, encoding := range []string{"", "gzip"} {
			for _, size := range []int{maxErrorBodyBytes, maxErrorBodyBytes + 1, len(object) + 128*1024} {
				name := refusal.name + " raw " + strconv.Itoa(size)
				if encoding != "" {
					name = refusal.name + " gzip " + strconv.Itoa(size)
				}
				t.Run(name, func(t *testing.T) {
					body := object + strings.Repeat(" ", size-len(object))
					if encoding == "gzip" {
						body = gzipped(t, body)
					}
					err := meAnsweredWith(t, body, encoding)
					if got := sdkerrors.AnswerStatus(err); got != http.StatusServiceUnavailable {
						t.Errorf("status %d, want 503", got)
					}
					if got := sdkerrors.RetryAfter(err); got == nil || *got != 30 {
						t.Errorf("retry after %v, want 30", got)
					}
					var serverErr *sdkerrors.ServerError
					if size > maxErrorBodyBytes {
						if !errors.As(err, &serverErr) || !sdkerrors.MutationOutcomeUnknown(err) || sdkerrors.IsRetryable(err) {
							t.Fatalf("%T %v: unknown=%v retryable=%v, want an unreadable answer read by its status",
								err, err, sdkerrors.MutationOutcomeUnknown(err), sdkerrors.IsRetryable(err))
						}
						return
					}
					if errors.As(err, &serverErr) || sdkerrors.MutationOutcomeUnknown(err) {
						t.Fatalf("%T %v: at the limit the body decodes to its refusal", err, err)
					}
				})
			}
		}
	}
}
