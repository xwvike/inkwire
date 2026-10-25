package display

import (
	"fmt"
	"image"
)

// ToneSteps is how finely a tone is stated: in sixteenths, because the dither
// that draws one is a 4x4 tile and a tile of sixteen cells has seventeen
// coverages, from none of them to all of them. The two ends are white and the
// ink itself, so a tone proper is one of the fifteen between.
const ToneSteps = 16

// A tone is an ink laid over white at some coverage — a grey, written as
// black at eight sixteenths. The panel has no such ink; what it has is pixels
// of the ink and pixels of white, and a tone is a fixed arrangement of the two.
//
// It is still an Ink, so that every primitive that takes one takes a tone
// without being told: a rectangle, a rounded box, a path, a stroke. They all
// end in Frame.Set, and that is the one place a tone becomes the ink or white
// at a particular pixel. The frame never holds a tone.
//
// The arrangement is an ordered dither keyed to the frame's own coordinates,
// not to the shape being drawn. Two boxes of the same tone side by side
// therefore meet without a seam, and the same page draws the same pixels every
// time. Error diffusion, which is what a photograph gets, gives a smoother
// gradient and neither of those: every pixel depends on the ones before it, so
// moving a box moves its texture. A tone is part of a design, and a design has
// to come out the same twice.
//
// The encoding keeps a tone inside the byte an Ink already is: the top bit
// marks it, the next three name the ink laid down, and the low four the
// coverage.
const (
	toneFlag      Ink = 0x80
	toneInkShift      = 4
	toneLevelMask Ink = 0x0f
)

// bayer is the 4x4 ordered-dither matrix. Each cell's rank is the coverage at
// which it turns on, so every level is the level below it with one more cell,
// and the cells that are on at any level are spread as evenly as sixteen
// allow.
var bayer = [4][4]uint8{
	{0, 8, 2, 10},
	{12, 4, 14, 6},
	{3, 11, 1, 9},
	{15, 7, 13, 5},
}

// Tone is ink laid over white at sixteenths of full coverage. Nothing, or
// less, is white and all of it, or more, is the ink itself, so the result is
// always something a panel can be asked to draw. White has no tone, since
// white over white is white.
func Tone(ink Ink, sixteenths int) Ink {
	if sixteenths <= 0 || ink == InkWhite {
		return InkWhite
	}
	if sixteenths >= ToneSteps {
		return ink
	}
	return toneFlag | ink<<toneInkShift | Ink(sixteenths)
}

// Toned reports whether this is a tone, and if so which ink it lays down and
// at how many sixteenths.
func (i Ink) Toned() (ink Ink, sixteenths int, toned bool) {
	if i&toneFlag == 0 {
		return i, 0, false
	}
	return (i &^ toneFlag) >> toneInkShift, int(i & toneLevelMask), true
}

// Physical is the ink this is at one pixel of the frame: the ink itself for
// one of the four, and the ink or white for a tone, by where the pixel falls
// in the dither.
func (i Ink) Physical(x, y int) Ink {
	ink, sixteenths, toned := i.Toned()
	if !toned {
		return i
	}
	if int(bayer[y&3][x&3]) < sixteenths {
		return ink
	}
	return InkWhite
}

func (i Ink) validTone() bool {
	ink, sixteenths, toned := i.Toned()
	return toned && ink != InkWhite && ink <= InkYellow && sixteenths > 0 && sixteenths < ToneSteps
}

func (i Ink) toneString() string {
	ink, sixteenths, _ := i.Toned()
	return fmt.Sprintf("%s %d/%d", ink, sixteenths, ToneSteps)
}

// haloWhere clears a ring around a glyph back to plain ground wherever the
// ring falls on a tone.
//
// Text over a tone is drawn into texture: a 1px stroke sitting among dither
// dots of the same ink has nothing around it to tell it from them, and at
// twelve pixels the letter is gone. One pixel of clear ground around every
// stroke is enough to bring it back. Only toned pixels are cleared — a solid
// rule or the next glyph beside this one is part of the design and keeps its
// ink.
//
// The ring is in the glyph's own coordinates, the box and predicate fillWhere
// takes, so a turned label gets a turned halo. The ground is white, or black
// for white text, which is the contrast either needs.
func (c *Canvas) haloWhere(bounds image.Rectangle, on func(x, y int) bool, ink Ink) {
	ground := InkWhite
	if ink == InkWhite {
		ground = InkBlack
	}
	ring := func(x, y int) (Ink, bool) {
		if on(x, y) {
			return 0, false
		}
		for dy := -1; dy <= 1; dy++ {
			for dx := -1; dx <= 1; dx++ {
				if on(x+dx, y+dy) {
					return ground, true
				}
			}
		}
		return 0, false
	}
	c.paintWhere(bounds.Inset(-1), ring, c.setDeviceOverTone)
}

// setDeviceOverTone is setDevice for a halo: it writes only where a tone put
// the pixel that is there now.
func (c *Canvas) setDeviceOverTone(point image.Point, ink Ink) {
	if !c.frame.tonedAt(point.X, point.Y) {
		return
	}
	c.setDevice(point, ink)
}
