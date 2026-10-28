package display

import "image"

// mask is a 1-bit coverage buffer over a half-open pixel rectangle. It exists
// so that shapes with no implicit stroker can still be stroked inward, and so
// that clipping can eventually be an intersection of arbitrary regions rather
// than of rectangles.
type mask struct {
	bounds image.Rectangle
	bits   []bool
}

func newMask(bounds image.Rectangle) *mask {
	if bounds.Empty() {
		return &mask{}
	}
	return &mask{bounds: bounds, bits: make([]bool, bounds.Dx()*bounds.Dy())}
}

// rasterizeMask evaluates inside at every pixel of bounds.
func rasterizeMask(bounds image.Rectangle, inside func(x, y int) bool) *mask {
	m := newMask(bounds)
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		row := (y - bounds.Min.Y) * bounds.Dx()
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			if inside(x, y) {
				m.bits[row+x-bounds.Min.X] = true
			}
		}
	}
	return m
}

// strokeMaskBounds is the region a mask must cover for an inward stroke to be
// correct inside clip. It follows the shape rather than the clip, because a
// mask cut off at the clip edge would erode into a stroke along that cut. The
// clip still bounds the allocation, inflated far enough that erosion at any
// visible pixel only ever reads real geometry.
func strokeMaskBounds(shape, clip image.Rectangle, width int) image.Rectangle {
	if width < 0 {
		width = 0
	}
	margin := width + 1
	return shape.Intersect(clip.Inset(-margin))
}

func (m *mask) empty() bool {
	return len(m.bits) == 0
}

func (m *mask) at(x, y int) bool {
	if !image.Pt(x, y).In(m.bounds) {
		return false
	}
	return m.bits[(y-m.bounds.Min.Y)*m.bounds.Dx()+x-m.bounds.Min.X]
}

func (m *mask) set(x, y int, on bool) {
	if !image.Pt(x, y).In(m.bounds) {
		return
	}
	m.bits[(y-m.bounds.Min.Y)*m.bounds.Dx()+x-m.bounds.Min.X] = on
}

func (m *mask) count() int {
	total := 0
	for _, on := range m.bits {
		if on {
			total++
		}
	}
	return total
}

// each visits every set pixel in row-major order.
func (m *mask) each(visit func(x, y int)) {
	for index, on := range m.bits {
		if on {
			visit(m.bounds.Min.X+index%m.bounds.Dx(), m.bounds.Min.Y+index/m.bounds.Dx())
		}
	}
}

// subtract clears every pixel that other has set.
func (m *mask) subtract(other *mask) {
	m.each(func(x, y int) {
		if other.at(x, y) {
			m.set(x, y, false)
		}
	})
}

// intersect clears every pixel that other does not have set.
func (m *mask) intersect(other *mask) {
	m.each(func(x, y int) {
		if !other.at(x, y) {
			m.set(x, y, false)
		}
	})
}

// erode keeps only the pixels whose whole disc of the given radius is set.
// Pixels outside the mask count as clear, so a shape that runs past the mask
// is eroded from that edge too; strokeMaskBounds is what keeps that from
// happening inside the visible area.
//
// A pixel's disc is all set exactly when the nearest clear pixel is farther
// than the radius, so this measures that distance for every pixel at once —
// Meijster's exact Euclidean distance transform, a pass down each column and
// one along each row — rather than testing the disc at every pixel, which
// would cost the disc's area for each one.
func (m *mask) erode(radius int) *mask {
	if radius <= 0 || m.empty() {
		return m.clone()
	}
	width, height := m.bounds.Dx(), m.bounds.Dy()
	// down[i] is how far pixel i is from the nearest clear pixel in its own
	// column, counting the rows just outside the mask as clear.
	down := make([]int, width*height)
	for x := 0; x < width; x++ {
		distance := 0
		for y := 0; y < height; y++ {
			distance = (distance + 1) * boolInt(m.bits[y*width+x])
			down[y*width+x] = distance
		}
		distance = 0
		for y := height - 1; y >= 0; y-- {
			distance = (distance + 1) * boolInt(m.bits[y*width+x])
			down[y*width+x] = min(down[y*width+x], distance)
		}
	}
	// Along each row, the nearest clear pixel is the lowest of the parabolas
	// (x-u)² + down(u)², over the columns u of the row and the clear columns
	// just outside it. The parabolas that are lowest somewhere are kept in
	// order with where each takes over.
	limit := radius * radius
	eroded := newMask(m.bounds)
	count := width + 2
	column := make([]int, count)
	takesOver := make([]int, count)
	lift := make([]int, count)
	for y := 0; y < height; y++ {
		for u := 1; u <= width; u++ {
			lift[u] = down[y*width+u-1] * down[y*width+u-1]
		}
		lift[0], lift[count-1] = 0, 0
		at := func(x, u int) int { return (x-u)*(x-u) + lift[u] }
		top := 0
		column[0], takesOver[0] = 0, 0
		for u := 1; u < count; u++ {
			for top >= 0 && at(takesOver[top], column[top]) > at(takesOver[top], u) {
				top--
			}
			if top < 0 {
				top = 0
				column[0] = u
				continue
			}
			from := column[top]
			over := 1 + floorDivide(u*u-from*from+lift[u]-lift[from], 2*(u-from))
			if over < count {
				top++
				column[top], takesOver[top] = u, over
			}
		}
		for u := count - 1; u >= 0; u-- {
			if u >= 1 && u <= width && at(u, column[top]) > limit {
				eroded.bits[y*width+u-1] = true
			}
			if u == takesOver[top] {
				top--
			}
		}
	}
	return eroded
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func floorDivide(numerator, denominator int) int {
	quotient := numerator / denominator
	if numerator%denominator != 0 && (numerator < 0) != (denominator < 0) {
		quotient--
	}
	return quotient
}

// innerBand is the inward stroke of the shape this mask describes: everything
// the erosion removed. It is a subset of the mask by construction, which is
// what makes a stroke built from it stay inside its own fill.
func (m *mask) innerBand(width int) *mask {
	band := m.clone()
	band.subtract(m.erode(width))
	return band
}

func (m *mask) clone() *mask {
	clone := &mask{bounds: m.bounds}
	clone.bits = append([]bool(nil), m.bits...)
	return clone
}
