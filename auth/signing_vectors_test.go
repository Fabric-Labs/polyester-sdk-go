package auth

import (
	"crypto/ed25519"
	"encoding/json"
	"os"
	"testing"
)

// Identical fixtures live in Go, Python and Rust. Expected queries follow
// the server's canonicalization: decoded key/value bytes sorted, then RFC 3986 escaped.
// Golden signatures use the public test seed 00..1f, timestamp 123, GET, no body.
func TestServerQuerySigningVectors(t *testing.T) {
	data, err := os.ReadFile("../testdata/signing_query_vectors.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Name           string `json:"name"`
		URL            string `json:"url"`
		CanonicalQuery string `json:"canonical_query"`
		SignatureHex   string `json:"signature_hex"`
	}
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	seed := make([]byte, ed25519.SeedSize)
	for i := range seed {
		seed[i] = byte(i)
	}
	creds := &Credentials{KeyID: "test-key", PrivateKey: ed25519.NewKeyFromSeed(seed)}
	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			query, err := CanonicalQuery(tc.URL)
			if err != nil {
				t.Fatal(err)
			}
			if query != tc.CanonicalQuery {
				t.Errorf("canonical query = %q, want %q", query, tc.CanonicalQuery)
			}
			headers, err := SignRequest(creds, "GET", tc.URL, nil, "123")
			if err != nil {
				t.Fatal(err)
			}
			if headers["X-API-SIGNATURE"] != tc.SignatureHex {
				t.Error("signature differs from server canonical query golden")
			}
		})
	}
}
