package display

import (
	"image"
	"testing"
)

// A deliberately asymmetric source, so that a wrong rotation cannot pass by
// symmetry. The single red pixel marks the top-left corner.
func marker(t *testing.T) *Frame {
	t.Helper()
	frame, err := NewFrame(3, 2, InkWhite)
	if err != nil {
		t.Fatal(err)
	}
	frame.Set(0, 0, InkRed)   // top left
	frame.Set(2, 0, InkBlack) // top right
	return frame
}

func inkAt(t *testing.T, frame *Frame, x, y int) Ink {
	t.Helper()
	ink, ok := frame.InkAt(x, y)
	if !ok {
		t.Fatalf("(%d,%d) is outside the %dx%d frame", x, y, frame.Width(), frame.Height())
	}
	return ink
}

func transformed(t *testing.T, source *Frame, transform Transform) *Frame {
	t.Helper()
	size := transform.Apply(image.Pt(source.Width(), source.Height()))
	target, err := NewFrame(size.X, size.Y, InkWhite)
	if err != nil {
		t.Fatal(err)
	}
	if err := NewCanvas(target).DrawFrame(source, image.Point{}, transform); err != nil {
		t.Fatal(err)
	}
	return target
}

// Each quarter turn moves the marked corner to the next corner clockwise, and
// swaps the frame's proportions on the odd turns.
func TestQuarterTurnsMoveTheCornerClockwise(t *testing.T) {
	source := marker(t)
	tests := []struct {
		turns   int
		size    image.Point
		redAt   image.Point
		blackAt image.Point
	}{
		{0, image.Pt(3, 2), image.Pt(0, 0), image.Pt(2, 0)},
		{1, image.Pt(2, 3), image.Pt(1, 0), image.Pt(1, 2)},
		{2, image.Pt(3, 2), image.Pt(2, 1), image.Pt(0, 1)},
		{3, image.Pt(2, 3), image.Pt(0, 2), image.Pt(0, 0)},
	}
	for _, test := range tests {
		turned := transformed(t, source, Transform{Turns: test.turns})
		if got := image.Pt(turned.Width(), turned.Height()); got != test.size {
			t.Errorf("%d turns produced %v, want %v", test.turns, got, test.size)
			continue
		}
		if ink := inkAt(t, turned, test.redAt.X, test.redAt.Y); ink != InkRed {
			t.Errorf("%d turns: the marked corner is not at %v", test.turns, test.redAt)
		}
		if ink := inkAt(t, turned, test.blackAt.X, test.blackAt.Y); ink != InkBlack {
			t.Errorf("%d turns: the second corner is not at %v", test.turns, test.blackAt)
		}
	}
}

// Four quarter turns are no turn at all, which is the cheapest check that the
// mapping is a permutation rather than something that loses pixels.
func TestFourQuarterTurnsComeBack(t *testing.T) {
	source := marker(t)
	turned := source
	for i := 0; i < 4; i++ {
		turned = transformed(t, turned, Transform{Turns: 1})
	}
	for y := 0; y < source.Height(); y++ {
		for x := 0; x < source.Width(); x++ {
			if inkAt(t, turned, x, y) != inkAt(t, source, x, y) {
				t.Fatalf("after four turns (%d,%d) differs", x, y)
			}
		}
	}
}

// Magnifying by a whole number turns each pixel into a block of its own ink,
// which is what makes it exact here.
func TestScaleRepeatsEachPixel(t *testing.T) {
	source := marker(t)
	for _, factor := range []int{2, 3, 4} {
		scaled := transformed(t, source, Transform{Scale: factor})
		if scaled.Width() != source.Width()*factor || scaled.Height() != source.Height()*factor {
			t.Fatalf("scale %d produced %dx%d", factor, scaled.Width(), scaled.Height())
		}
		for y := 0; y < scaled.Height(); y++ {
			for x := 0; x < scaled.Width(); x++ {
				want := inkAt(t, source, x/factor, y/factor)
				if got := inkAt(t, scaled, x, y); got != want {
					t.Fatalf("scale %d: (%d,%d) is %v, source (%d,%d) is %v",
						factor, x, y, got, x/factor, y/factor, want)
				}
			}
		}
	}
}

func TestScaleAndTurnCompose(t *testing.T) {
	source := marker(t)
	both := transformed(t, source, Transform{Scale: 2, Turns: 1})
	if both.Width() != 4 || both.Height() != 6 {
		t.Fatalf("a doubled quarter turn of 3x2 is %dx%d, want 4x6", both.Width(), both.Height())
	}
	// The marked corner occupies the whole 2x2 block it grew into.
	for _, at := range []image.Point{{2, 0}, {3, 0}, {2, 1}, {3, 1}} {
		if ink := inkAt(t, both, at.X, at.Y); ink != InkRed {
			t.Errorf("%v is %v, want the magnified corner", at, ink)
		}
	}
}

func TestIdentityAndInversion(t *testing.T) {
	if !(Transform{}).identity() || !(Transform{Scale: 1, Turns: 4}).identity() {
		t.Error("a transform that changes nothing did not say so")
	}
	transform := Transform{Scale: 3, Turns: 1}
	// A child asked to fill 9x30 after the transform must be drawn 10x3.
	if got := transform.Invert(image.Pt(9, 30)); got != image.Pt(10, 3) {
		t.Errorf("Invert = %v, want (10,3)", got)
	}
}
