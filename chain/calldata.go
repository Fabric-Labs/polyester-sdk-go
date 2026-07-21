// Package chain encodes FundingAccount / TradingGateway calldata for optional
// on-chain Funding ops (POLY-3569). It is not part of the API-key Connect surface.
//
// Encoding uses github.com/ethereum/go-ethereum/accounts/abi. Import this package
// only when you need Funding → Trading / Funding → external calldata helpers.
package chain

import (
	"encoding/hex"
	"fmt"
	"math/big"
	"strings"

	"github.com/Fabric-Labs/polyester-sdk-go/errors"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
)

// ChainCall is a contract call payload for a smart-account UserOperation.
type ChainCall struct {
	To    string
	Data  []byte
	Value *big.Int
}

// GuardApproval is the FundingAccount guard tuple
// (uint192 nonceSpace, uint256 deadline, bytes signature).
type GuardApproval struct {
	NonceSpace *big.Int
	Deadline   *big.Int
	Signature  []byte
}

// EmptyGuardApproval returns a zero approval used when required=true.
func EmptyGuardApproval() GuardApproval {
	return GuardApproval{
		NonceSpace: big.NewInt(0),
		Deadline:   big.NewInt(0),
		Signature:  []byte{},
	}
}

var (
	tradingGatewayABI = mustParseABI(`[
		{"type":"function","name":"deposit","inputs":[
			{"name":"uAssetId","type":"bytes32"},
			{"name":"uAmount","type":"uint256"}
		]},
		{"type":"function","name":"depositTo","inputs":[
			{"name":"toAccount","type":"address"},
			{"name":"uAssetId","type":"bytes32"},
			{"name":"uAmount","type":"uint256"}
		]}
	]`)

	guardTupleComponents = `[
		{"name":"nonceSpace","type":"uint192"},
		{"name":"deadline","type":"uint256"},
		{"name":"signature","type":"bytes"}
	]`

	fundingAccountABI = mustParseABI(fmt.Sprintf(`[
		{"type":"function","name":"withdrawToChain","inputs":[
			{"name":"request","type":"tuple","components":[
				{"name":"chainId","type":"uint16"},
				{"name":"zToken","type":"address"},
				{"name":"withdrawDestination","type":"bytes"},
				{"name":"zAmount","type":"uint256"},
				{"name":"maxFee","type":"uint256"}
			]}
		]},
		{"type":"function","name":"setExternalDestinationAllowlistRequired","inputs":[
			{"name":"required","type":"bool"},
			{"name":"guardSigIfFalse","type":"tuple","components":%s}
		]},
		{"type":"function","name":"setInternalAccountAllowlistRequired","inputs":[
			{"name":"required","type":"bool"},
			{"name":"guardSigIfFalse","type":"tuple","components":%s}
		]},
		{"type":"function","name":"addAllowedExternalDestinations","inputs":[
			{"name":"chainId","type":"uint16"},
			{"name":"destinations","type":"bytes[]"},
			{"name":"approval","type":"tuple","components":%s}
		]},
		{"type":"function","name":"removeAllowedExternalDestinations","inputs":[
			{"name":"chainId","type":"uint16"},
			{"name":"destinations","type":"bytes[]"},
			{"name":"approval","type":"tuple","components":%s}
		]},
		{"type":"function","name":"addAllowedInternalAccounts","inputs":[
			{"name":"accounts","type":"address[]"},
			{"name":"approval","type":"tuple","components":%s}
		]},
		{"type":"function","name":"removeAllowedInternalAccounts","inputs":[
			{"name":"accounts","type":"address[]"},
			{"name":"approval","type":"tuple","components":%s}
		]}
	]`, guardTupleComponents, guardTupleComponents, guardTupleComponents,
		guardTupleComponents, guardTupleComponents, guardTupleComponents))

	guardRegistryABI = mustParseABI(fmt.Sprintf(`[
		{"type":"function","name":"initializeSigner","inputs":[
			{"name":"signer","type":"address"}
		]},
		{"type":"function","name":"rotateSigner","inputs":[
			{"name":"newSigner","type":"address"},
			{"name":"approval","type":"tuple","components":%s}
		]}
	]`, guardTupleComponents))
)

func mustParseABI(jsonABI string) abi.ABI {
	parsed, err := abi.JSON(strings.NewReader(jsonABI))
	if err != nil {
		panic(err)
	}
	return parsed
}

