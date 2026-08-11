package display

import (
	"image"
	"strconv"
	"testing"
)

func TestDrawLineUsesRequestedWidthAndClip(t *testing.T) {
	frame := newTestFrame(t, 10, 7)
	canvas := NewCanvas(frame).Clip(image.Rect(2, 1, 8, 6))
	canvas.DrawLine(image.Pt(1, 3), image.Pt(8, 3), StrokeStyle{Ink: InkRed, Width: 3})

	assertInk(t, frame, 1, 3, InkWhite)
	assertInk(t, frame, 2, 2, InkRed)
	assertInk(t, frame, 7, 4, InkRed)
	assertInk(t, frame, 8, 3, InkWhite)
	assertInk(t, frame, 4, 1, InkWhite)
}

func TestDashedLinePatternAndOffset(t *testing.T) {
	frame := newTestFrame(t, 12, 5)
	canvas := NewCanvas(frame)
	canvas.DrawLine(image.Pt(1, 1), image.Pt(10, 1), StrokeStyle{
		Ink: InkBlack, Width: 1, Dash: []int{2, 2},
	})
	canvas.DrawLine(image.Pt(1, 3), image.Pt(10, 3), StrokeStyle{
		Ink: InkRed, Width: 1, Dash: []int{2}, DashOffset: 1,
	})

	assertInk(t, frame, 1, 1, InkBlack)
	assertInk(t, frame, 2, 1, InkBlack)
	assertInk(t, frame, 3, 1, InkWhite)
	assertInk(t, frame, 5, 1, InkBlack)
	assertInk(t, frame, 1, 3, InkRed)
	assertInk(t, frame, 2, 3, InkWhite)
	assertInk(t, frame, 3, 3, InkWhite)
	assertInk(t, frame, 4, 3, InkRed)
}

func TestDashContinuesAcrossPolylineSegments(t *testing.T) {
	frame := newTestFrame(t, 7, 7)
	NewCanvas(frame).DrawPolyline([]image.Point{
		image.Pt(1, 1), image.Pt(4, 1), image.Pt(4, 4),
	}, StrokeStyle{Ink: InkBlack, Width: 1, Dash: []int{3, 2}})

	assertInk(t, frame, 3, 1, InkBlack)
	assertInk(t, frame, 4, 1, InkWhite)
	assertInk(t, frame, 4, 2, InkWhite)
	assertInk(t, frame, 4, 3, InkBlack)
}

func TestDashedRectUsesSameStrokePattern(t *testing.T) {
	frame := newTestFrame(t, 10, 8)
	NewCanvas(frame).StrokeRect(image.Rect(1, 1, 9, 7), StrokeStyle{
		Ink: InkRed, Width: 1, Dash: []int{2, 2},
	})

	assertInk(t, frame, 1, 1, InkRed)
	assertInk(t, frame, 2, 1, InkRed)
	assertInk(t, frame, 3, 1, InkWhite)
	assertInk(t, frame, 8, 2, InkRed)
}

func TestCircleFillAndStroke(t *testing.T) {
	frame := newTestFrame(t, 15, 7)
	canvas := NewCanvas(frame)
	canvas.FillCircle(image.Pt(3, 3), 2, InkBlack)
	canvas.StrokeCircle(image.Pt(11, 3), 2, StrokeStyle{Ink: InkRed, Width: 1})

	assertInk(t, frame, 3, 3, InkBlack)
	assertInk(t, frame, 1, 3, InkBlack)
	assertInk(t, frame, 1, 1, InkWhite)
	assertInk(t, frame, 11, 3, InkWhite)
	assertInk(t, frame, 9, 3, InkRed)
	assertInk(t, frame, 13, 3, InkRed)
}

func TestCircleRadiusOneIsPixelCross(t *testing.T) {
	frame := newTestFrame(t, 5, 5)
	NewCanvas(frame).FillCircle(image.Pt(2, 2), 1, InkBlack)

	assertInk(t, frame, 2, 2, InkBlack)
	assertInk(t, frame, 1, 2, InkBlack)
	assertInk(t, frame, 2, 1, InkBlack)
	assertInk(t, frame, 1, 1, InkWhite)
}

func TestCircleStrokeSupportsExactWidthsOneThroughSix(t *testing.T) {
	const radius = 14
	center := image.Pt(15, 15)
	for width := 1; width <= 6; width++ {
		t.Run(strconv.Itoa(width)+"px", func(t *testing.T) {
			frame := newTestFrame(t, 31, 31)
			NewCanvas(frame).StrokeCircle(center, radius, StrokeStyle{Ink: InkBlack, Width: width})

			for offset := 0; offset < width; offset++ {
				assertInk(t, frame, center.X+radius-offset, center.Y, InkBlack)
			}
			assertInk(t, frame, center.X+radius-width, center.Y, InkWhite)
			assertInk(t, frame, center.X, center.Y, InkWhite)
			assertInkConnected(t, frame, InkBlack)
		})
	}
}

