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
	bounds := strokeMaskBounds(shape, c.logicalClip(), stroke.Width)
	if bounds.Empty() {
		return
	}
	band := rasterizeMask(bounds, inside).innerBand(stroke.Width)
	c.strokeBand(bounds, band.at, outlines, stroke)
	if len(stroke.Dash) == 0 && stroke.Join != StrokeJoinMiter {
		paintClosedJoins(c, outlines, inside, stroke)
	}
}

// The inward band supplies the body of a closed outline. Explicit non-miter
// joins need one additional geometric pass at vertices; restricting that pass
// to the shape keeps the long-standing invariant that a closed outline never
// grows beyond its own fill region.
func paintClosedJoins(c *Canvas, outlines [][]image.Point, inside func(x, y int) bool, stroke StrokeStyle) {
	radius := strokeRadius(stroke.Width)
	margin := int(math.Ceil(radius)) + 1
	for _, outline := range outlines {
		for index, center := range outline {
			previous := outline[(index+len(outline)-1)%len(outline)]
			next := outline[(index+1)%len(outline)]
			bounds := strokeCircleBounds(center, stroke.Width).Inset(-margin)
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
func (c *Canvas) strokeBand(bounds image.Rectangle, inBand func(x, y int) bool, outlines [][]image.Point, stroke StrokeStyle) {
	if bounds.Empty() {
		return
	}
	var dashed *mask
	if pattern := newDashPattern(stroke); pattern != nil {
		dashed = dashRegion(bounds, outlines, pattern)
	}
	c.fillWhere(bounds, func(x, y int) (Ink, bool) {
		if !inBand(x, y) || (dashed != nil && !dashed.at(x, y)) {
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

// dashRegion marks every pixel of bounds whose nearest point on any outline
// lands in an on-run of the pattern. Intersecting the inward band with this is
// what dashes a closed shape without moving where the band sits.
//
// Each outline is measured from its own origin, so a shape with several
// contours restarts the pattern on each of them, and a pixel is dashed by
// whichever contour it is actually closest to.
func dashRegion(bounds image.Rectangle, outlines [][]image.Point, pattern *dashPattern) *mask {
	region := newMask(bounds)
	starts := make([][]float64, len(outlines))
	for index, outline := range outlines {
		starts[index] = make([]float64, len(outline))
		position := 0.0
		for i := range outline {
			starts[index][i] = position
			next := outline[(i+1)%len(outline)]
			position += math.Hypot(float64(next.X-outline[i].X), float64(next.Y-outline[i].Y))
		}
	}

	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			best, at, found := math.Inf(1), 0.0, false
			for index, outline := range outlines {
				if len(outline) < 2 {
					continue
				}
				for i, from := range outline {
					to := outline[(i+1)%len(outline)]
					distance, along := distanceToSegment(x, y, from, to)
					if distance < best {
						best, at, found = distance, starts[index][i]+along, true
					}
				}
			}
			if found && pattern.on(at) {
				region.set(x, y, true)
			}
		}
	}
	return region
}

// distanceToSegment returns the distance from a pixel to a segment and how far
// along that segment the closest point lies.
func distanceToSegment(x, y int, from, to image.Point) (distance, along float64) {
	dx := float64(to.X - from.X)
	dy := float64(to.Y - from.Y)
	px := float64(x - from.X)
	py := float64(y - from.Y)
	lengthSquared := dx*dx + dy*dy
	if lengthSquared == 0 {
		return math.Hypot(px, py), 0
	}
	t := (px*dx + py*dy) / lengthSquared
	t = math.Min(1, math.Max(0, t))
	return math.Hypot(px-t*dx, py-t*dy), t * math.Sqrt(lengthSquared)
}
