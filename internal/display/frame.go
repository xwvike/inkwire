package display

import (
	"fmt"
	"image"
	"image/color"
)

// Ink is one of the three physical colors supported by the display.
type Ink uint8

const (
	InkBlack Ink = iota
	InkWhite
	InkRed
)

func (i Ink) valid() bool {
	return i <= InkRed
}

func (i Ink) RGBA() color.NRGBA {
	switch i {
	case InkBlack:
		return color.NRGBA{A: 0xff}
	case InkWhite:
		return color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}
	case InkRed:
		return color.NRGBA{R: 0xff, A: 0xff}
	default:
		return color.NRGBA{A: 0xff}
	}
}

// Frame is a device-independent, opaque black/white/red pixel surface.
type Frame struct {
	width  int
	height int
	pixels []Ink
}

func NewFrame(width, height int, background Ink) (*Frame, error) {
	if width <= 0 || height <= 0 {
		return nil, fmt.Errorf("frame dimensions must be positive, got %dx%d", width, height)
	}
	if !background.valid() {
		return nil, fmt.Errorf("invalid background ink %d", background)
	}
	f := &Frame{
		width:  width,
		height: height,
		pixels: make([]Ink, width*height),
	}
	f.Clear(background)
	return f, nil
}

func (f *Frame) Width() int {
	return f.width
}

func (f *Frame) Height() int {
	return f.height
}

func (f *Frame) Bounds() image.Rectangle {
	return image.Rect(0, 0, f.width, f.height)
}

func (f *Frame) ColorModel() color.Model {
	return color.NRGBAModel
}

func (f *Frame) At(x, y int) color.Color {
	ink, ok := f.InkAt(x, y)
	if !ok {
		return color.NRGBA{}
	}
	return ink.RGBA()
}

func (f *Frame) InkAt(x, y int) (Ink, bool) {
	if x < 0 || y < 0 || x >= f.width || y >= f.height {
		return InkBlack, false
	}
	return f.pixels[y*f.width+x], true
}

func (f *Frame) Set(x, y int, ink Ink) {
	if !ink.valid() || x < 0 || y < 0 || x >= f.width || y >= f.height {
		return
	}
	f.pixels[y*f.width+x] = ink
}

func (f *Frame) Clear(ink Ink) {
	if !ink.valid() {
		return
	}
	for i := range f.pixels {
		f.pixels[i] = ink
	}
}

// Canvas draws integer-aligned primitives into a Frame.
type Canvas struct {
	frame *Frame
	clip  image.Rectangle
}

func NewCanvas(frame *Frame) *Canvas {
	return &Canvas{frame: frame, clip: frame.Bounds()}
}

func (c *Canvas) Frame() *Frame {
	return c.frame
}

// Clip returns a child canvas sharing the same frame with a tighter clip.
func (c *Canvas) Clip(rect image.Rectangle) *Canvas {
	return &Canvas{frame: c.frame, clip: c.clip.Intersect(rect)}
}

func (c *Canvas) Set(x, y int, ink Ink) {
	if image.Pt(x, y).In(c.clip) {
		c.frame.Set(x, y, ink)
	}
}

func (c *Canvas) FillRect(rect image.Rectangle, ink Ink) {
	rect = rect.Intersect(c.clip)
	for y := rect.Min.Y; y < rect.Max.Y; y++ {
		for x := rect.Min.X; x < rect.Max.X; x++ {
			c.frame.Set(x, y, ink)
		}
	}
}
