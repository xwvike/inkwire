package markup

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/xwvike/inkwire/internal/compose"
	"github.com/xwvike/inkwire/internal/scene"
)

// Everything this package writes has to be a field the schema reads. The two
// lists are in different packages and neither imports the other, so the only
// thing keeping them together is this.
//
// The decoder refuses a field it does not know, which catches a wrong name the
// first time a page using it is rendered. That is late: a field this package
// writes on a node nobody has an example of would sit here unrendered and
// unrefused until somebody wrote the page. This checks the names without
// needing the page.
func TestEveryFieldThisPackageWritesIsAFieldTheSchemaReads(t *testing.T) {
	read := schemaFields(t)
	if len(read) < 40 {
		t.Fatalf("found %d field names in the schema, which cannot be right", len(read))
	}
	written := map[string]string{}
	for _, emitted := range []any{document{}, emitted{}, size{}, insets{}, stroke{}, run{}, command{},
		overrides{}, point{}, shape{}, rect{}, placed{}, pathValue{}, origin{}, layoutChild{}, gridChild{}, anchor{}} {
		structure := reflect.TypeOf(emitted)
		for index := range structure.NumField() {
			tag, ok := structure.Field(index).Tag.Lookup("json")
			if !ok {
				continue
			}
			name, _, _ := strings.Cut(tag, ",")
			if name == "" || name == "-" {
				continue
			}
			written[name] = structure.Name()
		}
	}
	for name, where := range written {
		if !read[name] {
			t.Errorf("markup writes %q on %s and no scene document has a field by that name", name, where)
		}
	}
}

// schemaFields is every json tag in the decoder, which is the whole of what a
// document may say.
func schemaFields(t *testing.T) map[string]bool {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), "../scene/decode.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	ast.Inspect(file, func(n ast.Node) bool {
		field, ok := n.(*ast.Field)
		if !ok || field.Tag == nil {
			return true
		}
		tag, err := strconv.Unquote(field.Tag.Value)
		if err != nil {
			return true
		}
		value, ok := reflect.StructTag(tag).Lookup("json")
		if !ok {
			return true
		}
		name, _, _ := strings.Cut(value, ",")
		if name != "" && name != "-" {
			names[name] = true
		}
		return true
	})
	return names
}

// A length is the one value with three spellings, and the difference between
// them is not cosmetic: the decoder tries a number before a string, so 5 and
// "5px" do not both work everywhere. This writes each kind and reads it back
// through the decoder that will read it in earnest.
func TestALengthSurvivesBeingWrittenAndReadBack(t *testing.T) {
	tests := []compose.Length{
		compose.Pixels(0),
		compose.Pixels(5),
		compose.Pixels(296),
		compose.Tenths(500),
		compose.Tenths(873),
		compose.Tenths(1000),
		compose.Calc(1000, -10),
		compose.Calc(500, 4),
		compose.Calc(873, -1),
	}
	for _, want := range tests {
		written, err := json.Marshal(lengthValue(want))
		if err != nil {
			t.Fatalf("%s: %v", want, err)
		}
		// A row's basis is a length and nothing else, so a document with one
		// item reads the value back on its own.
		source := `{"version":1,"size":{"width":10,"height":10},"root":{"type":"row","children":[` +
			`{"basis":` + string(written) + `,"node":{"type":"spacer"}}]}}`
		document, err := (scene.Decoder{}).Decode(strings.NewReader(source))
		if err != nil {
			t.Errorf("%s written as %s: %v", want, written, err)
			continue
		}
		got := document.Root.(compose.Row).Children[0].Basis
		if got != want {
			t.Errorf("%s written as %s came back as %s", want, written, got)
		}
	}
}

// An unset length is left out rather than written as zero. A box stated as
// zero is not drawn and a box with no stated size is measured, so the two
// cannot be spelled the same way.
func TestAnUnsetLengthIsLeftOut(t *testing.T) {
	if value := lengthValue(compose.Length{}); value != nil {
		t.Errorf("an unset length came out as %v, which omitempty will write", value)
	}
	written, err := json.Marshal(layoutChild{Node: &emitted{Type: "spacer"}})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(written), "basis") {
		t.Errorf("a child with no stated basis wrote one: %s", written)
	}
}
