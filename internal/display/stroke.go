package display

import (
	"image"
	"math"
)

// StrokeStyle describes an opaque, integer-aligned outline. Dash alternates
// on and off runs measured as distance along the outline, so a run keeps its
// length whatever angle the line is drawn at; an odd-length pattern repeats
// before cycling. Non-positive widths or dash lengths make the stroke a no-op.
type StrokeStyle struct {
	Ink        Ink   // Ink is the physical color written by the stroke.
	Width      int   // Width is the square brush size in pixels.
	Dash       []int // Dash holds alternating on and off lengths along the outline.
	DashOffset int   // DashOffset advances into Dash before drawing.
}

func (s StrokeStyle) valid() bool {
	if s.Width <= 0 || !s.Ink.valid() {
		return false
	}
	for _, length := range s.Dash {
		if length <= 0 {
			return false
		}
	}
	return true
}

// fillBrush stamps the brush at a point already worked out on the frame.
//
// The brush is square to the page rather than turned with the line. A turned
// square differs from an upright one only at the two ends of a run, and only
// by less than the width — which at one to three pixels is nothing at all,
// where a turned brush would cost a second inverse mapping for every stamp.
func (c *Canvas) fillBrush(center image.Point, stroke StrokeStyle) {
	offset := stroke.Width / 2
	minPoint := center.Sub(image.Pt(offset, offset))
	for y := minPoint.Y; y < minPoint.Y+stroke.Width; y++ {
		for x := minPoint.X; x < minPoint.X+stroke.Width; x++ {
			c.setDevice(image.Pt(x, y), stroke.Ink)
		}
	}
}

// mapPoints puts a run of points where the transform says they go.
//
// A line is turned by turning its ends and drawing between them, not by
// turning the pixels it was drawn as. Bresenham on the moved ends steps from
// one pixel to the next by construction, so the line it draws is connected
// whatever angle it is at; moving the pixels of an already-drawn line would
// space them out and leave a dotted line behind.
func (c *Canvas) mapPoints(points []image.Point) []image.Point {
	offset, whole := c.state.matrix.Offset()
	if whole && offset == (image.Point{}) {
		return points
	}
	moved := make([]image.Point, len(points))
	for index, point := range points {
		if whole {
			moved[index] = point.Add(offset)
			continue
		}
		moved[index] = c.state.matrix.ApplyPoint(point)
	}
	return moved
}

// The dash is measured as real distance travelled along the outline, not as a
// count of raster steps. A Bresenham step is 1px along an axis but sqrt(2)
// diagonally, so counting steps stretches a dash by up to 41% as a line tilts;
// accumulating the true step length keeps a run the length it was asked for at
// every angle. Closed shapes reach the same measure through dashRegion.
func (c *Canvas) strokePoints(points []image.Point, closed bool, stroke StrokeStyle) {
	if !stroke.valid() || len(points) == 0 {
		return
	}
	points = c.mapPoints(points)
	pattern := newDashPattern(stroke)
	if len(points) == 1 {
		if pattern == nil || pattern.on(0) {
			c.fillBrush(points[0], stroke)
		}
		return
	}

	travelled := 0.0
	previous := points[0]
	segmentCount := len(points) - 1
	if closed {
		segmentCount++
	}
	for segment := 0; segment < segmentCount; segment++ {
		from := points[segment%len(points)]
		to := points[(segment+1)%len(points)]
		// The first point of a segment repeats the previous segment's last one.
		skipShared := segment > 0
		drawBresenham(from, to, func(point image.Point) {
			if skipShared {
				skipShared = false
				return
			}
			travelled += math.Hypot(float64(point.X-previous.X), float64(point.Y-previous.Y))
			previous = point
			if pattern == nil || pattern.on(travelled) {
				c.fillBrush(point, stroke)
			}
		})
	}
}

func drawBresenham(from, to image.Point, draw func(image.Point)) {
	x0, y0 := from.X, from.Y
	x1, y1 := to.X, to.Y
	dx := abs(x1 - x0)
	sx := -1
	if x0 < x1 {
		sx = 1
	}
	dy := -abs(y1 - y0)
	sy := -1
	if y0 < y1 {
		sy = 1
	}
	err := dx + dy
	for {
		draw(image.Pt(x0, y0))
		if x0 == x1 && y0 == y1 {
			return
		}
		e2 := 2 * err
		if e2 >= dy {
			err += dy
			x0 += sx
		}
		if e2 <= dx {
			err += dx
			y0 += sy
		}
	}
}
