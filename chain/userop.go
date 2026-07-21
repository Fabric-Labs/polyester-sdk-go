package chain

import (
	"crypto/ecdsa"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/common/math"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/signer/core/apitypes"
)

const (
	UserOperationGasBufferBPS = 2000
	UserOperationMinGasBuffer = 50_000
	executeUserOpSelectorHex  = "541d63c8" // executeUserOpWithErrorString
	getNonceSelectorHex       = "35567e1a" // getNonce(address,uint192)
)

var (
	executeUserOpSelector = mustDecodeHex(executeUserOpSelectorHex)
	getNonceSelector      = mustDecodeHex(getNonceSelectorHex)
	stubECDSASignature    = mustDecodeHex(
		"fffffffffffffffffffffffffffffff000000000000000000000000000000000" +
			"7aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa1c",
	)
	typeUint8   = mustABIType("uint8")
	typeUint192 = mustABIType("uint192")
)

// UserOperationReceipt is a mined UserOperation receipt from the bundler.
type UserOperationReceipt struct {
	UserOperationHash string
	TransactionHash   string
	Success           bool
	Raw               map[string]any
}

// SmartAccount is an owner-key smart account: derive Safe, build/sign/submit Funding UserOps.
type SmartAccount struct {
	Address      string
	OwnerAddress string
	Environment  PolyesterChainEnvironment
	SaltNonce    *big.Int
	initializer  []byte
	factoryData  []byte
	ownerKey     *ecdsa.PrivateKey
	rpc          *JSONRPCClient
	bundler      *JSONRPCClient
	paymaster    *JSONRPCClient
}

// NewSmartAccount creates a SmartAccount from an owner private key hex.
// environment may be nil (defaults to PolyesterTestnetEnvironment).
// saltNonce may be nil (defaults to 0).
func NewSmartAccount(ownerPrivateKey string, environment *PolyesterChainEnvironment, saltNonce *big.Int) (*SmartAccount, error) {
	keyHex := strings.TrimPrefix(strings.TrimSpace(ownerPrivateKey), "0x")
	keyBytes, err := hex.DecodeString(keyHex)
	if err != nil {
		return nil, fmt.Errorf("invalid owner private key: %w", err)
	}
	priv, err := crypto.ToECDSA(keyBytes)
	if err != nil {
		return nil, fmt.Errorf("invalid owner private key: %w", err)
	}
	env := PolyesterTestnetEnvironment
	if environment != nil {
		env = *environment
	}
	if saltNonce == nil {
		saltNonce = big.NewInt(0)
	} else {
		saltNonce = new(big.Int).Set(saltNonce)
	}
	ownerAddr := crypto.PubkeyToAddress(priv.PublicKey)
	predicted, err := PredictSafeAddressWithData(PredictSafeAddressOptions{
		Owners:      []string{ownerAddr.Hex()},
		SaltNonce:   saltNonce,
		Environment: &env,
	})
	if err != nil {
		return nil, err
	}
	timeout := 60 * time.Second
	aa := env.AccountAbstraction
	return &SmartAccount{
		Address:      predicted.Address,
		OwnerAddress: ownerAddr.Hex(),
		Environment:  env,
		SaltNonce:    saltNonce,
		initializer:  predicted.Initializer,
		factoryData:  predicted.FactoryCalldata,
		ownerKey:     priv,
		rpc:          NewJSONRPCClient(env.RPCURL, timeout),
		bundler:      NewJSONRPCClient(aa.BundlerURL, timeout),
		paymaster:    NewJSONRPCClient(aa.PaymasterURL, timeout),
	}, nil
}

// IsDeployed reports whether the Safe has contract code at Address.
func (a *SmartAccount) IsDeployed() (bool, error) {
	var code string
	if err := a.rpc.RequestDecode("eth_getCode", []any{a.Address, "latest"}, &code); err != nil {
		return false, err
	}
	code = strings.TrimSpace(code)
	return code != "" && code != "0x" && code != "0x0", nil
}

