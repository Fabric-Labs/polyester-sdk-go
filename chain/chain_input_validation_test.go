package chain

import (
	"encoding/json"
	"errors"
	"math/big"
	"net/http"
	"strings"
	"testing"
	"time"

	sdkerrors "github.com/Fabric-Labs/polyester-sdk-go/errors"
	"github.com/Fabric-Labs/polyester-sdk-go/hardening"
)

// invalidSafeOwners are owners that are not an address someone controls: not
// 20 bytes of hex, or the zero address, which the Safe contract refuses as an
// owner.
var invalidSafeOwners = []string{
	"",
	"   ",
	"nothex",
	"0x",
	"0x1234",
	"0x" + strings.Repeat("a", 39),
	"0x" + strings.Repeat("a", 41),
	"0x" + strings.Repeat("a", 38) + "zz",
	"0xé" + strings.Repeat("a", 38),
	"0x0000000000000000000000000000000000000000",
	"0000000000000000000000000000000000000000",
	"0X0000000000000000000000000000000000000000",
}

func requireValidationNaming(t *testing.T, err error, field, input string) {
	t.Helper()
	var validation *sdkerrors.ValidationError
	if !errors.As(err, &validation) {
		t.Fatalf("owner %q: got %T %v, want *errors.ValidationError", input, err, err)
	}
	if !strings.Contains(validation.Msg, field) {
		t.Fatalf("owner %q: message %q does not name %s", input, validation.Msg, field)
	}
}

func TestPredictSafeAddressRefusesAnOwnerThatIsNotAnAddress(t *testing.T) {
	for _, owner := range invalidSafeOwners {
		got, err := PredictSafeAddress(owner, big.NewInt(0), nil)
		if err == nil {
			t.Fatalf("owner %q: predicted %s, want a validation error", owner, got)
		}
		requireValidationNaming(t, err, "owner_address", owner)

		got, err = PredictPolyesterSmartAccountAddress(owner, big.NewInt(0), nil)
		if err == nil {
			t.Fatalf("owner %q: alias predicted %s, want a validation error", owner, got)
		}
		requireValidationNaming(t, err, "owner_address", owner)

		predicted, err := PredictSafeAddressWithData(PredictSafeAddressOptions{
			Owners: []string{testOwner, owner},
		})
		if err == nil {
			t.Fatalf("owner %q: predicted %s, want a validation error", owner, predicted.Address)
		}
		requireValidationNaming(t, err, "PredictSafeAddressOptions.Owners[1]", owner)
	}
}

func TestPredictSafeAddressAcceptsEveryAddressSpelling(t *testing.T) {
	bare := strings.TrimPrefix(testOwner, "0x")
	for _, owner := range []string{
		testOwner,
		strings.ToLower(testOwner),
		"0x" + strings.ToUpper(bare),
		"0X" + bare,
		bare,
		" " + testOwner + "\n",
	} {
		got, err := PredictSafeAddress(owner, big.NewInt(0), nil)
		if err != nil {
			t.Fatalf("owner %q: %v", owner, err)
		}
		if got != testSalt0 {
			t.Fatalf("owner %q: got %s want %s", owner, got, testSalt0)
		}
	}
}

const (
	zipperEndpoint  = "0x1111111111111111111111111111111111111111"
	zipperToken     = "0x2222222222222222222222222222222222222222"
	feeFactoryWord  = "0x0000000000000000000000003333333333333333333333333333333333333333"
	decimalsWord    = "0x0000000000000000000000000000000000000000000000000000000000000012"
	feeWord         = "0x00000000000000000000000000000000000000000000000000000000000003e8"
	ethCallWordZero = "0x0000000000000000000000000000000000000000000000000000000000000000"
)

// malformedEthCallResults are eth_call results that are not one 0x-prefixed
// 32-byte ABI word: non-ASCII, short, odd length, non-hex, unprefixed, empty.
func malformedEthCallResults() []string {
	return []string{
		"0xé" + strings.Repeat("a", 39),
		"0x" + strings.Repeat("0", 62) + "é",
		"0xé",
		"0x1234",
		"0x" + strings.Repeat("0", 63),
		"0x" + strings.Repeat("0", 65),
		"0x" + strings.Repeat("0", 66),
		"0x" + strings.Repeat("0", 62) + "zz",
		"0x" + strings.Repeat("0", 62) + "-1",
		"-0x" + strings.Repeat("0", 64),
		strings.Repeat("0", 64),
		"not hex",
		"0x",
		"",
	}
}

