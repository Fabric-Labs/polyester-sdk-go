package chain

import (
	"fmt"
	"net"
	"net/url"
	"strings"

	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	"github.com/ethereum/go-ethereum/common"
)

// EnvNameEnv selects a named preset for polyester.FromEnv (devnet | testnet).
const EnvNameEnv = "POLYESTER_ENV"

// EntryPointConfig pins an ERC-4337 EntryPoint deployment.
type EntryPointConfig struct {
	Address string
	Version string
}

// SafeDeploymentConfig pins Safe / 4337 module addresses for CREATE2 prediction
// and UserOp construction.
type SafeDeploymentConfig struct {
	Version                  string
	SafeModuleSetupAddress   string
	Safe4337ModuleAddress    string
	SafeProxyFactoryAddress  string
	SafeSingletonAddress     string
	MultiSendAddress         string
	MultiSendCallOnlyAddress string
}

// AccountAbstractionEnvironment holds bundler / paymaster / EntryPoint / Safe pins.
type AccountAbstractionEnvironment struct {
	BundlerURL   string
	PaymasterURL string
	EntryPoint   EntryPointConfig
	Safe         SafeDeploymentConfig
}

// ContractsEnvironment holds Polyester contract addresses used by Funding UserOps.
type ContractsEnvironment struct {
	TradingGatewayAddress string
	FundingAccountAddress string
	GuardRegistryAddress  string
	ZipperEndpointAddress string
}

// PolyesterEnvironment is the complete SDK environment: API, realtime, RPC, AA,
// and contract pins. PolyesterChainEnvironment is a backward-compatible alias.
type PolyesterEnvironment struct {
	Name               string
	APIURL             string
	WebsocketURL       string
	RPCURL             string
	ChainID            int64
	ChainName          string
	ExplorerURL        string
	AccountAbstraction AccountAbstractionEnvironment
	Contracts          ContractsEnvironment
}

// PolyesterChainEnvironment is the historical name for PolyesterEnvironment.
type PolyesterChainEnvironment = PolyesterEnvironment

// CreateEnvironmentParams is the input to CreatePolyesterEnvironment.
type CreateEnvironmentParams struct {
	Name               string
	APIURL             string
	WebsocketURL       string
	RPCURL             string
	ChainID            int64
	ChainName          string
	ExplorerURL        string
	AccountAbstraction AccountAbstractionEnvironment
	Contracts          ContractsEnvironment
	AllowInsecure      bool
}

var localHosts = map[string]struct{}{
	"localhost": {},
	"127.0.0.1": {},
	"::1":       {},
	"[::1]":     {},
}