// GetNonce returns the next EntryPoint nonce.
//
// Matches viem/permissionless: when key is nil, use a fresh timestamp-based
// nonce key (Date.now() ms) so ops are not stuck on key 0 (Polyester's bundler
// rejects some key-0 mempool submissions). getNonce(sender, key) returns the
// full packed nonce (key<<64)|seq.
func (a *SmartAccount) GetNonce(key *big.Int) (*big.Int, error) {
	nonceKey := key
	if nonceKey == nil {
		nonceKey = big.NewInt(time.Now().UnixMilli())
	} else {
		nonceKey = new(big.Int).Set(nonceKey)
	}
	if nonceKey.Sign() < 0 || nonceKey.BitLen() > 192 {
		return nil, fmt.Errorf("nonce key out of uint192 range")
	}
	ep := a.Environment.AccountAbstraction.EntryPoint.Address
	data, err := abi.Arguments{
		{Type: typeAddress},
		{Type: typeUint192},
	}.Pack(common.HexToAddress(a.Address), nonceKey)
	if err != nil {
		return nil, fmt.Errorf("encode getNonce: %w", err)
	}
	calldata := append(append([]byte{}, getNonceSelector...), data...)
	var result string
	if err := a.rpc.RequestDecode("eth_call", []any{
		map[string]string{"to": ep, "data": hexutil.Encode(calldata)},
		"latest",
	}, &result); err != nil {
		return nil, err
	}
	n, ok := new(big.Int).SetString(strings.TrimPrefix(result, "0x"), 16)
	if !ok {
		return nil, fmt.Errorf("invalid getNonce result: %s", result)
	}
	return n, nil
}

