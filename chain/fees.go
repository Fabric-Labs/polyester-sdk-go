package chain

import (
	"encoding/hex"
	"fmt"
	"math/big"
	"time"

	"github.com/Fabric-Labs/polyester-sdk-go/environment"
	"github.com/Fabric-Labs/polyester-sdk-go/errors"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

// ZipperFeeQuote is the result of quoting a Zipper network fee for withdraws.
type ZipperFeeQuote struct {
	Fee            *big.Int
	ZTokenDecimals int
	FeeFactory     string
	ZipperEndpoint string
}

// QuoteZipperFee quotes Zipper network fee via feeFactory.getFee(uint16,address).
//
// Use the returned Fee (or a small buffer above it) as maxFee for
// EncodeFundingWithdrawToChain. When env or rpc is nil, environment.PolyesterDevnetEnvironment
// and its RPC URL are used.
//
// Each eth_call result must be 0x and exactly one 32-byte ABI word: an address
// word with its upper 12 bytes zero, a decimals word of at most 255. Any other
// result, such as non-ASCII, short, odd-length or non-hex text, is an
// *errors.TransportError.
func QuoteZipperFee(
	chainID uint16,
	zToken, zipperEndpoint string,
	env *environment.PolyesterEnvironment,
	rpc *JSONRPCClient,
) (ZipperFeeQuote, error) {
	if chainID == 0 {
		return ZipperFeeQuote{}, &errors.ValidationError{Msg: "chain_id must be a uint16 > 0"}
	}
	token, err := normalizeAddress(zToken, "z_token")
	if err != nil {
		return ZipperFeeQuote{}, err
	}
	endpoint, err := normalizeAddress(zipperEndpoint, "zipper_endpoint")
	if err != nil {
		return ZipperFeeQuote{}, err
	}

	if env == nil {
		env = &environment.PolyesterDevnetEnvironment
	}
	client := rpc
	if client == nil {
		client = NewJSONRPCClient(env.RPCURL, 60*time.Second)
	}

	feeFactorySel := crypto.Keccak256([]byte("feeFactory()"))[:4]
	var ffRaw string
	if err := client.RequestDecode("eth_call", []any{
		map[string]string{"to": endpoint, "data": "0x" + hex.EncodeToString(feeFactorySel)},
		"latest",
	}, &ffRaw); err != nil {
		return ZipperFeeQuote{}, fmt.Errorf("feeFactory eth_call: %w", err)
	}
	feeFactory, err := ethCallAddress(ffRaw, "feeFactory")
	if err != nil {
		return ZipperFeeQuote{}, err
	}

	decimalsSel := crypto.Keccak256([]byte("decimals()"))[:4]
	var decimalsRaw string
	if err := client.RequestDecode("eth_call", []any{
		map[string]string{"to": token, "data": "0x" + hex.EncodeToString(decimalsSel)},
		"latest",
	}, &decimalsRaw); err != nil {
		return ZipperFeeQuote{}, fmt.Errorf("decimals eth_call: %w", err)
	}
	// ERC-20 decimals() returns a uint8.
	decimals, err := ethCallUint(decimalsRaw, "decimals", 8)
	if err != nil {
		return ZipperFeeQuote{}, err
	}

	getFeeArgs := abi.Arguments{
		{Type: mustABIType("uint16")},
		{Type: mustABIType("address")},
	}
	getFeeSel := crypto.Keccak256([]byte("getFee(uint16,address)"))[:4]
	encoded, err := getFeeArgs.Pack(chainID, common.HexToAddress(token))
	if err != nil {
		return ZipperFeeQuote{}, fmt.Errorf("pack getFee: %w", err)
	}
	var feeRaw string
	if err := client.RequestDecode("eth_call", []any{
		map[string]string{
			"to":   feeFactory,
			"data": "0x" + hex.EncodeToString(append(getFeeSel, encoded...)),
		},
		"latest",
	}, &feeRaw); err != nil {
		return ZipperFeeQuote{}, fmt.Errorf("getFee eth_call: %w", err)
	}
	fee, err := ethCallUint(feeRaw, "getFee", 256)
	if err != nil {
		return ZipperFeeQuote{}, err
	}

	return ZipperFeeQuote{
		Fee:            fee,
		ZTokenDecimals: int(decimals.Int64()),
		FeeFactory:     feeFactory,
		ZipperEndpoint: endpoint,
	}, nil
}
