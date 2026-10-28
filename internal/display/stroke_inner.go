package display

import (
	"image"
	"math"
)

// Closed shapes stroke inward, so their outline never grows past the geometry
// it describes. Shapes with an implicit stroker already do this by testing the
// shape at two sizes; the ones here have no such test, so the band is built by
// eroding a mask of the fill instead. Either way the stroke is a subset of the
// fill by construction.
func (c *Canvas) strokeInward(shape image.Rectangle, inside func(x, y int) bool, outlines [][]image.Point, stroke StrokeStyle) {
	depth := inwardDepth(shape, stroke.Width)
	bounds := strokeMaskBounds(shape, c.logicalClip(), depth)
	if bounds.Empty() {
		return
	}
	if float64(bounds.Dx())*float64(bounds.Dy()) > maxMaskPixels {
		// A mask is worked in the coordinates the shape was stated in, and a
		// shape that large there is one drawn much smaller than it was stated:
		// the band is then a fraction of a pixel across, and a line along the
		// outline is the same picture for a fraction of the work.
		for _, outline := range outlines {
			c.strokePoints(outline, true, stroke)
		}
		return
	}
	band := rasterizeMask(bounds, inside).innerBand(depth)
	c.strokeBand(bounds, band.at, func() [][]image.Point { return outlines }, stroke)
	if len(stroke.Dash) == 0 && stroke.Join != StrokeJoinMiter {
		paintClosedJoins(c, outlines, inside, shape.Intersect(c.logicalClip()), stroke)
	}
}

// maxMaskPixels bounds the mask an inward stroke erodes.
const maxMaskPixels = 1 << 22

// inwardDepth is how far into a shape its stroke can reach. A stroke wider
// than half the shape fills it, and eroding by more than that leaves nothing
// either way, so the erosion — and the mask around it — need go no further.
func inwardDepth(shape image.Rectangle, width int) int {
	return min(width, min(shape.Dx(), shape.Dy())/2+1)
}

// The inward band supplies the body of a closed outline. Explicit non-miter
// joins need one additional geometric pass at vertices; restricting that pass
// to the shape keeps the long-standing invariant that a closed outline never
// grows beyond its own fill region.
func paintClosedJoins(c *Canvas, outlines [][]image.Point, inside func(x, y int) bool, area image.Rectangle, stroke StrokeStyle) {
	radius := strokeRadius(stroke.Width)
	margin := int(math.Ceil(radius)) + 1
	for _, outline := range outlines {
		for index, center := range outline {
			previous := outline[(index+len(outline)-1)%len(outline)]
			next := outline[(index+1)%len(outline)]
			bounds := strokeCircleBounds(center, stroke.Width).Inset(-margin).Intersect(area)
			for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
				for x := bounds.Min.X; x < bounds.Max.X; x++ {
					if !inside(x, y) || !styledJoinContains(float64(x), float64(y), center, previous, next, radius, stroke.Join) {
						continue
					}
					c.setDevice(c.devicePoint(image.Pt(x, y)), stroke.Ink)
				}
			}
		}
	}
}

// strokeBand paints a band the caller has already defined and applies the dash
// along outlines. Shapes that can test their own inside at two sizes describe
// the band directly and need no mask; the clip can bound the work for them,
// because their test knows the geometry with or without it.
// The band and its dashes are both worked out in the coordinates the shape was
// stated in, and fillWhere asks about them there — so a turned outline is the
// same band, asked about in the same place, and its dashes fall where they
// would have fallen. Turning the dash mask instead would space the dashes by
// how far the raster travelled rather than by how long the line is, which is
// the mistake dashPattern already exists to avoid.
func (c *Canvas) strokeBand(bounds image.Rectangle, inBand func(x, y int) bool, outlines func() [][]image.Point, stroke StrokeStyle) {
	if bounds.Empty() {
		return
	}
	var dashed *dashField
	if pattern := newDashPattern(stroke); pattern != nil {
		dashed = newDashField(bounds.Intersect(c.logicalClip()), outlines(), pattern, stroke.Width)
	}
	c.fillWhere(bounds, func(x, y int) (Ink, bool) {
		if !inBand(x, y) || (dashed != nil && !dashed.on(x, y)) {
			return 0, false
		}
		return stroke.Ink, true
	})
}

