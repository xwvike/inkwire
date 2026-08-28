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
