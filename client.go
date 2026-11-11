package polyester

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"connectrpc.com/connect"
	"github.com/Fabric-Labs/polyester-sdk-go/auth"
	"github.com/Fabric-Labs/polyester-sdk-go/catalogs"
	"github.com/Fabric-Labs/polyester-sdk-go/chain"
	"github.com/Fabric-Labs/polyester-sdk-go/codecs/decode"
	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	zipperv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/chain/zipper/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/gen/chain/zipper/v1/chainzipperv1connect"
	marketdatav1 "github.com/Fabric-Labs/polyester-sdk-go/gen/marketdata/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/gen/marketdata/v1/marketdatav1connect"
	"github.com/Fabric-Labs/polyester-sdk-go/internal/redact"
	"github.com/Fabric-Labs/polyester-sdk-go/realtime"
	"github.com/Fabric-Labs/polyester-sdk-go/services"
	"github.com/Fabric-Labs/polyester-sdk-go/transport"
)

var (
	DefaultAPIURL = chain.PolyesterDevnetEnvironment.APIURL
	DefaultWSURL  = chain.PolyesterDevnetEnvironment.WebsocketURL
)

// Config configures a Polyester client.
//
// APIPrivateKey is an auth.Secret, so no printing or encoding path reveals it:
// fmt prints [REDACTED] for every verb, also through reflection such as an
// unexported struct field; encoding/json, encoding/xml and log/slog write
// "[REDACTED]"; gob refuses to encode a Config. Decoding JSON still reads
// APIPrivateKey.
type Config struct {
	Environment         *chain.PolyesterEnvironment
	APIKeyID            string
	APIPrivateKey       auth.Secret
	APIURL              string
	WSURL               string
	DefaultSubAccountID *string
	DefaultAccountID    *string
	Timeout             time.Duration
	WireFormat          string
	HydrateCatalogs     bool
	HTTPClient          *http.Client
	// RecvWindow is the receive window of every signed API request: the
	// server processes a request only until its signing timestamp plus
	// RecvWindow, and refuses it after that as a retryable
	// errors.TimestampSkewError, or errors.CodeRequestDeadlineExceeded once
	// authenticated, having changed nothing. It is sent as the signed
	// X-API-RECV-WINDOW header and must be whole milliseconds from 1ms to
	// 60s; New refuses any other value. Zero sends no header, and the server
	// applies 10 seconds. Realtime token requests always use the default.
	RecvWindow time.Duration
	// CatalogTimeout bounds the catalog loads HydrateCatalogs begins: the
	// spot config and the funding (deposit/withdraw) config each load on
	// their own, and a read that times out or loses its connection is read
	// again, at most 3 times in all, 250ms and then 500ms apart, each retry
	// on connections of its own, all within CatalogTimeout of New. Zero
	// means twice Timeout; New refuses a negative value.
	CatalogTimeout time.Duration
}

// String redacts the API private key.
func (c Config) String() string {
	key := c.APIPrivateKey.String()
	envName := ""
	if c.Environment != nil {
		envName = c.Environment.Name
	}
	return fmt.Sprintf(
		"Config{Environment:%q APIKeyID:%q APIPrivateKey:%q APIURL:%q WSURL:%q Timeout:%s WireFormat:%q HydrateCatalogs:%t RecvWindow:%s CatalogTimeout:%s}",
		envName, c.APIKeyID, key, redact.URL(c.APIURL), redact.URL(c.WSURL), c.Timeout, c.WireFormat, c.HydrateCatalogs, c.RecvWindow, c.CatalogTimeout,
	)
}

// GoString redacts the API private key for %#v formatting.
func (c Config) GoString() string {
	return c.String()
}

// Format redacts the API private key for every fmt verb, including verbs
// such as %d that bypass String.
func (c Config) Format(f fmt.State, verb rune) {
	if verb == 'v' && f.Flag('#') {
		_, _ = io.WriteString(f, c.GoString())
		return
	}
	_, _ = fmt.Fprintf(f, fmt.FormatString(f, verb), c.String())
}

// redactedConfig is what encoders see. It lists Config's settings
// explicitly, so a field added to Config stays out of encoded output until it
// is added here. HTTPClient is not a setting that can be encoded.
type redactedConfig struct {
	Environment         *chain.PolyesterEnvironment
	APIKeyID            string
	APIPrivateKey       string
	APIURL              string
	WSURL               string
	DefaultSubAccountID *string
	DefaultAccountID    *string
	Timeout             time.Duration
	WireFormat          string
	HydrateCatalogs     bool
	RecvWindow          time.Duration
	CatalogTimeout      time.Duration
}

