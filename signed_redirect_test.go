package polyester

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"

	"connectrpc.com/connect"
	"github.com/Fabric-Labs/polyester-sdk-go/auth"
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	authv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/auth/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/gen/auth/v1/authv1connect"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
	"google.golang.org/protobuf/proto"
)

// signedHeaders are the API-key headers that must never leave the configured
// origin.
var signedHeaders = []string{"X-API-KEY-ID", "X-API-TIMESTAMP", "X-API-SIGNATURE"}

// originRecorder answers AuthService.Me and counts the requests it receives,
// separately counting those that carry any API-key header.
type originRecorder struct {
	authv1connect.UnimplementedAuthServiceHandler
	mu       sync.Mutex
	requests int
	signed   int
}

func (o *originRecorder) Me(context.Context, *connect.Request[authv1.MeRequest]) (*connect.Response[authv1.MeResponse], error) {
	return connect.NewResponse(&authv1.MeResponse{AccountId: 7, ApiKeyId: proto.String("ak_redirect_test")}), nil
}

func (o *originRecorder) handler() http.Handler {
	_, connectHandler := authv1connect.NewAuthServiceHandler(o)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		o.mu.Lock()
		o.requests++
		for _, name := range signedHeaders {
			if r.Header.Get(name) != "" {
				o.signed++
				break
			}
		}
		o.mu.Unlock()
		connectHandler.ServeHTTP(w, r)
	})
}

func (o *originRecorder) counts() (requests, signed int) {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.requests, o.signed
}

// serveOn starts handler on a loopback address and returns its base URL.
func serveOn(t *testing.T, addr string, handler http.Handler) string {
	t.Helper()
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		t.Skipf("cannot listen on %s: %v", addr, err)
	}
	srv := httptest.NewUnstartedServer(handler)
	_ = srv.Listener.Close()
	srv.Listener = listener
	srv.Start()
	t.Cleanup(srv.Close)
	return srv.URL
}

// redirectingOrigin answers every request with status and a Location on
// target, keeping the request path.
func redirectingOrigin(target *string, status int) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", *target+r.URL.Path)
		w.WriteHeader(status)
	})
}

func disposableKeyHex(t *testing.T) string {
	t.Helper()
	_, private, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(private.Seed())
}

// A signed request answered with a 307 or 308 that points at another host or
// another port must not reach that destination: neither its signature, nor its
// key ID, nor its timestamp, in either wire format, with the SDK's own HTTP
// client or one the caller supplies.
func TestSignedRequestIsNotForwardedAcrossOrigins(t *testing.T) {
	type move struct {
		name string
		// origins returns the redirecting origin's address and the
		// destination's, given a free port.
		listen func(t *testing.T) (redirectAddr, destinationAddr string)
	}
	moves := []move{
		{"port-only", func(t *testing.T) (string, string) { return "127.0.0.1:0", "127.0.0.1:0" }},
		{"host-only", func(t *testing.T) (string, string) {
			probe, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			port := strconv.Itoa(probe.Addr().(*net.TCPAddr).Port)
			_ = probe.Close()
			return "127.0.0.1:" + port, "127.0.0.2:" + port
		}},
	}
	clients := map[string]func() *http.Client{
		"sdk-client":    func() *http.Client { return nil },
		"caller-client": func() *http.Client { return &http.Client{} },
		"caller-client-follows-all": func() *http.Client {
			return &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return nil }}
		},
	}
	for _, status := range []int{http.StatusTemporaryRedirect, http.StatusPermanentRedirect} {
		for _, mv := range moves {
			for _, wire := range []string{"json", "binary"} {
				for clientName, newClient := range clients {
					name := strconv.Itoa(status) + "/" + mv.name + "/" + wire + "/" + clientName
					t.Run(name, func(t *testing.T) {
						redirectAddr, destinationAddr := mv.listen(t)
						destination := &originRecorder{}
						var target string
						redirectURL := serveOn(t, redirectAddr, redirectingOrigin(&target, status))
						target = serveOn(t, destinationAddr, destination.handler())
						if redirectURL == target {
							t.Fatalf("redirect origin and destination are the same: %s", target)
						}
						client, err := New(Config{
							APIURL:        redirectURL,
							APIKeyID:      "ak_redirect_test",
							APIPrivateKey: auth.NewSecret(disposableKeyHex(t)),
							WireFormat:    wire,
							HTTPClient:    newClient(),
						})
						if err != nil {
							t.Fatal(err)
						}
						t.Cleanup(func() { _ = client.Close() })

						_, err = client.Auth.Me(context.Background())

						requests, signed := destination.counts()
						if signed != 0 {
							t.Fatalf("destination received %d signed request(s) after a %d redirect", signed, status)
						}
						if requests != 0 {
							t.Fatalf("destination received %d request(s) after a %d redirect", requests, status)
						}
						var refused *sdkerrors.RedirectRefusedError
						if !errors.As(err, &refused) {
							t.Fatalf("Me error = %T %v, want *errors.RedirectRefusedError", err, err)
						}
						if refused.StatusCode != status || refused.Origin != target || refused.SameOrigin {
							t.Fatalf("refusal = %+v, want status %d to %s across origins", *refused, status, target)
						}
						if !errors.Is(err, sdkerrors.ErrPolyester) || sdkerrors.IsRetryable(err) || !sdkerrors.MutationOutcomeUnknown(err) {
							t.Fatalf("refusal must be a non-retryable SDK error with an unknown outcome: %v", err)
						}
					})
				}
			}
		}
	}
}