func isLocalHost(hostname string) bool {
	host := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(hostname)), ".")
	if _, ok := localHosts[host]; ok {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func normalizeEnvURL(value, label string, allowed []string, allowSearch, allowInsecure bool) (string, error) {
	if strings.TrimSpace(value) == "" {
		return "", &sdkerrors.ValidationError{Msg: label + " must be a non-empty string"}
	}
	if value != strings.TrimSpace(value) {
		return "", &sdkerrors.ValidationError{Msg: label + " must not contain surrounding whitespace"}
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", &sdkerrors.ValidationError{Msg: label + " must be a valid URL"}
	}
	okScheme := false
	for _, scheme := range allowed {
		if parsed.Scheme == scheme {
			okScheme = true
			break
		}
	}
	if !okScheme {
		return "", &sdkerrors.ValidationError{Msg: label + " must use " + strings.Join(allowed, " or ")}
	}
	insecure := (parsed.Scheme == "http" || parsed.Scheme == "ws") && !isLocalHost(parsed.Hostname())
	if insecure && !allowInsecure {
		return "", &sdkerrors.ValidationError{Msg: label + " must use a secure protocol for remote hosts"}
	}
	if !allowSearch && parsed.RawQuery != "" {
		return "", &sdkerrors.ValidationError{Msg: label + " must not include query parameters"}
	}
	if parsed.Fragment != "" {
		return "", &sdkerrors.ValidationError{Msg: label + " must not include a fragment"}
	}
	return strings.TrimRight(value, "/"), nil
}

func normalizeEnvAddress(value, label string) (string, error) {
	if !common.IsHexAddress(value) {
		return "", &sdkerrors.ValidationError{Msg: label + " must be a valid address"}
	}
	return common.HexToAddress(value).Hex(), nil
}

func normalizeEntryPoint(entryPoint EntryPointConfig) (EntryPointConfig, error) {
	if entryPoint.Version != "0.7" {
		return EntryPointConfig{}, &sdkerrors.ValidationError{Msg: "accountAbstraction.entryPoint.version must be 0.7"}
	}
	addr, err := normalizeEnvAddress(entryPoint.Address, "accountAbstraction.entryPoint.address")
	if err != nil {
		return EntryPointConfig{}, err
	}
	return EntryPointConfig{Address: addr, Version: entryPoint.Version}, nil
}

func normalizeSafe(safe SafeDeploymentConfig) (SafeDeploymentConfig, error) {
	if safe.Version != "1.4.1" && safe.Version != "1.5.0" {
		return SafeDeploymentConfig{}, &sdkerrors.ValidationError{
			Msg: `accountAbstraction.safe.version must be either "1.4.1" or "1.5.0"`,
		}
	}
	setup, err := normalizeEnvAddress(safe.SafeModuleSetupAddress, "accountAbstraction.safe.safeModuleSetupAddress")
	if err != nil {
		return SafeDeploymentConfig{}, err
	}
	module, err := normalizeEnvAddress(safe.Safe4337ModuleAddress, "accountAbstraction.safe.safe4337ModuleAddress")
	if err != nil {
		return SafeDeploymentConfig{}, err
	}
	factory, err := normalizeEnvAddress(safe.SafeProxyFactoryAddress, "accountAbstraction.safe.safeProxyFactoryAddress")
	if err != nil {
		return SafeDeploymentConfig{}, err
	}
	singleton, err := normalizeEnvAddress(safe.SafeSingletonAddress, "accountAbstraction.safe.safeSingletonAddress")
	if err != nil {
		return SafeDeploymentConfig{}, err
	}
	multiSend, err := normalizeEnvAddress(safe.MultiSendAddress, "accountAbstraction.safe.multiSendAddress")
	if err != nil {
		return SafeDeploymentConfig{}, err
	}
	callOnly := ""
	if safe.MultiSendCallOnlyAddress != "" {
		callOnly, err = normalizeEnvAddress(safe.MultiSendCallOnlyAddress, "accountAbstraction.safe.multiSendCallOnlyAddress")
		if err != nil {
			return SafeDeploymentConfig{}, err
		}
	}
	return SafeDeploymentConfig{
		Version:                  safe.Version,
		SafeModuleSetupAddress:   setup,
		Safe4337ModuleAddress:    module,
		SafeProxyFactoryAddress:  factory,
		SafeSingletonAddress:     singleton,
		MultiSendAddress:         multiSend,
		MultiSendCallOnlyAddress: callOnly,
	}, nil
}

// CreatePolyesterEnvironment validates and normalizes a complete environment.
func CreatePolyesterEnvironment(params CreateEnvironmentParams) (PolyesterEnvironment, error) {
	if strings.TrimSpace(params.Name) == "" {
		return PolyesterEnvironment{}, &sdkerrors.ValidationError{Msg: "name must be a non-empty string"}
	}
	if params.ChainID <= 0 {
		return PolyesterEnvironment{}, &sdkerrors.ValidationError{Msg: "chainID must be a positive integer"}
	}
	apiURL, err := normalizeEnvURL(params.APIURL, "apiURL", []string{"https", "http"}, false, params.AllowInsecure)
	if err != nil {
		return PolyesterEnvironment{}, err
	}
	wsURL, err := normalizeEnvURL(params.WebsocketURL, "websocketURL", []string{"wss", "ws"}, true, params.AllowInsecure)
	if err != nil {
		return PolyesterEnvironment{}, err
	}
	rpcURL, err := normalizeEnvURL(params.RPCURL, "rpcURL", []string{"https", "http"}, true, params.AllowInsecure)
	if err != nil {
		return PolyesterEnvironment{}, err
	}
	bundlerURL, err := normalizeEnvURL(params.AccountAbstraction.BundlerURL, "bundlerURL", []string{"https", "http"}, true, params.AllowInsecure)
	if err != nil {
		return PolyesterEnvironment{}, err
	}
	paymasterURL, err := normalizeEnvURL(params.AccountAbstraction.PaymasterURL, "paymasterURL", []string{"https", "http"}, true, params.AllowInsecure)
	if err != nil {
		return PolyesterEnvironment{}, err
	}
	entryPoint, err := normalizeEntryPoint(params.AccountAbstraction.EntryPoint)
	if err != nil {
		return PolyesterEnvironment{}, err
	}
	safe, err := normalizeSafe(params.AccountAbstraction.Safe)
	if err != nil {
		return PolyesterEnvironment{}, err
	}
	tradingGateway, err := normalizeEnvAddress(params.Contracts.TradingGatewayAddress, "contracts.tradingGatewayAddress")
	if err != nil {
		return PolyesterEnvironment{}, err
	}
	funding, err := normalizeEnvAddress(params.Contracts.FundingAccountAddress, "contracts.fundingAccountAddress")
	if err != nil {
		return PolyesterEnvironment{}, err
	}
	guard, err := normalizeEnvAddress(params.Contracts.GuardRegistryAddress, "contracts.guardRegistryAddress")
	if err != nil {
		return PolyesterEnvironment{}, err
	}
	zipper, err := normalizeEnvAddress(params.Contracts.ZipperEndpointAddress, "contracts.zipperEndpointAddress")
	if err != nil {
		return PolyesterEnvironment{}, err
	}
	return PolyesterEnvironment{
		Name:         strings.TrimSpace(params.Name),
		APIURL:       apiURL,
		WebsocketURL: wsURL,
		RPCURL:       rpcURL,
		ChainID:      params.ChainID,
		ChainName:    strings.TrimSpace(params.ChainName),
		ExplorerURL:  strings.TrimSpace(params.ExplorerURL),
		AccountAbstraction: AccountAbstractionEnvironment{
			BundlerURL:   bundlerURL,
			PaymasterURL: paymasterURL,
			EntryPoint:   entryPoint,
			Safe:         safe,
		},
		Contracts: ContractsEnvironment{
			TradingGatewayAddress: tradingGateway,
			FundingAccountAddress: funding,
			GuardRegistryAddress:  guard,
			ZipperEndpointAddress: zipper,
		},
	}, nil
}

// ParsePolyesterEnvironment re-validates an environment supplied to a client.
func ParsePolyesterEnvironment(environment PolyesterEnvironment) (PolyesterEnvironment, error) {
	return CreatePolyesterEnvironment(CreateEnvironmentParams{
		Name:               environment.Name,
		APIURL:             environment.APIURL,
		WebsocketURL:       environment.WebsocketURL,
		RPCURL:             environment.RPCURL,
		ChainID:            environment.ChainID,
		ChainName:          environment.ChainName,
		ExplorerURL:        environment.ExplorerURL,
		AccountAbstraction: environment.AccountAbstraction,
		Contracts:          environment.Contracts,
	})
}

// WithURLs copies env with endpoint overrides (market-maker / VPC case).
func (env PolyesterEnvironment) WithURLs(apiURL, websocketURL, rpcURL string) (PolyesterEnvironment, error) {
	if apiURL == "" {
		apiURL = env.APIURL
	}
	if websocketURL == "" {
		websocketURL = env.WebsocketURL
	}
	if rpcURL == "" {
		rpcURL = env.RPCURL
	}
	return CreatePolyesterEnvironment(CreateEnvironmentParams{
		Name:               env.Name,
		APIURL:             apiURL,
		WebsocketURL:       websocketURL,
		RPCURL:             rpcURL,
		ChainID:            env.ChainID,
		ChainName:          env.ChainName,
		ExplorerURL:        env.ExplorerURL,
		AccountAbstraction: env.AccountAbstraction,
		Contracts:          env.Contracts,
	})
}

// EnvironmentFromName resolves "devnet" / "testnet" (and polyester-* aliases).
func EnvironmentFromName(name string) (PolyesterEnvironment, error) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "devnet", "polyester-devnet":
		return PolyesterDevnetEnvironment, nil
	case "testnet", "polyester-testnet":
		return PolyesterTestnetEnvironment, nil
	default:
		return PolyesterEnvironment{}, &sdkerrors.ValidationError{
			Msg: fmt.Sprintf("unknown environment %q; expected \"devnet\" or \"testnet\"", name),
		}
	}
}