// EncodeTradingGatewayDeposit encodes TradingGateway.deposit(bytes32,uint256).
func EncodeTradingGatewayDeposit(tradingGateway string, uAssetID any, quantityScaled *big.Int) (ChainCall, error) {
	if quantityScaled == nil || quantityScaled.Sign() <= 0 {
		return ChainCall{}, &errors.ValidationError{Msg: "quantity_scaled must be > 0"}
	}
	to, err := normalizeAddress(tradingGateway, "trading_gateway")
	if err != nil {
		return ChainCall{}, err
	}
	asset, err := normalizeBytes32(uAssetID, "u_asset_id")
	if err != nil {
		return ChainCall{}, err
	}
	data, err := tradingGatewayABI.Pack("deposit", asset, quantityScaled)
	if err != nil {
		return ChainCall{}, fmt.Errorf("pack deposit: %w", err)
	}
	return ChainCall{To: to, Data: data, Value: big.NewInt(0)}, nil
}

// EncodeTradingGatewayDepositTo encodes TradingGateway.depositTo(address,bytes32,uint256).
func EncodeTradingGatewayDepositTo(tradingGateway, toAccount string, uAssetID any, quantityScaled *big.Int) (ChainCall, error) {
	if quantityScaled == nil || quantityScaled.Sign() <= 0 {
		return ChainCall{}, &errors.ValidationError{Msg: "quantity_scaled must be > 0"}
	}
	to, err := normalizeAddress(tradingGateway, "trading_gateway")
	if err != nil {
		return ChainCall{}, err
	}
	account, err := normalizeAddress(toAccount, "to_account")
	if err != nil {
		return ChainCall{}, err
	}
	asset, err := normalizeBytes32(uAssetID, "u_asset_id")
	if err != nil {
		return ChainCall{}, err
	}
	data, err := tradingGatewayABI.Pack("depositTo", common.HexToAddress(account), asset, quantityScaled)
	if err != nil {
		return ChainCall{}, fmt.Errorf("pack depositTo: %w", err)
	}
	return ChainCall{To: to, Data: data, Value: big.NewInt(0)}, nil
}

// EncodeFundingWithdrawToChain encodes
// FundingAccount.withdrawToChain((uint16,address,bytes,uint256,uint256)).
func EncodeFundingWithdrawToChain(
	fundingAccount string,
	chainID uint16,
	zToken string,
	withdrawDestination []byte,
	zAmount, maxFee *big.Int,
) (ChainCall, error) {
	if chainID == 0 {
		return ChainCall{}, &errors.ValidationError{Msg: "chain_id must be a uint16 > 0"}
	}
	if zAmount == nil || zAmount.Sign() <= 0 {
		return ChainCall{}, &errors.ValidationError{Msg: "z_amount must be > 0"}
	}
	if maxFee == nil || maxFee.Sign() < 0 {
		return ChainCall{}, &errors.ValidationError{Msg: "max_fee must be >= 0"}
	}
	if zAmount.Cmp(maxFee) <= 0 {
		return ChainCall{}, &errors.ValidationError{Msg: "z_amount must be greater than max_fee"}
	}
	if len(withdrawDestination) == 0 {
		return ChainCall{}, &errors.ValidationError{Msg: "withdraw_destination must not be empty"}
	}

	to, err := normalizeAddress(fundingAccount, "funding_account")
	if err != nil {
		return ChainCall{}, err
	}
	token, err := normalizeAddress(zToken, "z_token")
	if err != nil {
		return ChainCall{}, err
	}

	request := struct {
		ChainId             uint16         `json:"chainId"`
		ZToken              common.Address `json:"zToken"`
		WithdrawDestination []byte         `json:"withdrawDestination"`
		ZAmount             *big.Int       `json:"zAmount"`
		MaxFee              *big.Int       `json:"maxFee"`
	}{
		ChainId:             chainID,
		ZToken:              common.HexToAddress(token),
		WithdrawDestination: withdrawDestination,
		ZAmount:             zAmount,
		MaxFee:              maxFee,
	}
	data, err := fundingAccountABI.Pack("withdrawToChain", request)
	if err != nil {
		return ChainCall{}, fmt.Errorf("pack withdrawToChain: %w", err)
	}
	return ChainCall{To: to, Data: data, Value: big.NewInt(0)}, nil
}

type guardTuple struct {
	NonceSpace *big.Int `json:"nonceSpace"`
	Deadline   *big.Int `json:"deadline"`
	Signature  []byte   `json:"signature"`
}

