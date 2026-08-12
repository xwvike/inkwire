package display

import (
	"image"
	"testing"
)

// A shape's outline and its fill describe the same geometry, so a 1px stroke
// should never paint outside the fill. Where that already holds, this test
// keeps it holding. Where it does not, the exact overshoot is pinned so the
// number moves only on purpose.
//
// The clean pairs all derive stroke and fill from the same implicit
// point-in-shape test. Every violation goes through strokePoints, which stamps
// a brush along a Bresenham walk of the outline and therefore straddles the
// edge instead of staying inside it. Turning a dash on is enough to switch a
// closed shape from the first kind to the second.
func TestFillAndStrokeAgreeOnTheEdge(t *testing.T) {
	const size = 80
	box := image.Rect(12, 14, 66, 60)
	center, radius := image.Pt(38, 38), 24
	poly := []image.Point{{X: 20, Y: 18}, {X: 60, Y: 24}, {X: 55, Y: 58}, {X: 22, Y: 52}}

	var contour Path
	contour.MoveTo(image.Pt(20, 20))
	contour.LineTo(image.Pt(58, 26))
	contour.QuadraticTo(image.Pt(64, 48), image.Pt(40, 58))
	contour.LineTo(image.Pt(18, 46))
	contour.Close()

	thin := StrokeStyle{Ink: InkBlack, Width: 1}
	dashed := StrokeStyle{Ink: InkBlack, Width: 1, Dash: []int{3, 2}}

	for _, test := range []struct {
		name string
		// outside is how many stroke pixels currently land beyond the fill.
		// Zero is the goal; a non-zero value records a known defect.
		outside      int
		fill, stroke func(*Canvas)
	}{
		{"Rect", 0,
			func(c *Canvas) { c.FillRect(box, InkBlack) },
			func(c *Canvas) { c.StrokeRect(box, thin) }},
		{"Rect dashed", 0,
			func(c *Canvas) { c.FillRect(box, InkBlack) },
			func(c *Canvas) { c.StrokeRect(box, dashed) }},
		{"RoundRect", 0,
			func(c *Canvas) { c.FillRoundRect(box, 10, InkBlack) },
			func(c *Canvas) { c.StrokeRoundRect(box, 10, thin) }},
		{"RoundRect dashed", 0,
			func(c *Canvas) { c.FillRoundRect(box, 10, InkBlack) },
			func(c *Canvas) { c.StrokeRoundRect(box, 10, dashed) }},
		{"Circle", 0,
			func(c *Canvas) { c.FillCircle(center, radius, InkBlack) },
			func(c *Canvas) { c.StrokeCircle(center, radius, thin) }},
		{"Ellipse", 0,
			func(c *Canvas) { c.FillEllipse(box, InkBlack) },
			func(c *Canvas) { c.StrokeEllipse(box, thin) }},

		// Dashing a closed shape reroutes it through the brush stroker.
		{"Circle dashed", 46,
			func(c *Canvas) { c.FillCircle(center, radius, InkBlack) },
			func(c *Canvas) { c.StrokeCircle(center, radius, dashed) }},
		{"Ellipse dashed", 1,
			func(c *Canvas) { c.FillEllipse(box, InkBlack) },
			func(c *Canvas) { c.StrokeEllipse(box, dashed) }},

		// DrawArc samples the outline parametrically and rounds, so it does not
		// land on the implicit shape the matching fill uses.
		{"Circle vs DrawArc", 92,
			func(c *Canvas) { c.FillCircle(center, radius, InkBlack) },
			func(c *Canvas) { c.DrawArc(circleBounds(center, radius), 0, 360, thin) }},
		{"Ellipse vs DrawArc", 2,
			func(c *Canvas) { c.FillEllipse(box, InkBlack) },
			func(c *Canvas) { c.DrawArc(box, 0, 360, thin) }},

		// Polygons and paths have no implicit stroker at all, so they always
		// straddle. These are the widest gaps in the set.
		{"Polygon", 65,
			func(c *Canvas) { c.FillPolygon(poly, InkBlack) },
			func(c *Canvas) { c.StrokePolygon(poly, thin) }},
		{"Polygon dashed", 39,
			func(c *Canvas) { c.FillPolygon(poly, InkBlack) },
			func(c *Canvas) { c.StrokePolygon(poly, dashed) }},
		{"Path", 40,
			func(c *Canvas) { c.FillPath(contour, InkBlack) },
			func(c *Canvas) { c.StrokePath(contour, thin) }},
		{"Path dashed", 23,
			func(c *Canvas) { c.FillPath(contour, InkBlack) },
			func(c *Canvas) { c.StrokePath(contour, dashed) }},
	} {
		t.Run(test.name, func(t *testing.T) {
			filled := newTestFrame(t, size, size)
			stroked := newTestFrame(t, size, size)
			test.fill(NewCanvas(filled))
			test.stroke(NewCanvas(stroked))

			outside, total := 0, 0
			var first image.Point
			for y := 0; y < size; y++ {
				for x := 0; x < size; x++ {
					if ink, _ := stroked.InkAt(x, y); ink != InkBlack {
						continue
					}
					total++
					if ink, _ := filled.InkAt(x, y); ink != InkBlack {
						if outside == 0 {
							first = image.Pt(x, y)
						}
						outside++
					}
				}
			}
			if total == 0 {
				t.Fatal("the stroke painted nothing, so the case proves nothing")
			}
			if outside == test.outside {
				return
			}
			if test.outside == 0 {
				t.Fatalf("%d of %d stroke pixels now fall outside the fill, first %v", outside, total, first)
			}
			t.Fatalf("stroke pixels outside the fill = %d, pinned at %d; update the pin when this moves on purpose",
				outside, test.outside)
		})
	}
}

// FillEllipse over a square box and FillCircle describe the same circle but do
// not agree: expanding pointInEllipse for a (2r+1)-wide box gives a radius of
// r+0.5, while pointInCircle uses r. The ellipse is the one that matches the
// box it was given.
func TestCircleIsHalfAPixelSmallerThanItsBoundingBox(t *testing.T) {
	center := image.Pt(30, 30)
	for _, radius := range []int{3, 8, 20} {
		byCircle := newTestFrame(t, 64, 64)
		byEllipse := newTestFrame(t, 64, 64)
		NewCanvas(byCircle).FillCircle(center, radius, InkBlack)
		NewCanvas(byEllipse).FillEllipse(circleBounds(center, radius), InkBlack)

		for y := 0; y < 64; y++ {
			for x := 0; x < 64; x++ {
				inCircle, _ := byCircle.InkAt(x, y)
				inEllipse, _ := byEllipse.InkAt(x, y)
				if inCircle == InkBlack && inEllipse != InkBlack {
					t.Fatalf("radius %d: FillCircle painted (%d,%d) but FillEllipse did not", radius, x, y)
				}
			}
		}
		circlePixels := countInk(byCircle, InkBlack)
		ellipsePixels := countInk(byEllipse, InkBlack)
		if ellipsePixels <= circlePixels {
			t.Fatalf("radius %d: expected the ellipse to be the larger shape, got %d vs %d",
				radius, ellipsePixels, circlePixels)
		}
	}
}
