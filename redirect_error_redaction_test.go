package polyester

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"

	"connectrpc.com/connect"
	"github.com/Fabric-Labs/polyester-sdk-go/auth"
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	authv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/auth/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/gen/auth/v1/authv1connect"
	"github.com/Fabric-Labs/polyester-sdk-go/transport"
)

// Synthetic markers placed in a redirect Location. None is a real credential.
const (
	locationUser     = "synthetic-location-user"
	locationPassword = "synthetic-location-password"
	locationToken    = "synthetic-location-token"
	locationFragment = "synthetic-location-fragment"
)

// locationSecrets are the parts of a redirect Location that must never reach
// an error: its userinfo, its query and its fragment.
var locationSecrets = []string{locationUser, locationPassword, locationToken, locationFragment}

// malformedLocations are redirect targets net/http cannot parse, each with
// userinfo, a query token and a fragment.
var malformedLocations = map[string]string{
	"invalid-port": "http://" + locationUser + ":" + locationPassword + "@127.0.0.1:notaport/v1/me?token=" + locationToken + "#" + locationFragment,
	"invalid-ipv6": "http://" + locationUser + ":" + locationPassword + "@[::1/v1/me?token=" + locationToken + "#" + locationFragment,
}

// countingRedirect answers every request with status and location, and
// counts the requests it receives.
type countingRedirect struct {
	mu       sync.Mutex
	requests int
	status   int
	location string
}

func (c *countingRedirect) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	c.mu.Lock()
	c.requests++
	c.mu.Unlock()
	w.Header().Set("Location", c.location)
	w.WriteHeader(c.status)
}

func (c *countingRedirect) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.requests
}

// assertNoLocationSecrets fails when err, any error it wraps, or any field of
// any of them renders a secret or the raw Location.
func assertNoLocationSecrets(t *testing.T, err error, rawLocation string) {
	t.Helper()
	forbidden := append([]string{rawLocation}, locationSecrets...)
	for _, rendered := range renderedError(err) {
		for _, secret := range forbidden {
			if strings.Contains(rendered, secret) {
				t.Errorf("error exposes %q: %s", secret, rendered)
				return
			}
		}
	}
}

// renderedError returns every text err exposes: Error(), %v, %+v and %#v of
// err and of each error it wraps, and every string reachable through their
// fields.
func renderedError(err error) []string {
	var out []string
	seen := map[error]bool{}
	var walk func(error)
	walk = func(e error) {
		if e == nil || seen[e] {
			return
		}
		seen[e] = true
		out = append(out, e.Error(), fmt.Sprintf("%v", e), fmt.Sprintf("%+v", e), fmt.Sprintf("%#v", e), fmt.Sprintf("%s", e))
		out = append(out, fieldStrings(reflect.ValueOf(e), 0)...)
		switch unwrapper := e.(type) {
		case interface{ Unwrap() error }:
			walk(unwrapper.Unwrap())
		case interface{ Unwrap() []error }:
			for _, inner := range unwrapper.Unwrap() {
				walk(inner)
			}
		}
	}
	walk(err)
	return out
}

func fieldStrings(v reflect.Value, depth int) []string {
	if depth > 6 || !v.IsValid() {
		return nil
	}
	switch v.Kind() {
	case reflect.Pointer, reflect.Interface:
		if v.IsNil() {
			return nil
		}
		return fieldStrings(v.Elem(), depth+1)
	case reflect.String:
		return []string{v.String()}
	case reflect.Struct:
		var out []string
		for i := range v.NumField() {
			out = append(out, fieldStrings(v.Field(i), depth+1)...)
		}
		return out
	case reflect.Slice, reflect.Array:
		var out []string
		for i := range v.Len() {
			out = append(out, fieldStrings(v.Index(i), depth+1)...)
		}
		return out
	case reflect.Map:
		var out []string
		iter := v.MapRange()
		for iter.Next() {
			out = append(out, fieldStrings(iter.Key(), depth+1)...)
			out = append(out, fieldStrings(iter.Value(), depth+1)...)
		}
		return out
	}
	return nil
}

var redirectStatuses = []int{http.StatusMovedPermanently, http.StatusTemporaryRedirect, http.StatusPermanentRedirect}

// A signed request answered with a redirect whose Location does not parse is
// refused with a sanitized *RedirectRefusedError: no userinfo, query,
// fragment or raw Location in its text, its formatting or its fields, in
// either wire format.
func TestMalformedRedirectOfSignedRequestIsRefusedWithoutURLSecrets(t *testing.T) {
	for _, status := range redirectStatuses {
		for kind, location := range malformedLocations {
			for _, wire := range []string{"json", "binary"} {
				t.Run(strconv.Itoa(status)+"/"+kind+"/"+wire, func(t *testing.T) {
					origin := &countingRedirect{status: status, location: location}
					apiURL := serveOn(t, "127.0.0.1:0", origin)
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

					_, err = client.Auth.Me(context.Background())

					if err == nil {
						t.Fatal("Me succeeded after a malformed redirect")
					}
					assertNoLocationSecrets(t, err, location)
					if got := origin.count(); got != 1 {
						t.Errorf("origin received %d request(s), want 1", got)
					}
					var refused *sdkerrors.RedirectRefusedError
					if !errors.As(err, &refused) {
						t.Fatalf("Me error = %T %v, want *errors.RedirectRefusedError", err, err)
					}
					if refused.StatusCode != status || !refused.Malformed || refused.Origin != "" || refused.SameOrigin {
						t.Fatalf("refusal = %+v, want a malformed status %d redirect with no origin", *refused, status)
					}
					if !strings.Contains(err.Error(), "malformed") {
						t.Errorf("refusal should say the Location is malformed: %s", err)
					}
					if !errors.Is(err, sdkerrors.ErrPolyester) || sdkerrors.IsRetryable(err) || !sdkerrors.MutationOutcomeUnknown(err) {
						t.Fatalf("refusal must be a non-retryable SDK error with an unknown outcome: %v", err)
					}
				})
			}
		}
	}
}