// ethCallServer answers eth_call with result(to) and the request ID.
func ethCallServer(t *testing.T, result func(to string) any) *JSONRPCClient {
	t.Helper()
	srv := hardening.NewTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ID     int64            `json:"id"`
			Params []map[string]any `json:"params"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		to, _ := req.Params[0]["to"].(string)
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": result(to)})
	}))
	t.Cleanup(srv.Close)
	return NewJSONRPCClient(srv.URL, 5*time.Second)
}

// zipperServer answers feeFactory(), decimals() and getFee(..) by target
// address; the call at badCall (0, 1 or 2) gets bad.
func zipperServer(t *testing.T, badCall int, bad any) *JSONRPCClient {
	return ethCallServer(t, func(to string) any {
		call := 2 // getFee(uint16,address) at the fee factory
		switch to {
		case zipperEndpoint:
			call = 0 // feeFactory()
		case zipperToken:
			call = 1 // decimals()
		}
		if call == badCall {
			return bad
		}
		return []string{feeFactoryWord, decimalsWord, feeWord}[call]
	})
}

func TestQuoteZipperFeeDecodesWellFormedResults(t *testing.T) {
	quote, err := QuoteZipperFee(56, zipperToken, zipperEndpoint, nil, zipperServer(t, -1, nil))
	if err != nil {
		t.Fatal(err)
	}
	if quote.FeeFactory != "0x3333333333333333333333333333333333333333" ||
		quote.ZTokenDecimals != 18 || quote.Fee.Cmp(big.NewInt(1000)) != 0 {
		t.Fatalf("quote = %+v", quote)
	}
}

func requireTransportError(t *testing.T, err error, context string) {
	t.Helper()
	var transport *sdkerrors.TransportError
	if !errors.As(err, &transport) {
		t.Fatalf("%s: got %T %v, want *errors.TransportError", context, err, err)
	}
	if !strings.Contains(transport.Msg, "eth_call") {
		t.Fatalf("%s: message %q does not name eth_call", context, transport.Msg)
	}
}

func TestQuoteZipperFeeRefusesMalformedEthCallResults(t *testing.T) {
	for badCall := range 3 {
		for _, bad := range malformedEthCallResults() {
			_, err := QuoteZipperFee(56, zipperToken, zipperEndpoint, nil, zipperServer(t, badCall, bad))
			requireTransportError(t, err, "call "+string(rune('0'+badCall))+" result "+strconvQuote(bad))
		}
	}
	// An address word whose upper 12 bytes are not zero is not an ABI address.
	_, err := QuoteZipperFee(56, zipperToken, zipperEndpoint, nil,
		zipperServer(t, 0, "0x01"+strings.Repeat("0", 22)+strings.Repeat("3", 40)))
	requireTransportError(t, err, "dirty address word")
	// ERC-20 decimals is a uint8: a larger word is not a decimals answer.
	_, err = QuoteZipperFee(56, zipperToken, zipperEndpoint, nil,
		zipperServer(t, 1, "0x"+strings.Repeat("0", 60)+"0100"))
	requireTransportError(t, err, "decimals 256")
	_, err = QuoteZipperFee(56, zipperToken, zipperEndpoint, nil,
		zipperServer(t, 1, "0x01"+strings.Repeat("0", 62)))
	requireTransportError(t, err, "decimals 2^248")
}

func strconvQuote(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

func smartAccountWithRPC(rpc *JSONRPCClient) *SmartAccount {
	return &SmartAccount{
		Address:     testSalt0,
		Environment: PolyesterDevnetEnvironment,
		rpc:         rpc,
	}
}

func TestGetNonceDecodesOneABIWord(t *testing.T) {
	for _, tc := range []struct{ word, want string }{
		{ethCallWordZero, "0"},
		{"0x" + strings.Repeat("0", 63) + "5", "5"},
		// key 1, sequence 0: the packed nonce (key << 64) | seq.
		{"0x" + strings.Repeat("0", 47) + "1" + strings.Repeat("0", 16), "18446744073709551616"},
	} {
		account := smartAccountWithRPC(ethCallServer(t, func(string) any { return tc.word }))
		got, err := account.GetNonce(big.NewInt(0))
		if err != nil {
			t.Fatalf("word %s: %v", tc.word, err)
		}
		if got.String() != tc.want {
			t.Fatalf("word %s: got %s want %s", tc.word, got, tc.want)
		}
	}
}

func TestGetNonceRefusesMalformedEthCallResults(t *testing.T) {
	for _, bad := range append(malformedEthCallResults(), "0x0", "0x1", "0x05") {
		account := smartAccountWithRPC(ethCallServer(t, func(string) any { return bad }))
		got, err := account.GetNonce(big.NewInt(0))
		if err == nil {
			t.Fatalf("result %q: nonce %s, want an error", bad, got)
		}
		requireTransportError(t, err, "result "+strconvQuote(bad))
	}
}

// asInt reads a hex string as a JSON-RPC quantity: "0x" and at least one hex
// digit, with no sign and no leading zero ("0x0" is zero).
func TestAsIntReadsHexStringsAsJSONRPCQuantities(t *testing.T) {
	for in, want := range map[string]string{
		"0x0":                 "0",
		"0x1":                 "1",
		"0X1f":                "31",
		"0xABCdef":            "11259375",
		" 0x10 ":              "16",
		"0x10000000000000000": "18446744073709551616",
	} {
		got, err := asInt(in)
		if err != nil || got.String() != want {
			t.Errorf("%q: got (%v, %v), want %s", in, got, err, want)
		}
	}
	for _, in := range []string{"0x", "0X", " 0x ", "0x00", "0x01", "0x0f", "0x+1", "0x-1", "0x 1", "0xg", "0x1_0", "0xé"} {
		if got, err := asInt(in); err == nil {
			t.Errorf("%q: got %v, want an error", in, got)
		}
	}
}
