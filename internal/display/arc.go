package display

import (
	"image"
	"math"
)

// DrawArc strokes an elliptical arc. Zero degrees points right and positive
// sweeps move clockwise, matching screen coordinates.
// A sweep that closes the ellipse describes a region, not a line, so it is
// stroked inside bounds like StrokeEllipse. A partial sweep is an open curve
// with no inside and stays centred on the arc.
func (c *Canvas) DrawArc(oval Oval, startDegrees, sweepDegrees float64, stroke StrokeStyle) {
	if !stroke.valid() {
		return
	}
	points, closed := ellipseArcPoints(oval, startDegrees, sweepDegrees)
	if closed {
		c.StrokeEllipse(oval, stroke)
		return
	}
	if len(points) >= 1 {
		c.strokePoints(points, false, stroke)
	}
}

// FillPie fills an elliptical sector between an arc and its center.
func (c *Canvas) FillPie(oval Oval, startDegrees, sweepDegrees float64, ink Ink) {
	if !ink.valid() {
		return
	}
	points, closed := ellipseArcPoints(oval, startDegrees, sweepDegrees)
	if closed {
		c.FillEllipse(oval, ink)
		return
	}
	if len(points) == 1 {
		c.Set(points[0].X, points[0].Y, ink)
		return
	}
	if len(points) < 2 {
		return
	}
	center := oval.centrePixel()
	polygon := make([]image.Point, 1, len(points)+1)
	polygon[0] = center
	polygon = append(polygon, points...)
	c.FillPolygon(polygon, ink)
}

// FillChord fills the area between an elliptical arc and the line joining its endpoints.
func (c *Canvas) FillChord(oval Oval, startDegrees, sweepDegrees float64, ink Ink) {
	if !ink.valid() {
		return
	}
	points, closed := ellipseArcPoints(oval, startDegrees, sweepDegrees)
	if closed {
		c.FillEllipse(oval, ink)
		return
	}
	if len(points) == 1 {
		c.Set(points[0].X, points[0].Y, ink)
		return
	}
	if len(points) >= 3 {
		c.FillPolygon(points, ink)
	}
}

func ellipseArcPoints(oval Oval, startDegrees, sweepDegrees float64) ([]image.Point, bool) {
	if oval.Bounds.Empty() || invalidAngle(startDegrees) || invalidAngle(sweepDegrees) ||
		invalidAngle(oval.Rotation) || sweepDegrees == 0 {
		return nil, false
	}
	closed := math.Abs(sweepDegrees) >= 360
	if closed {
		sweepDegrees = math.Copysign(360, sweepDegrees)
	}
	points := sampleArc(oval, startDegrees, sweepDegrees)
	// Rounding a sample can put it a pixel outside the ellipse it came from,
	// so each is held inside the box the ellipse actually occupies — which is
	// the stated box while the ellipse is square to the page, and a larger one
	// once it is turned.
	extent := oval.Extent()
	for i := range points {
		points[i].X = min(max(points[i].X, extent.Min.X), extent.Max.X-1)
		points[i].Y = min(max(points[i].Y, extent.Min.Y), extent.Max.Y-1)
	}
	if closed && len(points) > 1 && points[len(points)-1] == points[0] {
		points = points[:len(points)-1]
	}
	return points, closed
}

func roundRectPoints(rect image.Rectangle, radius int) []image.Point {
	if rect.Empty() {
		return nil
	}
	radius = clampRadius(rect, radius)
	if radius == 0 {
		return []image.Point{
			rect.Min,
			image.Pt(rect.Max.X-1, rect.Min.Y),
			rect.Max.Sub(image.Pt(1, 1)),
			image.Pt(rect.Min.X, rect.Max.Y-1),
		}
	}
	centers := []image.Point{
		image.Pt(rect.Min.X+radius, rect.Min.Y+radius),
		image.Pt(rect.Max.X-1-radius, rect.Min.Y+radius),
		image.Pt(rect.Max.X-1-radius, rect.Max.Y-1-radius),
		image.Pt(rect.Min.X+radius, rect.Max.Y-1-radius),
	}
	starts := []float64{180, 270, 0, 90}
	var points []image.Point
	for i, center := range centers {
		arc := sampleArc(Upright(circleBounds(center, radius)), starts[i], 90)
		for _, point := range arc {
			points = appendDistinctPoint(points, point)
		}
	}
	return points
}

func sampleArc(oval Oval, startDegrees, sweepDegrees float64) []image.Point {
	radiusX, radiusY := oval.radii()
	length := math.Abs(sweepDegrees) * math.Pi / 180 * max(radiusX, radiusY)
	steps := max(1, int(math.Ceil(length)))
	points := make([]image.Point, 0, steps+1)
	for step := 0; step <= steps; step++ {
		x, y := oval.at(startDegrees + sweepDegrees*float64(step)/float64(steps))
		points = appendDistinctPoint(points, image.Pt(int(math.Round(x)), int(math.Round(y))))
	}
	return points
}

func invalidAngle(angle float64) bool {
	return math.IsNaN(angle) || math.IsInf(angle, 0)
}

func appendDistinctPoint(points []image.Point, point image.Point) []image.Point {
	if len(points) == 0 || points[len(points)-1] != point {
		return append(points, point)
	}
	return points
}
