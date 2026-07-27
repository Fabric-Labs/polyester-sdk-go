package transport

import (
	"net/http"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/Fabric-Labs/polyester-sdk-go/auth"
	"github.com/Fabric-Labs/polyester-sdk-go/connectx"
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
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
		APIURL:     "https://api-devnet.polyester.ai",
		WSURL:      "wss://api-devnet.polyester.ai",
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
}

// NewFactory builds a transport factory.
func NewFactory(cfg Config, creds *auth.Credentials, httpClient *http.Client) *Factory {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: cfg.Timeout}
	}
	httpClient = useragent.WrapClient(httpClient)
	f := &Factory{
		Config:      cfg,
		Credentials: creds,
		HTTP:        httpClient,
	}
	if creds != nil {
		f.authInterceptor = NewAPIKeyInterceptor(creds, cfg.APIURL, cfg.WireFormat)
	}
	return f
}

// ConnectOptions returns client options for generated Connect clients.
func (f *Factory) ConnectOptions(authenticated bool) []connect.ClientOption {
	opts := []connect.ClientOption{connect.WithReadMaxBytes(MaxConnectResponseBytes)}
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

// Close is a no-op when the HTTP client is caller-owned.
func (f *Factory) Close() error { return nil }
