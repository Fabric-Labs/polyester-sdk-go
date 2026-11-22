package polyester

import (
	"strings"
	"testing"

	"github.com/Fabric-Labs/polyester-sdk-go/environment"
)

// plaintextVPCParams is devnet with plaintext API and websocket URLs on a
// private, non-loopback host.
func plaintextVPCParams(allowInsecure bool) environment.CreateEnvironmentParams {
	d := DevnetEnvironment
	return environment.CreateEnvironmentParams{
		Name:               d.Name,
		APIURL:             "http://10.0.0.5:8080",
		WebsocketURL:       "ws://10.0.0.5:8080",
		RPCURL:             d.RPCURL,
		ChainID:            d.ChainID,
		ChainName:          d.ChainName,
		ExplorerURL:        d.ExplorerURL,
		AccountAbstraction: d.AccountAbstraction,
		Contracts:          d.Contracts,
		AllowInsecure:      allowInsecure,
	}
}

// An environment created with the plaintext opt-in keeps it, so New accepts it.
func TestNewAcceptsEnvironmentCreatedWithAllowInsecure(t *testing.T) {
	env, err := CreateEnvironment(plaintextVPCParams(true))
	if err != nil {
		t.Fatalf("CreateEnvironment: %v", err)
	}
	if !env.AllowInsecure {
		t.Fatal("environment dropped the AllowInsecure opt-in")
	}
	client, err := New(Config{Environment: &env, SkipCatalogHydration: true})
	if err != nil {
		t.Fatalf("New refused an environment created with AllowInsecure: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })
	if client.APIURL != "http://10.0.0.5:8080" || client.WSURL != "ws://10.0.0.5:8080" {
		t.Fatalf("client URLs %q, %q", client.APIURL, client.WSURL)
	}
	if !client.Environment.AllowInsecure {
		t.Fatal("client environment dropped the AllowInsecure opt-in")
	}

	// WithURLs keeps the opt-in of the environment it copies.
	moved, err := env.WithURLs("http://10.0.0.6:8080", "ws://10.0.0.6:8080", "")
	if err != nil {
		t.Fatalf("WithURLs on an opted-in environment: %v", err)
	}
	if _, err := environment.ParsePolyesterEnvironment(moved); err != nil {
		t.Fatalf("re-validating the WithURLs copy: %v", err)
	}
}

// Without the opt-in, a remote plaintext environment is still refused: by
// CreateEnvironment, and by New when the URLs are set on the struct directly.
func TestNewRefusesRemotePlaintextEnvironmentWithoutOptIn(t *testing.T) {
	if _, err := CreateEnvironment(plaintextVPCParams(false)); err == nil ||
		!strings.Contains(err.Error(), "secure protocol") {
		t.Fatalf("CreateEnvironment without AllowInsecure: err = %v, want a secure-protocol refusal", err)
	}

	env := DevnetEnvironment
	env.APIURL = "http://10.0.0.5:8080"
	env.WebsocketURL = "ws://10.0.0.5:8080"
	_, err := New(Config{Environment: &env, SkipCatalogHydration: true})
	if err == nil || !strings.Contains(err.Error(), "secure protocol") {
		t.Fatalf("New without AllowInsecure: err = %v, want a secure-protocol refusal", err)
	}

	if _, err := DevnetEnvironment.WithURLs("http://10.0.0.5:8080", "", ""); err == nil {
		t.Fatal("WithURLs on an environment without the opt-in accepted remote plaintext")
	}
}
