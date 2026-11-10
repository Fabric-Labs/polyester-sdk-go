package testutil

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// writeMethod names the client methods that change server state. A live test
// that calls one must opt in to writes first.
var writeMethod = regexp.MustCompile(`^(Create|Update|Delete|Copy|Modify|Replace|Cancel|Batch|Withdraw|Transfer|Pause|Resume|Start|Stop|Revoke|Claim|Rotate|Set)`)

// writeGates are the opt-ins a live test calls before writing.
var writeGates = map[string]bool{
	"RequireMutation":           true,
	"RequireFunded":             true,
	"RequireAccountWideCleanup": true,
}


// TestLiveWriteTestsRequireAWriteGate keeps every live test that writes
// behind POLYESTER_TEST_MUTATION (or a stricter gate), so a run with read-only
// credentials, or without the opt-in, never writes.
func TestLiveWriteTestsRequireAWriteGate(t *testing.T) {
	files, err := filepath.Glob(filepath.Join("..", "..", "tests", "integration", "*_test.go"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no live test files: %v", err)
	}
	fset := token.NewFileSet()
	var ungated []string
	checked := 0
	for _, file := range files {
		parsed, err := parser.ParseFile(fset, file, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		helpers := map[string]*ast.FuncDecl{}
		for _, decl := range parsed.Decls {
			if fn, ok := decl.(*ast.FuncDecl); ok && fn.Recv == nil {
				helpers[fn.Name.Name] = fn
			}
		}
		for name, fn := range helpers {
			if !strings.HasPrefix(name, "Test") {
				continue
			}
			checked++
			writes, gated := scanLiveTest(fn, helpers, map[string]bool{})
			if len(writes) > 0 && !gated {
				ungated = append(ungated, filepath.Base(file)+": "+name+" calls "+strings.Join(writes, ", "))
			}
		}
	}
	if checked < 50 {
		t.Fatalf("checked only %d live tests; the scan is not reading them", checked)
	}
	sort.Strings(ungated)
	if len(ungated) > 0 {
		t.Fatalf("live tests write without testutil.RequireMutation:\n  %s", strings.Join(ungated, "\n  "))
	}
}

// scanLiveTest returns the service writes fn makes, directly or through a
// helper in the same file, and whether it calls a write gate first.
func scanLiveTest(fn *ast.FuncDecl, helpers map[string]*ast.FuncDecl, seen map[string]bool) (writes []string, gated bool) {
	if fn.Body == nil || seen[fn.Name.Name] {
		return nil, false
	}
	seen[fn.Name.Name] = true
	ast.Inspect(fn.Body, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		switch callee := call.Fun.(type) {
		case *ast.SelectorExpr:
			if pkg, ok := callee.X.(*ast.Ident); ok && pkg.Name == "testutil" && writeGates[callee.Sel.Name] {
				gated = true
			}
			// client.<Service>.<Method>(...)
			if service, ok := callee.X.(*ast.SelectorExpr); ok && writeMethod.MatchString(callee.Sel.Name) && !isDryRun(call) {
				writes = append(writes, service.Sel.Name+"."+callee.Sel.Name)
			}
		case *ast.Ident:
			if helper, ok := helpers[callee.Name]; ok {
				w, g := scanLiveTest(helper, helpers, seen)
				writes = append(writes, w...)
				gated = gated || g
			}
		}
		return true
	})
	return writes, gated
}

// isDryRun reports a cancel-all that only counts what it would cancel: its
// dryRun argument or its DryRun field is the literal true.
func isDryRun(call *ast.CallExpr) bool {
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || !strings.HasPrefix(selector.Sel.Name, "CancelAll") {
		return false
	}
	isTrue := func(expr ast.Expr) bool {
		ident, ok := expr.(*ast.Ident)
		return ok && ident.Name == "true"
	}
	if selector.Sel.Name == "CancelAll" && len(call.Args) > 5 && isTrue(call.Args[5]) {
		return true
	}
	for _, arg := range call.Args {
		if literal, ok := arg.(*ast.CompositeLit); ok {
			for _, element := range literal.Elts {
				field, ok := element.(*ast.KeyValueExpr)
				if !ok {
					continue
				}
				if key, ok := field.Key.(*ast.Ident); ok && key.Name == "DryRun" && isTrue(field.Value) {
					return true
				}
			}
		}
	}
	return false
}