// redacted replaces the private key and URL userinfo, as String does. An
// unset key stays empty.
func (c Config) redacted() redactedConfig {
	key := c.APIPrivateKey.String()
	return redactedConfig{
		Environment:         c.Environment,
		APIKeyID:            c.APIKeyID,
		APIPrivateKey:       key,
		APIURL:              redact.URL(c.APIURL),
		WSURL:               redact.URL(c.WSURL),
		DefaultSubAccountID: c.DefaultSubAccountID,
		DefaultAccountID:    c.DefaultAccountID,
		Timeout:             c.Timeout,
		WireFormat:          c.WireFormat,
		HydrateCatalogs:     c.HydrateCatalogs,
		RecvWindow:          c.RecvWindow,
		CatalogTimeout:      c.CatalogTimeout,
	}
}

// MarshalJSON encodes Config with the API private key and URL userinfo
// redacted. This also covers log/slog's JSON handler. Decoding still reads
// APIPrivateKey.
func (c Config) MarshalJSON() ([]byte, error) {
	return json.Marshal(c.redacted())
}

// MarshalXML encodes Config with the API private key redacted.
func (c Config) MarshalXML(e *xml.Encoder, start xml.StartElement) error {
	return e.EncodeElement(c.redacted(), start)
}

// GobEncode refuses to encode Config, because gob would carry the API private
// key.
func (Config) GobEncode() ([]byte, error) {
	return nil, &sdkerrors.AuthError{Msg: "polyester.Config holds an API private key and cannot be gob-encoded"}
}

// recvWindowMilliseconds converts Config.RecvWindow to the header's whole
// milliseconds, refusing a value the server would refuse.
func recvWindowMilliseconds(window time.Duration) (int, error) {
	if window == 0 {
		return 0, nil
	}
	if window%time.Millisecond != 0 || window < auth.MinRecvWindowMS*time.Millisecond || window > auth.MaxRecvWindowMS*time.Millisecond {
		return 0, &sdkerrors.ValidationError{Msg: "RecvWindow must be whole milliseconds from 1ms to 60s, or 0 for the server default"}
	}
	return int(window / time.Millisecond), nil
}

// Client is the root Polyester SDK entrypoint.
type Client struct {
	Environment         chain.PolyesterEnvironment
	APIURL              string
	WSURL               string
	DefaultSubAccountID *string
	DefaultAccountID    *string

	Catalogs *catalogs.Manager
	Realtime *realtime.Client

	Auth               *services.AuthService
	MarketData         *services.MarketDataService
	Candles            *services.MarketDataService
	MarketOverview     *services.MarketOverviewService
	Zipper             *services.ZipperService
	ChainAnalytics     *services.ChainAnalyticsService
	Heatmap            *services.HeatmapService
	Lifecycle          *services.LifecycleService
	Balances           *services.BalancesService
	Orderbook          *services.OrderbookService
	Orders             *services.OrdersService
	Trades             *services.TradesService
	Triggers           *services.TriggersService
	Transfers          *services.TransfersService
	InternalTransfers  *services.InternalTransfersService
	Deposit            *services.DepositService
	APIKeys            *services.ApiKeysService
	Policies           *services.PoliciesService
	SubAccounts        *services.SubAccountsService
	AddressBook        *services.AddressBookService
	SocialVerification *services.SocialVerificationService
	GuardSigner        *services.GuardSignerService
	VIP                *services.VIPService
	Changelog          *services.ChangelogService
	Fees               *services.FeeService
	RateLimits         *services.RateLimitService
	Withdraw           *services.WithdrawService
	TradingWithdraws   *services.WithdrawService

	transport            *transport.Factory
	closeOnce            sync.Once
	catalogHydrationDone chan struct{}
	fundingHydrationDone chan struct{}
	catalogMu            sync.Mutex
	catalogLastError     error
	fundingLastError     error
	stopHydration        context.CancelFunc
	timeout              time.Duration
}