func TestEllipseUsesHalfOpenBounds(t *testing.T) {
	frame := newTestFrame(t, 10, 7)
	canvas := NewCanvas(frame)
	canvas.FillEllipse(image.Rect(1, 1, 9, 6), InkBlack)

	assertInk(t, frame, 4, 1, InkBlack)
	assertInk(t, frame, 1, 1, InkWhite)
	assertInk(t, frame, 8, 5, InkWhite)
	assertInk(t, frame, 9, 3, InkWhite)
}

func TestRoundRectFillAndStroke(t *testing.T) {
	frame := newTestFrame(t, 18, 8)
	canvas := NewCanvas(frame)
	canvas.FillRoundRect(image.Rect(0, 0, 8, 8), 3, InkBlack)
	canvas.StrokeRoundRect(image.Rect(10, 0, 18, 8), 3, StrokeStyle{Ink: InkRed, Width: 1})

	assertInk(t, frame, 0, 0, InkWhite)
	assertInk(t, frame, 3, 0, InkBlack)
	assertInk(t, frame, 0, 3, InkBlack)
	assertInk(t, frame, 10, 0, InkWhite)
	assertInk(t, frame, 13, 0, InkRed)
	assertInk(t, frame, 14, 4, InkWhite)
}

func TestPolylineDoesNotCloseButPolygonDoes(t *testing.T) {
	points := []image.Point{image.Pt(1, 1), image.Pt(5, 1), image.Pt(5, 5)}
	stroke := StrokeStyle{Ink: InkBlack, Width: 1}

	openFrame := newTestFrame(t, 7, 7)
	NewCanvas(openFrame).DrawPolyline(points, stroke)
	assertInk(t, openFrame, 3, 3, InkWhite)

	closedFrame := newTestFrame(t, 7, 7)
	NewCanvas(closedFrame).StrokePolygon(points, stroke)
	assertInk(t, closedFrame, 3, 3, InkBlack)
}

func TestFillPolygonUsesEvenOddRuleAndIncludesBoundary(t *testing.T) {
	frame := newTestFrame(t, 8, 7)
	points := []image.Point{image.Pt(1, 1), image.Pt(6, 1), image.Pt(3, 5)}
	NewCanvas(frame).FillPolygon(points, InkRed)

	assertInk(t, frame, 1, 1, InkRed)
	assertInk(t, frame, 3, 3, InkRed)
	assertInk(t, frame, 0, 2, InkWhite)
	assertInk(t, frame, 6, 4, InkWhite)
}

func TestFillConcavePolygonLeavesNotchEmpty(t *testing.T) {
	frame := newTestFrame(t, 9, 9)
	points := []image.Point{
		image.Pt(1, 1), image.Pt(7, 1), image.Pt(7, 7), image.Pt(5, 7),
		image.Pt(5, 3), image.Pt(3, 3), image.Pt(3, 7), image.Pt(1, 7),
	}
	NewCanvas(frame).FillPolygon(points, InkBlack)

	assertInk(t, frame, 2, 5, InkBlack)
	assertInk(t, frame, 4, 5, InkWhite)
	assertInk(t, frame, 6, 5, InkBlack)
}

func TestStrokeWiderThanShapeFillsInterior(t *testing.T) {
	frame := newTestFrame(t, 12, 5)
	canvas := NewCanvas(frame)
	stroke := StrokeStyle{Ink: InkRed, Width: 8}
	canvas.StrokeRect(image.Rect(0, 0, 5, 5), stroke)
	canvas.StrokeEllipse(image.Rect(7, 0, 12, 5), stroke)

	assertInk(t, frame, 2, 2, InkRed)
	assertInk(t, frame, 9, 2, InkRed)
}

func TestInvalidShapesAreNoOps(t *testing.T) {
	frame := newTestFrame(t, 5, 5)
	canvas := NewCanvas(frame)
	canvas.DrawLine(image.Pt(0, 0), image.Pt(4, 4), StrokeStyle{Ink: InkBlack})
	canvas.FillCircle(image.Pt(2, 2), -1, InkBlack)
	canvas.FillPolygon([]image.Point{image.Pt(1, 1), image.Pt(2, 2)}, InkBlack)
	canvas.StrokePolygon([]image.Point{image.Pt(1, 1), image.Pt(2, 2)}, StrokeStyle{Ink: InkBlack, Width: 1})
	canvas.DrawLine(image.Pt(0, 0), image.Pt(4, 0), StrokeStyle{Ink: InkBlack, Width: 1, Dash: []int{2, 0}})

	for y := 0; y < frame.Height(); y++ {
		for x := 0; x < frame.Width(); x++ {
			assertInk(t, frame, x, y, InkWhite)
		}
	}
}
