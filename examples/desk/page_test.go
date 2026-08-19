// Package desk holds five pages for a tag that lives next to a computer, each
// authored for the 296x128 panel it will spend its life on.
//
// They are separate documents rather than one adaptive document on purpose. A
// tag here has a fixed job, so the size it renders at is known when the page is
// written, and the layout can be tuned to it rather than negotiated at runtime.
//
// chart.json is the interesting one: it contains no chart. The series was
// turned into polyline coordinates by whatever produced the file, which is the
// dividing line worth keeping. Mapping values to pixels needs to know the axis,
// the range and the padding; drawing a line does not.
//
// disk.json is the one that shows what a grid is for. It was four rows, each
// holding a label, a bar and a figure, and four separate rows cannot agree on
// how wide the label should be: the width was written into each of them as 50,
// a number somebody arrived at by measuring the font. One grid with an
// automatic column measures it once, across every row, and the number goes
// away. The page changed slightly when it did, because 50 had been a pixel or
// two too tight all along and /backup was touching its bar.
package desk

import (
	"bytes"
	_ "embed"
	"testing"

	"github.com/xwvike/inkwire/internal/display"
	"github.com/xwvike/inkwire/internal/scene"
	"github.com/xwvike/inkwire/internal/testscene"
)

//go:embed claude.json
var claudeJSON []byte

//go:embed disk.json
var diskJSON []byte

//go:embed tasks.json
var tasksJSON []byte

//go:embed btc.json
var btcJSON []byte

//go:embed chart.json
var chartJSON []byte

func pages() []struct {
	name string
	body []byte
} {
	return []struct {
		name string
		body []byte
	}{
		{"claude", claudeJSON},
		{"disk", diskJSON},
		{"tasks", tasksJSON},
		{"btc", btcJSON},
		{"chart", chartJSON},
	}
}

func TestPagesMatchTheirReferences(t *testing.T) {
	for _, page := range pages() {
		t.Run(page.name, func(t *testing.T) {
			result, err := (scene.Decoder{BaseDir: "."}).Render(bytes.NewReader(page.body))
			if err != nil {
				t.Fatal(err)
			}
			// A page for a panel of known size has no excuse for a warning:
			// nothing about its layout is being negotiated at runtime.
			if len(result.Report.Warnings) != 0 {
				t.Errorf("warnings: %v", result.Report.Warnings)
			}
			if len(result.Report.MissingRunes) != 0 {
				t.Errorf("missing runes: %q", string(result.Report.MissingRunes))
			}
			testscene.AssertEncodesFor(t, 0x0033, result.Frame, result.Orientation)
			testscene.AssertMatchesPNG(t, page.name+".png", result.Frame)
		})
	}
}

// The BTC page is the reason the strikes are offered at whole-number
// multiples. Without them the largest digit available would be 16 pixels, and
// a price meant to be read from across a desk would be the same size as the
// footnote under it.
func TestThePriceUsesAnEnlargedStrike(t *testing.T) {
	fonts, err := display.NewBuiltinFontRegistry()
	if err != nil {
		t.Fatal(err)
	}
	set, ok := fonts.Match("monaco", 48)
	if !ok {
		t.Fatal("monaco 48 is not available, so the price cannot be drawn at that size")
	}
	glyph, ok := set.Resolve('9')
	if !ok {
		t.Fatal("monaco 48 cannot draw a digit")
	}
	native, _ := fonts.Match("monaco", 16)
	nativeGlyph, _ := native.Resolve('9')
	if glyph.Glyph.Height != nativeGlyph.Glyph.Height*3 {
		t.Errorf("the 48px digit is %d tall, want three times the 16px digit's %d",
			glyph.Glyph.Height, nativeGlyph.Glyph.Height)
	}
}
