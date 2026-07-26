package auth

import (
	"crypto/ed25519"
	"encoding/hex"

	"github.com/Fabric-Labs/polyester-sdk-go/models"
)

// GenerateEd25519Keypair generates a new Ed25519 keypair for API key creation.
func GenerateEd25519Keypair() models.Ed25519Keypair {
	publicKey, secretKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		panic(err)
	}
	seed := secretKey.Seed()
	return models.Ed25519Keypair{
		PublicKeyHex: hex.EncodeToString(publicKey),
		SecretKeyHex: hex.EncodeToString(seed),
		PublicKey:    publicKey,
		SecretKey:    seed,
	}
}