// SendCalls builds, sponsors, signs, and submits a UserOperation for calls.
// Only a single call is supported for now. When wait is true, polls for a receipt.
func (a *SmartAccount) SendCalls(calls []ChainCall, wait bool, receiptTimeout time.Duration) (*UserOperationReceipt, error) {
	if len(calls) == 0 {
		return nil, fmt.Errorf("at least one call is required")
	}
	if len(calls) != 1 {
		return nil, fmt.Errorf("multi-call UserOps are not implemented yet; submit one ChainCall at a time")
	}
	if receiptTimeout <= 0 {
		receiptTimeout = 30 * time.Second
	}

	callData, err := EncodeExecuteUserOpCallData(calls[0])
	if err != nil {
		return nil, err
	}
	deployed, err := a.IsDeployed()
	if err != nil {
		return nil, err
	}

	var factory *string
	var factoryData []byte
	initCode := []byte{}
	if !deployed {
		f := a.Environment.AccountAbstraction.Safe.SafeProxyFactoryAddress
		factory = &f
		factoryData = a.factoryData
		initCode = append(common.HexToAddress(f).Bytes(), factoryData...)
	}

	nonce, err := a.GetNonce(nil)
	if err != nil {
		return nil, err
	}

	var gasPrice map[string]any
	if err := a.paymaster.RequestDecode("pimlico_getUserOperationGasPrice", []any{}, &gasPrice); err != nil {
		return nil, err
	}
	fast, _ := gasPrice["fast"].(map[string]any)
	maxFee, err := asInt(fast["maxFeePerGas"])
	if err != nil {
		return nil, fmt.Errorf("gas price maxFeePerGas: %w", err)
	}
	maxPrio, err := asInt(fast["maxPriorityFeePerGas"])
	if err != nil {
		return nil, fmt.Errorf("gas price maxPriorityFeePerGas: %w", err)
	}

	userOp := map[string]any{
		"sender":               a.Address,
		"nonce":                hexInt(nonce),
		"callData":             hexutil.Encode(callData),
		"callGasLimit":         hexInt(big.NewInt(0)),
		"verificationGasLimit": hexInt(big.NewInt(0)),
		"preVerificationGas":   hexInt(big.NewInt(0)),
		"maxFeePerGas":         hexInt(maxFee),
		"maxPriorityFeePerGas": hexInt(maxPrio),
		"signature":            hexutil.Encode(StubSignature()),
	}
	if factory != nil {
		userOp["factory"] = common.HexToAddress(*factory).Hex()
		userOp["factoryData"] = hexutil.Encode(factoryData)
	}

	entryPoint := a.Environment.AccountAbstraction.EntryPoint.Address

	// Sponsor once for estimates, buffer gas (incl. paymaster), then re-sponsor so
	// paymasterData matches the final limits. Polyester's paymaster often returns
	// paymasterPostOpGasLimit=1; without a floor the bundler accepts then rejects.
	var sponsored map[string]any
	if err := a.paymaster.RequestDecode("pm_sponsorUserOperation", []any{
		userOp,
		entryPoint,
	}, &sponsored); err != nil {
		return nil, err
	}

	callGasRaw, err := asInt(sponsored["callGasLimit"])
	if err != nil {
		return nil, err
	}
	verificationGasRaw, err := asInt(sponsored["verificationGasLimit"])
	if err != nil {
		return nil, err
	}
	preVerificationRaw, err := asInt(sponsored["preVerificationGas"])
	if err != nil {
		return nil, err
	}
	callGas := AddUserOperationGasBuffer(callGasRaw)
	verificationGas := AddUserOperationGasBuffer(verificationGasRaw)
	preVerification := AddUserOperationGasBuffer(preVerificationRaw)

	pmVerRaw := big.NewInt(0)
	if v, err := asInt(sponsored["paymasterVerificationGasLimit"]); err == nil {
		pmVerRaw = v
	}
	pmPostRaw := big.NewInt(0)
	if v, err := asInt(sponsored["paymasterPostOpGasLimit"]); err == nil {
		pmPostRaw = v
	}
	pmVer := maxBig(
		AddUserOperationGasBuffer(pmVerRaw),
		big.NewInt(UserOperationMinGasBuffer),
	)
	pmPost := maxBig(
		AddUserOperationGasBuffer(pmPostRaw),
		big.NewInt(UserOperationMinGasBuffer*2),
	)

	bufferedOp := map[string]any{
		"sender":                        userOp["sender"],
		"nonce":                         userOp["nonce"],
		"callData":                      userOp["callData"],
		"callGasLimit":                  hexInt(callGas),
		"verificationGasLimit":          hexInt(verificationGas),
		"preVerificationGas":            hexInt(preVerification),
		"maxFeePerGas":                  userOp["maxFeePerGas"],
		"maxPriorityFeePerGas":          userOp["maxPriorityFeePerGas"],
		"signature":                     userOp["signature"],
		"paymasterVerificationGasLimit": hexInt(pmVer),
		"paymasterPostOpGasLimit":       hexInt(pmPost),
	}
	if factory != nil {
		bufferedOp["factory"] = userOp["factory"]
		bufferedOp["factoryData"] = userOp["factoryData"]
	}
	if err := a.paymaster.RequestDecode("pm_sponsorUserOperation", []any{
		bufferedOp,
		entryPoint,
	}, &sponsored); err != nil {
		return nil, err
	}

	// Keep the exact buffered limits we asked the paymaster to cover. Taking
	// higher sponsor-returned callGas without re-binding paymasterData causes
	// bundler accept-then-reject.
	var paymaster string
	if pm, ok := sponsored["paymaster"].(string); ok {
		paymaster = pm
	}
	pmDataBytes := []byte{}
	if pmData, ok := sponsored["paymasterData"].(string); ok && pmData != "" && pmData != "0x" {
		pmDataBytes = mustDecodeHex(pmData)
	}

	paymasterAndData := PackPaymasterAndData(paymaster, pmVer, pmPost, pmDataBytes)
	signature, err := SignSafeUserOperation(SignSafeUserOperationParams{
		OwnerKey:             a.ownerKey,
		Environment:          a.Environment,
		Sender:               a.Address,
		Nonce:                nonce,
		InitCode:             initCode,
		CallData:             callData,
		CallGasLimit:         callGas,
		VerificationGasLimit: verificationGas,
		PreVerificationGas:   preVerification,
		MaxFeePerGas:         maxFee,
		MaxPriorityFeePerGas: maxPrio,
		PaymasterAndData:     paymasterAndData,
	})
	if err != nil {
		return nil, err
	}

	finalOp := map[string]any{
		"sender":               a.Address,
		"nonce":                hexInt(nonce),
		"callData":             hexutil.Encode(callData),
		"callGasLimit":         hexInt(callGas),
		"verificationGasLimit": hexInt(verificationGas),
		"preVerificationGas":   hexInt(preVerification),
		"maxFeePerGas":         hexInt(maxFee),
		"maxPriorityFeePerGas": hexInt(maxPrio),
		"signature":            hexutil.Encode(signature),
	}
	if factory != nil {
		finalOp["factory"] = common.HexToAddress(*factory).Hex()
		finalOp["factoryData"] = hexutil.Encode(factoryData)
	}
	if paymaster != "" {
		finalOp["paymaster"] = common.HexToAddress(paymaster).Hex()
		finalOp["paymasterVerificationGasLimit"] = hexInt(pmVer)
		finalOp["paymasterPostOpGasLimit"] = hexInt(pmPost)
		finalOp["paymasterData"] = hexutil.Encode(pmDataBytes)
	}

	var userOpHash string
	if err := a.bundler.RequestDecode("eth_sendUserOperation", []any{
		finalOp,
		a.Environment.AccountAbstraction.EntryPoint.Address,
	}, &userOpHash); err != nil {
		return nil, err
	}
	if !wait {
		return &UserOperationReceipt{UserOperationHash: userOpHash}, nil
	}
	return a.WaitForReceipt(userOpHash, receiptTimeout, time.Second)
}

