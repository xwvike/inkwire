package scene

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// A node compose can draw and the schema cannot ask for is a node nobody can
// use, and that is not a hypothetical.
//
// Grid, quarter turns, shape clipping and anchored boxes were all added to
// compose and none of them ever reached this package. Nothing failed, because
// nothing was looking: the tests decoded documents that were written against
// the schema as it stood, so a gap in the schema was invisible by construction.
// They were only found by counting the two lists by hand.
//
// So this counts them. compose is the drawing model and the schema is the only
// way in, which makes the rule simple enough to check: everything compose
// declares, this package must be able to build.
func TestTheSchemaCanReachEveryNode(t *testing.T) {
	built := map[string]bool{}
	for _, node := range constructedNodes(t, ".") {
		built[node] = true
	}
	for _, node := range declaredNodes(t, "../compose") {
		if !built[node] {
			t.Errorf("compose.%s is a node and no document can ask for one; "+
				"give it a type in decodeNode or take it out of compose", node)
		}
	}
}

// The same gap one level down, which is where it hid the second time.
//
// The test above counts nodes, so it had nothing to say about compose.Calc: a
// length that is a share of the container and a number of pixels added
// together, written and commented and reachable from nothing, because
// lengthJSON only ever read a bare number or a percentage. A node that cannot
// be asked for and a length that cannot be asked for are the same bug.
func TestTheSchemaCanReachEveryKindOfLength(t *testing.T) {
	called := map[string]bool{}
	for _, name := range calledFunctions(t, ".") {
		called[name] = true
	}
	for _, name := range lengthConstructors(t, "../compose") {
		// Auto is the zero value, which is what a document says by leaving the
		// field out. There is nothing for this package to call.
		if name == "Auto" {
			continue
		}
		if !called[name] {
			t.Errorf("compose.%s builds a Length and no document can ask for one; "+
				"give it a spelling in lengthJSON or take it out of compose", name)
		}
	}
}

// lengthConstructors reports the exported package-level functions in compose
// that hand back a Length.
func lengthConstructors(t *testing.T, dir string) []string {
	t.Helper()
	var names []string
	for _, file := range goFiles(t, dir) {
		parsed, err := parser.ParseFile(token.NewFileSet(), file, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", file, err)
		}
		for _, declaration := range parsed.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Recv != nil || !function.Name.IsExported() {
				continue
			}
			results := function.Type.Results
			if results == nil || len(results.List) != 1 {
				continue
			}
			if ident, ok := results.List[0].Type.(*ast.Ident); ok && ident.Name == "Length" {
				names = append(names, function.Name.Name)
			}
		}
	}
	sort.Strings(names)
	return names
}

// calledFunctions reports the names this package calls as compose.Name(...).
func calledFunctions(t *testing.T, dir string) []string {
	t.Helper()
	found := map[string]bool{}
	for _, file := range goFiles(t, dir) {
		parsed, err := parser.ParseFile(token.NewFileSet(), file, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", file, err)
		}
		ast.Inspect(parsed, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			selector, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			if pkg, ok := selector.X.(*ast.Ident); ok && pkg.Name == "compose" {
				found[selector.Sel.Name] = true
			}
			return true
		})
	}
	var names []string
	for name := range found {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// constructedNodes reports the compose node types this package builds, found by
// looking for composite literals of the form compose.Name{}. Supporting types
// are filtered out by checking each name against the nodes compose declares.
func constructedNodes(t *testing.T, dir string) []string {
	t.Helper()
	declared := map[string]bool{}
	for _, node := range declaredNodes(t, "../compose") {
		declared[node] = true
	}
	found := map[string]bool{}
	for _, file := range goFiles(t, dir) {
		parsed, err := parser.ParseFile(token.NewFileSet(), file, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", file, err)
		}
		ast.Inspect(parsed, func(n ast.Node) bool {
			literal, ok := n.(*ast.CompositeLit)
			if !ok {
				return true
			}
			selector, ok := literal.Type.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			if pkg, ok := selector.X.(*ast.Ident); !ok || pkg.Name != "compose" {
				return true
			}
			if declared[selector.Sel.Name] {
				found[selector.Sel.Name] = true
			}
			return true
		})
	}
	var names []string
	for name := range found {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// declaredNodes reports the types in a package that implement compose.Node,
// found by looking for a composeNode method on them.
func declaredNodes(t *testing.T, dir string) []string {
	t.Helper()
	var names []string
	for _, file := range goFiles(t, dir) {
		parsed, err := parser.ParseFile(token.NewFileSet(), file, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", file, err)
		}
		for _, declaration := range parsed.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Name.Name != "composeNode" || function.Recv == nil {
				continue
			}
			if name := receiverName(function.Recv); name != "" {
				names = append(names, name)
			}
		}
	}
	sort.Strings(names)
	return names
}

func receiverName(fields *ast.FieldList) string {
	if len(fields.List) == 0 {
		return ""
	}
	switch receiver := fields.List[0].Type.(type) {
	case *ast.Ident:
		return receiver.Name
	case *ast.StarExpr:
		if ident, ok := receiver.X.(*ast.Ident); ok {
			return ident.Name
		}
	}
	return ""
}

func goFiles(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var files []string
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		files = append(files, filepath.Join(dir, name))
	}
	return files
}