// Control: a signed call to an origin that answers it directly succeeds in
// both wire formats, and that origin receives the signed request.
func TestSignedRequestWithoutRedirectSucceeds(t *testing.T) {
	for _, wire := range []string{"json", "binary"} {
		t.Run(wire, func(t *testing.T) {
			origin := &originRecorder{}
			apiURL := serveOn(t, "127.0.0.1:0", origin.handler())
			client, err := New(Config{
				APIURL:        apiURL,
				APIKeyID:      "ak_redirect_test",
				APIPrivateKey: auth.NewSecret(disposableKeyHex(t)),
				WireFormat:    wire,
			})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = client.Close() })
			me, err := client.Auth.Me(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if me.APIKeyID != "ak_redirect_test" {
				t.Fatalf("APIKeyID = %q, want ak_redirect_test", me.APIKeyID)
			}
			if requests, signed := origin.counts(); requests != 1 || signed != 1 {
				t.Fatalf("origin received %d request(s), %d signed; want 1 signed", requests, signed)
			}
		})
	}
}

// Unsigned public calls carry no credentials and still follow a redirect, on
// the same HTTP client that refuses to forward signed ones.
func TestUnsignedRequestStillFollowsRedirect(t *testing.T) {
	for _, wire := range []string{"json", "binary"} {
		t.Run(wire, func(t *testing.T) {
			destination := &originRecorder{}
			target := serveOn(t, "127.0.0.1:0", destination.handler())
			redirectURL := serveOn(t, "127.0.0.1:0", redirectingOrigin(&target, http.StatusTemporaryRedirect))
			client, err := New(Config{
				APIURL:        redirectURL,
				APIKeyID:      "ak_redirect_test",
				APIPrivateKey: auth.NewSecret(disposableKeyHex(t)),
				WireFormat:    wire,
			})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = client.Close() })
			public := authv1connect.NewAuthServiceClient(client.transport.HTTP, redirectURL, client.transport.ConnectOptions(false)...)
			if _, err := public.Me(context.Background(), connect.NewRequest(&authv1.MeRequest{})); err != nil {
				t.Fatal(err)
			}
			if requests, signed := destination.counts(); requests != 1 || signed != 0 {
				t.Fatalf("destination received %d request(s), %d signed; want 1 unsigned", requests, signed)
			}
		})
	}
}

// A redirect proves nothing about whether the origin that answered with it
// applied the mutation first: a refused redirect of a signed Orders.Cancel
// leaves its outcome unknown, is not retryable, and never reaches the
// redirect target.
func TestRedirectedMutationOutcomeIsUnknown(t *testing.T) {
	for _, status := range []int{http.StatusTemporaryRedirect, http.StatusPermanentRedirect} {
		for _, wire := range []string{"json", "binary"} {
			t.Run(strconv.Itoa(status)+"/"+wire, func(t *testing.T) {
				destination := &originRecorder{}
				target := serveOn(t, "127.0.0.1:0", destination.handler())
				var mu sync.Mutex
				applied := 0
				origin := serveOn(t, "127.0.0.1:0", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					// The origin applies the cancel, then answers with a redirect.
					if r.Header.Get("X-API-SIGNATURE") != "" && r.URL.Path == "/orders.v1.OrdersService/CancelOrder" {
						mu.Lock()
						applied++
						mu.Unlock()
					}
					w.Header().Set("Location", target+r.URL.Path)
					w.WriteHeader(status)
				}))
				client, err := New(Config{
					APIURL:        origin,
					APIKeyID:      "ak_redirect_test",
					APIPrivateKey: auth.NewSecret(disposableKeyHex(t)),
					WireFormat:    wire,
				})
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = client.Close() })
				symbolID := uint32(1)

				_, err = client.Orders.Cancel(context.Background(), nil, models.OrderKeyByID("1"), nil, &symbolID, nil)

				mu.Lock()
				defer mu.Unlock()
				if applied != 1 {
					t.Fatalf("origin applied %d cancel(s), want 1", applied)
				}
				if requests, _ := destination.counts(); requests != 0 {
					t.Fatalf("destination received %d request(s)", requests)
				}
				var refused *sdkerrors.RedirectRefusedError
				if !errors.As(err, &refused) {
					t.Fatalf("Cancel error = %T %v, want *errors.RedirectRefusedError", err, err)
				}
				if !sdkerrors.MutationOutcomeUnknown(err) {
					t.Fatal("a refused redirect of a mutation must leave its outcome unknown: the origin may have applied it")
				}
				if sdkerrors.IsRetryable(err) {
					t.Fatal("a refused redirect must not be retryable")
				}
			})
		}
	}
}
