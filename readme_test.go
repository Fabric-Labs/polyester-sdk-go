package polyester

import (
	"bytes"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// TestREADMEGoExamplesCompile compiles every ```go block in README.md
// against this module, so an example that no longer matches the API fails
// the build. Nothing is run and nothing is fetched: the blocks are built as
// packages of this module through an -overlay, with the same dependencies as
// the module's own build.
//
// A block that starts with a package clause is built as its own program.
// Any other block is a fragment: its leading import lines are kept, the
// packages it names (models, services, fmt, ...) are imported for it, and its
// statements become the body of a function in a package that declares the
// names the README's prose assumes (ctx, client, err, symbol, orderID, ...;
// see readmePrelude). Names a fragment declares at its top level are marked
// used, so a fragment may show a result without printing it. Errors name the
// README line.
func TestREADMEGoExamplesCompile(t *testing.T) {
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Fatalf("go command not found: %v", err)
	}
	repoRoot, err := filepath.Abs(".")
	if err != nil {
		t.Fatal(err)
	}
	readme := filepath.Join(repoRoot, "README.md")
	blocks, err := readmeGoBlocks(readme)
	if err != nil {
		t.Fatal(err)
	}
	if len(blocks) == 0 {
		t.Fatal("README.md has no ```go blocks")
	}

	// The packages exist only in the overlay, under a directory the module
	// does not have.
	const virtualDir = "internal/readmeexamples"
	if _, err := os.Stat(filepath.Join(repoRoot, virtualDir)); err == nil {
		t.Fatalf("%s exists; the README check needs that directory free", virtualDir)
	}
	tmp := t.TempDir()
	replace := map[string]string{}
	pkgs := map[string]bool{}
	add := func(pkg, name, src string) {
		backing := filepath.Join(tmp, pkg, name)
		writeFile(t, backing, src)
		replace[filepath.Join(repoRoot, virtualDir, pkg, name)] = backing
		pkgs["./"+virtualDir+"/"+pkg] = true
	}
	add("fragments", "prelude.go", readmePrelude)

	var parseErrs []string
	for _, b := range blocks {
		if strings.HasPrefix(strings.TrimSpace(b.code), "package ") {
			add(fmt.Sprintf("program_%d", b.line), "main.go", fmt.Sprintf("//line %s:%d\n%s", readme, b.line, b.code))
			continue
		}
		src, err := wrapReadmeFragment(b, readme)
		if err != nil {
			parseErrs = append(parseErrs, err.Error())
			continue
		}
		add("fragments", fmt.Sprintf("block_%d.go", b.line), src)
	}
	overlay, err := json.Marshal(map[string]any{"Replace": replace})
	if err != nil {
		t.Fatal(err)
	}
	overlayPath := filepath.Join(tmp, "overlay.json")
	writeFile(t, overlayPath, string(overlay))

	args := []string{"build", "-overlay=" + overlayPath, "-gcflags=-e"}
	for pkg := range pkgs {
		args = append(args, pkg)
	}
	sort.Strings(args[3:])
	cmd := exec.Command(goBin, args...)
	cmd.Dir = repoRoot
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOFLAGS=-mod=readonly", "GOPROXY=off")
	out, err := cmd.CombinedOutput()
	if err != nil || len(parseErrs) > 0 {
		t.Fatalf("README Go examples do not compile against this module (%d blocks):\n%s%s",
			len(blocks), strings.Join(append(parseErrs, ""), "\n"),
			strings.ReplaceAll(string(out), repoRoot+string(filepath.Separator), ""))
	}
}

// readmePrelude declares what README fragments may assume without showing:
// a client and context, and the values the surrounding prose introduces.
const readmePrelude = `package fragments

import (
	"context"
	"math/big"

	polyester "github.com/Fabric-Labs/polyester-sdk-go"
	"github.com/Fabric-Labs/polyester-sdk-go/models"
	"github.com/Fabric-Labs/polyester-sdk-go/orderbook"
)

var (
	ctx    context.Context
	client *polyester.Client
	err    error

	symbol       = "BTC-USDT"
	tif          = "gtc"
	price        = models.PriceFromDecimal("60000")
	orderID      string
	created      models.OrderMutationResult
	items        []models.BatchReplaceItem
	requestID    string
	subAccountID string
	txHash       string
	ob           *orderbook.Subscription

	ownerPrivateKeyHex string
	uAssetID           uint32
	quantityScaled     *big.Int
)
`

// readmeImports maps the package names README fragments use to their paths.
// "errors" is this SDK's errors package, as in the README's prose.
var readmeImports = map[string]string{
	"context":   "context",
	"fmt":       "fmt",
	"log":       "log",
	"os":        "os",
	"strings":   "strings",
	"time":      "time",
	"big":       "math/big",
	"polyester": "github.com/Fabric-Labs/polyester-sdk-go",
	"auth":      "github.com/Fabric-Labs/polyester-sdk-go/auth",
	"chain":     "github.com/Fabric-Labs/polyester-sdk-go/chain",
	"codecs":    "github.com/Fabric-Labs/polyester-sdk-go/codecs",
	"errors":    "github.com/Fabric-Labs/polyester-sdk-go/errors",
	"models":    "github.com/Fabric-Labs/polyester-sdk-go/models",
	"orderbook": "github.com/Fabric-Labs/polyester-sdk-go/orderbook",
	"realtime":  "github.com/Fabric-Labs/polyester-sdk-go/realtime",
	"services":  "github.com/Fabric-Labs/polyester-sdk-go/services",
}

