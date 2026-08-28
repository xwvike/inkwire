package display

import (
	"image"
	"testing"
)

func checkerPattern(t *testing.T) *Pattern {
	t.Helper()
	pattern, err := NewPattern([]string{"xo", "ox"}, map[rune]Ink{'x': InkBlack, 'o': InkWhite})
	if err != nil {
		t.Fatal(err)
	}
	return pattern
}

func TestNewPatternRejectsMalformedTiles(t *testing.T) {
	for _, test := range []struct {
		name string
		rows []string
		inks map[rune]Ink
	}{
		{"no rows", nil, map[rune]Ink{'x': InkBlack}},
		{"empty row", []string{""}, map[rune]Ink{'x': InkBlack}},
		{"ragged rows", []string{"xx", "x"}, map[rune]Ink{'x': InkBlack}},
		{"invalid ink", []string{"x"}, map[rune]Ink{'x': Ink(9)}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := NewPattern(test.rows, test.inks); err == nil {
				t.Fatal("NewPattern accepted a malformed tile")
			}
		})
	}
}

func TestFillPatternTilesTheRegion(t *testing.T) {
	frame := newTestFrame(t, 8, 8)
	NewCanvas(frame).FillPattern(image.Rect(0, 0, 4, 4), checkerPattern(t))

	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			want := InkWhite
			if (x+y)%2 == 0 {
				want = InkBlack
			}
			assertInk(t, frame, x, y, want)
		}
	}
	assertInk(t, frame, 5, 5, InkWhite) // outside the filled rectangle
}

// Runes with no ink leave the frame alone, so a hatch can overlay content.
func TestPatternLeavesUnmappedCellsAlone(t *testing.T) {
	hatch, err := NewPattern([]string{"x.", ".."}, map[rune]Ink{'x': InkBlack})
	if err != nil {
		t.Fatal(err)
	}
	frame := newTestFrame(t, 4, 4)
	canvas := NewCanvas(frame)
	canvas.FillRect(frame.Bounds(), InkRed)
	canvas.FillPattern(frame.Bounds(), hatch)

	assertInk(t, frame, 0, 0, InkBlack) // the painted cell
	assertInk(t, frame, 1, 0, InkRed)   // untouched, the red below shows through
	assertInk(t, frame, 1, 1, InkRed)
	assertInk(t, frame, 2, 2, InkBlack) // the tile repeats
}

// The tile is anchored to the frame, so two fills that meet do not each restart
// their own phase and leave a seam.
func TestPatternPhaseIsAnchoredToTheFrame(t *testing.T) {
	pattern := checkerPattern(t)
	together := newTestFrame(t, 8, 4)
	NewCanvas(together).FillPattern(image.Rect(0, 0, 8, 4), pattern)

	// The seam is at an odd column so that a tile anchored to each rectangle
	// instead of to the frame would visibly break here.
	split := newTestFrame(t, 8, 4)
	canvas := NewCanvas(split)
	canvas.FillPattern(image.Rect(0, 0, 3, 4), pattern)
	canvas.FillPattern(image.Rect(3, 0, 8, 4), pattern)

	for y := 0; y < 4; y++ {
		for x := 0; x < 8; x++ {
			want, _ := together.InkAt(x, y)
			got, _ := split.InkAt(x, y)
			if got != want {
				t.Fatalf("pixel (%d,%d) differs across the seam: split = %d, whole = %d", x, y, got, want)
			}
		}
	}
}

// Anchoring to the frame also means a translation moves the shape being filled
// without dragging the tile along, matching how clipping behaves.
func TestPatternPhaseIgnoresTranslation(t *testing.T) {
	// The offset has to be odd against a 2x2 tile, or the phase shift the test
	// is looking for lands back on itself and the case proves nothing.
	pattern := checkerPattern(t)
	direct := newTestFrame(t, 8, 8)
	NewCanvas(direct).FillPattern(image.Rect(3, 2, 7, 6), pattern)

	shifted := newTestFrame(t, 8, 8)
	canvas := NewCanvas(shifted)
	canvas.Translate(image.Pt(3, 2))
	canvas.FillPattern(image.Rect(0, 0, 4, 4), pattern)

	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			want, _ := direct.InkAt(x, y)
			got, _ := shifted.InkAt(x, y)
			if got != want {
				t.Fatalf("pixel (%d,%d): translated = %d, direct = %d", x, y, got, want)
			}
		}
	}
}

// Shaping a pattern fill is the clip's job, which is what keeps the API from
// needing a patterned variant of every fill primitive.
func TestPatternFillIsShapedByTheClip(t *testing.T) {
	frame := newTestFrame(t, 40, 40)
	canvas := NewCanvas(frame)

	var circle Path
	circle.Arc(Upright(image.Rect(8, 8, 32, 32)), 0, 360)
	canvas.Save()
	canvas.ClipPath(circle)
	canvas.FillPattern(frame.Bounds(), checkerPattern(t))
	canvas.Restore()

	assertInk(t, frame, 20, 20, InkBlack) // (20+20)%2 == 0, inside the circle
	assertInk(t, frame, 21, 20, InkWhite)
	assertInk(t, frame, 1, 1, InkWhite) // outside the circle, never painted
	if countInk(frame, InkBlack) == 0 {
		t.Fatal("the clipped pattern painted nothing")
	}
}

func TestDisplayListReplaysFillPattern(t *testing.T) {
	const size = 16
	pattern := checkerPattern(t)
	rect := image.Rect(2, 3, 14, 12)

	direct := newTestFrame(t, size, size)
	NewCanvas(direct).FillPattern(rect, pattern)

	list := &DisplayList{}
	list.FillPattern(rect, pattern)
	if got, want := list.Bounds(), rect; got != want {
		t.Fatalf("recorded bounds = %v, want %v", got, want)
	}
	replayed := newTestFrame(t, size, size)
	if err := list.Replay(NewCanvas(replayed)); err != nil {
		t.Fatal(err)
	}
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			want, _ := direct.InkAt(x, y)
			got, _ := replayed.InkAt(x, y)
			if got != want {
				t.Fatalf("pixel (%d,%d): replay = %d, canvas = %d", x, y, got, want)
			}
		}
	}
}

func TestDisplayListIgnoresNilPatternAndEmptyRect(t *testing.T) {
	list := &DisplayList{}
	list.FillPattern(image.Rect(0, 0, 4, 4), nil)
	list.FillPattern(image.Rectangle{}, checkerPattern(t))
	if list.Len() != 0 {
		t.Fatalf("recorded %d commands for degenerate pattern fills", list.Len())
	}
	// The canvas has to agree, or replay and direct drawing diverge.
	frame := newTestFrame(t, 4, 4)
	NewCanvas(frame).FillPattern(image.Rect(0, 0, 4, 4), nil)
	if countInk(frame, InkBlack) != 0 {
		t.Fatal("a nil pattern painted something")
	}
}