// WaitForReceipt polls eth_getUserOperationReceipt until mined or timeout.
// If pimlico_getUserOperationStatus reports rejected, fails fast.
func (a *SmartAccount) WaitForReceipt(userOperationHash string, timeout, pollInterval time.Duration) (*UserOperationReceipt, error) {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	if pollInterval <= 0 {
		pollInterval = time.Second
	}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		raw, err := a.bundler.Request("eth_getUserOperationReceipt", []any{userOperationHash})
		if err != nil {
			return nil, err
		}
		if len(raw) > 0 && string(raw) != "null" {
			var decoded map[string]any
			if err := json.Unmarshal(raw, &decoded); err != nil {
				return nil, fmt.Errorf("decode user op receipt: %w", err)
			}
			receipt, _ := decoded["receipt"].(map[string]any)
			success := false
			if s, ok := decoded["success"].(bool); ok {
				success = s
			} else if receipt != nil {
				switch status := receipt["status"].(type) {
				case float64:
					success = status == 1
				case string:
					success = status == "0x1" || status == "0x01" || status == "1"
				}
			}
			txHash := ""
			if receipt != nil {
				if h, ok := receipt["transactionHash"].(string); ok {
					txHash = h
				}
			}
			if txHash == "" {
				if h, ok := decoded["transactionHash"].(string); ok {
					txHash = h
				}
			}
			return &UserOperationReceipt{
				UserOperationHash: userOperationHash,
				TransactionHash:   txHash,
				Success:           success,
				Raw:               decoded,
			}, nil
		}

		var status map[string]any
		if err := a.bundler.RequestDecode("pimlico_getUserOperationStatus", []any{userOperationHash}, &status); err == nil {
			if s, ok := status["status"].(string); ok && s == "rejected" {
				return nil, fmt.Errorf("bundler rejected UserOperation %s: %v", userOperationHash, status)
			}
		}
		time.Sleep(pollInterval)
	}
	return nil, fmt.Errorf("timed out waiting for UserOperation receipt %s", userOperationHash)
}

// AddUserOperationGasBuffer applies +20% or +50k minimum buffer (Python/TS parity).
func AddUserOperationGasBuffer(gas *big.Int) *big.Int {
	if gas == nil {
		gas = big.NewInt(0)
	}
	percent := new(big.Int).Mul(gas, big.NewInt(UserOperationGasBufferBPS))
	percent.Div(percent, big.NewInt(10_000))
	minBuf := big.NewInt(UserOperationMinGasBuffer)
	if percent.Cmp(minBuf) < 0 {
		percent = minBuf
	}
	return new(big.Int).Add(gas, percent)
}