func resolveGuardTuple(approval *GuardApproval) (guardTuple, error) {
	guard := EmptyGuardApproval()
	if approval != nil {
		guard = *approval
	}
	if guard.NonceSpace == nil {
		guard.NonceSpace = big.NewInt(0)
	}
	if guard.Deadline == nil {
		guard.Deadline = big.NewInt(0)
	}
	if guard.NonceSpace.Sign() < 0 {
		return guardTuple{}, &errors.ValidationError{Msg: "approval.nonce_space must be >= 0"}
	}
	if guard.Deadline.Sign() < 0 {
		return guardTuple{}, &errors.ValidationError{Msg: "approval.deadline must be >= 0"}
	}
	if guard.Signature == nil {
		guard.Signature = []byte{}
	}
	return guardTuple(guard), nil
}

// EncodeSetExternalDestinationAllowlistRequired encodes
// setExternalDestinationAllowlistRequired(bool,(uint192,uint256,bytes)).
// When required is true, approval may be nil (empty guard tuple).
func EncodeSetExternalDestinationAllowlistRequired(
	fundingAccount string,
	required bool,
	approval *GuardApproval,
) (ChainCall, error) {
	to, err := normalizeAddress(fundingAccount, "funding_account")
	if err != nil {
		return ChainCall{}, err
	}
	tuple, err := resolveGuardTuple(approval)
	if err != nil {
		return ChainCall{}, err
	}
	data, err := fundingAccountABI.Pack("setExternalDestinationAllowlistRequired", required, tuple)
	if err != nil {
		return ChainCall{}, fmt.Errorf("pack setExternalDestinationAllowlistRequired: %w", err)
	}
	return ChainCall{To: to, Data: data, Value: big.NewInt(0)}, nil
}

// EncodeSetInternalAccountAllowlistRequired encodes
// setInternalAccountAllowlistRequired(bool,(uint192,uint256,bytes)).
func EncodeSetInternalAccountAllowlistRequired(
	fundingAccount string,
	required bool,
	approval *GuardApproval,
) (ChainCall, error) {
	to, err := normalizeAddress(fundingAccount, "funding_account")
	if err != nil {
		return ChainCall{}, err
	}
	tuple, err := resolveGuardTuple(approval)
	if err != nil {
		return ChainCall{}, err
	}
	data, err := fundingAccountABI.Pack("setInternalAccountAllowlistRequired", required, tuple)
	if err != nil {
		return ChainCall{}, fmt.Errorf("pack setInternalAccountAllowlistRequired: %w", err)
	}
	return ChainCall{To: to, Data: data, Value: big.NewInt(0)}, nil
}

// EncodeAddAllowedExternalDestinations encodes
// addAllowedExternalDestinations(uint16,bytes[],(uint192,uint256,bytes)).
func EncodeAddAllowedExternalDestinations(
	fundingAccount string,
	chainID uint16,
	destinations [][]byte,
	approval *GuardApproval,
) (ChainCall, error) {
	return encodeExternalDestinations("addAllowedExternalDestinations", fundingAccount, chainID, destinations, approval)
}

// EncodeRemoveAllowedExternalDestinations encodes
// removeAllowedExternalDestinations(uint16,bytes[],(uint192,uint256,bytes)).
func EncodeRemoveAllowedExternalDestinations(
	fundingAccount string,
	chainID uint16,
	destinations [][]byte,
	approval *GuardApproval,
) (ChainCall, error) {
	return encodeExternalDestinations("removeAllowedExternalDestinations", fundingAccount, chainID, destinations, approval)
}

func encodeExternalDestinations(
	method, fundingAccount string,
	chainID uint16,
	destinations [][]byte,
	approval *GuardApproval,
) (ChainCall, error) {
	if chainID == 0 {
		return ChainCall{}, &errors.ValidationError{Msg: "chain_id must be a uint16 > 0"}
	}
	if len(destinations) == 0 {
		return ChainCall{}, &errors.ValidationError{Msg: "destinations must be non-empty"}
	}
	for _, d := range destinations {
		if len(d) == 0 {
			return ChainCall{}, &errors.ValidationError{Msg: "destinations entries must not be empty"}
		}
	}
	to, err := normalizeAddress(fundingAccount, "funding_account")
	if err != nil {
		return ChainCall{}, err
	}
	tuple, err := resolveGuardTuple(approval)
	if err != nil {
		return ChainCall{}, err
	}
	data, err := fundingAccountABI.Pack(method, chainID, destinations, tuple)
	if err != nil {
		return ChainCall{}, fmt.Errorf("pack %s: %w", method, err)
	}
	return ChainCall{To: to, Data: data, Value: big.NewInt(0)}, nil
}

// EncodeAddAllowedInternalAccounts encodes
// addAllowedInternalAccounts(address[],(uint192,uint256,bytes)).
func EncodeAddAllowedInternalAccounts(
	fundingAccount string,
	accounts []string,
	approval *GuardApproval,
) (ChainCall, error) {
	return encodeInternalAccounts("addAllowedInternalAccounts", fundingAccount, accounts, approval)
}