// New creates a Polyester client.
func New(cfg Config) (*Client, error) {
	env := chain.PolyesterDevnetEnvironment
	if cfg.Environment != nil {
		parsed, err := chain.ParsePolyesterEnvironment(*cfg.Environment)
		if err != nil {
			return nil, err
		}
		env = parsed
	}
	if cfg.APIURL == "" {
		cfg.APIURL = env.APIURL
	}
	if err := validateAPIBaseURL(cfg.APIURL); err != nil {
		return nil, err
	}
	if cfg.WSURL == "" {
		cfg.WSURL = env.WebsocketURL
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 10 * time.Second
	}
	if cfg.CatalogTimeout < 0 {
		return nil, &sdkerrors.ValidationError{Msg: "CatalogTimeout must not be negative"}
	}
	if cfg.CatalogTimeout == 0 {
		cfg.CatalogTimeout = 2 * cfg.Timeout
	}
	wireFormat := transport.WireFormatFromString(cfg.WireFormat)
	recvWindowMS, err := recvWindowMilliseconds(cfg.RecvWindow)
	if err != nil {
		return nil, err
	}
	creds, err := auth.LoadCredentials(cfg.APIKeyID, cfg.APIPrivateKey.Reveal(), false)
	if err != nil {
		return nil, err
	}
	tcfg := transport.Config{
		APIURL:       cfg.APIURL,
		WSURL:        cfg.WSURL,
		Timeout:      cfg.Timeout,
		WireFormat:   wireFormat,
		RecvWindowMS: recvWindowMS,
	}
	factory := transport.NewFactory(tcfg, creds, cfg.HTTPClient)
	rt := realtime.NewClient(cfg.WSURL, cfg.APIURL, creds, factory.HTTP, 1000)
	cats := catalogs.NewManager()
	rt.SetBaseQuantityScales(cats.BaseQuantityScaleForSymbolID)

	defaultAccountID := cfg.DefaultAccountID
	if defaultAccountID == nil {
		if accountID := auth.AccountIDFromEnv(); accountID != "" {
			defaultAccountID = &accountID
		}
	}

	catalogHydrationDone := make(chan struct{})
	client := &Client{
		Environment:          env,
		APIURL:               cfg.APIURL,
		WSURL:                cfg.WSURL,
		DefaultSubAccountID:  cfg.DefaultSubAccountID,
		DefaultAccountID:     defaultAccountID,
		Catalogs:             cats,
		Realtime:             rt,
		transport:            factory,
		Auth:                 services.NewAuthService(factory, rt),
		MarketData:           services.NewMarketDataService(factory, cats, rt),
		MarketOverview:       services.NewMarketOverviewService(factory, cats, rt),
		Zipper:               services.NewZipperService(factory, cats, rt),
		ChainAnalytics:       services.NewChainAnalyticsService(factory),
		Heatmap:              services.NewHeatmapService(factory, cats, rt),
		Lifecycle:            services.NewLifecycleService(factory, rt),
		Balances:             services.NewBalancesService(factory, cats, cfg.DefaultSubAccountID, rt, defaultAccountID),
		Orderbook:            services.NewOrderbookService(factory, cats, rt),
		Trades:               services.NewTradesService(factory, cats, cfg.DefaultSubAccountID, rt, defaultAccountID),
		Transfers:            services.NewTransfersService(factory, cfg.DefaultSubAccountID, rt, defaultAccountID),
		InternalTransfers:    services.NewInternalTransfersService(factory, cats, cfg.DefaultSubAccountID),
		Deposit:              services.NewDepositService(factory, cfg.DefaultSubAccountID),
		APIKeys:              services.NewApiKeysService(factory, cfg.DefaultSubAccountID, rt, defaultAccountID),
		Policies:             services.NewPoliciesService(rt, defaultAccountID),
		SubAccounts:          services.NewSubAccountsService(factory, cfg.DefaultSubAccountID, rt, defaultAccountID),
		AddressBook:          services.NewAddressBookService(factory, cfg.DefaultSubAccountID, rt, defaultAccountID),
		SocialVerification:   services.NewSocialVerificationService(factory),
		GuardSigner:          services.NewGuardSignerService(factory, cfg.DefaultSubAccountID),
		VIP:                  services.NewVIPService(factory),
		Changelog:            services.NewChangelogService(factory),
		Fees:                 services.NewFeeService(factory, cats, cfg.DefaultSubAccountID),
		RateLimits:           services.NewRateLimitService(factory, cfg.DefaultSubAccountID),
		Withdraw:             services.NewWithdrawService(factory, cfg.DefaultSubAccountID),
		catalogHydrationDone: catalogHydrationDone,
		fundingHydrationDone: make(chan struct{}),
		timeout:              cfg.Timeout,
		stopHydration:        func() {},
	}
	client.Orders = services.NewOrdersService(factory, cats, cfg.DefaultSubAccountID, rt, defaultAccountID, catalogHydrationDone, client.CatalogsLastError)
	client.Triggers = services.NewTriggersService(factory, cats, cfg.DefaultSubAccountID, rt, defaultAccountID, catalogHydrationDone, client.CatalogsLastError)
	client.Candles = client.MarketData
	client.TradingWithdraws = client.Withdraw

	if cfg.HydrateCatalogs {
		// Begun before New returns: every call that needs a catalog waits for
		// it. The two load on their own, so spot trading never waits for, or
		// fails with, the funding config.
		deadline := time.Now().Add(cfg.CatalogTimeout)
		ctx, cancel := context.WithDeadline(context.Background(), deadline)
		client.stopHydration = cancel
		finishSpot := cats.BeginHydration()
		finishFunding := cats.BeginFundingHydration()
		go func() {
			defer close(client.catalogHydrationDone)
			finishSpot(client.hydrateSpot(ctx, deadline))
		}()
		go func() {
			defer close(client.fundingHydrationDone)
			finishFunding(client.hydrateFunding(ctx, deadline))
		}()
	} else {
		close(client.catalogHydrationDone)
		close(client.fundingHydrationDone)
	}
	return client, nil
}

