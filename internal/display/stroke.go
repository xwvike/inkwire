package display

import (
	"image"
	"math"
	"math/bits"
	"slices"
	"sort"
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
	brush := image.Rectangle{Min: minPoint, Max: minPoint.Add(image.Pt(stroke.Width, stroke.Width))}.Intersect(c.state.clip)
	for y := brush.Min.Y; y < brush.Max.Y; y++ {
		for x := brush.Min.X; x < brush.Max.X; x++ {
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
	stroke = c.mappedStroke(stroke)
	points = c.mapPoints(points)
	if stroke.Cap != StrokeCapSquare || stroke.Join != StrokeJoinMiter {
		c.strokePointsStyled(points, closed, stroke)
		return
	}
	c.strokePointsLegacy(points, closed, stroke)
}

// mappedStroke converts lengths used by line-based strokes into device pixels.
// Area-based strokes are inverse-mapped by fillWhere and scale with their
// geometry already; lines map their points first and therefore need their brush
// and dash lengths mapped explicitly.
func (c *Canvas) mappedStroke(stroke StrokeStyle) StrokeStyle {
	matrix := c.state.matrix
	scaleX := math.Hypot(matrix.A, matrix.B)
	scaleY := math.Hypot(matrix.C, matrix.D)
	scale := math.Min(scaleX, scaleY)
	if scale == 1 {
		return stroke
	}
	mapLength := func(value int, minimum bool) int {
		if value == 0 {
			return 0
		}
		mapped := int(math.Round(float64(float64(value) * scale)))
		if minimum && mapped < 1 {
			return 1
		}
		return mapped
	}
	stroke.Width = mapLength(stroke.Width, true)
	stroke.DashOffset = mapLength(stroke.DashOffset, false)
	if len(stroke.Dash) != 0 {
		stroke.Dash = slices.Clone(stroke.Dash)
		for index := range stroke.Dash {
			stroke.Dash[index] = mapLength(stroke.Dash[index], true)
		}
	}
	return stroke
}

// strokePointsLegacy is the original square-brush rasterizer. Keeping the
// zero-value path intact is important for borders and scene documents written
// before cap/join were added.
func (c *Canvas) strokePointsLegacy(points []image.Point, closed bool, stroke StrokeStyle) {
	// A brush wider than any panel covers the panel from wherever it is near
	// it; past maxBrushWidth the only difference more width could make is to
	// reach the frame from farther off, which is not a line any more.
	stroke.Width = min(stroke.Width, maxBrushWidth)
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
	visible := c.state.clip.Inset(-(stroke.Width + 2))
	brush := c.newBrush(stroke)
	for segment := 0; segment < segmentCount; segment++ {
		from := points[segment%len(points)]
		to := points[(segment+1)%len(points)]
		// The first point of a segment repeats the previous segment's last one.
		skipShared := segment > 0
		draw := func(point image.Point) {
			if skipShared {
				skipShared = false
				return
			}
			travelled += math.Hypot(float64(point.X-previous.X), float64(point.Y-previous.Y))
			previous = point
			if pattern == nil || pattern.on(travelled) {
				brush.stamp(point)
			} else {
				brush.lift()
			}
		}
		steps := max(abs(to.X-from.X), abs(to.Y-from.Y))
		if steps <= longSegment {
			drawBresenham(from, to, draw)
			brush.lift()
			continue
		}
		// A long segment is walked only where it can reach the clip. The steps
		// either side of that are counted rather than taken, so a dash still
		// falls where walking them would have put it.
		first, last, seen := visibleSteps(from, to, steps, visible)
		if !seen {
			travelled += bresenhamTravel(from, to, steps, steps)
			previous = to
			continue
		}
		if first > 0 {
			travelled += bresenhamTravel(from, to, steps, first)
			previous, skipShared = bresenhamStep(from, to, first), false
		}
		for step := first; step <= last; step++ {
			draw(bresenhamStep(from, to, step))
		}
		brush.lift()
		if last < steps {
			travelled += bresenhamTravel(from, to, steps, steps) - bresenhamTravel(from, to, steps, last)
			previous = to
		}
	}
}

// maxBrushWidth bounds the square brush a line is drawn with.
const maxBrushWidth = 4096

// brush stamps the square brush along a line. A narrow brush is stamped at
// each point as it comes. A wide one would paint nearly the same pixels again
// at every step, so the points of each unbroken stroke are kept and their
// brushes are painted together, a row at a time: along one segment the points
// only ever move one way across and one way down, so the brushes that reach a
// row are those of a run of consecutive points, and together they cover one
// unbroken span of it, from the first of them to the last. That is the same
// pixels for the cost of the rows instead of the cost of the brush times the
// steps.
type brush struct {
	canvas *Canvas
	stroke StrokeStyle
	wide   bool
	run    []image.Point
}

func (c *Canvas) newBrush(stroke StrokeStyle) *brush {
	return &brush{canvas: c, stroke: stroke, wide: stroke.Width > 8}
}

// stamp adds the next point of an unbroken stroke.
func (b *brush) stamp(point image.Point) {
	if !b.wide {
		b.canvas.fillBrush(point, b.stroke)
		return
	}
	b.run = append(b.run, point)
}

// lift ends the stroke being stamped — at a gap in a dash or the end of a
// segment — and paints it.
func (b *brush) lift() {
	run := b.run
	b.run = b.run[:0]
	if len(run) == 0 {
		return
	}
	width := b.stroke.Width
	offset := width / 2
	clip := b.canvas.state.clip
	low, high := run[0].Y, run[len(run)-1].Y
	rising := high < low
	if rising {
		low, high = high, low
	}
	rows := image.Rect(clip.Min.X, low-offset, clip.Max.X, high-offset+width).Intersect(clip)
	for y := rows.Min.Y; y < rows.Max.Y; y++ {
		// The brush at a point covers the rows from its y-offset to
		// y-offset+width-1, so the points reaching row y are those with y in
		// [y+offset-width+1, y+offset].
		bottom, top := y+offset-width+1, y+offset
		var first, last int
		if rising {
			first = sort.Search(len(run), func(i int) bool { return run[i].Y <= top })
			last = sort.Search(len(run), func(i int) bool { return run[i].Y < bottom }) - 1
		} else {
			first = sort.Search(len(run), func(i int) bool { return run[i].Y >= bottom })
			last = sort.Search(len(run), func(i int) bool { return run[i].Y > top }) - 1
		}
		if first > last {
			continue
		}
		left := min(run[first].X, run[last].X) - offset
		right := max(run[first].X, run[last].X) - offset + width
		for x := max(left, clip.Min.X); x < min(right, clip.Max.X); x++ {
			b.canvas.setDevice(image.Pt(x, y), b.stroke.Ink)
		}
	}
}

// longSegment is the length past which a segment is walked only where it can
// be seen. Every segment of an ordinary drawing is shorter, so it is walked
// whole and draws exactly what it always drew.
const longSegment = 4096

// visibleSteps is the range of Bresenham steps along a segment that can land
// in area, padded by a step either side.
func visibleSteps(from, to image.Point, steps int, area image.Rectangle) (first, last int, seen bool) {
	low, high, ok := clipSegment(
		floatPoint{float64(from.X), float64(from.Y)}, floatPoint{float64(to.X), float64(to.Y)}, area)
	if !ok {
		return 0, 0, false
	}
	first = max(0, int(math.Floor(float64(low*float64(steps))))-1)
	last = min(steps, int(math.Ceil(float64(high*float64(steps))))+1)
	return first, last, first <= last
}

// bresenhamStep is the pixel drawBresenham reaches after step steps.
//
// Along a line that is longer across than down, every step moves across and
// the steps down are the across count times the slope, rounded half up — which
// is what the error term of drawBresenham works out one step at a time, written
// down at once. A line longer down than across is the same with the axes
// swapped.
func bresenhamStep(from, to image.Point, step int) image.Point {
	across, down := abs(to.X-from.X), abs(to.Y-from.Y)
	var x, y int
	if across >= down {
		x, y = step, roundedShare(step, down, across)
	} else {
		x, y = roundedShare(step, across, down), step
	}
	if to.X < from.X {
		x = -x
	}
	if to.Y < from.Y {
		y = -y
	}
	return from.Add(image.Pt(x, y))
}

// roundedShare is floor((2*step*part + whole) / (2*whole)), worked in 128 bits
// so that a segment of any length cannot overflow it.
func roundedShare(step, part, whole int) int {
	if part == 0 {
		return 0
	}
	high, low := bits.Mul64(uint64(2*step), uint64(part))
	low, carry := bits.Add64(low, uint64(whole), 0)
	high += carry
	quotient, _ := bits.Div64(high, low, uint64(2*whole))
	return int(quotient)
}

// bresenhamTravel is the distance the first step steps of a Bresenham walk
// cover: one pixel for a straight step and the diagonal for a slanted one,
// and a step is slanted exactly when the shorter axis moves.
func bresenhamTravel(from, to image.Point, steps, step int) float64 {
	across, down := abs(to.X-from.X), abs(to.Y-from.Y)
	slanted := roundedShare(step, min(across, down), steps)
	return float64(step-slanted) + float64(float64(slanted)*math.Sqrt2)
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
	// A point is the centre of a pixel, so a line of even width through it
	// has an edge through the middle of a pixel on each side, and taking every
	// pixel whose centre is within reach takes both: a line four wide drawn
	// five where it runs along the grid and four where it does not. Each pixel
	// is asked about half a pixel along instead, which puts the line where the
	// square brush puts it — the width, from half of it before the point.
	bias := 0.0
	if stroke.Width%2 == 0 {
		bias = 0.5
	}
	if len(points) == 1 {
		if stroke.Cap == StrokeCapButt {
			return
		}
		if stroke.Cap == StrokeCapRound {
			bounds := strokeCircleBounds(points[0], stroke.Width).Intersect(c.state.clip)
			radiusSquared := float64(strokeRadius(stroke.Width) * strokeRadius(stroke.Width))
			for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
				for x := bounds.Min.X; x < bounds.Max.X; x++ {
					if strokeDistanceSquared(float64(x)+bias, float64(y)+bias, float64(points[0].X), float64(points[0].Y)) <= radiusSquared {
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
	if bounds.Empty() {
		return
	}
	pattern := newDashPattern(stroke)

	// The stroke is every pixel some part of it covers — a segment's body, a
	// cap, a dash's end, a join — so each part paints the pixels it covers,
	// looking only where it can reach. A pixel is painted once whichever parts
	// cover it, and the cost follows the parts rather than the area times the
	// number of them.
	painted := newMask(bounds)
	paint := func(x, y int) {
		painted.set(x, y, true)
		c.setDevice(image.Pt(x, y), stroke.Ink)
	}
	cover := func(box image.Rectangle, covers func(px, py float64) bool) {
		box = box.Intersect(bounds)
		for y := box.Min.Y; y < box.Max.Y; y++ {
			for x := box.Min.X; x < box.Max.X; x++ {
				if !painted.at(x, y) && covers(float64(x)+bias, float64(y)+bias) {
					paint(x, y)
				}
			}
		}
	}

	for _, segment := range segments {
		box := segmentBox(segment.from, segment.to, margin).Intersect(bounds)
		for y := box.Min.Y; y < box.Max.Y; y++ {
			low, high := bodyRow(segment, radius, y)
			for x := max(low, box.Min.X); x <= min(high, box.Max.X-1); x++ {
				if painted.at(x, y) {
					continue
				}
				distance, along, projection := distanceToFloatSegment(float64(x)+bias, float64(y)+bias, segment.from, segment.to)
				if distance > radius || projection < 0 || projection > 1 {
					continue
				}
				if pattern == nil || pattern.on(segment.start+along) {
					paint(x, y)
				}
			}
		}
	}
	// A square cap reaches the diagonal of its half-width from where it ends.
	capReach := int(math.Ceil(float64(radius*math.Sqrt2))) + 2
	if !closed && pattern == nil {
		last := len(points) - 1
		cover(pointBox(points[0], capReach), func(px, py float64) bool {
			return styledCapContains(px, py, points[0], segments[0].to, radius, stroke.Cap, nil, 0)
		})
		cover(pointBox(points[last], capReach), func(px, py float64) bool {
			return styledCapContains(px, py, points[last], segments[len(segments)-1].from, radius, stroke.Cap, nil, total)
		})
	}
	if pattern != nil && stroke.Cap != StrokeCapButt {
		for _, end := range nearbyDashEnds(segments, total, pattern, bounds.Inset(-capReach)) {
			center, direction, ok := strokePointAt(segments, end.position)
			if !ok {
				continue
			}
			cover(pointBox(end.at, capReach), func(px, py float64) bool {
				return dashCapCovers(px, py, center, direction, radius, stroke.Cap, end.start)
			})
		}
	}
	joinStart, joinEnd := 1, len(points)-1
	if closed {
		joinStart, joinEnd = 0, len(points)
	}
	// A miter is drawn only while its tip is within four half-widths.
	joinReach := int(math.Ceil(float64(radius*4))) + 2
	for index := joinStart; index < joinEnd; index++ {
		if pattern != nil && !pattern.on(strokeVertexDistance(segments, index, closed)) {
			continue
		}
		center := points[index%len(points)]
		previous := points[(index+len(points)-1)%len(points)]
		next := points[(index+1)%len(points)]
		cover(pointBox(center, joinReach), func(px, py float64) bool {
			return styledJoinContains(px, py, center, previous, next, radius, stroke.Join)
		})
	}
}

// bodyRow is a range of pixels in row y holding every pixel within radius of
// the segment whose nearest point is inside it: the row's slice of the
// segment swept sideways by radius, padded by a pixel.
func bodyRow(segment strokeSegment, radius float64, y int) (low, high int) {
	dx := float64(segment.to.X-segment.from.X) / segment.length
	dy := float64(segment.to.Y-segment.from.Y) / segment.length
	normalX, normalY := float64(-dy*radius), float64(dx*radius)
	corners := [4]floatPoint{
		{float64(segment.from.X) + normalX, float64(segment.from.Y) + normalY},
		{float64(segment.to.X) + normalX, float64(segment.to.Y) + normalY},
		{float64(segment.to.X) - normalX, float64(segment.to.Y) - normalY},
		{float64(segment.from.X) - normalX, float64(segment.from.Y) - normalY},
	}
	row := float64(y)
	left, right := math.Inf(1), math.Inf(-1)
	for index, from := range corners {
		to := corners[(index+1)%4]
		if (from.y-row)*(to.y-row) > 0 {
			continue
		}
		x := from.x
		if from.y != to.y {
			x = from.x + (to.x-from.x)*(row-from.y)/(to.y-from.y)
		} else {
			left, right = math.Min(left, math.Min(from.x, to.x)), math.Max(right, math.Max(from.x, to.x))
		}
		left, right = math.Min(left, x), math.Max(right, x)
	}
	if left > right {
		// The row passes within a pixel of a corner without crossing an edge.
		left, right = math.Inf(1), math.Inf(-1)
		for _, corner := range corners {
			if math.Abs(corner.y-row) <= 1 {
				left, right = math.Min(left, corner.x), math.Max(right, corner.x)
			}
		}
		if left > right {
			return 1, 0
		}
	}
	return int(math.Floor(left)) - 1, int(math.Ceil(right)) + 1
}

// dashEnd is one end of an on-run: where along the stroke it is, which end of
// the run it is, and roughly where it lands, for filing it.
type dashEnd struct {
	position float64
	start    bool
	at       image.Point
}

// nearbyDashEnds lists the ends of the stroke's on-runs that land in area.
//
// The runs are those of walking the pattern from the start of the stroke: the
// first piece is what is left of the pattern after its offset, and from the
// second on the pieces repeat every period. The walk is therefore taken up a
// whole number of periods short of each segment's part in area rather than
// from the start, which is what keeps a stroke far longer than the frame from
// costing a run for every dash along it.
func nearbyDashEnds(segments []strokeSegment, total float64, pattern *dashPattern, area image.Rectangle) []dashEnd {
	lengths := pattern.lengths
	phase := float64(pattern.offset)
	first := 0
	for phase >= float64(lengths[first]) && lengths[first] > 0 {
		phase -= float64(lengths[first])
		first = (first + 1) % len(lengths)
	}
	head := float64(lengths[first]) - phase
	period := float64(pattern.total)

	var found []dashEnd
	note := func(position float64, start bool, segment strokeSegment, low, high float64) {
		if position < low || position > high {
			return
		}
		t := (position - segment.start) / segment.length
		found = append(found, dashEnd{position: position, start: start, at: image.Pt(
			int(math.Round(float64(segment.from.X)+float64(t*float64(segment.to.X-segment.from.X)))),
			int(math.Round(float64(segment.from.Y)+float64(t*float64(segment.to.Y-segment.from.Y)))),
		)})
	}
	piece := func(position, length float64, index int, segment strokeSegment, low, high float64) {
		if index%2 != 0 {
			return
		}
		end := math.Min(total, position+length)
		if end > position {
			note(position, true, segment, low, high)
			note(end, false, segment, low, high)
		}
	}
	for _, segment := range segments {
		from := floatPoint{float64(segment.from.X), float64(segment.from.Y)}
		to := floatPoint{float64(segment.to.X), float64(segment.to.Y)}
		lowT, highT, ok := clipSegment(from, to, area)
		if !ok {
			continue
		}
		low := segment.start + float64(lowT*segment.length) - 1
		high := segment.start + float64(highT*segment.length) + 1
		piece(0, head, first, segment, low, high)
		position := head
		index := (first + 1) % len(lengths)
		if skip := math.Floor((low-head)/period) - 1; skip > 0 {
			position += float64(skip * period)
		}
		for position < total && position <= high {
			piece(position, float64(lengths[index]), index, segment, low, high)
			position += float64(lengths[index])
			index = (index + 1) % len(lengths)
		}
	}
	return found
}

// dashCapCovers is whether the cap at a dash's end, centred at center and
// facing along direction, covers a pixel.
func dashCapCovers(x, y float64, center, direction floatPoint, radius float64, cap StrokeCap, start bool) bool {
	dx, dy := x-center.x, y-center.y
	if cap == StrokeCapRound {
		return float64(dx*dx)+float64(dy*dy) <= radius*radius
	}
	if cap != StrokeCapSquare {
		return false
	}
	if start {
		direction.x, direction.y = -direction.x, -direction.y
	}
	projection := float64(dx*direction.x) + float64(dy*direction.y)
	if projection < 0 || projection > radius {
		return false
	}
	return math.Abs(float64(dx*direction.y)-float64(dy*direction.x)) <= radius
}

// strokePointAt is the point a distance along the stroke and the direction it
// is heading there. A distance where two segments meet belongs to the first.
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
	// Each segment starts where the one before it ends, so their ends rise
	// and the first that reaches the distance is found by halving.
	index, _ := slices.BinarySearchFunc(segments, position, func(segment strokeSegment, position float64) int {
		if segment.start+segment.length < position {
			return -1
		}
		return 1
	})
	segment := segments[min(index, len(segments)-1)]
	t := (position - segment.start) / segment.length
	return floatPoint{
		x: float64(segment.from.X) + float64(t*float64(segment.to.X-segment.from.X)),
		y: float64(segment.from.Y) + float64(t*float64(segment.to.Y-segment.from.Y)),
	}, floatPoint{
		x: float64(segment.to.X-segment.from.X) / segment.length,
		y: float64(segment.to.Y-segment.from.Y) / segment.length,
	}, true
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
	return float64(dx*dx) + float64(dy*dy)
}

func distanceToFloatSegment(x, y float64, from, to image.Point) (distance, along, projection float64) {
	dx := float64(to.X - from.X)
	dy := float64(to.Y - from.Y)
	lengthSquared := float64(dx*dx) + float64(dy*dy)
	if lengthSquared == 0 {
		return math.Hypot(x-float64(from.X), y-float64(from.Y)), 0, 0
	}
	projection = (float64((x-float64(from.X))*dx) + float64((y-float64(from.Y))*dy)) / lengthSquared
	clamped := math.Min(1, math.Max(0, projection))
	closestX := float64(from.X) + float64(clamped*dx)
	closestY := float64(from.Y) + float64(clamped*dy)
	return math.Hypot(x-closestX, y-closestY), float64(clamped * math.Sqrt(lengthSquared)), projection
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
	projection := (float64((x-float64(endpoint.X))*dx) + float64((y-float64(endpoint.Y))*dy)) / length
	if projection < 0 || projection > radius {
		return false
	}
	perpendicular := math.Abs(float64((x-float64(endpoint.X))*dy)-float64((y-float64(endpoint.Y))*dx)) / length
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
	prevNX, prevNY := float64(-prevDY*radius), float64(prevDX*radius)
	nextNX, nextNY := float64(-nextDY*radius), float64(nextDX*radius)
	for _, side := range []float64{-1, 1} {
		// The incoming segment points toward the vertex, so its offset normal
		// is opposite to the normal of center-to-previous used above.
		first := floatPoint{float64(center.X) - float64(side*prevNX), float64(center.Y) - float64(side*prevNY)}
		second := floatPoint{float64(center.X) + float64(side*nextNX), float64(center.Y) + float64(side*nextNY)}
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
		// The bodies on either side stop square at the vertex, so the gap
		// between them on the outside of the turn runs from the vertex out to
		// the tip: a bevel's triangle and the miter's beyond it. Filling only
		// the part beyond left the bevel's empty, a pinhole at every gentle
		// turn of a flattened curve.
		if pointInFloatPolygon(x, y, []floatPoint{{float64(center.X), float64(center.Y)}, first, miter, second}) {
			return true
		}
	}
	return false
}

type floatPoint struct{ x, y float64 }

func lineIntersection(first floatPoint, firstDX, firstDY float64, second floatPoint, secondDX, secondDY float64) (floatPoint, bool) {
	denominator := float64(firstDX*secondDY) - float64(firstDY*secondDX)
	if math.Abs(denominator) < 1e-9 {
		return floatPoint{}, false
	}
	dx, dy := second.x-first.x, second.y-first.y
	t := (float64(dx*secondDY) - float64(dy*secondDX)) / denominator
	return floatPoint{x: first.x + float64(t*firstDX), y: first.y + float64(t*firstDY)}, true
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
