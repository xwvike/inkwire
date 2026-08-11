package display

import "image"

// StrokeStyle describes an opaque, integer-aligned outline. Dash alternates
// on/off lengths in raster pixels; an odd-length pattern repeats before
// cycling. Non-positive widths or dash lengths make the stroke a no-op.
type StrokeStyle struct {
	Ink        Ink   // Ink is the physical color written by the stroke.
	Width      int   // Width is the square brush size in pixels.
	Dash       []int // Dash contains alternating on/off lengths in pixels.
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

func (c *Canvas) strokePoints(points []image.Point, closed bool, stroke StrokeStyle) {
	if !stroke.valid() || len(points) == 0 {
		return
	}
	dash := newDashCursor(stroke)
	if len(points) == 1 {
		if dash == nil || dash.on {
			c.fillBrush(points[0], stroke)
		}
		return
	}
	segmentCount := len(points) - 1
	if closed {
		segmentCount++
	}
	for segment := 0; segment < segmentCount; segment++ {
		from := points[segment%len(points)]
		to := points[(segment+1)%len(points)]
		skipFirst := segment > 0
		drawBresenham(from, to, func(point image.Point) {
			if skipFirst {
				skipFirst = false
				return
			}
			if dash == nil || dash.on {
				c.fillBrush(point, stroke)
			}
			if dash != nil {
				dash.advance()
			}
		})
	}
}

type dashCursor struct {
	pattern    []int
	virtualLen int
	index      int
	remaining  int
	on         bool
}

func newDashCursor(stroke StrokeStyle) *dashCursor {
	if len(stroke.Dash) == 0 {
		return nil
	}
	virtualLen := len(stroke.Dash)
	if virtualLen%2 != 0 {
		virtualLen *= 2
	}
	total := 0
	for i := 0; i < virtualLen; i++ {
		total += stroke.Dash[i%len(stroke.Dash)]
	}
	offset := stroke.DashOffset % total
	if offset < 0 {
		offset += total
	}
	cursor := &dashCursor{
		pattern:    stroke.Dash,
		virtualLen: virtualLen,
		remaining:  stroke.Dash[0],
		on:         true,
	}
	for offset >= cursor.remaining {
		offset -= cursor.remaining
		cursor.next()
	}
	cursor.remaining -= offset
	return cursor
}

func (d *dashCursor) advance() {
	d.remaining--
	if d.remaining == 0 {
		d.next()
	}
}

func (d *dashCursor) next() {
	d.index = (d.index + 1) % d.virtualLen
	d.remaining = d.pattern[d.index%len(d.pattern)]
	d.on = d.index%2 == 0
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
