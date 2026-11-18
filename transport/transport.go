package transport

import (
	"encoding/hex"
	"net/http"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/Fabric-Labs/polyester-sdk-go/auth"
	"github.com/Fabric-Labs/polyester-sdk-go/connectx"
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	"github.com/Fabric-Labs/polyester-sdk-go/internal/redact"
	"github.com/Fabric-Labs/polyester-sdk-go/useragent"
	"github.com/Fabric-Labs/polyester-sdk-go/wire"
)

// MaxConnectResponseBytes bounds decompressed unary Connect response messages.
const MaxConnectResponseBytes = 4 * 1024 * 1024

// Config holds HTTP and Connect client settings.
type Config struct {
	APIURL     string
	WSURL      string
	Timeout    time.Duration
	WireFormat connectx.WireFormat
	// RecvWindowMS is the receive window of every signed Connect request, in
	// milliseconds (1 to 60000); 0 sends no window and the server applies
	// 5000.
	RecvWindowMS int
}

// WireFormatFromString parses a wire format name.
func WireFormatFromString(value string) connectx.WireFormat {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "json":
		return connectx.WireJSON
	default:
		return connectx.WireBinary
	}
}

// DefaultConfig returns devnet defaults.
func DefaultConfig() Config {
	return Config{
		APIURL:     "https://api.devnet.polyester.com",
		WSURL:      "wss://api.devnet.polyester.com",
		Timeout:    10 * time.Second,
		WireFormat: connectx.WireBinary,
	}
}

// Factory owns HTTP clients and Connect client options.
type Factory struct {
	Config          Config
	Credentials     *auth.Credentials
	HTTP            *http.Client
	authInterceptor *APIKeyInterceptor
	// base is the HTTP client HTTP wraps, as the caller supplied it.
	base *http.Client
	// releaseSecrets forgets the secrets registerClientSecrets registered.
	releaseSecrets func()
}

// NewFactory builds a transport factory. Its HTTP client is a copy of
// httpClient, or a new one, that never follows a redirect of a request signed
// with an API key (see auth.RefuseSignedRedirects).
func NewFactory(cfg Config, creds *auth.Credentials, httpClient *http.Client) *Factory {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: cfg.Timeout}
	}
	f := &Factory{
		Config:         cfg,
		Credentials:    creds,
		HTTP:           wrapHTTPClient(httpClient),
		base:           httpClient,
		releaseSecrets: registerClientSecrets(cfg, creds, httpClient),
	}
	if creds != nil {
		f.authInterceptor = NewAPIKeyInterceptor(creds, cfg.APIURL, cfg.WireFormat)
		f.authInterceptor.RecvWindowMS = cfg.RecvWindowMS
	}
	return f
}

// registerClientSecrets has redact.Text remove the client's own secrets from
// every error: the userinfo of its API, WebSocket and proxy URLs, and its API
// private key. The returned function forgets them again.
func registerClientSecrets(cfg Config, creds *auth.Credentials, httpClient *http.Client) func() {
	secrets := append(redact.URLSecrets(cfg.APIURL), redact.URLSecrets(cfg.WSURL)...)
	if creds != nil {
		if key := creds.PrivateKey(); len(key) > 0 {
			secrets = append(secrets, hex.EncodeToString(key), hex.EncodeToString(key.Seed()))
		}
	}
	if pooled, ok := httpClient.Transport.(*http.Transport); ok && pooled.Proxy != nil {
		if req, err := http.NewRequest(http.MethodPost, cfg.APIURL, nil); err == nil {
			if proxy, err := pooled.Proxy(req); err == nil && proxy != nil {
				secrets = append(secrets, redact.URLSecrets(proxy.String())...)
			}
		}
	}
	return redact.Register(secrets...)
}

func wrapHTTPClient(client *http.Client) *http.Client {
	recording := *client
	recording.Transport = responseRefRoundTripper{base: unsentResendRoundTripper{base: client.Transport}}
	return wrapTimestampSkew(useragent.WrapClient(auth.RefuseSignedRedirects(&recording)))
}

// FreshHTTP returns an HTTP client like HTTP whose requests open connections
// of their own, and a function that closes them once the caller is done. A
// read retried through it can't queue behind a connection that dropped or
// stalled: over HTTP/2 one connection carries every request to the host.
// When the supplied client's transport is not an *http.Transport, which
// can't be copied, it returns HTTP itself and a function that does nothing.
func (f *Factory) FreshHTTP() (*http.Client, func()) {
	roundTripper := f.base.Transport
	if roundTripper == nil {
		roundTripper = http.DefaultTransport
	}
	pooled, ok := roundTripper.(*http.Transport)
	if !ok {
		return f.HTTP, func() {}
	}
	fresh := pooled.Clone()
	client := *f.base
	client.Transport = fresh
	return wrapHTTPClient(&client), fresh.CloseIdleConnections
}

// ConnectOptions returns client options for generated Connect clients.
func (f *Factory) ConnectOptions(authenticated bool) []connect.ClientOption {
	// The answer interceptor is outermost: every attempt inside it records
	// its answer, and the call's error carries the last one.
	opts := []connect.ClientOption{connect.WithReadMaxBytes(MaxConnectResponseBytes), connect.WithInterceptors(answerInterceptor{})}
	if f.Config.WireFormat == connectx.WireJSON {
		opts = append(opts, connect.WithProtoJSON())
	}
	if authenticated && f.authInterceptor != nil {
		opts = append(opts, connect.WithInterceptors(f.authInterceptor))
	}
	return opts
}

// RequireCredentials returns credentials or an auth error.
func (f *Factory) RequireCredentials() (*auth.Credentials, error) {
	if f.Credentials == nil {
		return nil, &sdkerrors.AuthError{Msg: "This endpoint requires Polyester API-key credentials"}
	}
	return f.Credentials, nil
}

// MapError converts Connect errors to SDK errors.
func MapError(err error) error {
	return wire.MapConnectError(err)
}

// Close forgets the client's secrets the factory registered for redaction.
// It leaves the HTTP client open: it may be caller-owned.
func (f *Factory) Close() error {
	if f.releaseSecrets != nil {
		f.releaseSecrets()
	}
	return nil
}
