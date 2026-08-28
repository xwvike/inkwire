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

// Ink is one of the physical colors supported by the display.
type Ink uint8

const (
	InkBlack Ink = iota
	InkWhite
	InkRed
	InkYellow
)

func (i Ink) valid() bool {
	return i <= InkYellow
}

// String names the ink. Anything that has to tell somebody an ink could not be
// shown has to be able to say which one, and every caller that needed that was
// writing the same four-case switch.
func (i Ink) String() string {
	switch i {
	case InkBlack:
		return "black"
	case InkWhite:
		return "white"
	case InkRed:
		return "red"
	case InkYellow:
		return "yellow"
	}
	return fmt.Sprintf("Ink(%d)", uint8(i))
}

func (i Ink) RGBA() color.NRGBA {
	switch i {
	case InkBlack:
		return color.NRGBA{A: 0xff}
	case InkWhite:
		return color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}
	case InkRed:
		return color.NRGBA{R: 0xff, A: 0xff}
	case InkYellow:
		return color.NRGBA{R: 0xff, G: 0xff, A: 0xff}
	default:
		return color.NRGBA{A: 0xff}
	}
}

// Frame is a device-independent, opaque ink surface.
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
	matrix Matrix
}

func NewCanvas(frame *Frame) *Canvas {
	return &Canvas{frame: frame, state: canvasState{clip: frame.Bounds(), matrix: Identity()}}
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
	// The region is worked out by asking, of each pixel the path could cover,
	// where that pixel was before the transform and whether the path holds it.
	// Undoing the transform on the pixel rather than applying it to the path is
	// what keeps a turned clip from having holes in it: a transform does not
	// send pixels onto pixels, so walking the path's own pixels forward would
	// leave gaps between them.
	inverse, invertible := c.state.matrix.Invert()
	if !invertible {
		c.state.clip = image.Rectangle{}
		return
	}
	bounds := c.deviceRect(contourBounds(contours)).Intersect(c.state.clip)
	region := rasterizeMask(bounds, func(x, y int) bool {
		return pointInPath(inverse.ApplyPoint(image.Pt(x, y)), contours)
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
	c.Transform(Translate(offset))
}

// Transform composes a transform inside the one already in force, which is the
// order a nested transform is read in: the inner one moves the point first.
func (c *Canvas) Transform(matrix Matrix) {
	c.state.matrix = matrix.Then(c.state.matrix)
}

// Matrix is the transform in force, which a primitive needs in order to work
// out where its geometry actually goes.
func (c *Canvas) Matrix() Matrix { return c.state.matrix }

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
	c.fillWhere(rect, func(int, int) (Ink, bool) { return ink, true })
}

// devicePoint is where a point in the coordinates a caller is drawing in lands
// on the frame.
//
// A whole-pixel move takes the arithmetic it always took, exactly, which is
// what keeps every page drawn before there was a transform drawing the same
// pixels it drew then.
func (c *Canvas) devicePoint(point image.Point) image.Point {
	if offset, whole := c.state.matrix.Offset(); whole {
		return point.Add(offset)
	}
	return c.state.matrix.ApplyPoint(point)
}

func (c *Canvas) deviceRect(rect image.Rectangle) image.Rectangle {
	return c.state.matrix.MapRect(rect)
}

// setDevice writes a pixel that has already been worked out in the frame's own
// coordinates, which is what a primitive does once it has transformed its own
// geometry. Going back through Set would transform it a second time.
func (c *Canvas) setDevice(point image.Point, ink Ink) {
	if !point.In(c.state.clip) {
		return
	}
	if c.state.mask != nil && !c.state.mask.at(point.X, point.Y) {
		return
	}
	c.frame.Set(point.X, point.Y, ink)
}

// logicalClip is the clip in the coordinates a caller is drawing in: the box
// that holds everything the clip could let through.
//
// Turned, it is larger than the clip itself, because a box that covers a turned
// box has corners the turned one does not reach. That is the right way round:
// a loop bounded by it tests some pixels that are then clipped out, where a
// smaller box would miss pixels that should have been drawn.
func (c *Canvas) logicalClip() image.Rectangle {
	if offset, whole := c.state.matrix.Offset(); whole {
		return c.state.clip.Sub(offset)
	}
	inverse, ok := c.state.matrix.Invert()
	if !ok {
		return image.Rectangle{}
	}
	return inverse.MapRect(c.state.clip)
}

// fillWhere paints the pixels of a box that a predicate accepts, asking the
// predicate in the coordinates the caller stated the box in.
//
// Every area this draws — a rectangle, a rounded one, a circle, an ellipse, a
// polygon, a path, a pattern — is a box and a question asked about each pixel
// in it. They differ only in the question, so they share this, and a transform
// they all had to learn separately is one they learn here once.
//
// # Why the loop runs over the frame and not over the box
//
// While the transform is a whole number of pixels the two are the same, and
// the box is walked because that is what was always walked and every reference
// image was drawn by it.
//
// Once it turns they are not the same, and only one of them is right. Walking
// the box and moving each pixel forward is how a picture is normally turned,
// and it leaves holes: a turn does not send pixels onto pixels, so some pixels
// of the frame are never anybody's destination. Walking the frame and asking
// where each of its pixels came from cannot leave a hole, because every pixel
// is asked about exactly once. That is why an image is turned this way round
// everywhere it is turned at all.
func (c *Canvas) fillWhere(bounds image.Rectangle, paint func(x, y int) (Ink, bool)) {
	if offset, whole := c.state.matrix.Offset(); whole {
		bounds = bounds.Intersect(c.logicalClip())
		for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
			for x := bounds.Min.X; x < bounds.Max.X; x++ {
				if ink, painted := paint(x, y); painted {
					c.setDevice(image.Pt(x+offset.X, y+offset.Y), ink)
				}
			}
		}
		return
	}
	inverse, invertible := c.state.matrix.Invert()
	if invertible == false || bounds.Empty() {
		return
	}
	device := c.deviceRect(bounds).Intersect(c.state.clip)
	for y := device.Min.Y; y < device.Max.Y; y++ {
		for x := device.Min.X; x < device.Max.X; x++ {
			from := inverse.ApplyPoint(image.Pt(x, y))
			if !from.In(bounds) {
				continue
			}
			if ink, painted := paint(from.X, from.Y); painted {
				c.setDevice(image.Pt(x, y), ink)
			}
		}
	}
}