func validateAPIBaseURL(raw string) error {
	if raw != strings.TrimSpace(raw) {
		return &sdkerrors.ValidationError{Msg: "APIURL must not contain surrounding whitespace"}
	}
	parsed, err := url.Parse(raw)
	if err != nil ||
		(parsed.Scheme != "http" && parsed.Scheme != "https") ||
		parsed.Host == "" ||
		parsed.Hostname() == "" ||
		parsed.Opaque != "" {
		return &sdkerrors.ValidationError{Msg: "APIURL must be an absolute HTTP(S) base URL"}
	}
	// url.Parse intentionally discards an empty fragment and represents a bare
	// query delimiter only through ForceQuery. Check the original text so no
	// literal delimiter can remain for Connect to append a procedure after.
	// Percent-escaped path content such as %3F and %23 remains legal.
	if strings.Contains(raw, "?") {
		return &sdkerrors.ValidationError{Msg: "APIURL base must not contain a query string"}
	}
	if strings.Contains(raw, "#") {
		return &sdkerrors.ValidationError{Msg: "APIURL base must not contain a fragment"}
	}
	return nil
}

// FromEnv creates a client using POLYESTER_* environment variables.
func FromEnv(overrides ...func(*Config)) (*Client, error) {
	cfg := Config{HydrateCatalogs: true}
	if id := strings.TrimSpace(os.Getenv(auth.APIKeyIDEnv)); id != "" {
		cfg.APIKeyID = id
	}
	if key := strings.TrimSpace(os.Getenv(auth.APIPrivateKeyEnv)); key != "" {
		cfg.APIPrivateKey = auth.NewSecret(key)
	}
	if accountID := auth.AccountIDFromEnv(); accountID != "" {
		cfg.DefaultAccountID = &accountID
	}
	if envName := strings.TrimSpace(os.Getenv(chain.EnvNameEnv)); envName != "" {
		env, err := chain.EnvironmentFromName(envName)
		if err != nil {
			return nil, err
		}
		cfg.Environment = &env
	}
	if apiURL := strings.TrimSpace(os.Getenv("POLYESTER_API_URL")); apiURL != "" {
		cfg.APIURL = apiURL
	}
	if wsURL := strings.TrimSpace(os.Getenv("POLYESTER_WS_URL")); wsURL != "" {
		cfg.WSURL = wsURL
	}
	for _, override := range overrides {
		if override != nil {
			override(&cfg)
		}
	}
	return New(cfg)
}