type readmeBlock struct {
	line int // README line of the block's first code line
	code string
}

func readmeGoBlocks(path string) ([]readmeBlock, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var (
		blocks []readmeBlock
		cur    *readmeBlock
		buf    strings.Builder
	)
	for i, line := range strings.Split(string(data), "\n") {
		n := i + 1
		line = strings.TrimSuffix(line, "\r")
		switch {
		case cur == nil && line == "```go":
			cur = &readmeBlock{line: n + 1}
			buf.Reset()
		case cur != nil && line == "```":
			cur.code = buf.String()
			blocks = append(blocks, *cur)
			cur = nil
		case cur != nil:
			buf.WriteString(line)
			buf.WriteByte('\n')
		}
	}
	if cur != nil {
		return nil, fmt.Errorf("README.md:%d: unterminated ```go block", cur.line-1)
	}
	return blocks, nil
}

// wrapReadmeFragment turns a fragment into a file of package fragments.
// Positions name the README at readme.
func wrapReadmeFragment(b readmeBlock, readme string) (string, error) {
	lines := strings.Split(strings.TrimSuffix(b.code, "\n"), "\n")
	var specs []string
	i := 0
	for i < len(lines) {
		l := strings.TrimSpace(lines[i])
		switch {
		case l == "":
			i++
			continue
		case l == "import (":
			i++
			for i < len(lines) && strings.TrimSpace(lines[i]) != ")" {
				if s := strings.TrimSpace(lines[i]); s != "" {
					specs = append(specs, s)
				}
				i++
			}
			i++
			continue
		case strings.HasPrefix(l, "import "):
			specs = append(specs, strings.TrimSpace(strings.TrimPrefix(l, "import ")))
			i++
			continue
		}
		break
	}
	body := strings.Join(lines[i:], "\n")
	bodyLine := b.line + i
	fn := fmt.Sprintf("readmeBlock%d", b.line)

	// Parse once without imports to find the packages and top-level names
	// the fragment uses.
	probe := fmt.Sprintf("package fragments\n\nfunc %s() error {\n{\n//line README.md:%d\n%s\n}\nreturn nil\n}\n", fn, bodyLine, body)
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "", probe, parser.ParseComments)
	if err != nil {
		return "", fmt.Errorf("README Go block does not parse: %w", err)
	}
	declared := map[string]bool{}
	ast.Inspect(file, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.AssignStmt:
			if n.Tok == token.DEFINE {
				for _, lhs := range n.Lhs {
					if id, ok := lhs.(*ast.Ident); ok {
						declared[id.Name] = true
					}
				}
			}
		case *ast.ValueSpec:
			for _, id := range n.Names {
				declared[id.Name] = true
			}
		}
		return true
	})
	explicit := map[string]bool{}
	for _, s := range specs {
		explicit[importName(s)] = true
	}
	used := map[string]bool{}
	ast.Inspect(file, func(n ast.Node) bool {
		if sel, ok := n.(*ast.SelectorExpr); ok {
			if id, ok := sel.X.(*ast.Ident); ok && !declared[id.Name] && !explicit[id.Name] {
				if _, known := readmeImports[id.Name]; known {
					used[id.Name] = true
				}
			}
		}
		return true
	})
	for name := range used {
		path := readmeImports[name]
		spec := strconv.Quote(path)
		if filepath.Base(path) != name {
			spec = name + " " + spec
		}
		specs = append(specs, spec)
	}
	sort.Strings(specs)

	// Mark the fragment's top-level names used.
	inner := file.Decls[0].(*ast.FuncDecl).Body.List[0].(*ast.BlockStmt)
	var uses []string
	for _, st := range inner.List {
		var names []*ast.Ident
		switch st := st.(type) {
		case *ast.AssignStmt:
			if st.Tok == token.DEFINE {
				for _, lhs := range st.Lhs {
					if id, ok := lhs.(*ast.Ident); ok {
						names = append(names, id)
					}
				}
			}
		case *ast.DeclStmt:
			if gd, ok := st.Decl.(*ast.GenDecl); ok && gd.Tok == token.VAR {
				for _, sp := range gd.Specs {
					names = append(names, sp.(*ast.ValueSpec).Names...)
				}
			}
		}
		for _, id := range names {
			if id.Name != "_" {
				uses = append(uses, "_ = "+id.Name)
			}
		}
	}

	var out bytes.Buffer
	out.WriteString("package fragments\n\n")
	if len(specs) > 0 {
		out.WriteString("import (\n")
		for _, s := range specs {
			out.WriteString("\t" + s + "\n")
		}
		out.WriteString(")\n\n")
	}
	fmt.Fprintf(&out, "func %s() error {\n{\n//line %s:%d\n%s\n//line %s:1\n%s\n}\nreturn nil\n}\n",
		fn, readme, bodyLine, body, fn+"_generated", strings.Join(uses, "\n"))
	return out.String(), nil
}

// importName is the name an import spec binds: its alias, or the last path
// element.
func importName(spec string) string {
	if fields := strings.Fields(spec); len(fields) == 2 {
		return fields[0]
	}
	path, err := strconv.Unquote(spec)
	if err != nil {
		return spec
	}
	return filepath.Base(path)
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
