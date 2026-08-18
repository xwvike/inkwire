package display

import (
	"fmt"
	"image"
	"image/color"
)

// MaxFramePixels bounds every rendered surface. The largest supported panel
// is 960x640; sixteen megapixels leaves ample room for previews and contact
// sheets without letting an input allocate unbounded memory.
const MaxFramePixels = 16 * 1024 * 1024

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
	if width > MaxFramePixels/height {
		return nil, fmt.Errorf("frame dimensions %dx%d exceed the %d pixel limit", width, height, MaxFramePixels)
	}
	if !background.valid() {
		return nil, fmt.Errorf("invalid background ink %d", background)
	}
	pixels := width * height
	f := &Frame{
		width:  width,
		height: height,
		pixels: make([]Ink, pixels),
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
	state canvasState
	stack []canvasState
}

// mask is nil whenever the clip is still a plain rectangle, which is the common
// case. Once a path narrows it, the mask holds the exact region in frame
// coordinates and clip stays as its bounding rectangle, so primitives can go on
// using clip to bound their loops. A mask is never modified after it is
// installed, so saved states and child canvases can share one.
type canvasState struct {
	clip   image.Rectangle
	mask   *mask
	offset image.Point
}

func NewCanvas(frame *Frame) *Canvas {
	return &Canvas{frame: frame, state: canvasState{clip: frame.Bounds()}}
}

func (c *Canvas) Frame() *Frame {
	return c.frame
}

// Clip returns a child canvas sharing the same frame with a tighter clip.
// The child inherits the current translation but owns an independent state stack.
func (c *Canvas) Clip(rect image.Rectangle) *Canvas {
	state := c.state
	state.clip = state.clip.Intersect(c.deviceRect(rect))
	return &Canvas{frame: c.frame, state: state}
}

// Save pushes the current translation and clipping state.
func (c *Canvas) Save() {
	c.stack = append(c.stack, c.state)
}

// Restore replaces the current state with the latest saved state. It returns
// false without changing the canvas when the stack is empty.
func (c *Canvas) Restore() bool {
	if len(c.stack) == 0 {
		return false
	}
	last := len(c.stack) - 1
	c.state = c.stack[last]
	c.stack = c.stack[:last]
	return true
}

// ClipRect intersects the current clip with rect in the current logical coordinates.
func (c *Canvas) ClipRect(rect image.Rectangle) {
	c.state.clip = c.state.clip.Intersect(c.deviceRect(rect))
}

// ClipPath intersects the current clip with the region path covers under the
// even-odd rule, so nested contours cut holes out of the visible area. Like
// ClipRect the region is fixed in frame coordinates once established, and
// translating afterwards moves what is drawn rather than where it is allowed.
func (c *Canvas) ClipPath(path Path) {
	contours := path.flatten()
	offset := c.state.offset
	bounds := c.deviceRect(contourBounds(contours)).Intersect(c.state.clip)
	region := rasterizeMask(bounds, func(x, y int) bool {
		return pointInPath(image.Pt(x-offset.X, y-offset.Y), contours)
	})
	c.state.clip = bounds
	if c.state.mask == nil {
		c.state.mask = region
		return
	}
	narrowed := c.state.mask.clone()
	narrowed.intersect(region)
	c.state.mask = narrowed
}

// Translate moves the origin for subsequent drawing by an integer offset.
func (c *Canvas) Translate(offset image.Point) {
	c.state.offset = c.state.offset.Add(offset)
}

func (c *Canvas) Set(x, y int, ink Ink) {
	point := c.devicePoint(image.Pt(x, y))
	if !point.In(c.state.clip) {
		return
	}
	if c.state.mask != nil && !c.state.mask.at(point.X, point.Y) {
		return
	}
	c.frame.Set(point.X, point.Y, ink)
}

func (c *Canvas) FillRect(rect image.Rectangle, ink Ink) {
	rect = rect.Intersect(c.logicalClip())
	for y := rect.Min.Y; y < rect.Max.Y; y++ {
		for x := rect.Min.X; x < rect.Max.X; x++ {
			c.Set(x, y, ink)
		}
	}
}

func (c *Canvas) devicePoint(point image.Point) image.Point {
	return point.Add(c.state.offset)
}

func (c *Canvas) deviceRect(rect image.Rectangle) image.Rectangle {
	return rect.Add(c.state.offset)
}

func (c *Canvas) logicalClip() image.Rectangle {
	return c.state.clip.Sub(c.state.offset)
}
