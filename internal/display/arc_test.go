package display

import (
	"image"
	"math"
	"testing"
)

func TestDrawArcUsesClockwiseScreenAngles(t *testing.T) {
	frame := newTestFrame(t, 11, 11)
	stroke := StrokeStyle{Ink: InkBlack, Width: 1}
	NewCanvas(frame).DrawArc(image.Rect(1, 1, 10, 10), 0, 90, stroke)

	assertInk(t, frame, 9, 5, InkBlack)
	assertInk(t, frame, 5, 9, InkBlack)
	assertInk(t, frame, 5, 1, InkWhite)
}

func TestDrawArcSupportsCounterClockwiseSweep(t *testing.T) {
	frame := newTestFrame(t, 11, 11)
	stroke := StrokeStyle{Ink: InkRed, Width: 1}
	NewCanvas(frame).DrawArc(image.Rect(1, 1, 10, 10), 0, -90, stroke)

	assertInk(t, frame, 9, 5, InkRed)
	assertInk(t, frame, 5, 1, InkRed)
	assertInk(t, frame, 5, 9, InkWhite)
}

func TestFillPieAndChord(t *testing.T) {
	pie := newTestFrame(t, 11, 11)
	NewCanvas(pie).FillPie(image.Rect(1, 1, 10, 10), 0, 90, InkBlack)
	assertInk(t, pie, 7, 7, InkBlack)
	assertInk(t, pie, 3, 3, InkWhite)

	chord := newTestFrame(t, 11, 11)
	NewCanvas(chord).FillChord(image.Rect(1, 1, 10, 10), 0, 180, InkRed)
	assertInk(t, chord, 5, 7, InkRed)
	assertInk(t, chord, 5, 3, InkWhite)
}

func TestFullPieMatchesFilledEllipse(t *testing.T) {
	pie := newTestFrame(t, 11, 11)
	ellipse := newTestFrame(t, 11, 11)
	bounds := image.Rect(1, 2, 10, 9)
	NewCanvas(pie).FillPie(bounds, 37, 720, InkBlack)
	NewCanvas(ellipse).FillEllipse(bounds, InkBlack)

	for y := 0; y < pie.Height(); y++ {
		for x := 0; x < pie.Width(); x++ {
			pieInk, _ := pie.InkAt(x, y)
			ellipseInk, _ := ellipse.InkAt(x, y)
			if pieInk != ellipseInk {
				t.Fatalf("pixel (%d,%d) differs: pie=%d ellipse=%d", x, y, pieInk, ellipseInk)
			}
		}
	}
}

func TestInvalidArcIsNoOp(t *testing.T) {
	frame := newTestFrame(t, 5, 5)
	canvas := NewCanvas(frame)
	canvas.DrawArc(frame.Bounds(), math.NaN(), 90, StrokeStyle{Ink: InkBlack, Width: 1})
	canvas.FillPie(frame.Bounds(), 0, math.Inf(1), InkBlack)

	assertInk(t, frame, 4, 2, InkWhite)
	assertInk(t, frame, 2, 2, InkWhite)
}
