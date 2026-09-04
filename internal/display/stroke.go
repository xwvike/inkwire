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
	Width      int   // Width is the stroke diameter in pixels.
	Dash       []int // Dash holds alternating on and off lengths along the outline.
	DashOffset int   // DashOffset advances into Dash before drawing.
	Cap        StrokeCap
	Join       StrokeJoin
}

// StrokeCap controls how an open stroke ends. The zero value keeps the
// renderer's historical square brush behaviour; SVG and CSS adapters set an
// explicit value when their document specifies the CSS/SVG defaults.
type StrokeCap uint8

const (
	StrokeCapSquare StrokeCap = iota
	StrokeCapButt
	StrokeCapRound
)

func (c StrokeCap) valid() bool { return c <= StrokeCapRound }

// StrokeJoin controls how adjacent segments meet. The zero value is the
// renderer's historical miter-compatible join.
type StrokeJoin uint8

const (
	StrokeJoinMiter StrokeJoin = iota
	StrokeJoinRound
	StrokeJoinBevel
)

func (j StrokeJoin) valid() bool { return j <= StrokeJoinBevel }

func (s StrokeStyle) valid() bool {
	if s.Width <= 0 || !s.Ink.valid() || !s.Cap.valid() || !s.Join.valid() {
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
	if stroke.Cap != StrokeCapSquare || stroke.Join != StrokeJoinMiter {
		c.strokePointsStyled(points, closed, stroke)
		return
	}
	c.strokePointsLegacy(points, closed, stroke)
}

// strokePointsLegacy is the original square-brush rasterizer. Keeping the
// zero-value path intact is important for borders and scene documents written
// before cap/join were added.
func (c *Canvas) strokePointsLegacy(points []image.Point, closed bool, stroke StrokeStyle) {
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

// strokePointsStyled rasterizes an explicitly styled stroke from its geometric
// definition. It is deliberately kept below the markup and scene layers: SVG,
// CSS and hand-written scene documents therefore share the same cap and join
// behaviour.
func (c *Canvas) strokePointsStyled(points []image.Point, closed bool, stroke StrokeStyle) {
	points = distinctStrokePoints(points, closed)
	if len(points) == 0 {
		return
	}
	if len(points) == 1 {
		if stroke.Cap == StrokeCapButt {
			return
		}
		if stroke.Cap == StrokeCapRound {
			bounds := strokeCircleBounds(points[0], stroke.Width).Intersect(c.state.clip)
			radiusSquared := strokeRadius(stroke.Width) * strokeRadius(stroke.Width)
			for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
				for x := bounds.Min.X; x < bounds.Max.X; x++ {
					if strokeDistanceSquared(float64(x), float64(y), float64(points[0].X), float64(points[0].Y)) <= radiusSquared {
						c.setDevice(image.Pt(x, y), stroke.Ink)
					}
				}
			}
			return
		}
		c.fillBrush(points[0], stroke)
		return
	}

	segments, total := strokeSegments(points, closed)
	if len(segments) == 0 {
		return
	}
	radius := strokeRadius(stroke.Width)
	margin := int(math.Ceil(radius)) + 2
	bounds := polygonBounds(points).Inset(-margin)
	bounds = bounds.Intersect(c.state.clip)
	pattern := newDashPattern(stroke)
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			px, py := float64(x), float64(y)
			paint := false
			for _, segment := range segments {
				distance, along, projection := distanceToFloatSegment(px, py, segment.from, segment.to)
				if distance > radius || projection < 0 || projection > 1 {
					continue
				}
				if pattern == nil || pattern.on(segment.start+along) {
					paint = true
					break
				}
			}
			if !paint && !closed {
				if pattern == nil {
					paint = styledCapContains(px, py, points[0], segments[0].to, radius, stroke.Cap, nil, 0)
					if !paint {
						last := len(points) - 1
						paint = styledCapContains(px, py, points[last], segments[len(segments)-1].from, radius, stroke.Cap, nil, total)
					}
				}
			}
			if !paint && pattern != nil && stroke.Cap != StrokeCapButt {
				for _, run := range dashRuns(total, pattern) {
					if dashCapContains(px, py, segments, run.start, radius, stroke.Cap, true) ||
						dashCapContains(px, py, segments, run.end, radius, stroke.Cap, false) {
						paint = true
						break
					}
				}
			}
			if !paint {
				start := 1
				end := len(points) - 1
				if closed {
					start = 0
					end = len(points)
				}
				for index := start; index < end; index++ {
					if pattern != nil && !pattern.on(strokeVertexDistance(segments, index, closed)) {
						continue
					}
					if styledJoinContains(px, py, points[index%len(points)], points[(index+len(points)-1)%len(points)], points[(index+1)%len(points)], radius, stroke.Join) {
						paint = true
						break
					}
				}
			}
			if paint {
				c.setDevice(image.Pt(x, y), stroke.Ink)
			}
		}
	}
}

