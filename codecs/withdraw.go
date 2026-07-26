package codecs

import (
	cryptorand "crypto/rand"
	"encoding/hex"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/Fabric-Labs/polyester-sdk-go/errors"
	chainwithdrawv1 "github.com/Fabric-Labs/polyester-sdk-go/gen/chain/withdraw/v1"
	typev1 "github.com/Fabric-Labs/polyester-sdk-go/gen/polyester/type/v1"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

const defaultTradingWithdrawDeadlineSeconds = 5 * 60

// NewTradingWithdrawIdempotencyKey returns a cryptographically random key.
// Generate it once per logical withdrawal, persist it, and reuse it on retry.
func NewTradingWithdrawIdempotencyKey() (string, error) {
	var random [16]byte
	if _, err := cryptorand.Read(random[:]); err != nil {
		return "", fmt.Errorf("secure randomness unavailable: %w", err)
	}
	return "wd-" + hex.EncodeToString(random[:]), nil
}

// NewTradingWithdrawNonce returns a cryptographically random, non-zero uint128
// decimal nonce. Persist it with the idempotency key before signing.
func NewTradingWithdrawNonce() (string, error) {
	for range 2 {
		var random [16]byte
		if _, err := cryptorand.Read(random[:]); err != nil {
			return "", fmt.Errorf("secure randomness unavailable: %w", err)
		}
		nonce := new(big.Int).SetBytes(random[:])
		if nonce.Sign() != 0 {
			return nonce.String(), nil
		}
	}
	return "", fmt.Errorf("secure random source returned a zero withdrawal nonce twice")
}

func defaultWithdrawDeadlineTsSec() (uint64, error) {
	now := time.Now().Unix()
	if now < 0 {
		return 0, &errors.ValidationError{Msg: "system clock is before UNIX_EPOCH"}
	}
	return uint64(now) + defaultTradingWithdrawDeadlineSeconds, nil
}

// BigIntToU128Proto encodes a non-negative big.Int as U128.
func BigIntToU128Proto(n *big.Int) *typev1.U128 {
	if n == nil || n.Sign() <= 0 {
		return &typev1.U128{}
	}
	hi := new(big.Int).Rsh(new(big.Int).Set(n), 64)
	lo := new(big.Int).And(n, new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 64), big.NewInt(1)))
	return &typev1.U128{Hi: hi.Uint64(), Lo: lo.Uint64()}
}

// StrToU128Proto scales a decimal string to U128 at the given scale.
func StrToU128Proto(value string, scale int) *typev1.U128 {
	if scale <= 0 {
		scale = LedgerScale
	}
	scaled, err := decimalToScaledBig(strings.TrimSpace(value), scale, "amount")
	if err != nil {
		return &typev1.U128{}
	}
	return BigIntToU128Proto(scaled)
}

func TradingWithdrawPayloadToProto(action string, assetID uint32, amount models.AssetAmountInput, idempotencyKey string, destinationChainID uint64, deadlineTsSec *uint64, nonce string, destinationAddress string, amountScale int) (*chainwithdrawv1.TradingWithdrawIntentPayload, error) {
	if strings.TrimSpace(idempotencyKey) == "" {
		return nil, &errors.ValidationError{Msg: "idempotency_key is required"}
	}
	nonceValue, ok := new(big.Int).SetString(strings.TrimSpace(nonce), 10)
	if !ok || nonceValue.Sign() <= 0 {
		return nil, &errors.ValidationError{Msg: "nonce must be a positive decimal integer"}
	}
	if nonceValue.BitLen() > 128 {
		return nil, &errors.ValidationError{Msg: "nonce exceeds uint128 range"}
	}
	if amountScale <= 0 {
		amountScale = LedgerScale
	}
	aid := assetID
	scaled, err := ResolveAssetAmountScaled(amount, amountScale, "amount", models.QuantityDomainLedgerE18, &aid)
	if err != nil {
		return nil, err
	}
	payload := &chainwithdrawv1.TradingWithdrawIntentPayload{
		AssetId:            assetID,
		DestinationAddress: destinationAddress,
		IdempotencyKey:     idempotencyKey,
		AmountE18:          BigIntToU128Proto(scaled),
	}
	switch strings.ToLower(strings.ReplaceAll(action, "-", "_")) {
	case "to_funding":
		payload.Action = chainwithdrawv1.TradingWithdrawAction_TO_FUNDING
	case "to_external_chain":
		payload.Action = chainwithdrawv1.TradingWithdrawAction_TO_EXTERNAL_CHAIN
		payload.DestinationChainId = destinationChainID
	default:
		payload.Action = chainwithdrawv1.TradingWithdrawAction_ACTION_UNSPECIFIED
	}
	if deadlineTsSec != nil {
		payload.DeadlineTsSec = *deadlineTsSec
	} else {
		payload.DeadlineTsSec, err = defaultWithdrawDeadlineTsSec()
		if err != nil {
			return nil, err
		}
	}
	payload.Nonce = BigIntToU128Proto(nonceValue)
	if payload.AmountE18 == nil || (payload.AmountE18.Hi == 0 && payload.AmountE18.Lo == 0) {
		return nil, &errors.ValidationError{Msg: "amount must be positive"}
	}
	return payload, nil
}
