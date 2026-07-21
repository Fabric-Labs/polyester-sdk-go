package chain

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

// PolyesterChainEnvironment is on-chain / AA settings for Funding UserOps
// (not API-key Connect).
type PolyesterChainEnvironment struct {
	Name               string
	APIURL             string
	WebsocketURL       string
	RPCURL             string
	ChainID            int64
	AccountAbstraction AccountAbstractionEnvironment
	Contracts          ContractsEnvironment
}

// PolyesterTestnetEnvironment matches the Python / TypeScript testnet pins
// (chain id 888168).
var PolyesterTestnetEnvironment = PolyesterChainEnvironment{
	Name:         "polyester-testnet",
	APIURL:       "https://api-devnet.polyester.ai",
	WebsocketURL: "wss://api-devnet.polyester.ai",
	RPCURL:       "https://rpc.polyester.tech",
	ChainID:      888168,
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
}
