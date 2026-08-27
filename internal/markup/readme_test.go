package markup

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"sort"
	"strconv"
	"strings"
	"testing"
)

var readmes = []string{"../../README.md", "../../README.zh-CN.md"}

// A list of supported properties in a document is a copy of the switch that
// supports them, and the copy is the one nobody remembers. The schema
// reference drifted from decode.go exactly this way, so this reads the switch.
//
// The promise this package makes is that a declaration is either applied or
// reported. A property implemented and not written down breaks the other half
// of it: an author has no way to find out it is there.
func TestBothReadmesNameEverySupportedProperty(t *testing.T) {
	implemented := supportedProperties(t)
	if len(implemented) < 30 {
		t.Fatalf("found %d properties, which cannot be right", len(implemented))
	}
	for _, path := range readmes {
		source, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		doc := string(source)
		for _, property := range implemented {
			// In a code span, so that a property named in a sentence about
			// something else does not read as documentation of it.
			if !strings.Contains(doc, "`"+property+"`") {
				t.Errorf("%s does not name the %s property, which style.go implements", path, property)
			}
		}
	}
}

// supportedProperties is every case of the switch whose default says a
// property is not implemented, which is the one place that decides.
func supportedProperties(t *testing.T) []string {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), "style.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	ast.Inspect(file, func(node ast.Node) bool {
		switch statement := node.(type) {
		case *ast.SwitchStmt:
			if !reportsAnUnimplementedProperty(statement) {
				return true
			}
			for _, clause := range statement.Body.List {
				for _, expression := range clause.(*ast.CaseClause).List {
					literal, ok := expression.(*ast.BasicLit)
					if !ok || literal.Kind != token.STRING {
						continue
					}
					name, err := strconv.Unquote(literal.Value)
					if err != nil {
						t.Fatal(err)
					}
					names = append(names, name)
				}
			}
		}
		return true
	})
	sort.Strings(names)
	return names
}

// reportsAnUnimplementedProperty picks the dispatch out of the several
// switches on a property name by what its default clause says.
func reportsAnUnimplementedProperty(statement *ast.SwitchStmt) bool {
	for _, clause := range statement.Body.List {
		body := clause.(*ast.CaseClause)
		if body.List != nil {
			continue
		}
		found := false
		ast.Inspect(body, func(node ast.Node) bool {
			if literal, ok := node.(*ast.BasicLit); ok && literal.Kind == token.STRING &&
				strings.Contains(literal.Value, "is not a property this renderer implements") {
				found = true
			}
			return true
		})
		return found
	}
	return false
}