func (c *Client) hydrateSpot(ctx context.Context, deadline time.Time) error {
	source := catalogSource{
		catalog:   "spot",
		name:      "spot config",
		operation: spotConfigOperation,
		read: func(ctx context.Context, httpClient connect.HTTPClient) error {
			client := marketdatav1connect.NewMarketDataServiceClient(httpClient, c.transport.Config.APIURL, c.transport.ConnectOptions(false)...)
			resp, err := client.GetSpotConfig(ctx, connect.NewRequest(&marketdatav1.GetSpotConfigRequest{}))
			if err != nil {
				return err
			}
			spot, err := decode.SpotConfigFromProto(resp.Msg)
			if err != nil {
				return &catalogInvalid{err: err}
			}
			if len(spot.Raw) == 0 {
				return &catalogInvalid{err: &sdkerrors.ValidationError{Msg: "empty spot config"}}
			}
			if err := c.Catalogs.HydrateSpotConfig(spot.Raw); err != nil {
				return &catalogInvalid{err: err}
			}
			return nil
		},
	}
	err := loadCatalog(ctx, c.transport, source, c.timeout, deadline)
	c.catalogMu.Lock()
	c.catalogLastError = err
	c.catalogMu.Unlock()
	return err
}

func (c *Client) hydrateFunding(ctx context.Context, deadline time.Time) error {
	source := catalogSource{
		catalog:   "funding",
		name:      "funding config",
		operation: fundingConfigOperation,
		read: func(ctx context.Context, httpClient connect.HTTPClient) error {
			client := chainzipperv1connect.NewZipperServiceClient(httpClient, c.transport.Config.APIURL, c.transport.ConnectOptions(false)...)
			resp, err := client.GetDepositWithdrawConfig(ctx, connect.NewRequest(&zipperv1.GetDepositWithdrawConfigRequest{}))
			if err != nil {
				return err
			}
			if err := c.Catalogs.HydrateZipperConfig(decode.DepositWithdrawConfigFromProto(resp.Msg)); err != nil {
				return &catalogInvalid{err: err}
			}
			return nil
		},
	}
	err := loadCatalog(ctx, c.transport, source, c.timeout, deadline)
	c.catalogMu.Lock()
	c.fundingLastError = err
	c.catalogMu.Unlock()
	return err
}

// CatalogsLastError returns why the spot config failed to load, if it did: a
// *errors.CatalogHydrationError whose Err is the failure.
func (c *Client) CatalogsLastError() error {
	c.catalogMu.Lock()
	defer c.catalogMu.Unlock()
	return c.catalogLastError
}

// WaitForCatalogs waits until the spot config, which every spot trading call
// needs, is ready. It returns nil once that has loaded and validated, without
// waiting for the funding config (see WaitForFundingCatalog), and at once when
// HydrateCatalogs is disabled. When the spot config could not be loaded it
// returns a *errors.CatalogHydrationError naming the operation, the kind of
// failure and the response reference, whose Err is the failure; calls that
// name a market then fail closed with it too. It returns ctx's error when ctx
// ends first; the load carries on.
func (c *Client) WaitForCatalogs(ctx context.Context) error {
	select {
	case <-c.catalogHydrationDone:
		return c.CatalogsLastError()
	case <-ctx.Done():
		return ctx.Err()
	}
}

// WaitForFundingCatalog waits until the funding (deposit/withdraw) config is
// ready, and returns a *errors.CatalogHydrationError naming it when it could
// not be loaded. Read Catalogs.Zipper, LedgerIDForAsset and
// QuantityScaleForAsset after it returns nil. It returns at once when
// HydrateCatalogs is disabled, and ctx's error when ctx ends first.
func (c *Client) WaitForFundingCatalog(ctx context.Context) error {
	select {
	case <-c.fundingHydrationDone:
		c.catalogMu.Lock()
		defer c.catalogMu.Unlock()
		return c.fundingLastError
	case <-ctx.Done():
		return ctx.Err()
	}
}

// StopAllTrading cancels every live trigger in scope, then all open orders in
// scope; see services.StopAllTrading. Orders.CancelAll and the dead-man
// switch cancel open orders only, never triggers.
func (c *Client) StopAllTrading(ctx context.Context, account services.AccountScope, scope services.StopAllTradingScope) (services.StopAllTradingResult, error) {
	return services.StopAllTrading(ctx, c.Triggers, c.Orders, account, scope)
}

// Close releases client resources, including tracked realtime subscriptions.
func (c *Client) Close() error {
	c.closeOnce.Do(func() {
		c.stopHydration()
		if c.Realtime != nil {
			c.Realtime.Close()
		}
		_ = c.transport.Close()
	})
	return nil
}
