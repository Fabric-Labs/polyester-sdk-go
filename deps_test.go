package polyester

import (
	"os/exec"
	"strings"
	"testing"
)

const (
	modulePath  = "github.com/Fabric-Labs/polyester-sdk-go"
	goEthereum  = "github.com/ethereum/go-ethereum"
	chainPkg    = modulePath + "/chain"
	testsPrefix = modulePath + "/tests/"
)

// TestOnlyChainLinksGoEthereum keeps go-ethereum (and the gnark-crypto and KZG
// code it pulls in) out of every binary that does not use the on-chain helpers.
// Only package chain may import it; the root client, environment and every
// other package must not reach it, directly or transitively.
func TestOnlyChainLinksGoEthereum(t *testing.T) {
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Fatalf("go toolchain not on PATH: %v", err)
	}
	cmd := exec.Command(goBin, "list", "-f", "{{.ImportPath}}{{range .Deps}} {{.}}{{end}}", "./...")
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			t.Fatalf("go list: %v\n%s", err, exitErr.Stderr)
		}
		t.Fatalf("go list: %v", err)
	}

	checked := map[string]bool{}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		pkg := fields[0]
		if pkg == chainPkg || strings.HasPrefix(pkg, chainPkg+"/") || strings.HasPrefix(pkg, testsPrefix) {
			continue
		}
		checked[pkg] = true
		for _, dep := range fields[1:] {
			if dep == goEthereum || strings.HasPrefix(dep, goEthereum+"/") {
				t.Errorf("%s links %s; only package chain may import go-ethereum", pkg, dep)
				break
			}
			if dep == chainPkg {
				t.Errorf("%s imports %s, which links go-ethereum", pkg, chainPkg)
				break
			}
		}
	}
	for _, pkg := range []string{modulePath, modulePath + "/environment"} {
		if !checked[pkg] {
			t.Errorf("go list did not report %s", pkg)
		}
	}
}