type dashRun struct{ start, end float64 }

func dashRuns(total float64, pattern *dashPattern) []dashRun {
	if pattern == nil || total <= 0 {
		return nil
	}
	phase := float64(pattern.offset)
	index := 0
	for phase >= float64(pattern.lengths[index]) && pattern.lengths[index] > 0 {
		phase -= float64(pattern.lengths[index])
		index = (index + 1) % len(pattern.lengths)
	}
	remaining := float64(pattern.lengths[index]) - phase
	position := 0.0
	var runs []dashRun
	for position < total {
		end := math.Min(total, position+remaining)
		if index%2 == 0 && end > position {
			runs = append(runs, dashRun{start: position, end: end})
		}
		position = end
		index = (index + 1) % len(pattern.lengths)
		remaining = float64(pattern.lengths[index])
	}
	return runs
}

func dashCapContains(x, y float64, segments []strokeSegment, position, radius float64, cap StrokeCap, start bool) bool {
	center, direction, ok := strokePointAt(segments, position)
	if !ok {
		return false
	}
	dx, dy := x-center.x, y-center.y
	if cap == StrokeCapRound {
		return dx*dx+dy*dy <= radius*radius
	}
	if cap != StrokeCapSquare {
		return false
	}
	if start {
		direction.x, direction.y = -direction.x, -direction.y
	}
	projection := dx*direction.x + dy*direction.y
	if projection < 0 || projection > radius {
		return false
	}
	return math.Abs(dx*direction.y-dy*direction.x) <= radius
}

func strokePointAt(segments []strokeSegment, position float64) (floatPoint, floatPoint, bool) {
	if len(segments) == 0 {
		return floatPoint{}, floatPoint{}, false
	}
	if position < 0 {
		position = 0
	}
	last := segments[len(segments)-1]
	if position > last.start+last.length {
		position = last.start + last.length
	}
	for _, segment := range segments {
		if position > segment.start+segment.length && segment != last {
			continue
		}
		t := (position - segment.start) / segment.length
		return floatPoint{
			x: float64(segment.from.X) + t*float64(segment.to.X-segment.from.X),
			y: float64(segment.from.Y) + t*float64(segment.to.Y-segment.from.Y),
		}, floatPoint{
			x: float64(segment.to.X-segment.from.X) / segment.length,
			y: float64(segment.to.Y-segment.from.Y) / segment.length,
		}, true
	}
	return floatPoint{}, floatPoint{}, false
}

type strokeSegment struct {
	from, to      image.Point
	start, length float64
}

func distinctStrokePoints(points []image.Point, closed bool) []image.Point {
	result := make([]image.Point, 0, len(points))
	for _, point := range points {
		if len(result) == 0 || result[len(result)-1] != point {
			result = append(result, point)
		}
	}
	if closed && len(result) > 1 && result[0] == result[len(result)-1] {
		result = result[:len(result)-1]
	}
	return result
}

func strokeSegments(points []image.Point, closed bool) ([]strokeSegment, float64) {
	count := len(points) - 1
	if closed {
		count = len(points)
	}
	segments := make([]strokeSegment, 0, count)
	start := 0.0
	for index := 0; index < count; index++ {
		from := points[index%len(points)]
		to := points[(index+1)%len(points)]
		length := math.Hypot(float64(to.X-from.X), float64(to.Y-from.Y))
		if length == 0 {
			continue
		}
		segments = append(segments, strokeSegment{from: from, to: to, start: start, length: length})
		start += length
	}
	return segments, start
}

func strokeRadius(width int) float64 { return float64(width) / 2 }

func strokeCircleBounds(center image.Point, width int) image.Rectangle {
	radius := int(math.Ceil(strokeRadius(width)))
	return image.Rect(center.X-radius, center.Y-radius, center.X+radius+1, center.Y+radius+1)
}

func strokeDistanceSquared(x, y, centerX, centerY float64) float64 {
	dx, dy := x-centerX, y-centerY
	return dx*dx + dy*dy
}

func distanceToFloatSegment(x, y float64, from, to image.Point) (distance, along, projection float64) {
	dx := float64(to.X - from.X)
	dy := float64(to.Y - from.Y)
	lengthSquared := dx*dx + dy*dy
	if lengthSquared == 0 {
		return math.Hypot(x-float64(from.X), y-float64(from.Y)), 0, 0
	}
	projection = ((x-float64(from.X))*dx + (y-float64(from.Y))*dy) / lengthSquared
	clamped := math.Min(1, math.Max(0, projection))
	closestX := float64(from.X) + clamped*dx
	closestY := float64(from.Y) + clamped*dy
	return math.Hypot(x-closestX, y-closestY), clamped * math.Sqrt(lengthSquared), projection
}

