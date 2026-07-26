package testutil

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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
		if strings.Contains(source, ".CancelAll(") &&
			strings.Contains(source, "false, nil)") &&
			!strings.Contains(source, "RequireAccountWideCleanup") {
			unguarded = append(unguarded, entry.Name())
		}
	}
	if len(unguarded) != 0 {
		t.Fatalf("unguarded non-dry-run CancelAll tests: %v", unguarded)
	}
}
