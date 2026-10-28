package scene

import (
	"bytes"
	"encoding/json"
	"image"
	"strings"
	"testing"
	"time"

	"github.com/xwvike/inkwire/internal/compose"
)

// Compacting keeps what the document says: every string, every structure and
// every number a panel could use, with only the whitespace gone.
func TestCompactingKeepsTheDocument(t *testing.T) {
	source := `{ "a": [1, -2.5, 3e2, {"b": "x\"y\\u00e9 <&>", "c": null}, true, false, []], "d": {}, "e": "中文" }`
	got := compacted([]byte(source))
	var before, after any
	if err := json.Unmarshal([]byte(source), &before); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(got, &after); err != nil {
		t.Fatalf("%s: %v", got, err)
	}
	if want, _ := json.Marshal(before); !bytes.Equal(want, mustMarshal(t, after)) {
		t.Fatalf("compacted to %s", got)
	}
	if want := `{"a":[1,-2.5,3e2,{"b":"x\"y\\u00e9 \u003c\u0026\u003e","c":null},true,false,[]],"d":{},"e":"中文"}`; string(got) != want {
		t.Fatalf("compacted to %s, want %s", got, want)
	}
	// What is not one JSON value is left for the decoder to refuse as it was.
	for _, broken := range []string{`{"a": }`, `{"a": 1} {"b": 2}`, `[1, 2`, ``} {
		if got := compacted([]byte(broken)); string(got) != broken {
			t.Errorf("%q became %q", broken, got)
		}
	}
}

func mustMarshal(t *testing.T, value any) []byte {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}

// A number far past anything a panel can show is held at the limit, so the
// document is still drawn — and drawn quickly — instead of overflowing the
// arithmetic that lays it out.
func TestAnAbsurdNumberIsHeldAtTheLimit(t *testing.T) {
	if got := string(compacted([]byte(`[1e300, -1e300, 268435456, 1.5, -7]`))); got != `[268435456,-268435456,268435456,1.5,-7]` {
		t.Fatalf("held as %s", got)
	}
	document, err := (Decoder{}).Decode(strings.NewReader(`{"version": 1, "size": {"width": 40, "height": 20},
		"root": {"type": "absolute", "children": [
			{"bounds": {"x": -1e300, "y": 0, "width": 1e300, "height": 1e300},
			 "node": {"type": "ellipse", "stroke": {"ink": "black", "width": 1e300, "dash": [3, 1e300]}}}]}}`))
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	if _, err := RenderForSize(document, image.Pt(40, 20)); err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("drawing took %v", elapsed)
	}
}

// A document nested hundreds of levels deep is read in a moment, however it
// is indented.
func TestADeepDocumentIsReadQuickly(t *testing.T) {
	const depth = 400
	var source strings.Builder
	source.WriteString(`{"version": 1, "root": `)
	for level := 0; level < depth; level++ {
		source.WriteString("{\n" + strings.Repeat("  ", level) + `"type": "padding", "insets": {"top": 1, "right": 1, "bottom": 1, "left": 1}, "child": `)
	}
	source.WriteString(`{"type": "rectangle", "fill": "black"}`)
	source.WriteString(strings.Repeat("}", depth) + "}")
	start := time.Now()
	if _, err := (Decoder{}).Decode(strings.NewReader(source.String())); err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("reading took %v", elapsed)
	}
}

func TestALeastMainSizeMayBeAuto(t *testing.T) {
	const document = `{"version":1,"size":{"width":100,"height":20},"root":{"type":"row","children":[
		{"node":{"type":"text","runs":[{"text":"word"}]},"shrink":1,"minMain":"auto"}]}}`
	decoded, err := (Decoder{}).Decode(strings.NewReader(document))
	if err != nil {
		t.Fatal(err)
	}
	row, ok := decoded.Root.(compose.Row)
	if !ok || !row.Children[0].MinContent || row.Children[0].MinMain.IsSet() {
		t.Fatalf("minMain auto decoded as %#v", decoded.Root)
	}
	if _, err := (Decoder{}).Decode(strings.NewReader(strings.Replace(document, `"auto"`, `"most"`, 1))); err == nil {
		t.Fatal("minMain \"most\" was accepted")
	}
}