// Control: a well-formed redirect Location carrying userinfo, a query and a
// fragment is refused with its origin only, and nothing reaches the target.
func TestValidRedirectOfSignedRequestNamesOriginOnly(t *testing.T) {
	for _, status := range redirectStatuses {
		for _, wire := range []string{"json", "binary"} {
			t.Run(strconv.Itoa(status)+"/"+wire, func(t *testing.T) {
				destination := &originRecorder{}
				target := serveOn(t, "127.0.0.1:0", destination.handler())
				host := strings.TrimPrefix(target, "http://")
				location := "http://" + locationUser + ":" + locationPassword + "@" + host + "/v1/me?token=" + locationToken + "#" + locationFragment
				origin := &countingRedirect{status: status, location: location}
				apiURL := serveOn(t, "127.0.0.1:0", origin)
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

				_, err = client.Auth.Me(context.Background())

				assertNoLocationSecrets(t, err, location)
				var refused *sdkerrors.RedirectRefusedError
				if !errors.As(err, &refused) {
					t.Fatalf("Me error = %T %v, want *errors.RedirectRefusedError", err, err)
				}
				if refused.StatusCode != status || refused.Malformed || refused.Origin != target || refused.SameOrigin {
					t.Fatalf("refusal = %+v, want status %d to %s", *refused, status, target)
				}
				if requests, _ := destination.counts(); requests != 0 {
					t.Fatalf("destination received %d request(s)", requests)
				}
			})
		}
	}
}

// An unsigned request cannot follow a malformed Location either; it gets the
// same sanitized refusal instead of net/http's error quoting the Location.
func TestMalformedRedirectOfUnsignedRequestIsRefusedWithoutURLSecrets(t *testing.T) {
	for kind, location := range malformedLocations {
		for _, wire := range []string{"json", "binary"} {
			t.Run(kind+"/"+wire, func(t *testing.T) {
				origin := &countingRedirect{status: http.StatusTemporaryRedirect, location: location}
				apiURL := serveOn(t, "127.0.0.1:0", origin)
				client, err := New(Config{APIURL: apiURL, WireFormat: wire})
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = client.Close() })
				public := authv1connect.NewAuthServiceClient(client.transport.HTTP, apiURL, client.transport.ConnectOptions(false)...)

				_, err = public.Me(context.Background(), connect.NewRequest(&authv1.MeRequest{}))
				mapped := transport.MapError(err)

				for _, e := range []error{err, mapped} {
					assertNoLocationSecrets(t, e, location)
				}
				var refused *sdkerrors.RedirectRefusedError
				if !errors.As(mapped, &refused) || !refused.Malformed {
					t.Fatalf("error = %T %v, want a malformed *errors.RedirectRefusedError", mapped, mapped)
				}
			})
		}
	}
}

// Control: when the caller's own redirect policy stops an unsigned redirect,
// net/http names the raw Location in its *url.Error; the SDK error keeps the
// cause but not the Location's userinfo, query or fragment.
func TestCallerStoppedRedirectErrorHasNoURLSecrets(t *testing.T) {
	for _, wire := range []string{"json", "binary"} {
		t.Run(wire, func(t *testing.T) {
			destination := &originRecorder{}
			target := serveOn(t, "127.0.0.1:0", destination.handler())
			host := strings.TrimPrefix(target, "http://")
			location := "http://" + locationUser + ":" + locationPassword + "@" + host + "/v1/me?token=" + locationToken + "#" + locationFragment
			apiURL := serveOn(t, "127.0.0.1:0", &countingRedirect{status: http.StatusFound, location: location})
			client, err := New(Config{
				APIURL:     apiURL,
				WireFormat: wire,
				HTTPClient: &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
					return errors.New("caller policy stops redirects")
				}},
			})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = client.Close() })
			public := authv1connect.NewAuthServiceClient(client.transport.HTTP, apiURL, client.transport.ConnectOptions(false)...)

			_, err = public.Me(context.Background(), connect.NewRequest(&authv1.MeRequest{}))
			mapped := transport.MapError(err)

			assertNoLocationSecrets(t, mapped, location)
			if !strings.Contains(mapped.Error(), "caller policy stops redirects") {
				t.Fatalf("error should keep the cause: %v", mapped)
			}
		})
	}
}
