package polyester

import (
	"testing"

	"github.com/Fabric-Labs/polyester-sdk-go/auth"
	"github.com/Fabric-Labs/polyester-sdk-go/chain"
)

// setPolyesterEnv sets every POLYESTER_* variable FromEnv reads.
func setPolyesterEnv(t *testing.T) {
	t.Helper()
	t.Setenv(auth.AccountIDEnv, "AcctFromEnv")
	t.Setenv(auth.APIKeyIDEnv, "ak_from_env")
	t.Setenv(auth.APIPrivateKeyEnv, auth.GenerateEd25519Keypair().SecretKeyHex.Reveal())
	t.Setenv(chain.EnvNameEnv, "testnet")
	t.Setenv("POLYESTER_API_URL", "https://env.invalid")
	t.Setenv("POLYESTER_WS_URL", "wss://env.invalid/ws")
}

// New uses only what the config carries: no POLYESTER_* variable fills a
// setting the config leaves unset. Only FromEnv reads the environment.
func TestNewReadsNothingFromTheEnvironment(t *testing.T) {
	setPolyesterEnv(t)
	client, err := New(Config{SkipCatalogHydration: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	if client.DefaultAccountID != nil {
		t.Errorf("DefaultAccountID = %q, want unset", *client.DefaultAccountID)
	}
	if client.transport.Credentials != nil {
		t.Errorf("credentials loaded from the environment: key %q", client.transport.Credentials.KeyID)
	}
	if client.Environment.Name != DevnetEnvironment.Name || client.APIURL != DefaultAPIURL || client.WSURL != DefaultWSURL {
		t.Errorf("environment %q, %q, %q: want the devnet defaults", client.Environment.Name, client.APIURL, client.WSURL)
	}
}

// FromEnv still reads every POLYESTER_* variable.
func TestFromEnvReadsTheEnvironment(t *testing.T) {
	setPolyesterEnv(t)
	client, err := FromEnv(func(c *Config) { c.SkipCatalogHydration = true })
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	if client.DefaultAccountID == nil || *client.DefaultAccountID != "AcctFromEnv" {
		t.Errorf("DefaultAccountID = %v, want AcctFromEnv", client.DefaultAccountID)
	}
	if client.transport.Credentials == nil || client.transport.Credentials.KeyID != "ak_from_env" {
		t.Error("FromEnv did not load the API key from the environment")
	}
	if client.Environment.Name != TestnetEnvironment.Name || client.APIURL != "https://env.invalid" || client.WSURL != "wss://env.invalid/ws" {
		t.Errorf("environment %q, %q, %q: want the environment's", client.Environment.Name, client.APIURL, client.WSURL)
	}
}
