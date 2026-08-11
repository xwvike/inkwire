package display

import "image"

func circleBounds(center image.Point, radius int) image.Rectangle {
	return image.Rect(center.X-radius, center.Y-radius, center.X+radius+1, center.Y+radius+1)
}

func pointInCircle(x, y int, center image.Point, radius int) bool {
	xDistance := int64(x - center.X)
	yDistance := int64(y - center.Y)
	radius64 := int64(radius)
	return xDistance*xDistance+yDistance*yDistance <= radius64*radius64
}

func pointInEllipse(x, y int, bounds image.Rectangle) bool {
	if !image.Pt(x, y).In(bounds) {
		return false
	}
	dx := int64(bounds.Dx())
	dy := int64(bounds.Dy())
	xDistance := int64(2*x + 1 - bounds.Min.X - bounds.Max.X)
	yDistance := int64(2*y + 1 - bounds.Min.Y - bounds.Max.Y)
	return xDistance*xDistance*dy*dy+yDistance*yDistance*dx*dx <= dx*dx*dy*dy
}

func pointInRoundRect(x, y int, rect image.Rectangle, radius int) bool {
	point := image.Pt(x, y)
	if !point.In(rect) {
		return false
	}
	if radius == 0 || (x >= rect.Min.X+radius && x < rect.Max.X-radius) ||
		(y >= rect.Min.Y+radius && y < rect.Max.Y-radius) {
		return true
	}

	centerX := rect.Min.X + radius
	if x >= rect.Max.X-radius {
		centerX = rect.Max.X - radius
	}
	centerY := rect.Min.Y + radius
	if y >= rect.Max.Y-radius {
		centerY = rect.Max.Y - radius
	}
	xDistance := int64(2*x + 1 - 2*centerX)
	yDistance := int64(2*y + 1 - 2*centerY)
	diameter := int64(2 * radius)
	return xDistance*xDistance+yDistance*yDistance <= diameter*diameter
}

func clampRadius(rect image.Rectangle, radius int) int {
	if radius <= 0 {
		return 0
	}
	return min(radius, min(rect.Dx(), rect.Dy())/2)
}

func insetRect(rect image.Rectangle, amount int) image.Rectangle {
	if amount <= 0 {
		return rect
	}
	inset := image.Rectangle{
		Min: rect.Min.Add(image.Pt(amount, amount)),
		Max: rect.Max.Sub(image.Pt(amount, amount)),
	}
	if inset.Empty() {
		return image.Rectangle{}
	}
	return inset
}

func strokeCenterRect(rect image.Rectangle, width int) image.Rectangle {
	negative := width / 2
	positive := width - negative - 1
	center := image.Rectangle{
		Min: rect.Min.Add(image.Pt(negative, negative)),
		Max: rect.Max.Sub(image.Pt(positive, positive)),
	}
	if center.Empty() {
		return image.Rectangle{}
	}
	return center
}

func polygonBounds(points []image.Point) image.Rectangle {
	minPoint, maxPoint := points[0], points[0]
	for _, point := range points[1:] {
		minPoint.X = min(minPoint.X, point.X)
		minPoint.Y = min(minPoint.Y, point.Y)
		maxPoint.X = max(maxPoint.X, point.X)
		maxPoint.Y = max(maxPoint.Y, point.Y)
	}
	return image.Rect(minPoint.X, minPoint.Y, maxPoint.X+1, maxPoint.Y+1)
}

func pointInPolygon(point image.Point, points []image.Point) bool {
	inside, boundary := classifyPointInPolygon(point, points)
	return inside || boundary
}

func classifyPointInPolygon(point image.Point, points []image.Point) (inside, boundary bool) {
	previous := points[len(points)-1]
	for _, current := range points {
		if pointOnSegment(point, previous, current) {
			return false, true
		}
		if (current.Y > point.Y) != (previous.Y > point.Y) {
			left := int64(point.X-current.X) * int64(previous.Y-current.Y)
			right := int64(previous.X-current.X) * int64(point.Y-current.Y)
			if (previous.Y > current.Y && left < right) || (previous.Y < current.Y && left > right) {
				inside = !inside
			}
		}
		previous = current
	}
	return inside, false
}

func pointOnSegment(point, from, to image.Point) bool {
	cross := int64(point.X-from.X)*int64(to.Y-from.Y) - int64(point.Y-from.Y)*int64(to.X-from.X)
	if cross != 0 {
		return false
	}
	return point.X >= min(from.X, to.X) && point.X <= max(from.X, to.X) &&
		point.Y >= min(from.Y, to.Y) && point.Y <= max(from.Y, to.Y)
}

func abs(value int) int {
	if value < 0 {
		return -value
	}
	return value
}
