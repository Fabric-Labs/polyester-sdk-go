package auth

import (
	"crypto/ed25519"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// Identical fixtures live in Go, Python and Rust. Expected queries follow
// the server's canonicalization: decoded key/value bytes sorted, then RFC 3986 escaped.
// The signed path is the decoded path: canonical_path names it where it is valid UTF-8.
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
		CanonicalPath  string `json:"canonical_path"`
		SignatureHex   string `json:"signature_hex"`
	}
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	paths := 0
	for _, tc := range cases {
		if strings.HasPrefix(tc.Name, "path-") {
			paths++
		}
	}
	if paths < 10 {
		t.Fatalf("path vectors = %d, want at least 10", paths)
	}
	seed := make([]byte, ed25519.SeedSize)
	for i := range seed {
		seed[i] = byte(i)
	}
	creds := testCredentials(t, "test-key", ed25519.NewKeyFromSeed(seed))
	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			query, err := CanonicalQuery(tc.URL)
			if err != nil {
				t.Fatal(err)
			}
			if query != tc.CanonicalQuery {
				t.Errorf("canonical query = %q, want %q", query, tc.CanonicalQuery)
			}
			if tc.CanonicalPath != "" {
				signed, err := CanonicalSigningString("123", "GET", tc.URL, nil)
				if err != nil {
					t.Fatal(err)
				}
				if got := strings.Split(signed, "\n")[2]; got != tc.CanonicalPath {
					t.Errorf("signed path = %q, want %q", got, tc.CanonicalPath)
				}
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
