package testutil

import (
	"os"
	"os/exec"
	"testing"
)

func TestSoftSkipUnderStrictLiveMustFatal(t *testing.T) {
	if os.Getenv("POLYESTER_SOFT_SKIP_PROBE") == "1" {
		SoftSkip(t, "probe soft skip under STRICT_LIVE")
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=^TestSoftSkipUnderStrictLiveMustFatal$", "-test.v")
	cmd.Env = append(os.Environ(),
		"POLYESTER_SOFT_SKIP_PROBE=1",
		"POLYESTER_TEST_STRICT_LIVE=1",
	)
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected Fatal under STRICT_LIVE=1, process succeeded:\n%s", out)
	}
	exitErr, ok := err.(*exec.ExitError)
	if !ok || exitErr.Success() {
		t.Fatalf("want non-zero exit, got %v\n%s", err, out)
	}
}

func TestSoftSkipWithoutStrictLiveSkips(t *testing.T) {
	if os.Getenv("POLYESTER_SOFT_SKIP_PROBE_SKIP") == "1" {
		SoftSkip(t, "probe soft skip without STRICT_LIVE")
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=^TestSoftSkipWithoutStrictLiveSkips$", "-test.v")
	cmd.Env = append(os.Environ(),
		"POLYESTER_SOFT_SKIP_PROBE_SKIP=1",
		"POLYESTER_TEST_STRICT_LIVE=",
	)
	// Clear STRICT_LIVE if parent had it set.
	filtered := make([]string, 0, len(cmd.Env))
	for _, e := range cmd.Env {
		if len(e) >= len("POLYESTER_TEST_STRICT_LIVE=") && e[:len("POLYESTER_TEST_STRICT_LIVE=")] == "POLYESTER_TEST_STRICT_LIVE=" {
			if e == "POLYESTER_TEST_STRICT_LIVE=" || e == "POLYESTER_TEST_STRICT_LIVE=0" || e == "POLYESTER_TEST_STRICT_LIVE=false" {
				filtered = append(filtered, e)
			}
			continue
		}
		filtered = append(filtered, e)
	}
	cmd.Env = append(filtered, "POLYESTER_TEST_STRICT_LIVE=0")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("expected skip (pass), got %v\n%s", err, out)
	}
}
