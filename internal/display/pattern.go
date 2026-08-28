package display

import (
	"fmt"
	"image"
)

// Pattern is a repeating tile of ink. With no grey available, a tile is the
// deterministic way to tell two regions apart: unlike dithering it does not
// depend on the surrounding pixels, so it can sit next to text without
// disturbing it, and it repeats exactly every time it is drawn.
//
// At this panel's pixel pitch a tile reads as visible texture rather than as a
// shade, so it distinguishes regions instead of simulating grey.
type Pattern struct {
	size  image.Point
	cells []patternCell
}

type patternCell struct {
	ink   Ink
	paint bool
}

// NewPattern builds a tile from rows of runes. A rune listed in inks paints
// that ink; every other rune leaves the frame untouched, which is what lets a
// hatch overlay whatever is already there.
//
//	NewPattern([]string{
//		"x...",
//		"..x.",
//	}, map[rune]Ink{'x': InkBlack})
func NewPattern(rows []string, inks map[rune]Ink) (*Pattern, error) {
	if len(rows) == 0 {
		return nil, fmt.Errorf("pattern must have at least one row")
	}
	width := len([]rune(rows[0]))
	if width == 0 {
		return nil, fmt.Errorf("pattern rows must not be empty")
	}
	for r, ink := range inks {
		if !ink.valid() {
			return nil, fmt.Errorf("pattern rune %q maps to invalid ink %d", r, ink)
		}
	}

	pattern := &Pattern{
		size:  image.Pt(width, len(rows)),
		cells: make([]patternCell, width*len(rows)),
	}
	for y, row := range rows {
		runes := []rune(row)
		if len(runes) != width {
			return nil, fmt.Errorf("pattern row %d has %d cells, want %d", y, len(runes), width)
		}
		for x, r := range runes {
			if ink, ok := inks[r]; ok {
				pattern.cells[y*width+x] = patternCell{ink: ink, paint: true}
			}
		}
	}
	return pattern, nil
}

// Size returns the tile dimensions.
func (p *Pattern) Size() image.Point {
	return p.size
}

// at reports the ink for a frame coordinate, and whether the cell paints at all.
func (p *Pattern) at(x, y int) (Ink, bool) {
	column := x % p.size.X
	if column < 0 {
		column += p.size.X
	}
	row := y % p.size.Y
	if row < 0 {
		row += p.size.Y
	}
	cell := p.cells[row*p.size.X+column]
	return cell.ink, cell.paint
}

// FillPattern tiles pattern across rect. The tile is anchored to frame
// coordinates rather than to rect, so two shapes filled with the same pattern
// line up along the edge they share instead of each starting its own phase.
// Shape the fill by clipping: ClipPath then FillPattern over the bounds.
func (c *Canvas) FillPattern(rect image.Rectangle, pattern *Pattern) {
	if pattern == nil {
		return
	}
	// A pattern is anchored to the frame rather than to what it fills, so that
	// two shapes filled with one tile line up with each other.
	c.fillWhere(rect, func(x, y int) (Ink, bool) {
		device := c.devicePoint(image.Pt(x, y))
		return pattern.at(device.X, device.Y)
	})
}
