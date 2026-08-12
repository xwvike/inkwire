package display

import (
	"image"
	"math"
	"testing"
)

// A dash is specified as a length, so it has to stay that length whatever
// angle the line runs at. Counting raster steps instead of distance stretched
// a run by up to sqrt(2), and nothing but the reference images noticed.
func TestDashRunsKeepTheirLengthAtEveryAngle(t *testing.T) {
	// A run can only end on a pixel, and a diagonal step covers sqrt(2), so the
	// achievable lengths are quantised that coarsely. Counting steps instead
	// would put the 45 degree run at 8*sqrt(2) = 11.3px, well outside.
	const length, requested, tolerance = 60, 8.0, 1.5
	stroke := StrokeStyle{Ink: InkBlack, Width: 1, Dash: []int{8, 5}}

	for _, degrees := range []float64{0, 15, 22.5, 30, 45, 60, 67.5, 75, 90} {
		frame := newTestFrame(t, 64, 64)
		radians := degrees * math.Pi / 180
		from := image.Pt(6, 54)
		to := image.Pt(
			from.X+int(math.Round(length*math.Cos(radians))),
			from.Y-int(math.Round(length*math.Sin(radians))),
		)
		NewCanvas(frame).DrawLine(from, to, stroke)

		// Walk exactly the pixels the stroke visits. Sampling the ideal line
		// instead would hit staircase pixels it never painted and split a
		// single run into several.
		var runs []float64
		var start image.Point
		lit := 0
		closeRun := func(end image.Point) {
			if lit > 0 {
				runs = append(runs, math.Hypot(float64(end.X-start.X), float64(end.Y-start.Y)))
				lit = 0
			}
		}
		last := from
		drawBresenham(from, to, func(point image.Point) {
			last = point
			if ink, _ := frame.InkAt(point.X, point.Y); ink == InkBlack {
				if lit == 0 {
					start = point
				}
				lit++
				return
			}
			closeRun(point)
		})
		closeRun(last)

		if len(runs) < 3 {
			t.Fatalf("%.1f degrees: only %d dash runs, too few to judge", degrees, len(runs))
		}
		// Drop the final run, which the end of the line can cut short.
		for _, run := range runs[:len(runs)-1] {
			if math.Abs(run-requested) > tolerance {
				t.Errorf("%.1f degrees: a dash run measured %.2fpx, want %.0f within %.1f of grid quantisation",
					degrees, run, requested, tolerance)
			}
		}
	}
}

// The same guarantee for a closed shape, which reaches it by projecting band
// pixels onto the outline rather than by walking it.
//
// The share of lit pixels is not the measure to use here: both the lit length
// and the total length scale by sqrt(2) together, so a step-counted dash gives
// the same share as a distance-measured one. What separates them is how long
// each run actually is.
func TestDashedPolygonRunsAreEvenAroundSlantedEdges(t *testing.T) {
	const requested, tolerance = 6.0, 1.5
	frame := newTestFrame(t, 80, 80)
	// A right triangle whose hypotenuse runs along x == y.
	points := []image.Point{{X: 10, Y: 10}, {X: 70, Y: 10}, {X: 70, Y: 70}}
	NewCanvas(frame).StrokePolygon(points, StrokeStyle{Ink: InkBlack, Width: 1, Dash: []int{6, 5}})

	measure := func(name string, step float64, at func(int) image.Point) {
		var runs []float64
		lit := 0
		for offset := 2; offset < 58; offset++ {
			point := at(offset)
			if ink, _ := frame.InkAt(point.X, point.Y); ink == InkBlack {
				lit++
				continue
			}
			if lit > 0 {
				runs = append(runs, float64(lit)*step)
				lit = 0
			}
		}
		// The first run starts wherever the walk began and the last is cut off
		// by where it ended, so neither says anything about the pattern.
		if len(runs) < 4 {
			t.Fatalf("%s edge produced %d dash runs, too few to judge", name, len(runs))
		}
		for _, run := range runs[1 : len(runs)-1] {
			if math.Abs(run-requested) > tolerance {
				t.Errorf("%s edge: a dash run measured %.2fpx, want %.0f within %.1f", name, run, requested, tolerance)
			}
		}
	}
	measure("horizontal", 1, func(offset int) image.Point { return image.Pt(10+offset, 10) })
	measure("diagonal", math.Sqrt2, func(offset int) image.Point { return image.Pt(70-offset, 70-offset) })
}
