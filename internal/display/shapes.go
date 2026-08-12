package display

import "image"

// StrokeRect draws the stroke inside rect.
func (c *Canvas) StrokeRect(rect image.Rectangle, stroke StrokeStyle) {
	if !stroke.valid() || rect.Empty() {
		return
	}
	if len(stroke.Dash) > 0 {
		center := strokeCenterRect(rect, stroke.Width)
		if center.Empty() {
			return
		}
		c.strokePoints([]image.Point{
			center.Min,
			image.Pt(center.Max.X-1, center.Min.Y),
			center.Max.Sub(image.Pt(1, 1)),
			image.Pt(center.Min.X, center.Max.Y-1),
		}, true, stroke)
		return
	}
	width := min(stroke.Width, min(rect.Dx(), rect.Dy()))
	c.FillRect(image.Rect(rect.Min.X, rect.Min.Y, rect.Max.X, rect.Min.Y+width), stroke.Ink)
	c.FillRect(image.Rect(rect.Min.X, rect.Max.Y-width, rect.Max.X, rect.Max.Y), stroke.Ink)
	c.FillRect(image.Rect(rect.Min.X, rect.Min.Y+width, rect.Min.X+width, rect.Max.Y-width), stroke.Ink)
	c.FillRect(image.Rect(rect.Max.X-width, rect.Min.Y+width, rect.Max.X, rect.Max.Y-width), stroke.Ink)
}

// DrawLine uses integer Bresenham rasterization and includes both endpoints.
// Thick lines use a square brush, giving them deterministic square caps.
func (c *Canvas) DrawLine(from, to image.Point, stroke StrokeStyle) {
	if !stroke.valid() {
		return
	}
	c.strokePoints([]image.Point{from, to}, false, stroke)
}

// DrawPolyline connects each adjacent pair of points without closing the path.
func (c *Canvas) DrawPolyline(points []image.Point, stroke StrokeStyle) {
	if !stroke.valid() || len(points) < 2 {
		return
	}
	c.strokePoints(points, false, stroke)
}

// StrokePolygon connects the final point back to the first point. The polygon
// is closed, so the stroke is drawn inside it and never paints outside the
// matching FillPolygon.
func (c *Canvas) StrokePolygon(points []image.Point, stroke StrokeStyle) {
	if !stroke.valid() || len(points) < 3 {
		return
	}
	c.strokeInward(polygonBounds(points), func(x, y int) bool {
		return pointInPolygon(image.Pt(x, y), points)
	}, [][]image.Point{points}, stroke)
}

// FillPolygon fills a simple polygon using the even-odd rule.
func (c *Canvas) FillPolygon(points []image.Point, ink Ink) {
	if !ink.valid() || len(points) < 3 {
		return
	}
	bounds := polygonBounds(points).Intersect(c.logicalClip())
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			if pointInPolygon(image.Pt(x, y), points) {
				c.Set(x, y, ink)
			}
		}
	}
}

// FillCircle fills a circle whose center and perimeter pixels are included.
func (c *Canvas) FillCircle(center image.Point, radius int, ink Ink) {
	if radius < 0 || !ink.valid() {
		return
	}
	drawBounds := circleBounds(center, radius).Intersect(c.logicalClip())
	for y := drawBounds.Min.Y; y < drawBounds.Max.Y; y++ {
		for x := drawBounds.Min.X; x < drawBounds.Max.X; x++ {
			if pointInCircle(x, y, center, radius) {
				c.Set(x, y, ink)
			}
		}
	}
}

