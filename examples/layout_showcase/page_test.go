// Package layout_showcase is one page that uses the layout nodes a document
// could not reach until recently: grid, anchored, transformed, clip and
// clipShape.
//
// Each of them is there for a reason the page would otherwise have to work
// around, and the tests below check the reason rather than the pixels alone.
//
//   - grid measures the label column once. CPU, MEM and DISK are three
//     separate readings that know nothing about each other, and an automatic
//     column is what makes their bars start at the same place without anyone
//     writing that place down.
//   - transformed turns the strip down the left edge a quarter turn, and
//     magnifies the figure in the badge. Both are exact here: whole-number
//     scaling and quarter turns move pixels to pixels on a panel with no greys.
//   - anchored puts the badge six from the bottom and six from the right,
//     which is not a number until the page has been laid out, and layers it
//     over the grid.
//   - clipShape makes the badge round.
//   - clip cuts the note off at the edge of its cell instead of letting it run
//     into the badge.
package layout_showcase

import (
	"testing"

	"github.com/xwvike/inkwire/internal/display"
	"github.com/xwvike/inkwire/internal/scene"
	"github.com/xwvike/inkwire/internal/testscene"
)

func renderPage(t *testing.T) scene.Result {
	t.Helper()
	result, err := (scene.Decoder{BaseDir: "."}).RenderFile("page.json")
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func TestPageMatchesReference(t *testing.T) {
	result := renderPage(t)
	payload, err := result.Payload()
	if err != nil || len(payload) != display.GiciskyPayloadSize {
		t.Fatalf("payload = %d bytes, %v", len(payload), err)
	}
	testscene.AssertMatchesPNG(t, "layout_showcase.png", result.Frame)
}

// The three bars line up because one column was measured across all three
// rows. Nothing in the document says where they start.
func TestTheAutomaticColumnLinesUpEveryLabel(t *testing.T) {
	frame := renderPage(t).Frame
	bands := [][2]int{{23, 37}, {41, 55}, {59, 73}}
	var edges []int
	for _, band := range bands {
		edges = append(edges, leftBorder(t, frame, band[0], band[1]))
	}
	for index, edge := range edges {
		if edge != edges[0] {
			t.Errorf("bar %d starts at x=%d, the first starts at x=%d; "+
				"the automatic column is not measuring across the rows", index, edge, edges[0])
		}
	}
	if edges[0] <= 21 {
		t.Errorf("the bars start at x=%d, which leaves no room for a label", edges[0])
	}
}

// leftBorder finds the first column of pixels that is black all the way down
// the band, which is the left edge of the bar's box and not part of a glyph.
func leftBorder(t *testing.T, frame *display.Frame, top, bottom int) int {
	t.Helper()
	for x := 20; x < 250; x++ {
		solid := true
		for y := top; y < bottom; y++ {
			if ink, _ := frame.InkAt(x, y); ink != display.InkBlack {
				solid = false
				break
			}
		}
		if solid {
			return x
		}
	}
	t.Fatalf("no bar border found in y[%d..%d)", top, bottom)
	return 0
}

// A transform draws its child onto a surface and copies the surface over. The
// surface has a background and a frame has no transparent ink, so without
// knowing which pixels the child actually reached, that background goes over
// whatever is underneath: the strip's black bar would be wiped out by the
// label sitting on it, and the page would show a white gap instead.
func TestTheRotatedStripKeepsTheBarUnderneathIt(t *testing.T) {
	frame := renderPage(t).Frame
	black, white := 0, 0
	for y := 0; y < frame.Height(); y++ {
		for x := 0; x < 16; x++ {
			switch ink, _ := frame.InkAt(x, y); ink {
			case display.InkBlack:
				black++
			case display.InkWhite:
				white++
			}
		}
	}
	if black < 1500 {
		t.Errorf("the strip has only %d black pixels; the bar under the turned label is gone", black)
	}
	if white < 50 {
		t.Errorf("the strip has only %d white pixels; the turned label is not on it", white)
	}
}

// The badge is round, sits where the far corner is rather than where a number
// said, and is painted after the grid it overlaps.
func TestTheBadgeIsRoundAndSitsAgainstTheFarCorner(t *testing.T) {
	frame := renderPage(t).Frame
	if ink, _ := frame.InkAt(270, 103); ink != display.InkRed {
		t.Errorf("the middle of the badge is %v, want red", ink)
	}
	for _, corner := range [][2]int{{252, 85}, {289, 122}} {
		if ink, _ := frame.InkAt(corner[0], corner[1]); ink != display.InkWhite {
			t.Errorf("(%d,%d) is %v; the circle is not clipping its corners",
				corner[0], corner[1], ink)
		}
	}
	// The figure is drawn at twice its strike, so the white of the glyph has
	// to survive inside the red.
	white := 0
	for y := 88; y < 120; y++ {
		for x := 255; x < 288; x++ {
			if ink, _ := frame.InkAt(x, y); ink == display.InkWhite {
				white++
			}
		}
	}
	if white < 40 {
		t.Errorf("only %d white pixels inside the badge; the magnified figure is missing", white)
	}
}

// Clipping and saying so are two different things, and this page wants both:
// the note is cut off at its cell, and the report names it.
func TestTheOverlongNoteIsClippedAndReported(t *testing.T) {
	result := renderPage(t)
	if len(result.Report.Warnings) != 1 {
		t.Fatalf("warnings = %v, want exactly the one clipped note", result.Report.Warnings)
	}
	if code := result.Report.Warnings[0].Code; code != "text-clipped" {
		t.Errorf("warning code = %q, want text-clipped", code)
	}
	if len(result.Report.MissingRunes) != 0 {
		t.Errorf("missing runes: %q", string(result.Report.MissingRunes))
	}
	// The badge is to the right of the note's cell, so nothing of the note may
	// reach it.
	frame := result.Frame
	for y := 77; y < 95; y++ {
		for x := 250; x < 296; x++ {
			if ink, _ := frame.InkAt(x, y); ink == display.InkBlack {
				t.Fatalf("(%d,%d) is black; the note ran past its cell", x, y)
			}
		}
	}
}
