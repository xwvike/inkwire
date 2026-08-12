package display

import (
	"image"
	"math"
	"slices"
)

type pathVerb uint8

const (
	pathMove pathVerb = iota
	pathLine
	pathQuadratic
	pathCubic
	pathClose
)

type pathCommand struct {
	verb   pathVerb
	points [3]image.Point
}

// Path is an integer-coordinate outline made of lines and Bezier curves.
// Curves are flattened to the display's pixel grid when drawn.
type Path struct {
	commands   []pathCommand
	current    image.Point
	start      image.Point
	hasCurrent bool
	afterClose bool
}

// MoveTo starts a new contour at point.
func (p *Path) MoveTo(point image.Point) {
	p.commands = append(p.commands, pathCommand{verb: pathMove, points: [3]image.Point{point}})
	p.current = point
	p.start = point
	p.hasCurrent = true
	p.afterClose = false
}

// LineTo appends a straight segment. On an empty Path it starts a contour at point.
func (p *Path) LineTo(point image.Point) {
	if !p.ensureCurrent(point) {
		return
	}
	p.commands = append(p.commands, pathCommand{verb: pathLine, points: [3]image.Point{point}})
	p.current = point
}

// QuadraticTo appends a quadratic Bezier segment ending at point.
func (p *Path) QuadraticTo(control, point image.Point) {
	if !p.ensureCurrent(point) {
		return
	}
	p.commands = append(p.commands, pathCommand{verb: pathQuadratic, points: [3]image.Point{control, point}})
	p.current = point
}

// CubicTo appends a cubic Bezier segment ending at point.
func (p *Path) CubicTo(control1, control2, point image.Point) {
	if !p.ensureCurrent(point) {
		return
	}
	p.commands = append(p.commands, pathCommand{verb: pathCubic, points: [3]image.Point{control1, control2, point}})
	p.current = point
}

// Arc appends an elliptical arc using the same clockwise degree convention as DrawArc.
func (p *Path) Arc(bounds image.Rectangle, startDegrees, sweepDegrees float64) {
	points, closed := ellipseArcPoints(bounds, startDegrees, sweepDegrees)
	if len(points) == 0 {
		return
	}
	if !p.hasCurrent || p.afterClose {
		p.MoveTo(points[0])
	} else if p.current != points[0] {
		p.LineTo(points[0])
	}
	for _, point := range points[1:] {
		p.LineTo(point)
	}
	if closed && p.current != points[0] {
		p.LineTo(points[0])
	}
}

// Close joins the current contour to its starting point.
func (p *Path) Close() {
	if !p.hasCurrent || p.afterClose {
		return
	}
	p.commands = append(p.commands, pathCommand{verb: pathClose})
	p.current = p.start
	p.afterClose = true
}

// Reset removes all contours from p.
func (p *Path) Reset() {
	*p = Path{}
}

// Empty reports whether p contains no commands.
func (p Path) Empty() bool {
	return len(p.commands) == 0
}

// Clone returns an independent copy of p.
func (p Path) Clone() Path {
	p.commands = slices.Clone(p.commands)
	return p
}

// Bounds returns the half-open bounds of p after curve flattening.
func (p Path) Bounds() image.Rectangle {
	return contourBounds(p.flatten())
}

func (p *Path) ensureCurrent(point image.Point) bool {
	if !p.hasCurrent {
		p.MoveTo(point)
		return false
	}
	if p.afterClose {
		p.MoveTo(p.current)
	}
	return true
}

type pathContour struct {
	points []image.Point
	closed bool
}

func (p Path) flatten() []pathContour {
	var contours []pathContour
	currentContour := -1
	var current image.Point
	for _, command := range p.commands {
		switch command.verb {
		case pathMove:
			contours = append(contours, pathContour{points: []image.Point{command.points[0]}})
			currentContour = len(contours) - 1
			current = command.points[0]
		case pathLine:
			if currentContour >= 0 {
				contours[currentContour].points = appendDistinctPoint(contours[currentContour].points, command.points[0])
				current = command.points[0]
			}
		case pathQuadratic:
			if currentContour >= 0 {
				points := flattenQuadratic(current, command.points[0], command.points[1])
				for _, point := range points[1:] {
					contours[currentContour].points = appendDistinctPoint(contours[currentContour].points, point)
				}
				current = command.points[1]
			}
		case pathCubic:
			if currentContour >= 0 {
				points := flattenCubic(current, command.points[0], command.points[1], command.points[2])
				for _, point := range points[1:] {
					contours[currentContour].points = appendDistinctPoint(contours[currentContour].points, point)
				}
				current = command.points[2]
			}
		case pathClose:
			if currentContour >= 0 {
				contours[currentContour].closed = true
				currentContour = -1
			}
		}
	}
	return contours
}

