package redact

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
	"testing"
)

const fakePassword = "fake-password-marker"

// unparsableURL is a variable so static checks don't reject the bad literal.
var unparsableURL = "http://fake-user:" + fakePassword + "@127.0.0.1:notaport"

func TestURLRemovesUserinfo(t *testing.T) {
	for raw, want := range map[string]string{
		"https://api.example.test/v1":                                "https://api.example.test/v1",
		"https://fake-user:" + fakePassword + "@api.example.test/v1": "https://api.example.test/v1",
		"wss://fake-user@ws.example.test/connection?x=1":             "wss://ws.example.test/connection",
		"https://api.example.test/v1?token=fake-token#fake-fragment": "https://api.example.test/v1",
		"https://api.example.test/v1?":                               "https://api.example.test/v1",
		"http://fake-user:" + fakePassword + "@127.0.0.1:notaport":   InvalidURL,
		"fake-user:" + fakePassword + "@api.example.test":            InvalidURL,
		"mailto:fake-user:" + fakePassword + "@example.test":         InvalidURL,
	} {
		if got := URL(raw); got != want {
			t.Errorf("URL(%q) = %q, want %q", raw, got, want)
		}
	}
}

func TestURLErrorRedactsParseErrors(t *testing.T) {
	_, err := url.Parse(unparsableURL)
	if err == nil || !strings.Contains(err.Error(), fakePassword) {
		t.Fatalf("precondition: the parser should quote the URL: %v", err)
	}
	redacted := URLError(err)
	if strings.Contains(redacted.Error(), fakePassword) || !strings.Contains(redacted.Error(), "invalid port") {
		t.Fatalf("URLError = %q", redacted)
	}
	other := errors.New("dial tcp: connection refused")
	if URLError(other) != other {
		t.Fatal("URLError must return other errors unchanged")
	}
}

// net/http quotes a redirect Location that does not parse in the text of the
// error it wraps in a *url.Error; URLError replaces that cause and the URL.
func TestURLErrorReplacesLocationParseFailure(t *testing.T) {
	location := unparsableURL + "/next?token=fake-token"
	_, parseErr := url.Parse(location)
	err := &url.Error{
		Op:  "Get",
		URL: "https://api.example.test/v1?token=fake-token",
		Err: errors.New(`failed to parse Location header "` + location + `": ` + parseErr.Error()),
	}
	redacted := URLError(err)
	for _, secret := range []string{fakePassword, "fake-user", "fake-token", location} {
		if strings.Contains(redacted.Error(), secret) {
			t.Fatalf("URLError = %q exposes %q", redacted, secret)
		}
	}
	if want := `Get "https://api.example.test/v1": redirect Location is not a valid URL`; redacted.Error() != want {
		t.Fatalf("URLError = %q, want %q", redacted, want)
	}
}

// Message rewrites an outer error's copy of a *url.Error's text.
func TestMessageRedactsCopiedURLErrorText(t *testing.T) {
	inner := &url.Error{Op: "Post", URL: "https://fake-user:" + fakePassword + "@api.example.test/v1?token=fake-token", Err: errors.New("stopped")}
	outer := fmt.Errorf("unavailable: %w", inner)
	got := Message(outer.Error(), outer)
	if strings.Contains(got, fakePassword) || strings.Contains(got, "fake-token") || got != `unavailable: Post "https://api.example.test/v1": stopped` {
		t.Fatalf("Message = %q", got)
	}
	other := errors.New("plain")
	if Message("plain", other) != "plain" {
		t.Fatal("Message must leave text without a *url.Error unchanged")
	}
}