// StrokeCircle draws the stroke inside the circle's bounding box.
func (c *Canvas) StrokeCircle(center image.Point, radius int, stroke StrokeStyle) {
	if radius < 0 || !stroke.valid() {
		return
	}
	if len(stroke.Dash) > 0 {
		centerRadius := max(0, radius-stroke.Width/2)
		c.DrawArc(circleBounds(center, centerRadius), 0, 360, stroke)
		return
	}
	innerRadius := radius - stroke.Width
	drawBounds := circleBounds(center, radius).Intersect(c.logicalClip())
	for y := drawBounds.Min.Y; y < drawBounds.Max.Y; y++ {
		for x := drawBounds.Min.X; x < drawBounds.Max.X; x++ {
			if pointInCircle(x, y, center, radius) &&
				(innerRadius < 0 || !pointInCircle(x, y, center, innerRadius)) {
				c.Set(x, y, stroke.Ink)
			}
		}
	}
}

// FillEllipse fills the pixels whose centers lie inside bounds' ellipse.
func (c *Canvas) FillEllipse(bounds image.Rectangle, ink Ink) {
	if !ink.valid() || bounds.Empty() {
		return
	}
	drawBounds := bounds.Intersect(c.logicalClip())
	for y := drawBounds.Min.Y; y < drawBounds.Max.Y; y++ {
		for x := drawBounds.Min.X; x < drawBounds.Max.X; x++ {
			if pointInEllipse(x, y, bounds) {
				c.Set(x, y, ink)
			}
		}
	}
}

// StrokeEllipse draws the stroke inside bounds.
func (c *Canvas) StrokeEllipse(bounds image.Rectangle, stroke StrokeStyle) {
	if !stroke.valid() || bounds.Empty() {
		return
	}
	if len(stroke.Dash) > 0 {
		centerBounds := insetRect(bounds, stroke.Width/2)
		if !centerBounds.Empty() {
			c.DrawArc(centerBounds, 0, 360, stroke)
		}
		return
	}
	inner := insetRect(bounds, stroke.Width)
	drawBounds := bounds.Intersect(c.logicalClip())
	for y := drawBounds.Min.Y; y < drawBounds.Max.Y; y++ {
		for x := drawBounds.Min.X; x < drawBounds.Max.X; x++ {
			if pointInEllipse(x, y, bounds) && (inner.Empty() || !pointInEllipse(x, y, inner)) {
				c.Set(x, y, stroke.Ink)
			}
		}
	}
}

// FillRoundRect fills rect with radius applied to all four corners.
func (c *Canvas) FillRoundRect(rect image.Rectangle, radius int, ink Ink) {
	if !ink.valid() || rect.Empty() {
		return
	}
	radius = clampRadius(rect, radius)
	drawBounds := rect.Intersect(c.logicalClip())
	for y := drawBounds.Min.Y; y < drawBounds.Max.Y; y++ {
		for x := drawBounds.Min.X; x < drawBounds.Max.X; x++ {
			if pointInRoundRect(x, y, rect, radius) {
				c.Set(x, y, ink)
			}
		}
	}
}

// StrokeRoundRect draws the stroke inside rect.
func (c *Canvas) StrokeRoundRect(rect image.Rectangle, radius int, stroke StrokeStyle) {
	if !stroke.valid() || rect.Empty() {
		return
	}
	radius = clampRadius(rect, radius)
	if len(stroke.Dash) > 0 {
		centerRect := strokeCenterRect(rect, stroke.Width)
		if centerRect.Empty() {
			return
		}
		centerRadius := max(0, radius-stroke.Width/2)
		c.strokePoints(roundRectPoints(centerRect, centerRadius), true, stroke)
		return
	}
	inner := insetRect(rect, stroke.Width)
	innerRadius := max(0, radius-stroke.Width)
	drawBounds := rect.Intersect(c.logicalClip())
	for y := drawBounds.Min.Y; y < drawBounds.Max.Y; y++ {
		for x := drawBounds.Min.X; x < drawBounds.Max.X; x++ {
			if pointInRoundRect(x, y, rect, radius) && (inner.Empty() || !pointInRoundRect(x, y, inner, innerRadius)) {
				c.Set(x, y, stroke.Ink)
			}
		}
	}
}
