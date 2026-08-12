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
func (m *mask) erode(radius int) *mask {
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
