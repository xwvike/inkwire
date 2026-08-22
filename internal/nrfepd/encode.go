package nrfepd

import (
	"fmt"

	"github.com/xwvike/inkwire/internal/display"
)

// Encode packs a frame into the planes this firmware expects.
//
// The two families disagree about almost everything, which is why this is a
// second function rather than a parameter on the first. Gicisky rotates a
// quarter turn and sets a bit to mean ink; here the panel is written the way
// it is seen, row by row, and a set bit means white. The colour plane is
// inverted on top of that: a clear bit is red.
//
// A red pixel comes out clear in both planes. That is what the reference
// implementation does, and it is the reading that survives either way of
// combining the two: panels where the colour plane wins show red, and the
// black plane alone would have shown ink rather than paper.
//
// The model says how many planes there are. A frame with red on it bound for a
// black and white panel is refused rather than quietly flattened, because
// losing a colour is not something the picture shows you afterwards.
//
// It lived in display until the drawing layer stopped knowing the names of tag
// families. gicisky.Encode was always next to its own protocol; this is now
// next to its own too.
func Encode(frame *display.Frame, model Model) (black, colour []byte, err error) {
	if frame == nil {
		return nil, nil, fmt.Errorf("frame must not be nil")
	}
	if err := model.Packable(); err != nil {
		return nil, nil, err
	}
	red := model.Palette != PaletteBW
	width, height := frame.Width(), frame.Height()
	if width <= 0 || height <= 0 {
		return nil, nil, fmt.Errorf("frame must have a positive size, got %dx%d", width, height)
	}
	// A row starts on a byte boundary, so a width that is not a multiple of
	// eight leaves spare bits at the end of every row that no pixel owns.
	// Both planes start white and ink is cleared into them, which is what
	// leaves those bits white: building them the other way up would run a
	// black stripe down the right edge of every row on any panel whose width
	// is not a multiple of eight.
	stride := (width + 7) / 8
	black = fillWhite(stride * height)
	if red {
		colour = fillWhite(stride * height)
	}
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			index := y*stride + x/8
			mask := byte(0x80 >> uint(x%8))
			ink, _ := frame.InkAt(x, y)
			switch {
			case ink == display.InkWhite:
			// An ink this panel has no plane for is refused rather than drawn
			// as something else. Yellow used to fall through to black here,
			// which made the two families disagree about the same mistake:
			// gicisky refused it and this one silently changed the picture.
			// Callers who would rather have the page than the refusal go
			// through panel.Render, which flattens and says so in the report.
			case ink == display.InkRed && !red, ink == display.InkYellow:
				return nil, nil, fmt.Errorf("%s panel cannot show %s ink at (%d,%d)",
					model.Palette, ink, x, y)
			case ink == display.InkRed:
				colour[index] &^= mask
				black[index] &^= mask
			default:
				black[index] &^= mask
			}
		}
	}
	return black, colour, nil
}

func fillWhite(size int) []byte {
	plane := make([]byte, size)
	for index := range plane {
		plane[index] = 0xff
	}
	return plane
}