// dashPattern evaluates a dash by arc length rather than by counting raster
// steps. Counting steps makes a dash on a 45-degree run sqrt(2) times longer
// than the same dash drawn horizontally; measuring real distance does not.
type dashPattern struct {
	lengths []int
	total   int
	offset  int
}

func newDashPattern(stroke StrokeStyle) *dashPattern {
	if len(stroke.Dash) == 0 {
		return nil
	}
	// An odd-length pattern repeats once so that on and off runs alternate.
	count := len(stroke.Dash)
	if count%2 != 0 {
		count *= 2
	}
	lengths := make([]int, count)
	total := 0
	for i := range lengths {
		lengths[i] = stroke.Dash[i%len(stroke.Dash)]
		total += lengths[i]
	}
	if total <= 0 {
		return nil
	}
	offset := stroke.DashOffset % total
	if offset < 0 {
		offset += total
	}
	return &dashPattern{lengths: lengths, total: total, offset: offset}
}

func (d *dashPattern) on(position float64) bool {
	position = math.Mod(position+float64(d.offset), float64(d.total))
	if position < 0 {
		position += float64(d.total)
	}
	for index, length := range d.lengths {
		if position < float64(length) {
			return index%2 == 0
		}
		position -= float64(length)
	}
	return true
}

// dashField says of a pixel whether its nearest point on any outline lands in
// an on-run of the pattern. Asking that only of the band's own pixels is what
// dashes a closed shape without moving where the band sits.
//
// Each outline is measured from its own origin, so a shape with several
// contours restarts the pattern on each of them, and a pixel is dashed by
// whichever contour it is actually closest to.
//
// A band pixel is never far from its outline, so the segments worth measuring
// are the ones filed near it; only a pixel with none of them within reach is
// measured against every segment, which is the answer the near ones give
// whenever they are within reach, ties and all.
type dashField struct {
	segments []dashSegment
	pattern  *dashPattern
	nearby   *tileIndex
	reach    float64
}

type dashSegment struct {
	from, to image.Point
	start    float64
}

func newDashField(area image.Rectangle, outlines [][]image.Point, pattern *dashPattern, width int) *dashField {
	field := &dashField{pattern: pattern, nearby: newTileIndex(area), reach: float64(width + 3)}
	margin := width + 3
	for _, outline := range outlines {
		if len(outline) < 2 {
			continue
		}
		position := 0.0
		for i, from := range outline {
			to := outline[(i+1)%len(outline)]
			field.nearby.add(len(field.segments), segmentBox(from, to, margin))
			field.segments = append(field.segments, dashSegment{from: from, to: to, start: position})
			position += math.Hypot(float64(to.X-from.X), float64(to.Y-from.Y))
		}
	}
	return field
}

func (f *dashField) on(x, y int) bool {
	best, at, found := math.Inf(1), 0.0, false
	for _, id := range f.nearby.at(x, y) {
		segment := f.segments[id]
		if distance, along := distanceToSegment(x, y, segment.from, segment.to); distance < best {
			best, at, found = distance, segment.start+along, true
		}
	}
	if !found || best > f.reach {
		best, found = math.Inf(1), false
		for _, segment := range f.segments {
			if distance, along := distanceToSegment(x, y, segment.from, segment.to); distance < best {
				best, at, found = distance, segment.start+along, true
			}
		}
	}
	return found && f.pattern.on(at)
}

// distanceToSegment returns the distance from a pixel to a segment and how far
// along that segment the closest point lies.
func distanceToSegment(x, y int, from, to image.Point) (distance, along float64) {
	dx := float64(to.X - from.X)
	dy := float64(to.Y - from.Y)
	px := float64(x - from.X)
	py := float64(y - from.Y)
	lengthSquared := float64(dx*dx) + float64(dy*dy)
	if lengthSquared == 0 {
		return math.Hypot(px, py), 0
	}
	t := (float64(px*dx) + float64(py*dy)) / lengthSquared
	t = math.Min(1, math.Max(0, t))
	return math.Hypot(px-float64(t*dx), py-float64(t*dy)), float64(t * math.Sqrt(lengthSquared))
}
