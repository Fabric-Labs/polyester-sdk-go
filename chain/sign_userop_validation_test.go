package chain

import (
	"crypto/ecdsa"
	"errors"
	"math/big"
	"strings"
	"testing"

	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	"github.com/ethereum/go-ethereum/crypto"
)

func validSignSafeUserOperationParams(t *testing.T) SignSafeUserOperationParams {
	t.Helper()
	key, err := crypto.HexToECDSA("4c0883a69102937d6231471b5dbb6204fe5129617082792ae468d01a3f362318")
	if err != nil {
		t.Fatal(err)
	}
	return SignSafeUserOperationParams{
		OwnerKey:             key,
		Environment:          PolyesterDevnetEnvironment,
		Sender:               "0x00000000000000000000000000000000000000a1",
		Nonce:                big.NewInt(1),
		CallGasLimit:         big.NewInt(100_000),
		VerificationGasLimit: big.NewInt(200_000),
		PreVerificationGas:   big.NewInt(50_000),
		MaxFeePerGas:         big.NewInt(2),
		MaxPriorityFeePerGas: big.NewInt(1),
		ValidUntil:           1 << 40,
	}
}

func TestSignSafeUserOperationSignsValidInput(t *testing.T) {
	signature, err := SignSafeUserOperation(validSignSafeUserOperationParams(t))
	if err != nil {
		t.Fatal(err)
	}
	if len(signature) != 6+6+65 {
		t.Fatalf("signature length %d, want 77", len(signature))
	}
}

// Every missing or invalid input is a validation error naming its field, not a
// panic or a signature over a zero value.
func TestSignSafeUserOperationRefusesInvalidInput(t *testing.T) {
	for _, tc := range []struct {
		field  string
		mutate func(*SignSafeUserOperationParams)
	}{
		{"OwnerKey", func(p *SignSafeUserOperationParams) { p.OwnerKey = nil }},
		{"OwnerKey", func(p *SignSafeUserOperationParams) { p.OwnerKey = &ecdsa.PrivateKey{} }},
		{"Environment.ChainID", func(p *SignSafeUserOperationParams) { p.Environment = PolyesterEnvironment{} }},
		{"Environment.AccountAbstraction.Safe.Safe4337ModuleAddress", func(p *SignSafeUserOperationParams) {
			p.Environment.AccountAbstraction.Safe.Safe4337ModuleAddress = ""
		}},
		{"Environment.AccountAbstraction.EntryPoint.Address", func(p *SignSafeUserOperationParams) {
			p.Environment.AccountAbstraction.EntryPoint.Address = "entrypoint"
		}},
		{"Sender", func(p *SignSafeUserOperationParams) { p.Sender = "" }},
		{"Nonce", func(p *SignSafeUserOperationParams) { p.Nonce = nil }},
		{"Nonce", func(p *SignSafeUserOperationParams) { p.Nonce = big.NewInt(-1) }},
		{"CallGasLimit", func(p *SignSafeUserOperationParams) { p.CallGasLimit = nil }},
		{"VerificationGasLimit", func(p *SignSafeUserOperationParams) { p.VerificationGasLimit = nil }},
		{"VerificationGasLimit", func(p *SignSafeUserOperationParams) { p.VerificationGasLimit = new(big.Int).Lsh(big.NewInt(1), 128) }},
		{"PreVerificationGas", func(p *SignSafeUserOperationParams) { p.PreVerificationGas = nil }},
		{"MaxFeePerGas", func(p *SignSafeUserOperationParams) { p.MaxFeePerGas = nil }},
		{"MaxPriorityFeePerGas", func(p *SignSafeUserOperationParams) { p.MaxPriorityFeePerGas = nil }},
		{"ValidAfter", func(p *SignSafeUserOperationParams) { p.ValidAfter = 1 << 48 }},
		{"ValidUntil", func(p *SignSafeUserOperationParams) { p.ValidUntil = 1 << 63 }},
	} {
		t.Run(tc.field, func(t *testing.T) {
			p := validSignSafeUserOperationParams(t)
			tc.mutate(&p)
			var signature []byte
			var err error
			func() {
				defer func() {
					if r := recover(); r != nil {
						t.Fatalf("panicked: %v", r)
					}
				}()
				signature, err = SignSafeUserOperation(p)
			}()
			var validation *sdkerrors.ValidationError
			if !errors.As(err, &validation) || signature != nil {
				t.Fatalf("got signature=%x err=%v, want *errors.ValidationError", signature, err)
			}
			if !strings.Contains(err.Error(), tc.field) {
				t.Fatalf("error %q does not name %s", err, tc.field)
			}
		})
	}
}
