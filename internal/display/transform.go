package display

import (
	"fmt"
	"image"
)

// Transform is a whole-number magnification and a whole number of quarter
// turns: the only two transforms that are exact on this panel.
//
// Both are usually poor ways to transform an image, because both throw away
// the greys that antialiasing put between edges. There are no greys here. A
// pixel is set or it is not, so magnifying by a whole number reproduces the
// drawing exactly at n times the size, and turning by a quarter is a
// transposition that moves every pixel to another pixel. Nothing is
// interpolated, so nothing is lost and nothing is invented.
//
// Anything else would have to resample. Half a pixel has no meaning here, and
// a 30 degree turn would have to decide which of two pixels a sample belongs
// to; either answer thins some strokes and thickens others, which at these
// sizes is the difference between a legible glyph and a smudge.
type Transform struct {
	// Scale magnifies by a whole number. One leaves the size alone.
	Scale int
	// Turns rotates clockwise by this many quarter turns.
	Turns int
}

func (t Transform) normalised() Transform {
	if t.Scale < 1 {
		t.Scale = 1
	}
	t.Turns = ((t.Turns % 4) + 4) % 4
	return t
}

func (t Transform) identity() bool {
	normal := t.normalised()
	return normal.Scale == 1 && normal.Turns == 0
}

// Apply reports the size a source of this size becomes.
func (t Transform) Apply(size image.Point) image.Point {
	normal := t.normalised()
	size = image.Pt(size.X*normal.Scale, size.Y*normal.Scale)
	if normal.Turns%2 == 1 {
		size.X, size.Y = size.Y, size.X
	}
	return size
}

// Invert reports the size a source must be to become this size, which is what
// a layout needs in order to give a transformed child the right box.
func (t Transform) Invert(size image.Point) image.Point {
	normal := t.normalised()
	if normal.Turns%2 == 1 {
		size.X, size.Y = size.Y, size.X
	}
	return image.Pt(size.X/normal.Scale, size.Y/normal.Scale)
}

// Coverage records which pixels of a frame a drawing actually reached.
//
// A frame has no transparent ink. Every pixel is black, white or red, so a
// frame that has been drawn onto cannot say by itself which parts are the
// drawing and which are the background it was created with. Copying one frame
// over another therefore brings that background along, and a rotated label
// laid over a filled bar erases the bar.
//
// It is worked out by drawing the same commands twice, onto one surface that
// starts white and one that starts black. Where the two agree something was
// drawn, whatever colour it was; where they differ nothing was, because all
// that showed through was the background they started as.
type Coverage struct {
	width, height int
	drawn         []bool
}

// NewCoverage compares two renderings of the same drawing over different
// backgrounds. They must be the same size, and must be what the same commands
// produced, or the answer means nothing.
func NewCoverage(overWhite, overBlack *Frame) (*Coverage, error) {
	if overWhite == nil || overBlack == nil {
		return nil, fmt.Errorf("both renderings are needed to tell a drawing from its background")
	}
	width, height := overWhite.Width(), overWhite.Height()
	if overBlack.Width() != width || overBlack.Height() != height {
		return nil, fmt.Errorf("renderings differ in size: %dx%d and %dx%d",
			width, height, overBlack.Width(), overBlack.Height())
	}
	coverage := &Coverage{width: width, height: height, drawn: make([]bool, width*height)}
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			light, _ := overWhite.InkAt(x, y)
			dark, _ := overBlack.InkAt(x, y)
			coverage.drawn[y*width+x] = light == dark
		}
	}
	return coverage, nil
}

// At reports whether anything was drawn at this pixel.
func (c *Coverage) At(x, y int) bool {
	if c == nil {
		return true
	}
	if x < 0 || y < 0 || x >= c.width || y >= c.height {
		return false
	}
	return c.drawn[y*c.width+x]
}

// DrawFrame copies source onto the canvas with the transform applied, ink for
// ink. It does not go through DrawImage: that reduces colours to the panel's
// three, and this source already is those three, so passing it through a
// reduction could only lose what is already exact.
//
// A nil covered copies every pixel, which is what a frame standing on its own
// wants. Anything drawn over something else should pass one, or it carries its
// own background over the top of whatever was already there.
func (c *Canvas) DrawFrame(source *Frame, at image.Point, transform Transform, covered *Coverage) error {
	if source == nil {
		return fmt.Errorf("source frame must not be nil")
	}
	normal := transform.normalised()
	size := normal.Apply(image.Pt(source.Width(), source.Height()))
	for y := 0; y < size.Y; y++ {
		for x := 0; x < size.X; x++ {
			from := normal.source(image.Pt(x, y), source.Width(), source.Height())
			if !covered.At(from.X, from.Y) {
				continue
			}
			ink, ok := source.InkAt(from.X, from.Y)
			if !ok {
				continue
			}
			c.Set(at.X+x, at.Y+y, ink)
		}
	}
	return nil
}

// source maps a destination pixel back to the source pixel it came from, which
// is the direction that leaves no gaps however the sizes divide.
//
// Turning clockwise sends source (x, y) to (height-1-y, x), so coming back the
// other way reads the destination's x as the source's y and its y as the
// source's x, counted from the far edge.
func (t Transform) source(at image.Point, width, height int) image.Point {
	x, y := at.X/t.Scale, at.Y/t.Scale
	switch t.Turns {
	case 1:
		return image.Pt(y, height-1-x)
	case 2:
		return image.Pt(width-1-x, height-1-y)
	case 3:
		return image.Pt(width-1-y, x)
	}
	return image.Pt(x, y)
}
