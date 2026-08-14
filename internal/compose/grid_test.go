package compose

import (
	"image"
	"testing"

	"github.com/xwvike/inkwire/internal/display"
)

// cellsOf renders a grid of coloured boxes and reports the rectangle each ink
// covers, which is the only way to check that a track really is the width it
// was supposed to be.
func cellsOf(t *testing.T, grid Grid, size image.Point) map[display.Ink]image.Rectangle {
	t.Helper()
	compiler, err := NewDefaultCompiler()
	if err != nil {
		t.Fatal(err)
	}
	compiled, _, err := compiler.Compile(Document{Size: size, Root: grid})
	if err != nil {
		t.Fatal(err)
	}
	frame, err := compiled.Render()
	if err != nil {
		t.Fatal(err)
	}
	found := map[display.Ink]image.Rectangle{}
	for y := 0; y < frame.Height(); y++ {
		for x := 0; x < frame.Width(); x++ {
			ink, _ := frame.InkAt(x, y)
			if ink == display.InkWhite {
				continue
			}
			pixel := image.Rect(x, y, x+1, y+1)
			if existing, ok := found[ink]; ok {
				found[ink] = existing.Union(pixel)
			} else {
				found[ink] = pixel
			}
		}
	}
	return found
}

func filled(ink display.Ink) Node { return Rectangle{Fill: &ink} }

func TestFixedAndFractionalTracks(t *testing.T) {
	got := cellsOf(t, Grid{
		Columns: []Track{{Size: Pixels(30)}, {Fraction: 1}, {Fraction: 2}},
		Children: []GridChild{
			{Node: filled(display.InkBlack)},
			{Node: filled(display.InkRed)},
			{Node: filled(display.InkBlack)},
		},
	}, image.Pt(120, 20))
	// Thirty fixed, ninety left, split one to two.
	if got[display.InkRed] != image.Rect(30, 0, 60, 20) {
		t.Errorf("the single-fraction track is %v, want (30,0)-(60,20)", got[display.InkRed])
	}
}

// The reason a grid exists: a column measured once across every row, so rows
// that know nothing about each other still line up.
func TestAutomaticTracksShareTheirWidthAcrossRows(t *testing.T) {
	label := func(text string) Node {
		return Text{Runs: []display.TextRun{{
			Text: text, Style: display.TextStyle{Font: "monaco", Size: 12, Ink: display.InkBlack},
		}}}
	}
	grid := Grid{
		Columns: []Track{autoTrack(), {Fraction: 1}},
		Children: []GridChild{
			{Node: label("/")}, {Node: filled(display.InkRed)},
			{Node: label("/backup")}, {Node: filled(display.InkRed)},
		},
	}
	got := cellsOf(t, grid, image.Pt(160, 40))
	// The bars start where the widest label ends, not where each row's own
	// label ends, so they share a left edge.
	if got[display.InkRed].Min.X != 49 {
		t.Errorf("the bars begin at x=%d; the widest label is seven Monaco characters, or 49 pixels",
			got[display.InkRed].Min.X)
	}
}

func TestAutoPlacementFillsRowByRow(t *testing.T) {
	got := cellsOf(t, Grid{
		Columns: []Track{{Fraction: 1}, {Fraction: 1}},
		Children: []GridChild{
			{Node: filled(display.InkBlack)},
			{Node: filled(display.InkBlack)},
			{Node: filled(display.InkRed)},
		},
	}, image.Pt(100, 40))
	// The third child wraps onto an implicit second row, on the left.
	if got[display.InkRed] != image.Rect(0, 20, 50, 40) {
		t.Errorf("the third child is at %v, want the start of a second row", got[display.InkRed])
	}
}

func TestSpanCoversSeveralTracks(t *testing.T) {
	got := cellsOf(t, Grid{
		Columns: []Track{{Fraction: 1}, {Fraction: 1}, {Fraction: 1}},
		Children: []GridChild{
			{Node: filled(display.InkRed), ColumnSpan: 2},
			{Node: filled(display.InkBlack)},
		},
	}, image.Pt(90, 20))
	if got[display.InkRed] != image.Rect(0, 0, 60, 20) {
		t.Errorf("the spanning child covers %v, want two of three tracks", got[display.InkRed])
	}
}

func TestExplicitPlacement(t *testing.T) {
	got := cellsOf(t, Grid{
		Columns:  []Track{{Fraction: 1}, {Fraction: 1}},
		Rows:     []Track{{Fraction: 1}, {Fraction: 1}},
		Children: []GridChild{{Node: filled(display.InkRed), Column: 2, Row: 2}},
	}, image.Pt(100, 40))
	if got[display.InkRed] != image.Rect(50, 20, 100, 40) {
		t.Errorf("the placed child is at %v, want the second column of the second row", got[display.InkRed])
	}
}

func TestGapsSeparateTracks(t *testing.T) {
	got := cellsOf(t, Grid{
		Columns:   []Track{{Fraction: 1}, {Fraction: 1}},
		ColumnGap: 10,
		Children: []GridChild{
			{Node: filled(display.InkBlack)},
			{Node: filled(display.InkRed)},
		},
	}, image.Pt(110, 20))
	if got[display.InkBlack] != image.Rect(0, 0, 50, 20) {
		t.Errorf("the first track is %v", got[display.InkBlack])
	}
	if got[display.InkRed] != image.Rect(60, 0, 110, 20) {
		t.Errorf("the second track is %v, want it to start after the gap", got[display.InkRed])
	}
}
