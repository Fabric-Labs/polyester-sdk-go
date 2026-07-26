package testutil

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestA7StrictLiveMissingCredsFailsRequireLiveClient(t *testing.T) {
	if os.Getenv("POLYESTER_A7_MISSING_CREDS_PROBE") == "1" {
		RequireLiveClient(t)
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=^TestA7StrictLiveMissingCredsFailsRequireLiveClient$", "-test.v")
	cmd.Env = filterEnv(os.Environ(),
		"POLYESTER_API_KEY_ID",
		"POLYESTER_API_PRIVATE_KEY",
		"POLYESTER_TEST_STRICT_LIVE",
		"POLYESTER_TEST_DISABLE_DOTENV",
	)
	cmd.Env = append(cmd.Env,
		"POLYESTER_A7_MISSING_CREDS_PROBE=1",
		"POLYESTER_TEST_STRICT_LIVE=1",
		"POLYESTER_TEST_DISABLE_DOTENV=1",
		"POLYESTER_API_KEY_ID=",
		"POLYESTER_API_PRIVATE_KEY=",
	)
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("STRICT_LIVE + missing creds must fail:\n%s", out)
	}
	combined := string(out)
	if !strings.Contains(strings.ToLower(combined), "strict live") &&
		!strings.Contains(combined, "soft skip") {
		t.Fatalf("want strict-live failure message, got:\n%s", combined)
	}
}

func TestA7StrictLiveMalformedCredsFails(t *testing.T) {
	if os.Getenv("POLYESTER_A7_MALFORMED_CREDS_PROBE") == "1" {
		RequireLiveClient(t)
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=^TestA7StrictLiveMalformedCredsFails$", "-test.v")
	cmd.Env = filterEnv(os.Environ(),
		"POLYESTER_API_KEY_ID",
		"POLYESTER_API_PRIVATE_KEY",
		"POLYESTER_TEST_STRICT_LIVE",
		"POLYESTER_TEST_DISABLE_DOTENV",
	)
	cmd.Env = append(cmd.Env,
		"POLYESTER_A7_MALFORMED_CREDS_PROBE=1",
		"POLYESTER_TEST_STRICT_LIVE=1",
		"POLYESTER_TEST_DISABLE_DOTENV=1",
		"POLYESTER_API_KEY_ID=ak_test",
		"POLYESTER_API_PRIVATE_KEY=not-valid-hex-key",
	)
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("STRICT_LIVE + malformed key must fail:\n%s", out)
	}
	combined := strings.ToLower(string(out))
	if !strings.Contains(combined, "hex") &&
		!strings.Contains(combined, "private key") &&
		!strings.Contains(combined, "auth") &&
		!strings.Contains(combined, "fatal") {
		t.Fatalf("want malformed-key failure, got:\n%s", out)
	}
}

func filterEnv(env []string, dropKeys ...string) []string {
	drop := make(map[string]struct{}, len(dropKeys))
	for _, k := range dropKeys {
		drop[k] = struct{}{}
	}
	out := make([]string, 0, len(env))
	for _, e := range env {
		key, _, _ := strings.Cut(e, "=")
		if _, ok := drop[key]; ok {
			continue
		}
		out = append(out, e)
	}
	return out
}
