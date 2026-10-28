package display

import (
	"image"
	"testing"
)

func TestPathFlattensQuadraticAndCubicCurves(t *testing.T) {
	quadraticFrame := newTestFrame(t, 12, 10)
	stroke := StrokeStyle{Ink: InkBlack, Width: 1}

	var quadratic Path
	quadratic.MoveTo(image.Pt(1, 7))
	quadratic.QuadraticTo(image.Pt(5, 1), image.Pt(9, 7))
	NewCanvas(quadraticFrame).StrokePath(quadratic, stroke)

	cubicFrame := newTestFrame(t, 12, 10)
	var cubic Path
	cubic.MoveTo(image.Pt(1, 8))
	cubic.CubicTo(image.Pt(1, 2), image.Pt(9, 2), image.Pt(9, 8))
	NewCanvas(cubicFrame).StrokePath(cubic, StrokeStyle{Ink: InkRed, Width: 1})

	assertInk(t, quadraticFrame, 1, 7, InkBlack)
	assertInk(t, quadraticFrame, 5, 4, InkBlack)
	assertInk(t, quadraticFrame, 9, 7, InkBlack)
	assertInk(t, cubicFrame, 5, 4, InkRed)
	assertInk(t, cubicFrame, 9, 8, InkRed)
}

func TestPathStrokesAZeroLengthSegment(t *testing.T) {
	frame := newTestFrame(t, 8, 8)
	var path Path
	path.MoveTo(image.Pt(3, 3))
	path.LineTo(image.Pt(3, 3))

	NewCanvas(frame).StrokePath(path, StrokeStyle{Ink: InkBlack, Width: 1})

	assertInk(t, frame, 3, 3, InkBlack)
}

func TestPathMoveOnlyDoesNotStroke(t *testing.T) {
	frame := newTestFrame(t, 8, 8)
	var path Path
	path.MoveTo(image.Pt(3, 3))

	NewCanvas(frame).StrokePath(path, StrokeStyle{Ink: InkBlack, Width: 1})

	assertInk(t, frame, 3, 3, InkWhite)
}

func TestStyledPathZeroLengthSegmentUsesItsCap(t *testing.T) {
	for _, test := range []struct {
		name string
		cap  StrokeCap
		want Ink
	}{
		{"butt", StrokeCapButt, InkWhite},
		{"round", StrokeCapRound, InkBlack},
	} {
		t.Run(test.name, func(t *testing.T) {
			frame := newTestFrame(t, 8, 8)
			var path Path
			path.MoveTo(image.Pt(3, 3))
			path.LineTo(image.Pt(3, 3))
			NewCanvas(frame).StrokePath(path, StrokeStyle{Ink: InkBlack, Width: 3, Cap: test.cap})
			assertInk(t, frame, 3, 3, test.want)
		})
	}
}

func TestStyledStrokeCapsFollowTheirGeometry(t *testing.T) {
	for _, test := range []struct {
		name string
		cap  StrokeCap
		left bool
	}{
		{"butt", StrokeCapButt, false},
		{"round", StrokeCapRound, true},
		{"square", StrokeCapSquare, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			frame := newTestFrame(t, 12, 8)
			NewCanvas(frame).DrawLine(image.Pt(4, 4), image.Pt(8, 4), StrokeStyle{
				Ink: InkBlack, Width: 2, Cap: test.cap,
			})
			ink, _ := frame.InkAt(3, 4)
			painted := ink == InkBlack
			if painted != test.left {
				t.Fatalf("pixel before endpoint painted=%v, want %v", painted, test.left)
			}
			assertInk(t, frame, 4, 4, InkBlack)
		})
	}
}

func TestStyledRoundJoinPaintsTheVertex(t *testing.T) {
	frame := newTestFrame(t, 12, 12)
	NewCanvas(frame).DrawPolyline([]image.Point{{X: 3, Y: 3}, {X: 6, Y: 6}, {X: 9, Y: 3}}, StrokeStyle{
		Ink: InkBlack, Width: 4, Cap: StrokeCapButt, Join: StrokeJoinRound,
	})
	assertInk(t, frame, 6, 6, InkBlack)
}

