package markup

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// The scene format is written down twice: once here, as what a page compiles
// to, and once in the scene decoder, as what it accepts. Nothing ties the two
// together but this. The decoder refuses a field it does not know, so a field
// spelt differently on the two sides is an error on any page that reaches it,
// and a page that reaches it may be one no test writes.

// decoderStructs reads the scene decoder's source for the JSON fields each of
// its structs takes, with an embedded struct's fields counted as its own, as
// encoding/json counts them.
func decoderStructs(t *testing.T) (map[string][]string, []string) {
	t.Helper()
	directory := filepath.Join("..", "scene")
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	fileSet := token.NewFileSet()
	types := map[string]*ast.StructType{}
	var nodeTypes []string
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fileSet, filepath.Join(directory, name), nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(file, func(node ast.Node) bool {
			switch node := node.(type) {
			case *ast.TypeSpec:
				if structType, ok := node.Type.(*ast.StructType); ok {
					types[node.Name.Name] = structType
				}
			case *ast.FuncDecl:
				if node.Name.Name == "decodeNode" {
					nodeTypes = append(nodeTypes, caseStrings(node)...)
				}
			}
			return true
		})
	}
	fields := map[string][]string{}
	var fieldsOf func(name string) []string
	fieldsOf = func(name string) []string {
		if known, ok := fields[name]; ok {
			return known
		}
		structType, ok := types[name]
		if !ok {
			return nil
		}
		var names []string
		for _, field := range structType.Fields.List {
			if field.Tag == nil {
				if len(field.Names) == 0 {
					if embedded, ok := field.Type.(*ast.Ident); ok {
						names = append(names, fieldsOf(embedded.Name)...)
					}
				}
				continue
			}
			tag, err := strconv.Unquote(field.Tag.Value)
			if err != nil {
				t.Fatal(err)
			}
			if jsonName := jsonFieldName(reflect.StructTag(tag)); jsonName != "" {
				names = append(names, jsonName)
			}
		}
		fields[name] = names
		return names
	}
	for name := range types {
		fieldsOf(name)
	}
	if len(nodeTypes) == 0 {
		t.Fatal("found no node types in the scene decoder's decodeNode")
	}
	return fields, nodeTypes
}

// caseStrings is every string a switch in the function compares against.
func caseStrings(function *ast.FuncDecl) []string {
	var found []string
	ast.Inspect(function, func(node ast.Node) bool {
		clause, ok := node.(*ast.CaseClause)
		if !ok {
			return true
		}
		for _, expression := range clause.List {
			if literal, ok := expression.(*ast.BasicLit); ok && literal.Kind == token.STRING {
				if value, err := strconv.Unquote(literal.Value); err == nil {
					found = append(found, value)
				}
			}
		}
		return true
	})
	return found
}

func jsonFieldName(tag reflect.StructTag) string {
	name, _, _ := strings.Cut(tag.Get("json"), ",")
	if name == "-" {
		return ""
	}
	return name
}

func TestEveryFieldMarkupWritesIsOneTheSceneDecoderReads(t *testing.T) {
	decoder, _ := decoderStructs(t)
	// A node is any decoder struct with a type; markup writes every kind of
	// node with the one struct, so each of its fields has to be some node's.
	nodeFields := map[string]bool{}
	for _, names := range decoder {
		if slices.Contains(names, "type") {
			for _, name := range names {
				nodeFields[name] = true
			}
		}
	}
	seen := map[reflect.Type]bool{}
	var walk func(written reflect.Type, at string)
	walk = func(written reflect.Type, at string) {
		for written.Kind() == reflect.Pointer || written.Kind() == reflect.Slice || written.Kind() == reflect.Array || written.Kind() == reflect.Map {
			written = written.Elem()
		}
		if written.Kind() != reflect.Struct || seen[written] {
			return
		}
		seen[written] = true
		var names []string
		for index := range written.NumField() {
			field := written.Field(index)
			name := jsonFieldName(field.Tag)
			if !field.IsExported() || name == "" {
				continue
			}
			names = append(names, name)
			walk(field.Type, at+"."+name)
		}
		if written == reflect.TypeOf(emitted{}) {
			for _, name := range names {
				if !nodeFields[name] {
					t.Errorf("markup writes %q on a node (%s), and no node in the scene decoder reads it", name, at)
				}
			}
			return
		}
		// Any other struct is one the decoder reads whole, so one of its
		// structs has to take every field this one writes.
		for _, taken := range decoder {
			if containsAll(taken, names) {
				return
			}
		}
		sort.Strings(names)
		t.Errorf("markup writes %s (%s) with the fields %v, and no struct in the scene decoder reads all of them", written.Name(), at, names)
	}
	// A node's children, and the shapes and points some fields hold, are
	// written through an any, which the walk cannot see into, so the types
	// that go there are named.
	for _, written := range []any{document{}, layoutChild{}, gridChild{}, anchor{}, placed{},
		shape{}, point{}, origin{}, pathValue{}, command{}, overrides{}, rect{}} {
		walk(reflect.TypeOf(written), reflect.TypeOf(written).Name())
	}
	for _, reached := range []any{emitted{}, inlineItem{}, run{}, stroke{}, lengthInsets{}} {
		if !seen[reflect.TypeOf(reached)] {
			t.Errorf("the walk did not reach %T; the test is not looking everywhere markup writes", reached)
		}
	}
}

func containsAll(have, want []string) bool {
	for _, name := range want {
		if !slices.Contains(have, name) {
			return false
		}
	}
	return true
}

func TestEveryKindOfNodeMarkupWritesIsOneTheSceneDecoderReads(t *testing.T) {
	_, nodeTypes := decoderStructs(t)
	literal := regexp.MustCompile(`\bType:\s*"([A-Za-z]+)"`)
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	found := 0
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		source, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		for _, match := range literal.FindAllStringSubmatch(string(source), -1) {
			found++
			if !slices.Contains(nodeTypes, match[1]) {
				t.Errorf("%s writes a node of type %q, which the scene decoder does not read", name, match[1])
			}
		}
	}
	if found < 20 {
		t.Fatalf("found %d node types written; the pattern no longer matches how markup writes them", found)
	}
}
