package display

import (
	"image"
	"testing"
)

func TestToneEndsAreTheInksThemselves(t *testing.T) {
	for _, ink := range []Ink{InkBlack, InkRed, InkYellow} {
		if got := Tone(ink, 0); got != InkWhite {
			t.Errorf("%s at 0/16 is %s, want white", ink, got)
		}
		if got := Tone(ink, ToneSteps); got != ink {
			t.Errorf("%s at 16/16 is %s, want the ink", ink, got)
		}
		if _, _, toned := Tone(ink, 8).Toned(); !toned {
			t.Errorf("%s at 8/16 is not a tone", ink)
		}
	}
	if got := Tone(InkWhite, 8); got != InkWhite {
		t.Errorf("white at 8/16 is %s, want white", got)
	}
	if got := Tone(InkBlack, 8).String(); got != "black 8/16" {
		t.Errorf("a mid grey names itself %q", got)
	}
}

// Every level covers exactly its share of every 4x4 tile, and each level is the
// one below it with cells added rather than moved, so a lighter grey is never
// darker anywhere than a darker one.
func TestEachLevelCoversItsShareAndContainsTheOneBelow(t *testing.T) {
	previous := map[image.Point]bool{}
	for level := 1; level < ToneSteps; level++ {
		tone := Tone(InkBlack, level)
		on := map[image.Point]bool{}
		for y := 0; y < 4; y++ {
			for x := 0; x < 4; x++ {
				if tone.Physical(x, y) == InkBlack {
					on[image.Pt(x, y)] = true
				} else if tone.Physical(x, y) != InkWhite {
					t.Fatalf("level %d draws %s, want black or white", level, tone.Physical(x, y))
				}
			}
		}
		if len(on) != level {
			t.Errorf("level %d covers %d of 16 cells", level, len(on))
		}
		for cell := range previous {
			if !on[cell] {
				t.Errorf("level %d turns off %v, which level %d had on", level, cell, level-1)
			}
		}
		previous = on
	}
}

// The frame holds inks a panel has. A tone is resolved on the way in, and
// anything reading the frame afterwards — the encoder, a PNG — sees only the
// ink and white.
func TestAFrameNeverHoldsATone(t *testing.T) {
	frame := newTestFrame(t, 12, 12)
	canvas := NewCanvas(frame)
	canvas.FillRect(frame.Bounds(), Tone(InkRed, 5))
	for y := 0; y < 12; y++ {
		for x := 0; x < 12; x++ {
			if got, _ := frame.InkAt(x, y); got != InkRed && got != InkWhite {
				t.Fatalf("pixel (%d,%d) holds %s", x, y, got)
			}
		}
	}
}

// The dither is keyed to the frame, not to the shape, so two boxes of one tone
// side by side draw exactly what one box covering both would. Keyed to each
// box's own corner, a box starting on an odd column would restart the pattern
// and leave a visible seam down the join.
func TestNeighbouringTonesMeetWithoutASeam(t *testing.T) {
	tone := Tone(InkBlack, 6)
	whole := newTestFrame(t, 21, 7)
	NewCanvas(whole).FillRect(whole.Bounds(), tone)

	split := newTestFrame(t, 21, 7)
	canvas := NewCanvas(split)
	canvas.FillRect(image.Rect(0, 0, 9, 7), tone)
	canvas.FillRect(image.Rect(9, 0, 21, 7), tone)

	for y := 0; y < 7; y++ {
		for x := 0; x < 21; x++ {
			want, _ := whole.InkAt(x, y)
			if got, _ := split.InkAt(x, y); got != want {
				t.Fatalf("pixel (%d,%d) is %s split and %s whole", x, y, got, want)
			}
		}
	}
}

// The halo clears a ring around a glyph only where a tone put the pixel. A
// solid mark in the same ring is part of the design and keeps its ink, and
// with no tone on the page the halo has nothing to do at all.
func TestTheHaloClearsToneAndNothingElse(t *testing.T) {
	frame := newTestFrame(t, 9, 9)
	canvas := NewCanvas(frame)
	canvas.FillRect(frame.Bounds(), Tone(InkBlack, ToneSteps-1))
	canvas.FillRect(image.Rect(3, 0, 4, 9), InkBlack) // a solid rule beside the glyph

	dot := func(x, y int) bool { return x == 4 && y == 4 }
	canvas.haloWhere(image.Rect(4, 4, 5, 5), dot, InkBlack)

	for y := 3; y <= 5; y++ {
		for x := 3; x <= 5; x++ {
			switch {
			case x == 4 && y == 4:
				// The glyph's own pixel is the glyph's to draw, not the halo's.
			case x == 3:
				assertInk(t, frame, x, y, InkBlack)
			default:
				assertInk(t, frame, x, y, InkWhite)
			}
		}
	}
	// Outside the ring the tone is untouched: at 15/16 only one cell in a tile
	// is white, and (7,7) is not it.
	assertInk(t, frame, 7, 7, Tone(InkBlack, ToneSteps-1).Physical(7, 7))
}

// White text needs the opposite ground to stand out from a grey.
func TestTheHaloAroundWhiteTextIsBlack(t *testing.T) {
	frame := newTestFrame(t, 5, 5)
	canvas := NewCanvas(frame)
	canvas.FillRect(frame.Bounds(), Tone(InkBlack, 1))
	canvas.haloWhere(image.Rect(2, 2, 3, 3), func(x, y int) bool { return x == 2 && y == 2 }, InkWhite)
	assertInk(t, frame, 1, 1, InkBlack)
	assertInk(t, frame, 3, 3, InkBlack)
}

// Text drawn through the ordinary path gets the halo, and only where it sits on
// a tone.
func TestTextOverAToneIsHaloed(t *testing.T) {
	registry, err := NewBuiltinFontRegistry()
	if err != nil {
		t.Fatal(err)
	}
	draw := func(ground Ink) *Frame {
		frame := newTestFrame(t, 40, 16)
		canvas := NewCanvas(frame)
		canvas.FillRect(frame.Bounds(), ground)
		if _, err := canvas.DrawTextBox(registry, TextBox{
			Bounds:     frame.Bounds(),
			Runs:       []TextRun{{Text: "HI", Style: TextStyle{Font: DefaultFont, Ink: InkBlack}}},
			LineHeight: 16,
		}); err != nil {
			t.Fatal(err)
		}
		return frame
	}
	plain, toned := draw(InkWhite), draw(Tone(InkBlack, ToneSteps-1))

	// Every pixel next to a stroke of the letters is clear on the grey just as
	// it is on white; elsewhere the grey is still there.
	cleared, kept := 0, 0
	for y := 0; y < 16; y++ {
		for x := 0; x < 40; x++ {
			glyph, _ := plain.InkAt(x, y)
			if glyph == InkBlack {
				assertInk(t, toned, x, y, InkBlack)
				continue
			}
			near := false
			for dy := -1; dy <= 1; dy++ {
				for dx := -1; dx <= 1; dx++ {
					if ink, inside := plain.InkAt(x+dx, y+dy); inside && ink == InkBlack {
						near = true
					}
				}
			}
			got, _ := toned.InkAt(x, y)
			if near {
				if got != InkWhite {
					t.Fatalf("pixel (%d,%d) beside a stroke is %s, want the halo's white", x, y, got)
				}
				cleared++
			} else if got == InkBlack {
				kept++
			}
		}
	}
	if cleared == 0 || kept == 0 {
		t.Fatalf("cleared %d and kept %d pixels of grey; the test drew nothing to compare", cleared, kept)
	}
}