// StrokePath draws every contour in path. Contours that end with Close are
// closed regions, so they are stroked inward and stay inside the area the
// matching FillPath covers; open contours have no inside and stay centred on
// the line, which is also what DrawLine and DrawPolyline do.
func (c *Canvas) StrokePath(path Path, stroke StrokeStyle) {
	if !stroke.valid() {
		return
	}
	var closed []pathContour
	for _, contour := range path.flatten() {
		switch {
		case len(contour.points) < 2:
		case contour.closed && len(contour.points) >= 3:
			closed = append(closed, contour)
		default:
			c.strokePoints(contour.points, false, stroke)
		}
	}
	if len(closed) == 0 {
		return
	}
	outlines := make([][]image.Point, len(closed))
	for index, contour := range closed {
		outlines[index] = contour.points
	}
	// Nesting is resolved together under the even-odd rule, so a contour inside
	// another strokes into the ring between them rather than across the hole.
	c.strokeInward(contourBounds(closed), func(x, y int) bool {
		return pointInPath(image.Pt(x, y), closed)
	}, outlines, stroke)
}

// FillPath fills all contours together using the even-odd rule. Open contours
// are implicitly closed, allowing nested contours to leave unpainted holes.
func (c *Canvas) FillPath(path Path, ink Ink) {
	if !ink.valid() {
		return
	}
	contours := path.flatten()
	bounds := contourBounds(contours).Intersect(c.logicalClip())
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			if pointInPath(image.Pt(x, y), contours) {
				c.Set(x, y, ink)
			}
		}
	}
}

func flattenQuadratic(from, control, to image.Point) []image.Point {
	steps := curveSteps(from, control, to)
	points := make([]image.Point, 0, steps+1)
	for step := 0; step <= steps; step++ {
		t := float64(step) / float64(steps)
		u := 1 - t
		point := image.Pt(
			int(math.Round(u*u*float64(from.X)+2*u*t*float64(control.X)+t*t*float64(to.X))),
			int(math.Round(u*u*float64(from.Y)+2*u*t*float64(control.Y)+t*t*float64(to.Y))),
		)
		points = appendDistinctPoint(points, point)
	}
	return points
}

func flattenCubic(from, control1, control2, to image.Point) []image.Point {
	steps := curveSteps(from, control1, control2, to)
	points := make([]image.Point, 0, steps+1)
	for step := 0; step <= steps; step++ {
		t := float64(step) / float64(steps)
		u := 1 - t
		point := image.Pt(
			int(math.Round(u*u*u*float64(from.X)+3*u*u*t*float64(control1.X)+3*u*t*t*float64(control2.X)+t*t*t*float64(to.X))),
			int(math.Round(u*u*u*float64(from.Y)+3*u*u*t*float64(control1.Y)+3*u*t*t*float64(control2.Y)+t*t*t*float64(to.Y))),
		)
		points = appendDistinctPoint(points, point)
	}
	return points
}

func curveSteps(points ...image.Point) int {
	length := 0.0
	for i := 1; i < len(points); i++ {
		dx := float64(points[i].X) - float64(points[i-1].X)
		dy := float64(points[i].Y) - float64(points[i-1].Y)
		length += math.Hypot(dx, dy)
	}
	return min(4096, max(1, int(math.Ceil(length))))
}

func contourBounds(contours []pathContour) image.Rectangle {
	var bounds image.Rectangle
	hasPoint := false
	for _, contour := range contours {
		for _, point := range contour.points {
			if !hasPoint {
				bounds = image.Rect(point.X, point.Y, point.X+1, point.Y+1)
				hasPoint = true
				continue
			}
			bounds.Min.X = min(bounds.Min.X, point.X)
			bounds.Min.Y = min(bounds.Min.Y, point.Y)
			bounds.Max.X = max(bounds.Max.X, point.X+1)
			bounds.Max.Y = max(bounds.Max.Y, point.Y+1)
		}
	}
	return bounds
}

func pointInPath(point image.Point, contours []pathContour) bool {
	inside := false
	for _, contour := range contours {
		if len(contour.points) < 3 {
			continue
		}
		inContour, boundary := classifyPointInPolygon(point, contour.points)
		if boundary {
			return true
		}
		if inContour {
			inside = !inside
		}
	}
	return inside
}
