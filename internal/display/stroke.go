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

func (c *Canvas) fillBrush(center image.Point, stroke StrokeStyle) {
	offset := stroke.Width / 2
	minPoint := center.Sub(image.Pt(offset, offset))
	c.FillRect(image.Rectangle{Min: minPoint, Max: minPoint.Add(image.Pt(stroke.Width, stroke.Width))}, stroke.Ink)
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
