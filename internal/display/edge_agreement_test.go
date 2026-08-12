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

		// Dashing used to reroute a closed shape through the brush stroker,
		// putting 46 of 86 circle pixels outside the fill. It now only selects
		// which parts of the same band are painted.
		{"Circle dashed", 0,
			func(c *Canvas) { c.FillCircle(center, radius, InkBlack) },
			func(c *Canvas) { c.StrokeCircle(center, radius, dashed) }},
		{"Ellipse dashed", 0,
			func(c *Canvas) { c.FillEllipse(box, InkBlack) },
			func(c *Canvas) { c.StrokeEllipse(box, dashed) }},

		// A full sweep closes the ellipse, so DrawArc strokes it as a region.
		{"Ellipse vs DrawArc", 0,
			func(c *Canvas) { c.FillEllipse(box, InkBlack) },
			func(c *Canvas) { c.DrawArc(box, 0, 360, thin) }},

		// Not an alignment defect and not a defect at all: DrawArc agrees
		// exactly with FillEllipse over this box, and comparing it against
		// FillCircle compares two parameterisations. A circle given as a centre
		// and a radius is measured between pixel centres; a shape given as a
		// box is measured across whole pixels, which over a (2r+1) box means a
		// radius of r+0.5. Forcing either onto the other's measure costs more
		// than it saves, so the gap is recorded rather than removed. See
		// TestCircleAndEllipseAreDifferentParameterisations.
		{"Circle vs DrawArc", 92,
			func(c *Canvas) { c.FillCircle(center, radius, InkBlack) },
			func(c *Canvas) { c.DrawArc(circleBounds(center, radius), 0, 360, thin) }},

		// Polygons have no implicit stroker, so they build the inward band by
		// eroding a mask of the fill instead. Both forms were straddling the
		// edge before that: 65 of 141 pixels solid, 39 of 85 dashed.
		{"Polygon", 0,
			func(c *Canvas) { c.FillPolygon(poly, InkBlack) },
			func(c *Canvas) { c.StrokePolygon(poly, thin) }},
		{"Polygon dashed", 0,
			func(c *Canvas) { c.FillPolygon(poly, InkBlack) },
			func(c *Canvas) { c.StrokePolygon(poly, dashed) }},
		// A closed contour is a region, so it strokes inward like a polygon.
		// It was straddling by 40 of 130 pixels solid and 23 of 78 dashed.
		{"Path", 0,
			func(c *Canvas) { c.FillPath(contour, InkBlack) },
			func(c *Canvas) { c.StrokePath(contour, thin) }},
		{"Path dashed", 0,
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

// A circle given as a centre and a radius, and an ellipse given as a box, are
// measured differently on purpose, and each measure is the right one for its
// own parameterisation. Unifying them was tried and reverted: pushing circles
// onto the box measure turns a radius of one into a filled 3x3 square, and
// pushing ellipses onto the pixel-centre measure leaves an even-sided ellipse a
// whole row short of the box it was handed. This test pins both reasons so the
// trade is not quietly re-litigated.
func TestCircleAndEllipseAreDifferentParameterisations(t *testing.T) {
	center := image.Pt(30, 30)
	for _, radius := range []int{3, 8, 20} {
		byCircle := newTestFrame(t, 64, 64)
		byEllipse := newTestFrame(t, 64, 64)
		NewCanvas(byCircle).FillCircle(center, radius, InkBlack)
		NewCanvas(byEllipse).FillEllipse(circleBounds(center, radius), InkBlack)

		// The circle is the smaller shape and sits wholly inside the ellipse,
		// so the two never disagree about a pixel, only about how far to reach.
		for y := 0; y < 64; y++ {
			for x := 0; x < 64; x++ {
				inCircle, _ := byCircle.InkAt(x, y)
				inEllipse, _ := byEllipse.InkAt(x, y)
				if inCircle == InkBlack && inEllipse != InkBlack {
					t.Fatalf("radius %d: FillCircle painted (%d,%d) but FillEllipse did not", radius, x, y)
				}
			}
		}
		if circle, ellipse := countInk(byCircle, InkBlack), countInk(byEllipse, InkBlack); ellipse <= circle {
			t.Fatalf("radius %d: expected the ellipse to be the larger shape, got %d vs %d", radius, ellipse, circle)
		}
	}

	// Why circles keep the pixel-centre measure.
	cross := newTestFrame(t, 5, 5)
	NewCanvas(cross).FillCircle(image.Pt(2, 2), 1, InkBlack)
	if got := countInk(cross, InkBlack); got != 5 {
		t.Fatalf("a radius of one covered %d pixels, want the 5-pixel cross", got)
	}

	// Why ellipses keep the whole-pixel measure: an even-sided box still has to
	// be touched on all four edges.
	box := image.Rect(1, 1, 9, 6) // 8x5, both centres between pixels on one axis
	even := newTestFrame(t, 10, 7)
	NewCanvas(even).FillEllipse(box, InkBlack)
	for _, edge := range []image.Point{{X: 4, Y: 1}, {X: 4, Y: 5}, {X: 1, Y: 3}, {X: 8, Y: 3}} {
		if ink, _ := even.InkAt(edge.X, edge.Y); ink != InkBlack {
			t.Fatalf("an 8x5 ellipse does not reach %v on the edge of its own box", edge)
		}
	}
}
