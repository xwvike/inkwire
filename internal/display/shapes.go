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
//
// A circle given as a centre and a radius is measured between pixel centres:
// it is symmetric about the pixel at center and 2*radius+1 pixels across. That
// is what makes a radius of one the familiar five-pixel cross. It is therefore
// slightly smaller than the ellipse inscribed in the same bounding box, which
// is measured across whole pixels instead; see FillEllipse.
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

// StrokeCircle draws the stroke inside the circle's bounding box. Dashing only
// selects which parts of that band are painted, so it does not move the ring.
func (c *Canvas) StrokeCircle(center image.Point, radius int, stroke StrokeStyle) {
	if radius < 0 || !stroke.valid() {
		return
	}
	bounds := circleBounds(center, radius)
	innerRadius := radius - stroke.Width
	c.strokeBand(bounds, func(x, y int) bool {
		return pointInCircle(x, y, center, radius) &&
			(innerRadius < 0 || !pointInCircle(x, y, center, innerRadius))
	}, closedOutline(bounds), stroke)
}

// FillEllipse fills the pixels whose centers lie inside bounds' ellipse.
//
// A shape given as a box is measured across whole pixels, so the ellipse
// touches all four edges of bounds even when a side has an even length and its
// centre falls between two pixels. Measuring between pixel centres instead
// would pull an even-sided ellipse a whole row short of its own box.
//
// This is a different parameterisation from FillCircle, not a disagreement
// with it: over the bounding box of a circle of radius r this ellipse has a
// radius of r+0.5 and so is the larger of the two.
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
	inner := insetRect(bounds, stroke.Width)
	c.strokeBand(bounds, func(x, y int) bool {
		return pointInEllipse(x, y, bounds) && (inner.Empty() || !pointInEllipse(x, y, inner))
	}, closedOutline(bounds), stroke)
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
	inner := insetRect(rect, stroke.Width)
	innerRadius := max(0, radius-stroke.Width)
	c.strokeBand(rect, func(x, y int) bool {
		return pointInRoundRect(x, y, rect, radius) && (inner.Empty() || !pointInRoundRect(x, y, inner, innerRadius))
	}, [][]image.Point{roundRectPoints(rect, radius)}, stroke)
}

// closedOutline is the perimeter of an ellipse as a polyline, used only to
// measure how far along the edge a pixel sits when applying a dash.
func closedOutline(bounds image.Rectangle) [][]image.Point {
	points, _ := ellipseArcPoints(bounds, 0, 360)
	if len(points) < 2 {
		return nil
	}
	return [][]image.Point{points}
}