// EncodeExecuteUserOpCallData encodes Safe4337Module.executeUserOpWithErrorString
// for a single call.
func EncodeExecuteUserOpCallData(call ChainCall) ([]byte, error) {
	value := call.Value
	if value == nil {
		value = big.NewInt(0)
	}
	data := call.Data
	if data == nil {
		data = []byte{}
	}
	encoded, err := abi.Arguments{
		{Type: typeAddress},
		{Type: typeUint256},
		{Type: typeBytes},
		{Type: typeUint8},
	}.Pack(common.HexToAddress(call.To), value, data, uint8(0))
	if err != nil {
		return nil, fmt.Errorf("encode executeUserOpWithErrorString: %w", err)
	}
	return append(append([]byte{}, executeUserOpSelector...), encoded...), nil
}

// PackPaymasterAndData packs EntryPoint v0.7 paymasterAndData.
func PackPaymasterAndData(paymaster string, paymasterVerificationGasLimit, paymasterPostOpGasLimit *big.Int, paymasterData []byte) []byte {
	if paymaster == "" {
		return []byte{}
	}
	if paymasterVerificationGasLimit == nil {
		paymasterVerificationGasLimit = big.NewInt(0)
	}
	if paymasterPostOpGasLimit == nil {
		paymasterPostOpGasLimit = big.NewInt(0)
	}
	if paymasterData == nil {
		paymasterData = []byte{}
	}
	out := make([]byte, 0, 20+16+16+len(paymasterData))
	out = append(out, common.HexToAddress(paymaster).Bytes()...)
	out = append(out, common.LeftPadBytes(paymasterVerificationGasLimit.Bytes(), 16)...)
	out = append(out, common.LeftPadBytes(paymasterPostOpGasLimit.Bytes(), 16)...)
	out = append(out, paymasterData...)
	return out
}

// StubSignature returns the stub ECDSA signature used for gas estimation.
func StubSignature() []byte {
	out := make([]byte, 0, 6+6+len(stubECDSASignature))
	out = append(out, make([]byte, 6)...)
	out = append(out, make([]byte, 6)...)
	out = append(out, stubECDSASignature...)
	return out
}

// SignSafeUserOperationParams are inputs for EIP-712 SafeOp v0.7 signing.
type SignSafeUserOperationParams struct {
	OwnerKey             *ecdsa.PrivateKey
	Environment          PolyesterChainEnvironment
	Sender               string
	Nonce                *big.Int
	InitCode             []byte
	CallData             []byte
	CallGasLimit         *big.Int
	VerificationGasLimit *big.Int
	PreVerificationGas   *big.Int
	MaxFeePerGas         *big.Int
	MaxPriorityFeePerGas *big.Int
	PaymasterAndData     []byte
	ValidAfter           uint64
	ValidUntil           uint64
}