func styledCapContains(x, y float64, endpoint, inside image.Point, radius float64, cap StrokeCap, pattern *dashPattern, position float64) bool {
	if pattern != nil && !pattern.on(position) {
		return false
	}
	dx, dy := float64(inside.X-endpoint.X), float64(inside.Y-endpoint.Y)
	length := math.Hypot(dx, dy)
	if length == 0 {
		return cap == StrokeCapRound && strokeDistanceSquared(x, y, float64(endpoint.X), float64(endpoint.Y)) <= radius*radius
	}
	if cap == StrokeCapButt {
		return false
	}
	if cap == StrokeCapRound {
		return strokeDistanceSquared(x, y, float64(endpoint.X), float64(endpoint.Y)) <= radius*radius
	}
	// Square caps extend the segment by one half-width in its direction.
	projection := ((x-float64(endpoint.X))*dx + (y-float64(endpoint.Y))*dy) / length
	if projection < 0 || projection > radius {
		return false
	}
	perpendicular := math.Abs((x-float64(endpoint.X))*dy-(y-float64(endpoint.Y))*dx) / length
	return perpendicular <= radius
}

func strokeVertexDistance(segments []strokeSegment, index int, closed bool) float64 {
	if closed && index == 0 {
		return 0
	}
	if index-1 < 0 || index-1 >= len(segments) {
		return 0
	}
	return segments[index-1].start + segments[index-1].length
}

func styledJoinContains(x, y float64, center, previous, next image.Point, radius float64, join StrokeJoin) bool {
	if join == StrokeJoinRound {
		return strokeDistanceSquared(x, y, float64(center.X), float64(center.Y)) <= radius*radius
	}
	prevDX, prevDY := float64(previous.X-center.X), float64(previous.Y-center.Y)
	nextDX, nextDY := float64(next.X-center.X), float64(next.Y-center.Y)
	prevLength, nextLength := math.Hypot(prevDX, prevDY), math.Hypot(nextDX, nextDY)
	if prevLength == 0 || nextLength == 0 {
		return false
	}
	prevDX, prevDY = prevDX/prevLength, prevDY/prevLength
	nextDX, nextDY = nextDX/nextLength, nextDY/nextLength
	prevNX, prevNY := -prevDY*radius, prevDX*radius
	nextNX, nextNY := -nextDY*radius, nextDX*radius
	for _, side := range []float64{-1, 1} {
		// The incoming segment points toward the vertex, so its offset normal
		// is opposite to the normal of center-to-previous used above.
		first := floatPoint{float64(center.X) - side*prevNX, float64(center.Y) - side*prevNY}
		second := floatPoint{float64(center.X) + side*nextNX, float64(center.Y) + side*nextNY}
		if join == StrokeJoinBevel {
			if pointInFloatPolygon(x, y, []floatPoint{{float64(center.X), float64(center.Y)}, first, second}) {
				return true
			}
			continue
		}
		incomingX, incomingY := -prevDX, -prevDY
		miter, ok := lineIntersection(first, incomingX, incomingY, second, nextDX, nextDY)
		if !ok || math.Hypot(miter.x-float64(center.X), miter.y-float64(center.Y)) > radius*4 {
			if pointInFloatPolygon(x, y, []floatPoint{{float64(center.X), float64(center.Y)}, first, second}) {
				return true
			}
			continue
		}
		if pointInFloatPolygon(x, y, []floatPoint{first, miter, second}) {
			return true
		}
	}
	return false
}

type floatPoint struct{ x, y float64 }

func lineIntersection(first floatPoint, firstDX, firstDY float64, second floatPoint, secondDX, secondDY float64) (floatPoint, bool) {
	denominator := firstDX*secondDY - firstDY*secondDX
	if math.Abs(denominator) < 1e-9 {
		return floatPoint{}, false
	}
	dx, dy := second.x-first.x, second.y-first.y
	t := (dx*secondDY - dy*secondDX) / denominator
	return floatPoint{x: first.x + t*firstDX, y: first.y + t*firstDY}, true
}

func pointInFloatPolygon(x, y float64, polygon []floatPoint) bool {
	inside := false
	for index, point := range polygon {
		previous := polygon[(index+len(polygon)-1)%len(polygon)]
		if (point.y > y) != (previous.y > y) && x < (previous.x-point.x)*(y-point.y)/(previous.y-point.y)+point.x {
			inside = !inside
		}
	}
	return inside
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
