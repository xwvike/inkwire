package markup

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/xwvike/inkwire/internal/compose"
	"github.com/xwvike/inkwire/internal/scene"
)

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
