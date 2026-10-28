package display

import (
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"path/filepath"
	"strings"
	"testing"
)

// Go lets a compiler fuse a float multiplication into the addition after it,
// rounding once instead of twice, and on arm64 it does. WebAssembly and amd64
// do not. So a*b + c can come out a bit different on the machine that pushes
// to the panel than in the editor's preview, and a bit is enough: an image's
// measured cut moved from 103 to 104 that way, and the pixels between moved
// with it. A conversion, float64(a*b), rounds the product on its own, which
// the spec says no fusion may undo, so every product in the drawing code that
// can reach an add is written that way.
//
// This finds the ones that are not: a product added or subtracted, or put
// where it can be added later — stored, returned, or handed to a function
// that may be inlined into an add.
func TestNoProductCanBeFusedIntoAnAdd(t *testing.T) {
	for _, dir := range []string{".", "../compose", "../markup", "../scene", "../panel"} {
		for _, site := range fusibleProducts(t, dir) {
			t.Errorf("%s: a float product that can be fused into an add; write it as float64(...) so it rounds the same everywhere", site)
		}
	}
}

func fusibleProducts(t *testing.T, dir string) []string {
	t.Helper()
	fileSet := token.NewFileSet()
	var files []*ast.File
	entries, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range entries {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fileSet, name, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		files = append(files, file)
	}
	if len(files) == 0 {
		t.Fatalf("no source in %s", dir)
	}
	info := &types.Info{Types: map[ast.Expr]types.TypeAndValue{}}
	config := types.Config{Importer: importer.ForCompiler(fileSet, "source", nil)}
	if _, err := config.Check(files[0].Name.Name, fileSet, files, info); err != nil {
		t.Fatalf("type-checking %s: %v", dir, err)
	}
	isFloat := func(expression ast.Expr) bool {
		value, ok := info.Types[expression]
		if !ok || value.Value != nil {
			return false
		}
		basic, ok := value.Type.Underlying().(*types.Basic)
		return ok && basic.Info()&types.IsFloat != 0
	}
	var found []string
	check := func(expression ast.Expr) {
		inner := expression
		for {
			parenthesised, ok := inner.(*ast.ParenExpr)
			if !ok {
				break
			}
			inner = parenthesised.X
		}
		if product, ok := inner.(*ast.BinaryExpr); ok && product.Op == token.MUL && isFloat(product) {
			found = append(found, fileSet.Position(product.Pos()).String())
		}
	}
	for _, file := range files {
		ast.Inspect(file, func(node ast.Node) bool {
			switch node := node.(type) {
			case *ast.BinaryExpr:
				if (node.Op == token.ADD || node.Op == token.SUB) && isFloat(node) {
					check(node.X)
					check(node.Y)
				}
			case *ast.AssignStmt:
				for _, value := range node.Rhs {
					check(value)
				}
			case *ast.ValueSpec:
				for _, value := range node.Values {
					check(value)
				}
			case *ast.ReturnStmt:
				for _, value := range node.Results {
					check(value)
				}
			case *ast.CompositeLit:
				for _, element := range node.Elts {
					if pair, ok := element.(*ast.KeyValueExpr); ok {
						element = pair.Value
					}
					check(element)
				}
			case *ast.CallExpr:
				if function, ok := info.Types[node.Fun]; ok && function.IsType() {
					return true // a conversion is the fix
				}
				for _, argument := range node.Args {
					check(argument)
				}
			}
			return true
		})
	}
	return found
}