func TestClosedRoundJoinIsAppliedInsideTheShape(t *testing.T) {
	points := []image.Point{{X: 2, Y: 2}, {X: 10, Y: 2}, {X: 6, Y: 10}}
	miter := newTestFrame(t, 14, 14)
	round := newTestFrame(t, 14, 14)
	NewCanvas(miter).StrokePolygon(points, StrokeStyle{Ink: InkBlack, Width: 3, Join: StrokeJoinMiter})
	NewCanvas(round).StrokePolygon(points, StrokeStyle{Ink: InkBlack, Width: 3, Join: StrokeJoinRound})
	for y := 0; y < 14; y++ {
		for x := 0; x < 14; x++ {
			ink, _ := round.InkAt(x, y)
			if ink == InkBlack {
				if !pointInPolygon(image.Pt(x, y), points) {
					t.Fatalf("round join painted outside the polygon at (%d,%d)", x, y)
				}
			}
		}
	}
}

func TestStyledBevelJoinDoesNotUseTheMiterExtension(t *testing.T) {
	points := []image.Point{{X: 3, Y: 8}, {X: 6, Y: 3}, {X: 9, Y: 8}}
	miter := newTestFrame(t, 14, 12)
	bevel := newTestFrame(t, 14, 12)
	NewCanvas(miter).DrawPolyline(points, StrokeStyle{Ink: InkBlack, Width: 4, Cap: StrokeCapButt, Join: StrokeJoinMiter})
	NewCanvas(bevel).DrawPolyline(points, StrokeStyle{Ink: InkBlack, Width: 4, Cap: StrokeCapButt, Join: StrokeJoinBevel})
	miterPixels, bevelPixels := 0, 0
	for y := 0; y < 12; y++ {
		for x := 0; x < 14; x++ {
			if ink, _ := miter.InkAt(x, y); ink == InkBlack {
				miterPixels++
			}
			if ink, _ := bevel.InkAt(x, y); ink == InkBlack {
				bevelPixels++
			}
		}
	}
	if miterPixels <= bevelPixels {
		t.Fatalf("miter painted %d pixels, bevel %d; expected its extension", miterPixels, bevelPixels)
	}
}

func TestStyledRoundCapAppliesToDashRuns(t *testing.T) {
	frame := newTestFrame(t, 14, 8)
	NewCanvas(frame).DrawLine(image.Pt(2, 4), image.Pt(12, 4), StrokeStyle{
		Ink: InkBlack, Width: 2, Cap: StrokeCapRound, Dash: []int{3, 3},
	})
	// The first on-run ends at x=5, and a line of even width is drawn from
	// half its width before its points, so the run's body stops at x=4 and
	// its cap is the pixel after. Without per-run caps x=5 is in the gap.
	assertInk(t, frame, 5, 4, InkBlack)
	assertInk(t, frame, 6, 4, InkWhite)
}

func TestFillPathUsesNestedContoursAsHoles(t *testing.T) {
	frame := newTestFrame(t, 11, 11)
	var path Path
	path.MoveTo(image.Pt(1, 1))
	path.LineTo(image.Pt(9, 1))
	path.LineTo(image.Pt(9, 9))
	path.LineTo(image.Pt(1, 9))
	path.Close()
	path.MoveTo(image.Pt(3, 3))
	path.LineTo(image.Pt(7, 3))
	path.LineTo(image.Pt(7, 7))
	path.LineTo(image.Pt(3, 7))
	path.Close()
	NewCanvas(frame).FillPath(path, InkBlack)

	assertInk(t, frame, 2, 5, InkBlack)
	assertInk(t, frame, 5, 5, InkWhite)
	assertInk(t, frame, 3, 5, InkBlack)
}

func TestFillPathImplicitlyClosesOpenContourAndClips(t *testing.T) {
	frame := newTestFrame(t, 9, 9)
	var path Path
	path.MoveTo(image.Pt(0, 0))
	path.LineTo(image.Pt(8, 0))
	path.LineTo(image.Pt(4, 8))
	NewCanvas(frame).Clip(image.Rect(2, 2, 7, 7)).FillPath(path, InkRed)

	assertInk(t, frame, 4, 4, InkRed)
	assertInk(t, frame, 4, 1, InkWhite)
	assertInk(t, frame, 1, 4, InkWhite)
}

func TestPathArcBoundsCloneAndReset(t *testing.T) {
	var path Path
	path.Arc(Upright(image.Rect(2, 3, 11, 12)), 180, 180)
	clone := path.Clone()
	path.Reset()

	if !path.Empty() {
		t.Fatal("Reset did not empty path")
	}
	if clone.Empty() {
		t.Fatal("Clone shared reset state with source")
	}
	if got, want := clone.Bounds(), image.Rect(2, 3, 11, 8); got != want {
		t.Fatalf("arc bounds = %v, want %v", got, want)
	}
}

