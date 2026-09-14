package polyester

import "github.com/Fabric-Labs/polyester-sdk-go/chain"

// Environment is the first-class network configuration passed to New.
type Environment = chain.PolyesterEnvironment

var (
	// DevnetEnvironment is the default client / chain-helper network.
	DevnetEnvironment = chain.PolyesterDevnetEnvironment
	// TestnetEnvironment is public testnet (api-testnet.polyester.com, chain 888169).
	TestnetEnvironment = chain.PolyesterTestnetEnvironment
)

// CreateEnvironment validates a complete environment, including custom / VPC URLs.
func CreateEnvironment(params chain.CreateEnvironmentParams) (Environment, error) {
	return chain.CreatePolyesterEnvironment(params)
}

// EnvironmentFromName resolves "devnet" / "testnet".
func EnvironmentFromName(name string) (Environment, error) {
	return chain.EnvironmentFromName(name)
}
