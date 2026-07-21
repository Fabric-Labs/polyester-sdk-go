package chain

import (
	"encoding/hex"
	"fmt"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

// SAFE_PROXY_CREATION_CODE is SafeProxy creation bytecode from
// @safe-global/safe-contracts v1.4.1 (must match SafeProxyFactory.proxyCreationCode
// on Polyester). Copied exactly from Python / TypeScript.
const SAFE_PROXY_CREATION_CODE = "" +
	"608060405234801561001057600080fd5b506040516101e63803806101e68339818101604052602081101561003357600080fd5b" +
	"8101908080519060200190929190505050600073ffffffffffffffffffffffffffffffffffffffff168173ffffffffffffffff" +
	"ffffffffffffffffffffffff1614156100ca576040517f08c379a0000000000000000000000000000000000000000000000000" +
	"0000000081526004018080602001828103825260228152602001806101c46022913960400191505060405180910390fd5b8060" +
	"00806101000a81548173ffffffffffffffffffffffffffffffffffffffff021916908373ffffffffffffffffffffffffffffff" +
	"ffffffffff1602179055505060ab806101196000396000f3fe608060405273ffffffffffffffffffffffffffffffffffffffff" +
	"600054167fa619486e000000000000000000000000000000000000000000000000000000006000351415605057806000526020" +
	"6000f35b3660008037600080366000845af43d6000803e60008114156070573d6000fd5b3d6000f3fea2646970667358221220" +
	"03d1488ee65e08fa41e58e888a9865554c535f2c77126a82cb4c0f917f31441364736f6c63430007060033496e76616c696420" +
	"73696e676c65746f6e20616464726573732070726f7669646564"

var (
	setupSelector         = mustDecodeHex("b63e800d")
	enableModulesSelector = mustDecodeHex("8d0dc49f")
	multiSendSelector     = mustDecodeHex("8d80ff0a")
	createProxySelector   = mustDecodeHex("1688f0b9")

	typeAddress    = mustABIType("address")
	typeAddressArr = mustABIType("address[]")
	typeUint256    = mustABIType("uint256")
	typeBytes      = mustABIType("bytes")
)

// PredictedSafe is a CREATE2 Safe address plus factory deploy data (zero RPC).
type PredictedSafe struct {
	Address         string
	Initializer     []byte
	FactoryCalldata []byte
}

// PredictSafeAddressOptions configures CREATE2 Safe prediction.
type PredictSafeAddressOptions struct {
	Owners      []string
	SaltNonce   *big.Int
	Threshold   *big.Int
	Safe        *SafeDeploymentConfig
	Environment *PolyesterChainEnvironment
}

// PredictSafeAddressWithData returns the deterministic CREATE2 Safe address and
// factory deploy data (zero RPC).
func PredictSafeAddressWithData(opts PredictSafeAddressOptions) (*PredictedSafe, error) {
	if len(opts.Owners) == 0 {
		return nil, fmt.Errorf("owners must be non-empty")
	}
	env := PolyesterTestnetEnvironment
	if opts.Environment != nil {
		env = *opts.Environment
	}
	cfg := env.AccountAbstraction.Safe
	if opts.Safe != nil {
		cfg = *opts.Safe
	}
	saltNonce := big.NewInt(0)
	if opts.SaltNonce != nil {
		saltNonce = new(big.Int).Set(opts.SaltNonce)
	}
	owners := make([]common.Address, len(opts.Owners))
	for i, o := range opts.Owners {
		owners[i] = common.HexToAddress(o)
	}
	threshold := big.NewInt(int64(len(owners)))
	if opts.Threshold != nil {
		threshold = new(big.Int).Set(opts.Threshold)
	}

	initializer, err := getInitializer(owners, threshold, cfg)
	if err != nil {
		return nil, err
	}
	factoryCalldata, err := encodeCreateProxy(cfg.SafeSingletonAddress, initializer, saltNonce)
	if err != nil {
		return nil, err
	}

	singleton := common.HexToAddress(cfg.SafeSingletonAddress)
	deploymentCode := append(mustDecodeHex(SAFE_PROXY_CREATION_CODE), common.LeftPadBytes(singleton.Bytes(), 32)...)
	initHash := crypto.Keccak256(initializer)
	saltInput := append(initHash, common.LeftPadBytes(saltNonce.Bytes(), 32)...)
	salt := crypto.Keccak256(saltInput)
	address := create2Address(common.HexToAddress(cfg.SafeProxyFactoryAddress), salt, crypto.Keccak256(deploymentCode))

	return &PredictedSafe{
		Address:         address.Hex(),
		Initializer:     initializer,
		FactoryCalldata: factoryCalldata,
	}, nil
}

// PredictSafeAddress predicts the Polyester Safe for a single owner
// (main account = salt 0).
func PredictSafeAddress(ownerAddress string, saltNonce *big.Int, environment *PolyesterChainEnvironment) (string, error) {
	predicted, err := PredictSafeAddressWithData(PredictSafeAddressOptions{
		Owners:      []string{ownerAddress},
		SaltNonce:   saltNonce,
		Environment: environment,
	})
	if err != nil {
		return "", err
	}
	return predicted.Address, nil
}

// PredictPolyesterSmartAccountAddress is an alias matching the TypeScript name.
func PredictPolyesterSmartAccountAddress(ownerAddress string, saltNonce *big.Int, environment *PolyesterChainEnvironment) (string, error) {
	return PredictSafeAddress(ownerAddress, saltNonce, environment)
}

func getInitializer(owners []common.Address, threshold *big.Int, safe SafeDeploymentConfig) ([]byte, error) {
	enableModules, err := abi.Arguments{{Type: typeAddressArr}}.Pack([]common.Address{common.HexToAddress(safe.Safe4337ModuleAddress)})
	if err != nil {
		return nil, fmt.Errorf("encode enableModules: %w", err)
	}
	enableModules = append(append([]byte{}, enableModulesSelector...), enableModules...)

	multiCalls := [][]byte{
		encodeInternalTx(common.HexToAddress(safe.SafeModuleSetupAddress), enableModules, big.NewInt(0), 1),
	}
	multiSendCalldata, err := encodeMultiSend(multiCalls)
	if err != nil {
		return nil, err
	}

	args, err := abi.Arguments{
		{Type: typeAddressArr},
		{Type: typeUint256},
		{Type: typeAddress},
		{Type: typeBytes},
		{Type: typeAddress},
		{Type: typeAddress},
		{Type: typeUint256},
		{Type: typeAddress},
	}.Pack(
		owners,
		threshold,
		common.HexToAddress(safe.MultiSendAddress),
		multiSendCalldata,
		common.HexToAddress(safe.Safe4337ModuleAddress),
		common.Address{},
		big.NewInt(0),
		common.Address{},
	)
	if err != nil {
		return nil, fmt.Errorf("encode setup: %w", err)
	}
	return append(append([]byte{}, setupSelector...), args...), nil
}

func encodeInternalTx(to common.Address, data []byte, value *big.Int, operation byte) []byte {
	if value == nil {
		value = big.NewInt(0)
	}
	out := make([]byte, 0, 1+20+32+32+len(data))
	out = append(out, operation)
	out = append(out, to.Bytes()...)
	out = append(out, common.LeftPadBytes(value.Bytes(), 32)...)
	out = append(out, common.LeftPadBytes(big.NewInt(int64(len(data))).Bytes(), 32)...)
	out = append(out, data...)
	return out
}

func encodeMultiSend(txs [][]byte) ([]byte, error) {
	packed := joinBytes(txs...)
	encoded, err := abi.Arguments{{Type: typeBytes}}.Pack(packed)
	if err != nil {
		return nil, fmt.Errorf("encode multiSend: %w", err)
	}
	return append(append([]byte{}, multiSendSelector...), encoded...), nil
}

func encodeCreateProxy(singleton string, initializer []byte, saltNonce *big.Int) ([]byte, error) {
	encoded, err := abi.Arguments{
		{Type: typeAddress},
		{Type: typeBytes},
		{Type: typeUint256},
	}.Pack(common.HexToAddress(singleton), initializer, saltNonce)
	if err != nil {
		return nil, fmt.Errorf("encode createProxyWithNonce: %w", err)
	}
	return append(append([]byte{}, createProxySelector...), encoded...), nil
}

func create2Address(factory common.Address, salt, initCodeHash []byte) common.Address {
	data := make([]byte, 0, 1+20+32+32)
	data = append(data, 0xff)
	data = append(data, factory.Bytes()...)
	data = append(data, salt...)
	data = append(data, initCodeHash...)
	return common.BytesToAddress(crypto.Keccak256(data)[12:])
}

func joinBytes(parts ...[]byte) []byte {
	n := 0
	for _, p := range parts {
		n += len(p)
	}
	out := make([]byte, 0, n)
	for _, p := range parts {
		out = append(out, p...)
	}
	return out
}

func mustDecodeHex(s string) []byte {
	s = strings.TrimPrefix(s, "0x")
	b, err := hex.DecodeString(s)
	if err != nil {
		panic(err)
	}
	return b
}

func mustABIType(t string) abi.Type {
	parsed, err := abi.NewType(t, "", nil)
	if err != nil {
		panic(err)
	}
	return parsed
}
