package testutil

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// nonDryRunCancelAll matches CancelAll(..., dryRun=false, requestID).
// Avoid matching unrelated calls that merely end with ", false, nil)" such as
// ListOpen(..., includeAttachedRisk, includeAttachedRiskState, triggerID).
var nonDryRunCancelAll = regexp.MustCompile(`\.CancelAll\([^;]*,\s*false,\s*nil\)`)

func TestNonDryRunCancelAllTestsRequireDedicatedAccountGate(t *testing.T) {
	integrationRoot := filepath.Join("..", "..", "tests", "integration")
	entries, err := os.ReadDir(integrationRoot)
	if err != nil {
		t.Fatal(err)
	}
	var unguarded []string
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		path := filepath.Join(integrationRoot, entry.Name())
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		source := string(raw)
		if nonDryRunCancelAll.MatchString(source) &&
			!strings.Contains(source, "RequireAccountWideCleanup") {
			unguarded = append(unguarded, entry.Name())
		}
	}
	if len(unguarded) != 0 {
		t.Fatalf("unguarded non-dry-run CancelAll tests: %v", unguarded)
	}
}
