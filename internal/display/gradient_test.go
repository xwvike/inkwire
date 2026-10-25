package display

import (
	"image"
	"testing"
)

func blackShare(frame *Frame, column image.Rectangle) float64 {
	black := 0
	for y := column.Min.Y; y < column.Max.Y; y++ {
		for x := column.Min.X; x < column.Max.X; x++ {
			if ink, _ := frame.InkAt(x, y); ink == InkBlack {
				black++
			}
		}
	}
	return float64(black) / float64(column.Dx()*column.Dy())
}

// A gradient from white to black runs the whole way: the first band is white,
// the last is black, and nothing gets lighter on the way.
func TestAGradientRunsTowardsTheSideItNames(t *testing.T) {
	stops := []GradientStop{{Ink: InkWhite, At: 0}, {Ink: InkBlack, At: 1}}
	for _, direction := range []GradientDirection{GradientToRight, GradientToLeft, GradientToBottom, GradientToTop} {
		frame := newTestFrame(t, 64, 64)
		NewCanvas(frame).FillGradient(frame.Bounds(), 0, &Gradient{Direction: direction, Stops: stops})

		// Strips of four pixels across the direction of travel, so that each
		// holds whole dither tiles and its share is the tone it was given.
		strip := func(step int) image.Rectangle {
			switch direction {
			case GradientToRight:
				return image.Rect(step*4, 0, step*4+4, 64)
			case GradientToLeft:
				return image.Rect(60-step*4, 0, 64-step*4, 64)
			case GradientToBottom:
				return image.Rect(0, step*4, 64, step*4+4)
			}
			return image.Rect(0, 60-step*4, 64, 64-step*4)
		}
		previous := -1.0
		for step := 0; step < 16; step++ {
			share := blackShare(frame, strip(step))
			if share < previous {
				t.Fatalf("to %s: strip %d is lighter (%v) than the one before (%v)", direction, step, share, previous)
			}
			previous = share
		}
		// The end bands are half as wide as the rest — seventeen levels share
		// sixteen strips — so it is the outermost line at each end that is
		// pure, not the outermost strip.
		start, end := strip(0), strip(15)
		switch direction {
		case GradientToRight:
			start.Max.X, end.Min.X = start.Min.X+1, end.Max.X-1
		case GradientToLeft:
			start.Min.X, end.Max.X = start.Max.X-1, end.Min.X+1
		case GradientToBottom:
			start.Max.Y, end.Min.Y = start.Min.Y+1, end.Max.Y-1
		case GradientToTop:
			start.Min.Y, end.Max.Y = start.Max.Y-1, end.Min.Y+1
		}
		if first := blackShare(frame, start); first != 0 {
			t.Errorf("to %s starts %v black, want white", direction, first)
		}
		if last := blackShare(frame, end); last != 1 {
			t.Errorf("to %s ends %v black, want black", direction, last)
		}
	}
}

// Stops are honoured where they are placed: the tone at a stop is the stop's,
// beyond the last stop the last colour holds, and a hard stop is a hard edge.
func TestAGradientHonoursItsStops(t *testing.T) {
	frame := newTestFrame(t, 64, 8)
	NewCanvas(frame).FillGradient(frame.Bounds(), 0, &Gradient{Direction: GradientToRight, Stops: []GradientStop{
		{Ink: InkBlack, At: 0}, {Ink: InkBlack, At: 0.25}, {Ink: InkWhite, At: 0.25}, {Ink: Tone(InkBlack, 8), At: 0.75},
	}})
	if share := blackShare(frame, image.Rect(0, 0, 16, 8)); share != 1 {
		t.Errorf("the first quarter is %v black, want all of it", share)
	}
	if share := blackShare(frame, image.Rect(16, 0, 20, 8)); share > 1.0/16 {
		t.Errorf("just after the hard stop is %v black, want next to none", share)
	}
	if share := blackShare(frame, image.Rect(48, 0, 64, 8)); share != 0.5 {
		t.Errorf("past the last stop is %v black, want the stop's half", share)
	}
}

func TestAGradientIsOneInkOverWhite(t *testing.T) {
	frame := newTestFrame(t, 32, 4)
	NewCanvas(frame).FillGradient(frame.Bounds(), 0, &Gradient{Direction: GradientToRight,
		Stops: []GradientStop{{Ink: InkWhite}, {Ink: InkRed, At: 1}}})
	for x := 0; x < 32; x++ {
		for y := 0; y < 4; y++ {
			if ink, _ := frame.InkAt(x, y); ink != InkRed && ink != InkWhite {
				t.Fatalf("(%d,%d) is %s in a white-to-red gradient", x, y, ink)
			}
		}
	}

	for _, bad := range []*Gradient{
		{Stops: []GradientStop{{Ink: InkBlack}}},
		{Stops: []GradientStop{{Ink: InkBlack}, {Ink: InkRed, At: 1}}},
		{Stops: []GradientStop{{Ink: Tone(InkBlack, 4)}, {Ink: Tone(InkRed, 4), At: 1}}},
		{Stops: []GradientStop{{Ink: InkBlack, At: 0.5}, {Ink: InkWhite, At: 0.2}}},
		{Direction: GradientToLeft + 1, Stops: []GradientStop{{Ink: InkBlack}, {Ink: InkWhite, At: 1}}},
	} {
		if err := bad.Validate(); err == nil {
			t.Errorf("%+v was accepted", bad)
		}
	}
}

// A rounded gradient is cut to the same corners a rounded fill is.
func TestARoundedGradientHasTheCornersOfARoundedFill(t *testing.T) {
	bounds := image.Rect(0, 0, 24, 16)
	gradient := newTestFrame(t, 24, 16)
	NewCanvas(gradient).FillGradient(bounds, 6, &Gradient{Stops: []GradientStop{{Ink: InkBlack}, {Ink: InkBlack, At: 1}}})
	fill := newTestFrame(t, 24, 16)
	NewCanvas(fill).FillRoundRect(bounds, 6, InkBlack)
	for y := 0; y < 16; y++ {
		for x := 0; x < 24; x++ {
			want, _ := fill.InkAt(x, y)
			if got, _ := gradient.InkAt(x, y); got != want {
				t.Fatalf("(%d,%d) is %s, a rounded fill has %s", x, y, got, want)
			}
		}
	}
}

// Recorded and replayed, a gradient draws what drawing it directly does.
func TestAGradientSurvivesTheDisplayList(t *testing.T) {
	g := &Gradient{Direction: GradientToBottom, Stops: []GradientStop{{Ink: InkWhite}, {Ink: InkBlack, At: 1}}}
	direct := newTestFrame(t, 16, 32)
	NewCanvas(direct).FillGradient(direct.Bounds(), 3, g)

	var list DisplayList
	list.FillGradient(image.Rect(0, 0, 16, 32), 3, g)
	replayed := newTestFrame(t, 16, 32)
	if err := list.Replay(NewCanvas(replayed)); err != nil {
		t.Fatal(err)
	}
	for y := 0; y < 32; y++ {
		for x := 0; x < 16; x++ {
			want, _ := direct.InkAt(x, y)
			if got, _ := replayed.InkAt(x, y); got != want {
				t.Fatalf("(%d,%d) replays as %s, drawn directly %s", x, y, got, want)
			}
		}
	}
}
