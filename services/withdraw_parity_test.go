package services

import (
	"crypto/ed25519"
	"math/big"
	"testing"

	"github.com/Fabric-Labs/polyester-sdk-go/auth"
	"github.com/Fabric-Labs/polyester-sdk-go/codecs"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
	"github.com/Fabric-Labs/polyester-sdk-go/transport"
)

func TestPreparedAPIKeyWithdrawSignsExactPersistentPayload(t *testing.T) {
	seed := make([]byte, ed25519.SeedSize)
	for i := range seed {
		seed[i] = 7
	}
	creds := &auth.Credentials{KeyID: "ak-test", PrivateKey: ed25519.NewKeyFromSeed(seed)}
	service := NewWithdrawService(transport.NewFactory(transport.DefaultConfig(), creds, nil), nil)
	deadline := uint64(1_800_000_000)
	amount := models.MustAssetAmountScaled(125).
		WithScale(2).
		WithDomain(models.QuantityDomainLedgerE18).
		WithAssetID(7)
	prepared, err := service.PrepareAPIKeyToFunding(PrepareAPIKeyWithdrawParams{
		AssetID: 7, Amount: models.AssetAmountFromScaled(amount),
		IdempotencyKey: "prepared-withdraw", AmountScale: 2,
		DeadlineTsSec: &deadline, Nonce: "42",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !ed25519.Verify(creds.PrivateKey.Public().(ed25519.PublicKey), prepared.DeterministicPayloadBytes(), prepared.PayloadSignature()) {
		t.Fatal("signature does not cover deterministic payload bytes")
	}
	payload := prepared.Payload()
	if payload.DeadlineTsSec != deadline {
		t.Fatalf("deadline=%d", payload.DeadlineTsSec)
	}
	got := codecs.U128ToBig(payload.AmountE18.Hi, payload.AmountE18.Lo)
	want, _ := new(big.Int).SetString("1250000000000000000", 10)
	if got.Cmp(want) != 0 {
		t.Fatalf("amount_e18=%s want %s", got, want)
	}
	payload.AssetId = 99
	if prepared.Payload().AssetId != 7 {
		t.Fatal("Payload leaked mutable prepared state")
	}
	restored, err := PreparedTradingWithdrawFromBytes(prepared.RequestBytes())
	if err != nil {
		t.Fatal(err)
	}
	if string(restored.RequestBytes()) != string(prepared.RequestBytes()) {
		t.Fatal("prepared request persistence round-trip changed bytes")
	}
}

func TestPreparedWithdrawRejectsInvalidPersistenceBytes(t *testing.T) {
	if _, err := PreparedTradingWithdrawFromBytes(nil); err == nil {
		t.Fatal("expected invalid prepared bytes rejection")
	}
}

func TestInternalTransferAmountUsesCanonicalE18(t *testing.T) {
	scale := 2
	amount := models.MustAssetAmountScaled(125).
		WithScale(scale).
		WithDomain(models.QuantityDomainLedgerE18).
		WithAssetID(7)
	wire, err := internalTransferAmountE18(models.AssetAmountFromScaled(amount), &scale, 7)
	if err != nil {
		t.Fatal(err)
	}
	got := codecs.U128ToBig(wire.Hi, wire.Lo)
	want, _ := new(big.Int).SetString("1250000000000000000", 10)
	if got.Cmp(want) != 0 {
		t.Fatalf("amount_e18=%s want %s", got, want)
	}
}
