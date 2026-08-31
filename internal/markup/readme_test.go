package markup

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"
)

var markupDocs = []string{"../../MARKUP.md", "../../MARKUP.zh-CN.md"}

// A list of supported properties in a document is a copy of the switch that
// supports them, and the copy is the one nobody remembers. This reads the
// switch so the Markup references cannot drift from the implementation.
//
// The promise this package makes is that a declaration is either applied or
// reported. A property implemented and not written down breaks the other half
// of it: an author has no way to find out it is there.
func TestMarkupDocsNameEverySupportedProperty(t *testing.T) {
	implemented := supportedProperties(t)
	if len(implemented) < 30 {
		t.Fatalf("found %d properties, which cannot be right", len(implemented))
	}
	for _, path := range markupDocs {
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

// inherit, initial, unset and revert are accepted for any property, so every
// property has to be handled by the two switches that carry them out. The
// switches were a short list of the ones somebody had needed, and a property
// that was not on it did nothing and said nothing — "padding-top: initial"
// left the padding where it was.
//
// This reads the same switch the property test reads, so the reset behavior
// and the documented property list cannot drift.
func TestInheritAndInitialCoverEveryImplementedProperty(t *testing.T) {
	implemented := supportedProperties(t)
	for _, function := range []string{"inheritOne", "reset"} {
		handled := propertiesHandledBy(t, function)
		for _, property := range implemented {
			if !handled[property] {
				t.Errorf("%s does not handle %s, so inherit/initial on it would do nothing",
					function, property)
			}
		}
		// The other direction as well: a case for a property that no longer
		// exists is a case nobody will ever reach.
		for property := range handled {
			if !slices.Contains(implemented, property) {
				t.Errorf("%s handles %s, which apply does not implement", function, property)
			}
		}
	}
}

// propertiesHandledBy collects the case names of the switch inside one
// function, which for these two is the whole of what they do.
func propertiesHandledBy(t *testing.T, name string) map[string]bool {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), "style.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	handled := map[string]bool{}
	for _, declared := range file.Decls {
		function, ok := declared.(*ast.FuncDecl)
		if !ok || function.Name.Name != name {
			continue
		}
		ast.Inspect(function, func(node ast.Node) bool {
			clause, ok := node.(*ast.CaseClause)
			if !ok {
				return true
			}
			for _, expression := range clause.List {
				literal, ok := expression.(*ast.BasicLit)
				if !ok || literal.Kind != token.STRING {
					continue
				}
				property, err := strconv.Unquote(literal.Value)
				if err != nil {
					t.Fatal(err)
				}
				handled[property] = true
			}
			return true
		})
	}
	if len(handled) == 0 {
		t.Fatalf("no switch found in %s", name)
	}
	return handled
}