// SignSafeUserOperation signs an EIP-712 SafeOp and packs uint48/uint48/bytes.
func SignSafeUserOperation(p SignSafeUserOperationParams) ([]byte, error) {
	module := p.Environment.AccountAbstraction.Safe.Safe4337ModuleAddress
	entryPoint := p.Environment.AccountAbstraction.EntryPoint.Address
	typed := apitypes.TypedData{
		Types: apitypes.Types{
			"EIP712Domain": []apitypes.Type{
				{Name: "chainId", Type: "uint256"},
				{Name: "verifyingContract", Type: "address"},
			},
			"SafeOp": []apitypes.Type{
				{Name: "safe", Type: "address"},
				{Name: "nonce", Type: "uint256"},
				{Name: "initCode", Type: "bytes"},
				{Name: "callData", Type: "bytes"},
				{Name: "verificationGasLimit", Type: "uint128"},
				{Name: "callGasLimit", Type: "uint128"},
				{Name: "preVerificationGas", Type: "uint256"},
				{Name: "maxPriorityFeePerGas", Type: "uint128"},
				{Name: "maxFeePerGas", Type: "uint128"},
				{Name: "paymasterAndData", Type: "bytes"},
				{Name: "validAfter", Type: "uint48"},
				{Name: "validUntil", Type: "uint48"},
				{Name: "entryPoint", Type: "address"},
			},
		},
		PrimaryType: "SafeOp",
		Domain: apitypes.TypedDataDomain{
			ChainId:           math.NewHexOrDecimal256(p.Environment.ChainID),
			VerifyingContract: common.HexToAddress(module).Hex(),
		},
		Message: apitypes.TypedDataMessage{
			"safe":                 common.HexToAddress(p.Sender).Hex(),
			"nonce":                p.Nonce.String(),
			"initCode":             hexutil.Encode(nilToEmpty(p.InitCode)),
			"callData":             hexutil.Encode(nilToEmpty(p.CallData)),
			"verificationGasLimit": p.VerificationGasLimit.String(),
			"callGasLimit":         p.CallGasLimit.String(),
			"preVerificationGas":   p.PreVerificationGas.String(),
			"maxPriorityFeePerGas": p.MaxPriorityFeePerGas.String(),
			"maxFeePerGas":         p.MaxFeePerGas.String(),
			"paymasterAndData":     hexutil.Encode(nilToEmpty(p.PaymasterAndData)),
			"validAfter":           fmt.Sprintf("%d", p.ValidAfter),
			"validUntil":           fmt.Sprintf("%d", p.ValidUntil),
			"entryPoint":           common.HexToAddress(entryPoint).Hex(),
		},
	}
	hash, _, err := apitypes.TypedDataAndHash(typed)
	if err != nil {
		return nil, fmt.Errorf("hash SafeOp typed data: %w", err)
	}
	sig, err := crypto.Sign(hash, p.OwnerKey)
	if err != nil {
		return nil, fmt.Errorf("sign SafeOp: %w", err)
	}
	// crypto.Sign returns V as 0/1; eth_account / Safe expect 27/28.
	sig[64] += 27

	out := make([]byte, 0, 6+6+len(sig))
	out = append(out, common.LeftPadBytes(big.NewInt(int64(p.ValidAfter)).Bytes(), 6)...)
	out = append(out, common.LeftPadBytes(big.NewInt(int64(p.ValidUntil)).Bytes(), 6)...)
	out = append(out, sig...)
	return out, nil
}

func maxBig(a, b *big.Int) *big.Int {
	if a == nil {
		a = big.NewInt(0)
	}
	if b == nil {
		b = big.NewInt(0)
	}
	if a.Cmp(b) >= 0 {
		return new(big.Int).Set(a)
	}
	return new(big.Int).Set(b)
}

func nilToEmpty(b []byte) []byte {
	if b == nil {
		return []byte{}
	}
	return b
}

func hexInt(n *big.Int) string {
	if n == nil {
		return "0x0"
	}
	return hexutil.EncodeBig(n)
}

func asInt(value any) (*big.Int, error) {
	switch v := value.(type) {
	case nil:
		return nil, fmt.Errorf("nil integer")
	case int:
		return big.NewInt(int64(v)), nil
	case int64:
		return big.NewInt(v), nil
	case float64:
		return big.NewInt(int64(v)), nil
	case json.Number:
		i, err := v.Int64()
		if err == nil {
			return big.NewInt(i), nil
		}
		n, ok := new(big.Int).SetString(string(v), 10)
		if !ok {
			return nil, fmt.Errorf("cannot parse json.Number %q", v)
		}
		return n, nil
	case string:
		s := strings.TrimSpace(v)
		if strings.HasPrefix(s, "0x") || strings.HasPrefix(s, "0X") {
			n, ok := new(big.Int).SetString(s[2:], 16)
			if !ok {
				return nil, fmt.Errorf("cannot parse hex int %q", v)
			}
			return n, nil
		}
		n, ok := new(big.Int).SetString(s, 10)
		if !ok {
			return nil, fmt.Errorf("cannot parse int %q", v)
		}
		return n, nil
	default:
		return nil, fmt.Errorf("cannot convert %T to int", value)
	}
}
