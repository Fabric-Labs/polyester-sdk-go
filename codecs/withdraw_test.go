package codecs

import (
	"strings"
	"testing"

	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

func TestTradingWithdrawRequiresStableIdempotencyKey(t *testing.T) {
	_, err := TradingWithdrawPayloadToProto(
		"to_funding",
		7,
		models.AssetAmountFromDecimal("1"),
		"",
		0,
		nil,
		"42",
		"",
		18,
	)
	if _, ok := err.(*sdkerrors.ValidationError); !ok {
		t.Fatalf("err=%T %v", err, err)
	}
}

func TestTradingWithdrawRejectsInvalidNonce(t *testing.T) {
	for _, nonce := range []string{"", "0", "-1", strings.Repeat("9", 50)} {
		t.Run(nonce, func(t *testing.T) {
			_, err := TradingWithdrawPayloadToProto(
				"to_funding",
				7,
				models.AssetAmountFromDecimal("1"),
				"stable-withdraw",
				0,
				nil,
				nonce,
				"",
				18,
			)
			if _, ok := err.(*sdkerrors.ValidationError); !ok {
				t.Fatalf("err=%T %v", err, err)
			}
		})
	}
}

func TestTradingWithdrawPreservesCallerRetryIdentifiers(t *testing.T) {
	deadline := uint64(1_800_000_000)
	payload, err := TradingWithdrawPayloadToProto(
		"to_funding",
		7,
		models.AssetAmountFromDecimal("1"),
		"stable-withdraw",
		0,
		&deadline,
		"340282366920938463463374607431768211455",
		"",
		18,
	)
	if err != nil {
		t.Fatal(err)
	}
	if payload.IdempotencyKey != "stable-withdraw" {
		t.Fatalf("idempotency key=%q", payload.IdempotencyKey)
	}
	if payload.Nonce.Hi != ^uint64(0) || payload.Nonce.Lo != ^uint64(0) {
		t.Fatalf("nonce=%#v", payload.Nonce)
	}
}

func TestTradingWithdrawGeneratorsReturnExplicitValues(t *testing.T) {
	first, err := NewTradingWithdrawIdempotencyKey()
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewTradingWithdrawIdempotencyKey()
	if err != nil {
		t.Fatal(err)
	}
	if first == second || !strings.HasPrefix(first, "wd-") || len(first) != 35 {
		t.Fatalf("first=%q second=%q", first, second)
	}
	nonce, err := NewTradingWithdrawNonce()
	if err != nil {
		t.Fatal(err)
	}
	if nonce == "" || nonce == "0" {
		t.Fatalf("nonce=%q", nonce)
	}
}
