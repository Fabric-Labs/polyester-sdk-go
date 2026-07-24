package polyester

import (
	"context"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/Fabric-Labs/polyester-sdk-go/auth"
	"github.com/Fabric-Labs/polyester-sdk-go/catalogs"
	"github.com/Fabric-Labs/polyester-sdk-go/realtime"
	"github.com/Fabric-Labs/polyester-sdk-go/services"
	"github.com/Fabric-Labs/polyester-sdk-go/transport"
)

const (
	DefaultAPIURL = "https://api-devnet.polyester.ai"
	DefaultWSURL  = "wss://api-devnet.polyester.ai"
)

// Config configures a Polyester client.
type Config struct {
	APIKeyID            string
	APIPrivateKey       string
	APIURL              string
	WSURL               string
	DefaultSubAccountID *string
	DefaultAccountID    *string
	Timeout             time.Duration
	WireFormat          string
	HydrateCatalogs     bool
	HTTPClient          *http.Client
}

// Client is the root Polyester SDK entrypoint.
type Client struct {
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
	Whiteboard         *services.WhiteboardService
	Polychart          *services.PolychartService
	Layout             *services.LayoutService
	GuardSigner        *services.GuardSignerService
	Withdraw           *services.WithdrawService
	TradingWithdraws   *services.WithdrawService

	transport            *transport.Factory
	closeOnce            sync.Once
	catalogHydrationDone chan struct{}
}

// New creates a Polyester client.
func New(cfg Config) (*Client, error) {
	if cfg.APIURL == "" {
		cfg.APIURL = DefaultAPIURL
	}
	if cfg.WSURL == "" {
		cfg.WSURL = DefaultWSURL
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 10 * time.Second
	}
	wireFormat := transport.WireFormatFromString(cfg.WireFormat)
	creds, err := auth.LoadCredentials(cfg.APIKeyID, cfg.APIPrivateKey, false)
	if err != nil {
		return nil, err
	}
	tcfg := transport.Config{
		APIURL:     cfg.APIURL,
		WSURL:      cfg.WSURL,
		Timeout:    cfg.Timeout,
		WireFormat: wireFormat,
	}
	factory := transport.NewFactory(tcfg, creds, cfg.HTTPClient)
	rt := realtime.NewClient(cfg.WSURL, cfg.APIURL, creds, factory.HTTP, 1000)
	cats := catalogs.NewManager()

	defaultAccountID := cfg.DefaultAccountID
	if defaultAccountID == nil {
		if accountID := auth.AccountIDFromEnv(); accountID != "" {
			defaultAccountID = &accountID
		}
	}

	client := &Client{
		APIURL:               cfg.APIURL,
		WSURL:                cfg.WSURL,
		DefaultSubAccountID:  cfg.DefaultSubAccountID,
		DefaultAccountID:     defaultAccountID,
		Catalogs:             cats,
		Realtime:             rt,
		transport:            factory,
		Auth:                 services.NewAuthService(factory, rt),
		MarketData:           services.NewMarketDataService(factory, cats, rt),
		MarketOverview:       services.NewMarketOverviewService(factory, rt),
		Zipper:               services.NewZipperService(factory, cats, rt),
		ChainAnalytics:       services.NewChainAnalyticsService(factory),
		Heatmap:              services.NewHeatmapService(factory, cats, rt),
		Lifecycle:            services.NewLifecycleService(factory, rt),
		Balances:             services.NewBalancesService(factory, cats, cfg.DefaultSubAccountID, rt, defaultAccountID),
		Orderbook:            services.NewOrderbookService(factory, cats, rt),
		Orders:               services.NewOrdersService(factory, cats, cfg.DefaultSubAccountID, rt, defaultAccountID),
		Trades:               services.NewTradesService(factory, cats, cfg.DefaultSubAccountID, rt, defaultAccountID),
		Triggers:             services.NewTriggersService(factory, cats, cfg.DefaultSubAccountID, rt, defaultAccountID),
		Transfers:            services.NewTransfersService(factory, cfg.DefaultSubAccountID, rt, defaultAccountID),
		InternalTransfers:    services.NewInternalTransfersService(factory, cats, cfg.DefaultSubAccountID),
		Deposit:              services.NewDepositService(factory, cfg.DefaultSubAccountID),
		APIKeys:              services.NewApiKeysService(factory, cfg.DefaultSubAccountID, rt, defaultAccountID),
		Policies:             services.NewPoliciesService(rt, defaultAccountID),
		SubAccounts:          services.NewSubAccountsService(factory, cfg.DefaultSubAccountID, rt, defaultAccountID),
		AddressBook:          services.NewAddressBookService(factory, cfg.DefaultSubAccountID, rt, defaultAccountID),
		SocialVerification:   services.NewSocialVerificationService(factory),
		Whiteboard:           services.NewWhiteboardService(factory),
		Polychart:            services.NewPolychartService(factory),
		Layout:               services.NewLayoutService(factory),
		GuardSigner:          services.NewGuardSignerService(factory, cfg.DefaultSubAccountID),
		Withdraw:             services.NewWithdrawService(factory, cfg.DefaultSubAccountID),
		catalogHydrationDone: make(chan struct{}),
	}
	client.Candles = client.MarketData
	client.TradingWithdraws = client.Withdraw

	if cfg.HydrateCatalogs {
		go func() {
			defer close(client.catalogHydrationDone)
			client.hydrateCatalogsBestEffort()
		}()
	} else {
		close(client.catalogHydrationDone)
	}
	return client, nil
}

// FromEnv creates a client using POLYESTER_* environment variables.
func FromEnv(overrides ...func(*Config)) (*Client, error) {
	cfg := Config{HydrateCatalogs: true}
	if id := strings.TrimSpace(os.Getenv(auth.APIKeyIDEnv)); id != "" {
		cfg.APIKeyID = id
	}
	if key := strings.TrimSpace(os.Getenv(auth.APIPrivateKeyEnv)); key != "" {
		cfg.APIPrivateKey = key
	}
	if accountID := auth.AccountIDFromEnv(); accountID != "" {
		cfg.DefaultAccountID = &accountID
	}
	for _, override := range overrides {
		if override != nil {
			override(&cfg)
		}
	}
	return New(cfg)
}

func (c *Client) hydrateCatalogsBestEffort() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if spot, err := c.MarketData.GetSpotConfig(ctx); err == nil {
		c.Catalogs.HydrateSpotConfig(spot.Raw)
	}
	if zipper, err := c.Zipper.GetDepositWithdrawConfig(ctx); err == nil {
		c.Catalogs.HydrateZipperConfig(zipper)
	}
}

// WaitForCatalogs waits for the client's best-effort background catalog hydration.
// It returns immediately when HydrateCatalogs is disabled. Hydration failures remain
// best-effort; callers receive an error only when the context is canceled.
func (c *Client) WaitForCatalogs(ctx context.Context) error {
	select {
	case <-c.catalogHydrationDone:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Close releases client resources.
func (c *Client) Close() error {
	c.closeOnce.Do(func() {
		_ = c.transport.Close()
	})
	return nil
}