func TestFullPathArcConnectsBackToStart(t *testing.T) {
	frame := newTestFrame(t, 11, 11)
	var path Path
	path.Arc(Upright(image.Rect(1, 1, 10, 10)), 0, 360)
	NewCanvas(frame).StrokePath(path, StrokeStyle{Ink: InkBlack, Width: 1})

	assertInk(t, frame, 9, 5, InkBlack)
	assertInk(t, frame, 5, 9, InkBlack)
	assertInk(t, frame, 1, 5, InkBlack)
	assertInk(t, frame, 5, 1, InkBlack)
}

func TestLineAfterCloseStartsAtClosedContourOrigin(t *testing.T) {
	frame := newTestFrame(t, 8, 8)
	var path Path
	path.MoveTo(image.Pt(1, 1))
	path.LineTo(image.Pt(5, 1))
	path.LineTo(image.Pt(3, 4))
	path.Close()
	path.LineTo(image.Pt(1, 6))
	NewCanvas(frame).StrokePath(path, StrokeStyle{Ink: InkBlack, Width: 1})

	assertInk(t, frame, 1, 1, InkBlack)
	assertInk(t, frame, 1, 4, InkBlack)
	assertInk(t, frame, 1, 6, InkBlack)
}

// An open arc is a run of short segments turning a little at each point. The
// turn leaves a wedge on its outside that the join has to fill; filling only
// the miter's tip of it left the rest, a white pixel inside the stroke at
// every gentle turn.
func TestAnArcHasNoHolesAtItsTurns(t *testing.T) {
	frame := newTestFrame(t, 90, 60)
	var path Path
	path.MoveTo(image.Pt(10, 50))
	path.Arc(Upright(image.Rect(10, 15, 81, 86)), 180, 180)
	NewCanvas(frame).StrokePath(path, StrokeStyle{Ink: InkBlack, Width: 4, Cap: StrokeCapButt, Join: StrokeJoinMiter})
	for y := 1; y < 59; y++ {
		for x := 1; x < 89; x++ {
			if ink, _ := frame.InkAt(x, y); ink == InkBlack {
				continue
			}
			surrounded := true
			for _, step := range []image.Point{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
				if ink, _ := frame.InkAt(x+step.X, y+step.Y); ink != InkBlack {
					surrounded = false
				}
			}
			if surrounded {
				t.Fatalf("(%d,%d) is a hole in the stroke", x, y)
			}
		}
	}
}

// A line of even width has its edges between pixels whichever way it runs, so
// along the grid it is as many pixels across as it is wide, and not one more
// for the pixels its edges pass through the middle of.
func TestAnEvenWidthIsThatManyPixelsAcross(t *testing.T) {
	for _, width := range []int{2, 4, 6} {
		frame := newTestFrame(t, 20, 20)
		NewCanvas(frame).DrawLine(image.Pt(3, 10), image.Pt(16, 10), StrokeStyle{Ink: InkBlack, Width: width, Cap: StrokeCapButt})
		across := 0
		for y := 0; y < 20; y++ {
			if ink, _ := frame.InkAt(10, y); ink == InkBlack {
				across++
			}
		}
		if across != width {
			t.Errorf("a line %d wide is %d pixels across", width, across)
		}
	}
}

// A circle's flat sides are as wide at the top as at the left: the pixel on
// each axis does not stand out alone past the rest of the edge.
func TestACircleHasNoNubsOnItsAxes(t *testing.T) {
	frame := newTestFrame(t, 80, 80)
	center := image.Pt(40, 40)
	NewCanvas(frame).StrokeCircle(center, 35, StrokeStyle{Ink: InkBlack, Width: 4})
	for _, edge := range []struct {
		at, along image.Point
	}{
		{image.Pt(40, 5), image.Pt(1, 0)},
		{image.Pt(40, 75), image.Pt(1, 0)},
		{image.Pt(5, 40), image.Pt(0, 1)},
		{image.Pt(75, 40), image.Pt(0, 1)},
	} {
		for _, side := range []int{-1, 1} {
			beside := edge.at.Add(edge.along.Mul(side))
			if ink, _ := frame.InkAt(beside.X, beside.Y); ink != InkBlack {
				t.Errorf("the edge at %v stands alone; %v beside it is not drawn", edge.at, beside)
			}
		}
	}
}
