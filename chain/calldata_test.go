package chain_test

import (
	"bytes"
	"encoding/hex"
	"math/big"
	"strings"
	"testing"

	"github.com/Fabric-Labs/polyester-sdk-go/chain"
	"github.com/Fabric-Labs/polyester-sdk-go/errors"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

const (
	tradingGateway  = "0x4444444444444444444444444444444444444444"
	fundingAccount  = "0x1111111111111111111111111111111111111111"
	internalAccount = "0x3333333333333333333333333333333333333333"
	uAssetID        = "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	zToken          = "0x5555555555555555555555555555555555555555"
)

func selector(sig string) []byte {
	return crypto.Keccak256([]byte(sig))[:4]
}

func TestEncodeTradingGatewayDeposit(t *testing.T) {
	call, err := chain.EncodeTradingGatewayDeposit(tradingGateway, uAssetID, big.NewInt(1_000_000))
	if err != nil {
		t.Fatal(err)
	}
	if call.To != strings.ToLower(tradingGateway) {
		t.Fatalf("to=%s", call.To)
	}
	if call.Value.Sign() != 0 {
		t.Fatalf("value=%v", call.Value)
	}
	wantSel := selector("deposit(bytes32,uint256)")
	if !bytes.Equal(call.Data[:4], wantSel) {
		t.Fatalf("selector=%x want %x", call.Data[:4], wantSel)
	}

	args := abi.Arguments{
		{Type: mustType("bytes32")},
		{Type: mustType("uint256")},
	}
	decoded, err := args.Unpack(call.Data[4:])
	if err != nil {
		t.Fatal(err)
	}
	gotAsset := decoded[0].([32]byte)
	wantAsset, _ := hex.DecodeString(uAssetID[2:])
	if !bytes.Equal(gotAsset[:], wantAsset) {
		t.Fatalf("asset mismatch")
	}
	if decoded[1].(*big.Int).Cmp(big.NewInt(1_000_000)) != 0 {
		t.Fatalf("amount=%v", decoded[1])
	}
}

func TestEncodeTradingGatewayDepositTo(t *testing.T) {
	call, err := chain.EncodeTradingGatewayDepositTo(
		tradingGateway, internalAccount, uAssetID, big.NewInt(1_000_000),
	)
	if err != nil {
		t.Fatal(err)
	}
	wantSel := selector("depositTo(address,bytes32,uint256)")
	if !bytes.Equal(call.Data[:4], wantSel) {
		t.Fatalf("selector=%x want %x", call.Data[:4], wantSel)
	}
	args := abi.Arguments{
		{Type: mustType("address")},
		{Type: mustType("bytes32")},
		{Type: mustType("uint256")},
	}
	decoded, err := args.Unpack(call.Data[4:])
	if err != nil {
		t.Fatal(err)
	}
	got := decoded[0].(common.Address).Hex()
	if !strings.EqualFold(got, internalAccount) {
		t.Fatalf("account=%s", got)
	}
}

func TestEncodeFundingWithdrawToChain(t *testing.T) {
	destination := []byte{0x12, 0x34}
	call, err := chain.EncodeFundingWithdrawToChain(
		fundingAccount, 56, zToken, destination, big.NewInt(2_000_000), big.NewInt(1000),
	)
	if err != nil {
		t.Fatal(err)
	}
	wantSel := selector("withdrawToChain((uint16,address,bytes,uint256,uint256))")
	if !bytes.Equal(call.Data[:4], wantSel) {
		t.Fatalf("selector=%x want %x", call.Data[:4], wantSel)
	}

	tupleType, err := abi.NewType("tuple", "", []abi.ArgumentMarshaling{
		{Name: "chainId", Type: "uint16"},
		{Name: "zToken", Type: "address"},
		{Name: "withdrawDestination", Type: "bytes"},
		{Name: "zAmount", Type: "uint256"},
		{Name: "maxFee", Type: "uint256"},
	})
	if err != nil {
		t.Fatal(err)
	}
	args := abi.Arguments{{Type: tupleType}}
	decoded, err := args.Unpack(call.Data[4:])
	if err != nil {
		t.Fatal(err)
	}
	req := decoded[0].(struct {
		ChainId             uint16         `json:"chainId"`
		ZToken              common.Address `json:"zToken"`
		WithdrawDestination []byte         `json:"withdrawDestination"`
		ZAmount             *big.Int       `json:"zAmount"`
		MaxFee              *big.Int       `json:"maxFee"`
	})
	if req.ChainId != 56 {
		t.Fatalf("chainId=%d", req.ChainId)
	}
	if !strings.EqualFold(req.ZToken.Hex(), zToken) {
		t.Fatalf("zToken=%s", req.ZToken.Hex())
	}
	if !bytes.Equal(req.WithdrawDestination, destination) {
		t.Fatalf("destination=%x", req.WithdrawDestination)
	}
	if req.ZAmount.Cmp(big.NewInt(2_000_000)) != 0 || req.MaxFee.Cmp(big.NewInt(1000)) != 0 {
		t.Fatalf("amounts z=%v fee=%v", req.ZAmount, req.MaxFee)
	}
}

func TestEncodeSetExternalDestinationAllowlistRequired(t *testing.T) {
	call, err := chain.EncodeSetExternalDestinationAllowlistRequired(fundingAccount, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	wantSel := selector("setExternalDestinationAllowlistRequired(bool,(uint192,uint256,bytes))")
	if !bytes.Equal(call.Data[:4], wantSel) {
		t.Fatalf("selector=%x want %x", call.Data[:4], wantSel)
	}
}

func TestEncodeSetInternalAccountAllowlistRequired(t *testing.T) {
	call, err := chain.EncodeSetInternalAccountAllowlistRequired(fundingAccount, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	wantSel := selector("setInternalAccountAllowlistRequired(bool,(uint192,uint256,bytes))")
	if !bytes.Equal(call.Data[:4], wantSel) {
		t.Fatalf("selector=%x want %x", call.Data[:4], wantSel)
	}
	args := abi.Arguments{
		{Type: mustType("bool")},
		{Type: mustGuardTupleType(t)},
	}
	decoded, err := args.Unpack(call.Data[4:])
	if err != nil {
		t.Fatal(err)
	}
	if decoded[0].(bool) != true {
		t.Fatalf("required=%v", decoded[0])
	}
}

func TestEncodeAddRemoveAllowedExternalDestinations(t *testing.T) {
	destinations := [][]byte{{0x12, 0x34}, {0xab, 0xcd}}
	approval := &chain.GuardApproval{
		NonceSpace: big.NewInt(7),
		Deadline:   big.NewInt(123),
		Signature:  []byte{0xab, 0xcd},
	}

	addCall, err := chain.EncodeAddAllowedExternalDestinations(fundingAccount, 56, destinations, approval)
	if err != nil {
		t.Fatal(err)
	}
	wantAdd := selector("addAllowedExternalDestinations(uint16,bytes[],(uint192,uint256,bytes))")
	if !bytes.Equal(addCall.Data[:4], wantAdd) {
		t.Fatalf("add selector=%x want %x", addCall.Data[:4], wantAdd)
	}
	addArgs := abi.Arguments{
		{Type: mustType("uint16")},
		{Type: mustType("bytes[]")},
		{Type: mustGuardTupleType(t)},
	}
	decoded, err := addArgs.Unpack(addCall.Data[4:])
	if err != nil {
		t.Fatal(err)
	}
	if decoded[0].(uint16) != 56 {
		t.Fatalf("chainId=%v", decoded[0])
	}
	gotDest := decoded[1].([][]byte)
	if len(gotDest) != 2 || !bytes.Equal(gotDest[0], destinations[0]) || !bytes.Equal(gotDest[1], destinations[1]) {
		t.Fatalf("destinations=%x", gotDest)
	}

	removeCall, err := chain.EncodeRemoveAllowedExternalDestinations(fundingAccount, 56, destinations, nil)
	if err != nil {
		t.Fatal(err)
	}
	wantRemove := selector("removeAllowedExternalDestinations(uint16,bytes[],(uint192,uint256,bytes))")
	if !bytes.Equal(removeCall.Data[:4], wantRemove) {
		t.Fatalf("remove selector=%x want %x", removeCall.Data[:4], wantRemove)
	}
}

func TestEncodeAddRemoveAllowedInternalAccounts(t *testing.T) {
	accounts := []string{internalAccount, "0x6666666666666666666666666666666666666666"}
	addCall, err := chain.EncodeAddAllowedInternalAccounts(fundingAccount, accounts, nil)
	if err != nil {
		t.Fatal(err)
	}
	wantAdd := selector("addAllowedInternalAccounts(address[],(uint192,uint256,bytes))")
	if !bytes.Equal(addCall.Data[:4], wantAdd) {
		t.Fatalf("add selector=%x want %x", addCall.Data[:4], wantAdd)
	}
	addArgs := abi.Arguments{
		{Type: mustType("address[]")},
		{Type: mustGuardTupleType(t)},
	}
	decoded, err := addArgs.Unpack(addCall.Data[4:])
	if err != nil {
		t.Fatal(err)
	}
	got := decoded[0].([]common.Address)
	if len(got) != 2 || !strings.EqualFold(got[0].Hex(), accounts[0]) || !strings.EqualFold(got[1].Hex(), accounts[1]) {
		t.Fatalf("accounts=%v", got)
	}

	removeCall, err := chain.EncodeRemoveAllowedInternalAccounts(fundingAccount, accounts, nil)
	if err != nil {
		t.Fatal(err)
	}
	wantRemove := selector("removeAllowedInternalAccounts(address[],(uint192,uint256,bytes))")
	if !bytes.Equal(removeCall.Data[:4], wantRemove) {
		t.Fatalf("remove selector=%x want %x", removeCall.Data[:4], wantRemove)
	}
}

func TestEncodeInitializeAndRotateGuardSigner(t *testing.T) {
	guardRegistry := "0xd71F60FD6f784Cc0aD8c25441568C48705D95f64"
	signer := "0x7777777777777777777777777777777777777777"

	initCall, err := chain.EncodeInitializeGuardSigner(guardRegistry, signer)
	if err != nil {
		t.Fatal(err)
	}
	wantInit := selector("initializeSigner(address)")
	if !bytes.Equal(initCall.Data[:4], wantInit) {
		t.Fatalf("init selector=%x want %x", initCall.Data[:4], wantInit)
	}
	if initCall.To != strings.ToLower(guardRegistry) {
		t.Fatalf("to=%s", initCall.To)
	}
	initArgs := abi.Arguments{{Type: mustType("address")}}
	decoded, err := initArgs.Unpack(initCall.Data[4:])
	if err != nil {
		t.Fatal(err)
	}
	if !strings.EqualFold(decoded[0].(common.Address).Hex(), signer) {
		t.Fatalf("signer=%v", decoded[0])
	}

	approval := &chain.GuardApproval{
		NonceSpace: big.NewInt(1),
		Deadline:   big.NewInt(999),
		Signature:  []byte{0x01, 0x02},
	}
	rotateCall, err := chain.EncodeRotateGuardSigner(guardRegistry, signer, approval)
	if err != nil {
		t.Fatal(err)
	}
	wantRotate := selector("rotateSigner(address,(uint192,uint256,bytes))")
	if !bytes.Equal(rotateCall.Data[:4], wantRotate) {
		t.Fatalf("rotate selector=%x want %x", rotateCall.Data[:4], wantRotate)
	}
	rotateArgs := abi.Arguments{
		{Type: mustType("address")},
		{Type: mustGuardTupleType(t)},
	}
	decodedRotate, err := rotateArgs.Unpack(rotateCall.Data[4:])
	if err != nil {
		t.Fatal(err)
	}
	if !strings.EqualFold(decodedRotate[0].(common.Address).Hex(), signer) {
		t.Fatalf("newSigner=%v", decodedRotate[0])
	}
}

func mustGuardTupleType(t *testing.T) abi.Type {
	t.Helper()
	typ, err := abi.NewType("tuple", "", []abi.ArgumentMarshaling{
		{Name: "nonceSpace", Type: "uint192"},
		{Name: "deadline", Type: "uint256"},
		{Name: "signature", Type: "bytes"},
	})
	if err != nil {
		t.Fatal(err)
	}
	return typ
}

func TestDepositRejectsNonPositive(t *testing.T) {
	_, err := chain.EncodeTradingGatewayDeposit(tradingGateway, uAssetID, big.NewInt(0))
	if err == nil {
		t.Fatal("expected error")
	}
	var ve *errors.ValidationError
	if !asValidation(err, &ve) {
		t.Fatalf("want ValidationError, got %T", err)
	}
}

func TestWithdrawRejectsAmountNotGreaterThanFee(t *testing.T) {
	_, err := chain.EncodeFundingWithdrawToChain(
		fundingAccount, 1, zToken, []byte{0x12, 0x34}, big.NewInt(100), big.NewInt(100),
	)
	if err == nil {
		t.Fatal("expected error")
	}
	var ve *errors.ValidationError
	if !asValidation(err, &ve) {
		t.Fatalf("want ValidationError, got %T", err)
	}
}

func mustType(typ string) abi.Type {
	t, err := abi.NewType(typ, "", nil)
	if err != nil {
		panic(err)
	}
	return t
}

func asValidation(err error, target **errors.ValidationError) bool {
	ve, ok := err.(*errors.ValidationError)
	if !ok {
		return false
	}
	*target = ve
	return true
}
