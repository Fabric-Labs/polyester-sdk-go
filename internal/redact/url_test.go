package redact

import (
	"errors"
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
		"wss://fake-user@ws.example.test/connection?x=1":             "wss://ws.example.test/connection?x=1",
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