// EncodeRemoveAllowedInternalAccounts encodes
// removeAllowedInternalAccounts(address[],(uint192,uint256,bytes)).
func EncodeRemoveAllowedInternalAccounts(
	fundingAccount string,
	accounts []string,
	approval *GuardApproval,
) (ChainCall, error) {
	return encodeInternalAccounts("removeAllowedInternalAccounts", fundingAccount, accounts, approval)
}

func encodeInternalAccounts(
	method, fundingAccount string,
	accounts []string,
	approval *GuardApproval,
) (ChainCall, error) {
	if len(accounts) == 0 {
		return ChainCall{}, &errors.ValidationError{Msg: "accounts must be non-empty"}
	}
	to, err := normalizeAddress(fundingAccount, "funding_account")
	if err != nil {
		return ChainCall{}, err
	}
	addrs := make([]common.Address, len(accounts))
	for i, account := range accounts {
		normalized, err := normalizeAddress(account, "accounts")
		if err != nil {
			return ChainCall{}, err
		}
		addrs[i] = common.HexToAddress(normalized)
	}
	tuple, err := resolveGuardTuple(approval)
	if err != nil {
		return ChainCall{}, err
	}
	data, err := fundingAccountABI.Pack(method, addrs, tuple)
	if err != nil {
		return ChainCall{}, fmt.Errorf("pack %s: %w", method, err)
	}
	return ChainCall{To: to, Data: data, Value: big.NewInt(0)}, nil
}

// EncodeInitializeGuardSigner encodes GuardRegistry.initializeSigner(address).
func EncodeInitializeGuardSigner(guardRegistry, signer string) (ChainCall, error) {
	to, err := normalizeAddress(guardRegistry, "guard_registry")
	if err != nil {
		return ChainCall{}, err
	}
	signerAddr, err := normalizeAddress(signer, "signer")
	if err != nil {
		return ChainCall{}, err
	}
	data, err := guardRegistryABI.Pack("initializeSigner", common.HexToAddress(signerAddr))
	if err != nil {
		return ChainCall{}, fmt.Errorf("pack initializeSigner: %w", err)
	}
	return ChainCall{To: to, Data: data, Value: big.NewInt(0)}, nil
}

// EncodeRotateGuardSigner encodes GuardRegistry.rotateSigner(address,(uint192,uint256,bytes)).
func EncodeRotateGuardSigner(guardRegistry, newSigner string, approval *GuardApproval) (ChainCall, error) {
	to, err := normalizeAddress(guardRegistry, "guard_registry")
	if err != nil {
		return ChainCall{}, err
	}
	signerAddr, err := normalizeAddress(newSigner, "new_signer")
	if err != nil {
		return ChainCall{}, err
	}
	tuple, err := resolveGuardTuple(approval)
	if err != nil {
		return ChainCall{}, err
	}
	data, err := guardRegistryABI.Pack("rotateSigner", common.HexToAddress(signerAddr), tuple)
	if err != nil {
		return ChainCall{}, fmt.Errorf("pack rotateSigner: %w", err)
	}
	return ChainCall{To: to, Data: data, Value: big.NewInt(0)}, nil
}

func normalizeAddress(value, field string) (string, error) {
	addr := strings.TrimSpace(value)
	if !strings.HasPrefix(addr, "0x") || len(addr) != 42 {
		return "", &errors.ValidationError{Msg: field + " must be a 20-byte 0x-prefixed address"}
	}
	if _, err := hex.DecodeString(addr[2:]); err != nil {
		return "", &errors.ValidationError{Msg: field + " is not a valid hex address"}
	}
	return strings.ToLower(addr), nil
}

func normalizeBytes32(value any, field string) ([32]byte, error) {
	var out [32]byte
	switch v := value.(type) {
	case [32]byte:
		return v, nil
	case []byte:
		if len(v) != 32 {
			return out, &errors.ValidationError{Msg: field + " must be exactly 32 bytes"}
		}
		copy(out[:], v)
		return out, nil
	case string:
		text := strings.TrimSpace(v)
		text = strings.TrimPrefix(text, "0x")
		if len(text) != 64 {
			return out, &errors.ValidationError{Msg: field + " must be 32 bytes (64 hex chars)"}
		}
		raw, err := hex.DecodeString(text)
		if err != nil {
			return out, &errors.ValidationError{Msg: field + " is not valid hex"}
		}
		copy(out[:], raw)
		return out, nil
	default:
		return out, &errors.ValidationError{Msg: field + " must be hex string or 32 bytes"}
	}
}
