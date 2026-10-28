package display

import "image"

// These are the per-pixel even-odd tests the fills were first drawn with. The
// fills now work a row at a time through polygonRegion; these stay as the
// statement of what that must agree with.

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

// erodeByDisc is erode as it was first written: a pixel survives when every
// pixel of its disc is set.
func (m *mask) erodeByDisc(radius int) *mask {
	if radius <= 0 || m.empty() {
		return m.clone()
	}
	offsets := discOffsets(radius)
	eroded := newMask(m.bounds)
	m.each(func(x, y int) {
		for _, offset := range offsets {
			if !m.at(x+offset.X, y+offset.Y) {
				return
			}
		}
		eroded.set(x, y, true)
	})
	return eroded
}

func discOffsets(radius int) []image.Point {
	offsets := make([]image.Point, 0, (2*radius+1)*(2*radius+1))
	for dy := -radius; dy <= radius; dy++ {
		for dx := -radius; dx <= radius; dx++ {
			if dx*dx+dy*dy <= radius*radius {
				offsets = append(offsets, image.Pt(dx, dy))
			}
		}
	}
	return offsets
}