func mustEnvironment(params CreateEnvironmentParams) PolyesterEnvironment {
	env, err := CreatePolyesterEnvironment(params)
	if err != nil {
		panic(err)
	}
	return env
}

// PolyesterDevnetEnvironment matches TypeScript POLYESTER_DEVNET_ENVIRONMENT
// plus zipper catalog extras (funding / guard / zipper endpoint).
var PolyesterDevnetEnvironment = mustEnvironment(CreateEnvironmentParams{
	Name:         "polyester-devnet",
	APIURL:       "https://api-devnet.polyester.ai",
	WebsocketURL: "wss://api-devnet.polyester.ai",
	RPCURL:       "https://rpc.polyester.tech",
	ChainID:      888168,
	ChainName:    "Polyester Chain Devnet",
	ExplorerURL:  "https://devnet.polyesterscan.com",
	AccountAbstraction: AccountAbstractionEnvironment{
		BundlerURL:   "https://bundler.polyester.tech",
		PaymasterURL: "https://paymaster.polyester.tech",
		EntryPoint: EntryPointConfig{
			Address: "0x59a4B77766509c4507D79eFF8089474eC3daC174",
			Version: "0.7",
		},
		Safe: SafeDeploymentConfig{
			Version:                  "1.4.1",
			SafeModuleSetupAddress:   "0x80791683D9C079A37Debc67EaDdbFcBC6f0FF2bB",
			Safe4337ModuleAddress:    "0x0713FF3d4c1b4f177833a372b1e3cb977540EA11",
			SafeProxyFactoryAddress:  "0xF8F0F649Dd3bFa9095206691E9fb2356c26216dE",
			SafeSingletonAddress:     "0x92abEa238FEA8908c397cE65366ea9278f0AeC7A",
			MultiSendAddress:         "0x70C8a8CcB45a8E2589B0f019374fc923dA34E4c7",
			MultiSendCallOnlyAddress: "0x375C86a08DA98d1944D7B3c736307A72186CcAf1",
		},
	},
	Contracts: ContractsEnvironment{
		TradingGatewayAddress: "0xD3fecf5D39131e23b6B0f872cA0a21c8A5a30932",
		FundingAccountAddress: "0xBfF4F6224BC10f233dDB1E61E770d9832aabC7c4",
		GuardRegistryAddress:  "0xd71F60FD6f784Cc0aD8c25441568C48705D95f64",
		ZipperEndpointAddress: "0xae6B981BE9B73421eB1ba5372d1A4A937d63ffFB",
	},
})

