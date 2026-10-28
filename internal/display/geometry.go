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

func abs(value int) int {
	if value < 0 {
		return -value
	}
	return value
}
