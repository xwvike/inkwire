package display

import (
	"image"
	"math"
	"testing"
)

// A turned ellipse was the one thing the drawing model could not say, and the
// reason was never the panel. Turning a picture that is already pixels would
// mean resampling, and with no greys that thins some strokes and thickens
// others — which is why a transform here is a whole number of quarter turns.
// An ellipse is not pixels yet when it is turned. It is sampled, and sampling
// it at an angle costs two multiplications and loses nothing.
//
// These are the properties that say so.

func filled(t *testing.T, oval Oval) map[image.Point]bool {
	t.Helper()
	frame, err := NewFrame(64, 64, InkWhite)
	if err != nil {
		t.Fatal(err)
	}
	NewCanvas(frame).FillEllipse(oval, InkBlack)
	set := map[image.Point]bool{}
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			if ink, _ := frame.InkAt(x, y); ink == InkBlack {
				set[image.Pt(x, y)] = true
			}
		}
	}
	return set
}

// A circle is the same circle whichever way it is turned, because it has no
// long axis to turn. This is the property that catches a rotation applied to
// the wrong thing: get the arithmetic backwards and a turned circle drifts.
func TestTurningACircleChangesNothing(t *testing.T) {
	box := image.Rect(10, 10, 41, 41)
	upright := filled(t, Upright(box))
	if len(upright) == 0 {
		t.Fatal("the circle drew nothing")
	}
	for _, angle := range []float64{15, 37, 45, 90, 123.5, 180, -60} {
		turned := filled(t, Oval{Bounds: box, Rotation: angle})
		if len(turned) != len(upright) {
			t.Errorf("at %g degrees the circle covers %d pixels, upright it covers %d",
				angle, len(turned), len(upright))
			continue
		}
		for point := range upright {
			if !turned[point] {
				t.Errorf("at %g degrees the circle lost %v", angle, point)
				break
			}
		}
	}
}

// A quarter turn swaps an ellipse's axes, so an ellipse turned by one covers
// exactly what the ellipse with its axes already swapped covers. Both answers
// are worked out by different routes and have to agree.
func TestAQuarterTurnIsTheSameAsSwappingTheAxes(t *testing.T) {
	turned := filled(t, Oval{Bounds: image.Rect(12, 20, 53, 41), Rotation: 90})
	swapped := filled(t, Upright(image.Rect(22, 10, 43, 51)))
	if len(turned) != len(swapped) {
		t.Fatalf("turned covers %d pixels, swapped covers %d", len(turned), len(swapped))
	}
	for point := range swapped {
		if !turned[point] {
			t.Fatalf("turning by a quarter and swapping the axes disagree at %v", point)
		}
	}
}

// Half a turn leaves an ellipse where it was, because it is symmetric about
// both axes. Anything else means the centre moved.
func TestHalfATurnLeavesAnEllipseWhereItWas(t *testing.T) {
	box := image.Rect(8, 16, 49, 37)
	upright := filled(t, Upright(box))
	for _, angle := range []float64{180, -180, 360} {
		turned := filled(t, Oval{Bounds: box, Rotation: angle})
		if len(turned) != len(upright) {
			t.Errorf("at %g degrees it covers %d pixels, at rest %d", angle, len(turned), len(upright))
		}
	}
}

// An ellipse turned out of square reaches outside the box it was stated in,
// the way a circle reaches outside a box its centre sits at the edge of. How
// far is the extent, and it is what the report and the clip are worked out
// from — so it has to hold every pixel that was actually drawn.
func TestTheExtentHoldsEverythingDrawn(t *testing.T) {
	for _, angle := range []float64{0, 20, 45, 70, 135, -30} {
		oval := Oval{Bounds: image.Rect(16, 24, 49, 41), Rotation: angle}
		extent := oval.Extent()
		for point := range filled(t, oval) {
			if !point.In(extent) {
				t.Fatalf("at %g degrees %v was drawn outside the extent %v", angle, point, extent)
			}
		}
		if angle == 0 && extent != oval.Bounds {
			t.Errorf("an untured ellipse's extent is %v, not its own box %v", extent, oval.Bounds)
		}
	}
}

// The longest an ellipse can be is twice its longer radius, whichever way it
// is turned. A rotation that stretched or shrank it would show up here.
func TestTurningDoesNotChangeHowLargeAnEllipseIs(t *testing.T) {
	box := image.Rect(0, 0, 41, 21)
	oval := Upright(box)
	radiusX, radiusY := oval.radii()
	longest := 2 * math.Max(radiusX, radiusY)
	for angle := 0.0; angle < 360; angle += 7.5 {
		extent := Oval{Bounds: box, Rotation: angle}.Extent()
		for _, span := range []int{extent.Dx(), extent.Dy()} {
			if float64(span) > longest+2 {
				t.Errorf("at %g degrees a span is %d, and the ellipse is only %g long",
					angle, span, longest)
			}
		}
	}
}

// The sampled outline and the filled area are two different pieces of
// arithmetic — one walks the parametric angle, the other tests every pixel —
// and a turn has to move both the same way. If only one of them learned about
// rotation, an outline would sit beside its fill instead of on it.
func TestTheOutlineAndTheFillAgreeWhenTurned(t *testing.T) {
	oval := Oval{Bounds: image.Rect(8, 12, 57, 33), Rotation: 35}
	inside := filled(t, oval)
	points, closed := ellipseArcPoints(oval, 0, 360)
	if !closed || len(points) < 8 {
		t.Fatalf("the outline came back as %d points, closed=%v", len(points), closed)
	}
	for _, point := range points {
		if inside[point] {
			continue
		}
		// A sample may round to the pixel just outside the filled area; one
		// step in any direction has to be inside it.
		near := false
		for dy := -1; dy <= 1 && !near; dy++ {
			for dx := -1; dx <= 1 && !near; dx++ {
				near = inside[point.Add(image.Pt(dx, dy))]
			}
		}
		if !near {
			t.Fatalf("the outline passes through %v, which the fill does not reach", point)
		}
	}
}

// Turning an arc turns the ellipse it runs on and the angles are then measured
// around that, which is what SVG's own arc command means by its rotation. On a
// circle the two are interchangeable, and that is worth pinning: it is the one
// case where a rotation can be folded into the start angle instead.
func TestOnACircleTurningAndStartingLaterAreTheSame(t *testing.T) {
	box := image.Rect(4, 4, 45, 45)
	stroke := StrokeStyle{Ink: InkBlack, Width: 1}

	turned, err := NewFrame(64, 64, InkWhite)
	if err != nil {
		t.Fatal(err)
	}
	NewCanvas(turned).DrawArc(Oval{Bounds: box, Rotation: 30}, 0, 120, stroke)

	later, err := NewFrame(64, 64, InkWhite)
	if err != nil {
		t.Fatal(err)
	}
	NewCanvas(later).DrawArc(Upright(box), 30, 120, stroke)

	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			a, _ := turned.InkAt(x, y)
			b, _ := later.InkAt(x, y)
			if a != b {
				t.Fatalf("at %d,%d a turned circle's arc and a later-starting one differ", x, y)
			}
		}
	}
}
