package display

import (
	"image"
	"slices"
	"testing"

	"golang.org/x/text/encoding/simplifiedchinese"
)

// The rune-to-index map is built by decoding GBK bytes. Re-encoding every entry
// through the independent encoder and recomputing the documented区位 formula
// catches a map entry that would silently address the wrong glyph.
func TestGB2312IndicesMatchTheOffsetFormula(t *testing.T) {
	encoder := simplifiedchinese.GBK.NewEncoder()
	checked := 0
	for r, index := range gb2312Indices() {
		if r == '―' {
			continue // deliberate compatibility override, covered separately
		}
		raw, err := encoder.Bytes([]byte(string(r)))
		if err != nil || len(raw) != 2 {
			continue // not representable as a two-byte GBK sequence
		}
		if want := (int(raw[0])-0xa1)*94 + int(raw[1]) - 0xa1; index != want {
			t.Fatalf("rune %q (U+%04X) maps to index %d, formula on %X gives %d", r, r, index, raw, want)
		}
		checked++
	}
	if checked < 7000 {
		t.Fatalf("only %d indices checked, the test is too weak to mean anything", checked)
	}
}

// A GB2312 index can be in range and still land on a slot the strike never
// filled. Those runes must be reported as missing rather than drawn as nothing.
func TestUnassignedSlotsResolveAsMissing(t *testing.T) {
	registry, err := NewBuiltinFontRegistry()
	if err != nil {
		t.Fatal(err)
	}
	// A2E3 is a GBK addition, so no HZK strike carries a euro sign.
	for _, name := range []string{"HZK12", "HZK14", "HZK16", "ui-12", "ui-14", "ui-16"} {
		set, ok := registry.Lookup(name)
		if !ok {
			t.Fatalf("font set %s is missing", name)
		}
		if _, ok := set.Resolve('€'); ok {
			t.Errorf("%s claims to have a glyph for the euro sign", name)
		}
	}
	// The strikes disagree about the GBK extensions: HZK12 and HZK14 carry the
	// lowercase Roman numerals, HZK16 leaves the same slots empty.
	for _, name := range []string{"HZK12", "HZK14"} {
		set, _ := registry.Lookup(name)
		if _, ok := set.Resolve('ⅰ'); !ok {
			t.Errorf("%s lost its lowercase Roman numerals", name)
		}
	}
	set, _ := registry.Lookup("HZK16")
	if _, ok := set.Resolve('ⅰ'); ok {
		t.Error("HZK16 unexpectedly gained a lowercase Roman numeral")
	}
}

// Both space characters are blank on purpose and must keep advancing the pen.
func TestSpacesKeepBlankGlyphsAndAdvance(t *testing.T) {
	registry, err := NewBuiltinFontRegistry()
	if err != nil {
		t.Fatal(err)
	}
	for _, pair := range []struct {
		set string
		r   rune
	}{{"MONACO10", ' '}, {"HZK12", '　'}, {"ui-12", ' '}, {"ui-12", '　'}} {
		set, ok := registry.Lookup(pair.set)
		if !ok {
			t.Fatalf("font set %s is missing", pair.set)
		}
		resolved, ok := set.Resolve(pair.r)
		if !ok {
			t.Fatalf("%s no longer resolves U+%04X", pair.set, pair.r)
		}
		if resolved.Glyph.Advance <= 0 {
			t.Errorf("%s U+%04X advances by %d", pair.set, pair.r, resolved.Glyph.Advance)
		}
		if !isBlankRecord(resolved.Glyph.Data) {
			t.Errorf("%s U+%04X is no longer blank", pair.set, pair.r)
		}
	}
	// Control characters share the ASCII strike's empty records but are not
	// spaces, so they must surface as missing.
	monaco, _ := registry.Lookup("MONACO10")
	if _, ok := monaco.Resolve('\r'); ok {
		t.Error("MONACO10 claims to have a glyph for a carriage return")
	}
}

// MissingRunes is the only signal available before a payload reaches the tag,
// so an unassigned slot has to reach it instead of laying out as empty space.
func TestLayoutReportsUnassignedSlotsAsMissing(t *testing.T) {
	registry, err := NewBuiltinFontRegistry()
	if err != nil {
		t.Fatal(err)
	}
	layout, err := LayoutText(registry, TextBox{
		Bounds: image.Rect(0, 0, 200, 40),
		Runs:   []TextRun{{Text: "€9　9", Style: TextStyle{Font: "ui", Size: 12}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	missing := layout.MissingRunes()
	if !slices.Contains(missing, '€') {
		t.Fatalf("MissingRunes() = %q, want it to contain the euro sign", string(missing))
	}
	if slices.Contains(missing, '　') {
		t.Fatalf("MissingRunes() = %q, the ideographic space must not be reported", string(missing))
	}
}
