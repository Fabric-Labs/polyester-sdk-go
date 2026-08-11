package auth

import (
	"crypto/ed25519"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Fabric-Labs/polyester-sdk-go/errors"
)

func TestCanonicalQuerySortsAndEncodesValues(t *testing.T) {
	got, err := CanonicalQuery("https://api.example.test/path?b=2&a=hello world")
	if err != nil {
		t.Fatal(err)
	}
	want := "a=hello%20world&b=2"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestCanonicalQueryPreservesHyphensInChannelParam(t *testing.T) {
	got, err := CanonicalQuery("https://api.example.test/v1/rt/subscribe?channel=private:auth:api-keys:account:proto")
	if err != nil {
		t.Fatal(err)
	}
	want := "channel=private%3Aauth%3Aapi-keys%3Aaccount%3Aproto"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestCanonicalQuerySharedVectors(t *testing.T) {
	cases := []struct {
		url, want string
	}{
		{"https://api.example.test/x?z=1&a=hello world&m=a+b", "a=hello%20world&m=a%20b&z=1"},
		{"https://api.example.test/x?z=1&a=hello%20world&m=a%2Bb", "a=hello%20world&m=a%2Bb&z=1"},
		{"https://api.example.test/x?b=&a=1", "a=1&b="},
		{"https://api.example.test/x?a=1&a=2&b=0", "a=1&a=2&b=0"},
		{"https://api.example.test/x?path=foo/bar&name=a_b.c~d-e", "name=a_b.c~d-e&path=foo%2Fbar"},
		{"https://api.example.test/x?msg=%E2%9C%93&plain=ok", "msg=%E2%9C%93&plain=ok"},
	}
	for _, tc := range cases {
		got, err := CanonicalQuery(tc.url)
		if err != nil {
			t.Fatal(err)
		}
		if got != tc.want {
			t.Fatalf("url=%q got %q want %q", tc.url, got, tc.want)
		}
	}
}

func TestCanonicalSigningStringMatchesContract(t *testing.T) {
	got, err := CanonicalSigningString(
		"123",
		"post",
		"https://api.example.test/foo/bar?b=2&a=1",
		[]byte("{}"),
	)
	if err != nil {
		t.Fatal(err)
	}
	want := "123\nPOST\n/foo/bar\na=1&b=2\n44136fa355b3678a1146ad16f7e8649e94fb4fc21fe77e8310c060f61caaff8a"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestSignRequestReturnsPolyesterHeaders(t *testing.T) {
	_, private, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	creds := &Credentials{KeyID: "key_123", PrivateKey: private}
	headers, err := SignRequest(creds, "POST", "https://api.example.test/foo", []byte("{}"), "123")
	if err != nil {
		t.Fatal(err)
	}
	if headers["X-API-KEY-ID"] != "key_123" {
		t.Fatalf("key id: %s", headers["X-API-KEY-ID"])
	}
	if headers["X-API-TIMESTAMP"] != "123" {
		t.Fatalf("timestamp: %s", headers["X-API-TIMESTAMP"])
	}
	if len(headers["X-API-SIGNATURE"]) != 128 {
		t.Fatalf("signature length: %d", len(headers["X-API-SIGNATURE"]))
	}
}

func TestTenThousandIdenticalRequestsGetUniqueBoundedAuthTuples(t *testing.T) {
	_, private, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	creds := &Credentials{KeyID: "key_123", PrivateKey: private}
	const count = 10_000
	var wg sync.WaitGroup
	type signedAt struct {
		headers      map[string]string
		observedAtMS int64
	}
	results := make(chan signedAt, count)
	stopTicker := make(chan struct{})
	timerTicks := make(chan struct{}, count)
	go func() {
		ticker := time.NewTicker(10 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				timerTicks <- struct{}{}
			case <-stopTicker:
				return
			}
		}
	}()
	before := time.Now().UnixMilli()
	wg.Add(count)
	for range count {
		go func() {
			defer wg.Done()
			signed, signErr := SignRequest(creds, "POST", "https://api.example.test/foo", []byte("{}"), "")
			if signErr != nil {
				t.Errorf("sign: %v", signErr)
				return
			}
			results <- signedAt{headers: signed, observedAtMS: time.Now().UnixMilli()}
		}()
	}
	wg.Wait()
	close(stopTicker)
	close(results)
	timestamps := make(map[int64]struct{}, count)
	signatures := make(map[string]struct{}, count)
	for result := range results {
		item := result.headers
		ts, parseErr := strconv.ParseInt(item["X-API-TIMESTAMP"], 10, 64)
		if parseErr != nil {
			t.Fatal(parseErr)
		}
		if ts < before || ts > result.observedAtMS+MaxSigningFutureSkewMS {
			t.Fatalf(
				"timestamp %d outside per-request bounded interval [%d,%d]",
				ts, before, result.observedAtMS+MaxSigningFutureSkewMS,
			)
		}
		if _, exists := timestamps[ts]; exists {
			t.Fatalf("duplicate timestamp %d", ts)
		}
		timestamps[ts] = struct{}{}
		signature := item["X-API-SIGNATURE"]
		if _, exists := signatures[signature]; exists {
			t.Fatalf("duplicate signature for timestamp %d", ts)
		}
		signatures[signature] = struct{}{}
	}
	if len(timestamps) != count || len(signatures) != count {
		t.Fatalf("unique timestamps=%d signatures=%d want %d", len(timestamps), len(signatures), count)
	}
	if len(timerTicks) < 100 {
		t.Fatalf("scheduler timer advanced only %d times during signing backpressure", len(timerTicks))
	}
}

func TestSignRequestRejectsMalformedURL(t *testing.T) {
	_, private, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = SignRequest(
		&Credentials{KeyID: "key_123", PrivateKey: private},
		"POST",
		"://not-a-url",
		[]byte("{}"),
		"",
	)
	if err == nil {
		t.Fatal("malformed URL must fail closed")
	}
}

func TestCredentialsStringRedactsPrivateKey(t *testing.T) {
	_, private, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	creds := &Credentials{KeyID: "key_123", PrivateKey: private}
	secret := hex.EncodeToString(private.Seed())
	rendered := creds.String()
	if !strings.Contains(rendered, "[REDACTED]") {
		t.Fatalf("expected redaction, got %s", rendered)
	}
	if strings.Contains(rendered, secret) {
		t.Fatalf("private key leaked: %s", rendered)
	}
	goRendered := creds.GoString()
	if !strings.Contains(goRendered, "[REDACTED]") {
		t.Fatalf("GoString expected redaction, got %s", goRendered)
	}
	if strings.Contains(goRendered, secret) {
		t.Fatalf("private key leaked via GoString: %s", goRendered)
	}
	if strings.Contains(fmt.Sprintf("%#v", creds), secret) {
		t.Fatalf("private key leaked via %%#v: %#v", creds)
	}
}

func TestLoadCredentialsFromEnv(t *testing.T) {
	_, private, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("POLYESTER_API_KEY_ID", "ak_test")
	t.Setenv("POLYESTER_API_PRIVATE_KEY", hex.EncodeToString(private.Seed()))
	creds, err := LoadCredentials("", "", true)
	if err != nil {
		t.Fatal(err)
	}
	if creds == nil {
		t.Fatal("expected credentials")
	}
	if string(private.Seed()) != string(creds.PrivateKey.Seed()) {
		t.Fatal("private key mismatch")
	}
}

func TestLoadCredentialsRequiresBoth(t *testing.T) {
	t.Setenv("POLYESTER_API_KEY_ID", "ak_test")
	t.Setenv("POLYESTER_API_PRIVATE_KEY", "")
	_, err := LoadCredentials("", "", true)
	if err == nil {
		t.Fatal("expected error")
	}
	if _, ok := err.(*errors.AuthError); !ok {
		t.Fatalf("expected AuthError, got %T", err)
	}
}

func TestMalformedPrivateKeyErrorDoesNotDiscloseSecret(t *testing.T) {
	secret := "not-hex-private-key-material"
	_, err := LoadCredentials("ak_test", secret, false)
	if err == nil {
		t.Fatal("expected malformed private key error")
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("malformed secret disclosed in error: %v", err)
	}
	if strings.Contains(fmt.Sprintf("%#v", err), secret) {
		t.Fatalf("malformed secret disclosed in formatted error: %#v", err)
	}
}
