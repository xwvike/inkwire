// Package fridge is the densest page here: a to-do list, a menu, a note and a
// water counter, on the panel a family would actually put on a fridge door.
//
// It is worth having because of the numbers it does not contain. An earlier
// version of this page placed everything by hand. Six to-do rows were six
// separate rows, each repeating the same four measurements for the checkbox,
// the gap, the task and the member badge; the five water slots were five
// blocks with four spacers threaded between them; the second magnet clip sat
// at x 356 because 400 minus 356 minus 20 is 24; the tape on the note card sat
// at x 45 because 134 minus 44 over 2 is 45. Every one of those numbers is a
// sum somebody did and then wrote down.
//
// The page says the sums instead. One grid holds the to-do rows and states its
// four columns once. One grid holds the water slots with a gap of two. The two
// clips are both drawn 24 in from their own edge. The tape is
// left: calc(50% - 22px). The tests below check exactly those four, because a
// number that is worked out can go wrong in a way a number that is written
// down cannot: silently, and only on the next page that reuses it.
//
// # This page is 400x300
//
// That is the 4.2" part rather than the 2.9" one the Gicisky driver writes to,
// so there is no Payload assertion below: that encoding refuses any page which
// is not its device's own size. The page is not unreachable for it, though. It
// goes to a 4.2" panel through the EPD-nRF5 driver, which asks the panel what
// it is and builds the page for the answer.
package fridge

import (
	"image"
	"testing"

	"github.com/xwvike/inkwire/internal/display"
	"github.com/xwvike/inkwire/internal/scene"
	"github.com/xwvike/inkwire/internal/testscene"
)

func renderPage(t *testing.T) scene.Result {
	t.Helper()
	return testscene.RenderPage(t, ".", "page")
}

func TestPageMatchesReference(t *testing.T) {
	result := renderPage(t)
	if size := result.Frame.Bounds().Size(); size != image.Pt(400, 300) {
		t.Fatalf("frame is %v, want 400x300", size)
	}
	if len(result.Report.Warnings) != 0 {
		t.Errorf("warnings: %v", result.Report.Warnings)
	}
	if len(result.Report.MissingRunes) != 0 {
		t.Errorf("missing runes: %q", string(result.Report.MissingRunes))
	}
	testscene.AssertMatchesPNG(t, "fridge.png", result.Frame)
}

// The six checkboxes line up because one grid column says how wide they are.
// In the original the same 14 was written into all six rows, and the way that
// page would have gone wrong is one of them being changed and the others not.
func TestOneColumnHoldsEveryCheckbox(t *testing.T) {
	frame := renderPage(t).Frame
	var edges []int
	// The six rows the grid actually places, read off measure rather than
	// counted from the top of the card: the bands here used to start a row
	// late and still passed, because every row it did catch agreed.
	for _, top := range []int{93, 120, 147, 174, 201, 228} {
		edges = append(edges, firstMark(t, frame, top, top+14, 18, 60))
	}
	for index, edge := range edges {
		if edge != edges[0] {
			t.Errorf("checkbox %d starts at x=%d, the first at x=%d", index+1, edge, edges[0])
		}
	}
}

// Both clips are the same distance from their own edge, said once each rather
// than worked out from the page width.
func TestBothMagnetClipsAreInsetTheSame(t *testing.T) {
	frame := renderPage(t).Frame
	left := firstMark(t, frame, 6, 10, 10, 120)
	right := lastMark(t, frame, 6, 10, 280, 380)
	const width = 400
	if left != 24 {
		t.Errorf("the left clip starts at x=%d, want 24", left)
	}
	if gap := width - 1 - right; gap != 24 {
		t.Errorf("the right clip ends %d from the edge, want 24", gap)
	}
}

// calc(50% - 22px) puts the tape in the middle of the card whatever the card
// is, rather than at the 45 somebody worked out for this one.
func TestTheTapeIsCentredOnItsCard(t *testing.T) {
	frame := renderPage(t).Frame
	start := firstMark(t, frame, 161, 164, 270, 380)
	end := lastMark(t, frame, 161, 164, 270, 380)
	if width := end - start + 1; width != 44 {
		t.Fatalf("the tape is %d wide, want 44", width)
	}
	// The card spans x 256 to 389 inclusive.
	const cardStart, cardEnd = 256, 389
	if before, after := start-cardStart, cardEnd-end; before != after {
		t.Errorf("the tape sits %d from the left of the card and %d from the right", before, after)
	}
}

// A gap of two, once, instead of four spacers.
func TestTheWaterSlotsAreEvenlySpaced(t *testing.T) {
	frame := renderPage(t).Frame
	var starts []int
	running := false
	// Five eights and four twos is forty-eight, so the slots end at 107 and
	// the millilitre figure beyond that is not one of them.
	for x := 60; x < 108; x++ {
		marked := false
		for y := 276; y < 284; y++ {
			if ink, _ := frame.InkAt(x, y); ink != display.InkBlack {
				marked = true
				break
			}
		}
		if marked && !running {
			starts = append(starts, x)
		}
		running = marked
	}
	if len(starts) != 5 {
		t.Fatalf("found %d water slots at %v, want 5", len(starts), starts)
	}
	for index := 1; index < len(starts); index++ {
		if pitch := starts[index] - starts[index-1]; pitch != 10 {
			t.Errorf("slot %d starts %d after the one before, want 10 (eight wide and a gap of two)", index+1, pitch)
		}
	}
}

func firstMark(t *testing.T, frame *display.Frame, top, bottom, from, to int) int {
	t.Helper()
	for x := from; x < to; x++ {
		for y := top; y < bottom; y++ {
			if ink, _ := frame.InkAt(x, y); ink != display.InkWhite {
				return x
			}
		}
	}
	t.Fatalf("nothing drawn in x[%d..%d) y[%d..%d)", from, to, top, bottom)
	return 0
}

func lastMark(t *testing.T, frame *display.Frame, top, bottom, from, to int) int {
	t.Helper()
	for x := to - 1; x >= from; x-- {
		for y := top; y < bottom; y++ {
			if ink, _ := frame.InkAt(x, y); ink != display.InkWhite {
				return x
			}
		}
	}
	t.Fatalf("nothing drawn in x[%d..%d) y[%d..%d)", from, to, top, bottom)
	return 0
}
