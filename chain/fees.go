package chain

import (
	"encoding/hex"
	"fmt"
	"math/big"
	"strings"
	"time"

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
// EncodeFundingWithdrawToChain. When env or rpc is nil, PolyesterTestnetEnvironment
// and its RPC URL are used.
func QuoteZipperFee(
	chainID uint16,
	zToken, zipperEndpoint string,
	env *PolyesterChainEnvironment,
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
		env = &PolyesterTestnetEnvironment
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
	ffRaw = strings.TrimPrefix(strings.ToLower(ffRaw), "0x")
	if len(ffRaw) < 40 {
		return ZipperFeeQuote{}, fmt.Errorf("feeFactory returned short result: %s", ffRaw)
	}
	feeFactory := "0x" + ffRaw[len(ffRaw)-40:]

	decimalsSel := crypto.Keccak256([]byte("decimals()"))[:4]
	var decimalsRaw string
	if err := client.RequestDecode("eth_call", []any{
		map[string]string{"to": token, "data": "0x" + hex.EncodeToString(decimalsSel)},
		"latest",
	}, &decimalsRaw); err != nil {
		return ZipperFeeQuote{}, fmt.Errorf("decimals eth_call: %w", err)
	}
	decimals, ok := new(big.Int).SetString(strings.TrimPrefix(decimalsRaw, "0x"), 16)
	if !ok {
		return ZipperFeeQuote{}, fmt.Errorf("decimals decode failed: %s", decimalsRaw)
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
	fee, ok := new(big.Int).SetString(strings.TrimPrefix(feeRaw, "0x"), 16)
	if !ok {
		return ZipperFeeQuote{}, fmt.Errorf("fee decode failed: %s", feeRaw)
	}

	return ZipperFeeQuote{
		Fee:            fee,
		ZTokenDecimals: int(decimals.Int64()),
		FeeFactory:     feeFactory,
		ZipperEndpoint: endpoint,
	}, nil
}