// PolyesterTestnetEnvironment matches TypeScript POLYESTER_TESTNET_ENVIRONMENT
// plus zipper catalog extras.
var PolyesterTestnetEnvironment = mustEnvironment(CreateEnvironmentParams{
	Name:         "polyester-testnet",
	APIURL:       "https://api-testnet.polyester.com",
	WebsocketURL: "wss://api-testnet.polyester.com",
	RPCURL:       "https://rpc.polyester.live",
	ChainID:      888169,
	ChainName:    "Polyester Chain Testnet",
	ExplorerURL:  "https://testnet.polyesterscan.com",
	AccountAbstraction: AccountAbstractionEnvironment{
		BundlerURL:   "https://bundler.polyester.live",
		PaymasterURL: "https://paymaster.polyester.live",
		EntryPoint: EntryPointConfig{
			Address: "0x35c524a72ffb4D348d616cDD340D176c8f3C8B2C",
			Version: "0.7",
		},
		Safe: SafeDeploymentConfig{
			Version:                  "1.4.1",
			SafeModuleSetupAddress:   "0xdA9510c95Ab50EAd5A3DD28FA6BACce497dCF1fB",
			Safe4337ModuleAddress:    "0xE278E4BCb71b095f7dAaa1bcEc1950696Fc40C74",
			SafeProxyFactoryAddress:  "0x2b8250158D58dD6D5e89313fa940586C9054A547",
			SafeSingletonAddress:     "0x6f00AB12B6A8aFf400F14f4Cd738549f0F53390d",
			MultiSendAddress:         "0xA38fEFA19ff5d8E3d988b2a0e6C8A2ae099fd97D",
			MultiSendCallOnlyAddress: "0xE99b6c6d550B322347EeE11f4e8643377D8475A8",
		},
	},
	Contracts: ContractsEnvironment{
		TradingGatewayAddress: "0x20ef1BCeE69D73Ce1649E688dAA9A7AcF441f0EE",
		FundingAccountAddress: "0x57D15F393772041b4107943CAFa8A70b620212D1",
		GuardRegistryAddress:  "0xB0E23DDa102c5d37AcBA583cf84E5aC214e7521C",
		ZipperEndpointAddress: "0xD439270f881b56727EaaB878CE4e80eB08A25BEB",
	},
})
