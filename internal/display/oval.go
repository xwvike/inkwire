package display

import (
	"image"
	"math"
	"math/bits"
)

// Oval is an ellipse: the box it would fit in square to the page, and how far
// it is turned out of square.
//
// The box was the whole of it until now, which made every ellipse, arc, pie
// and chord here square to the page. That was not a limit of the panel. A
// pixel is set or it is not, and turning a drawing that is already pixels
// would mean resampling — but an ellipse is not pixels yet when it is turned.
// It is sampled, and sampling it at an angle costs two multiplications and
// loses nothing, because the curve is worked out where it actually goes
// rather than a picture of it being moved afterwards.
//
// What was missing was somewhere to say the angle. SVG's own arc command
// carries one for the same reason.
//
// The rotation is clockwise in degrees about the centre of the box, and it
// turns the ellipse rather than the box: a turned ellipse reaches outside the
// box it was stated in, the way a circle reaches outside a box its centre sits
// at the edge of. Extent says how far.
type Oval struct {
	Bounds   image.Rectangle
	Rotation float64
}

// Upright is the ellipse that fits a box square to the page, which is every
// ellipse that has no reason to be otherwise.
func Upright(bounds image.Rectangle) Oval { return Oval{Bounds: bounds} }

// turned reports whether the rotation makes any difference. A whole number of
// half turns leaves an ellipse where it was, because an ellipse is symmetric
// about both of its axes.
func (o Oval) turned() bool {
	return math.Mod(math.Abs(o.Rotation), 180) > 1e-9
}

// centre is the middle of the box in half-pixel terms: an even span has its
// centre between two pixels, which is why this is not an integer.
func (o Oval) centre() (float64, float64) {
	return float64(o.Bounds.Min.X+o.Bounds.Max.X-1) / 2,
		float64(o.Bounds.Min.Y+o.Bounds.Max.Y-1) / 2
}

// radii are half of one less than each span, so that the ellipse touches the
// last pixel inside the box rather than the edge of it.
func (o Oval) radii() (float64, float64) {
	return float64(o.Bounds.Dx()-1) / 2, float64(o.Bounds.Dy()-1) / 2
}

// centrePixel is the pixel a filled sector is drawn from. An even span has its
// centre between two pixels and this takes the lower, which is what the
// integer arithmetic here did before there was anything else to say about an
// ellipse — and what every reference image was drawn with.
func (o Oval) centrePixel() image.Point {
	return image.Pt((o.Bounds.Min.X+o.Bounds.Max.X-1)/2, (o.Bounds.Min.Y+o.Bounds.Max.Y-1)/2)
}

// turn returns the sine and cosine of the rotation, which every point needs.
func (o Oval) turn() (float64, float64) {
	return math.Sincos(o.Rotation * math.Pi / 180)
}

// at is the point on the ellipse at a parametric angle, in degrees clockwise
// from the ellipse's own long axis.
//
// This is the whole of what rotation costs: the point is worked out on an
// upright ellipse and then turned about the centre, which is two multiplies
// and two adds per sample.
func (o Oval) at(degrees float64) (float64, float64) {
	centreX, centreY := o.centre()
	radiusX, radiusY := o.radii()
	sine, cosine := math.Sincos(degrees * math.Pi / 180)
	alongX, alongY := float64(radiusX*cosine), float64(radiusY*sine)
	if !o.turned() {
		return centreX + alongX, centreY + alongY
	}
	turnSine, turnCosine := o.turn()
	return centreX + float64(alongX*turnCosine) - float64(alongY*turnSine),
		centreY + float64(alongX*turnSine) + float64(alongY*turnCosine)
}

// contains reports whether a pixel is inside the ellipse.
//
// Upright, this is the integer test it always was, exact and with nothing to
// round. Turned, the point is brought back to where it would be if the ellipse
// were upright and the same test is applied — which is the same answer, worked
// out the same way, with the turn undone first.
func (o Oval) contains(x, y int) bool {
	spanX := int64(o.Bounds.Dx())
	spanY := int64(o.Bounds.Dy())
	if spanX <= 0 || spanY <= 0 {
		return false
	}
	// Distances in half-pixels from the centre, so that an even span has no
	// pixel sitting on the middle and an odd one does.
	fromX := int64(2*x + 1 - o.Bounds.Min.X - o.Bounds.Max.X)
	fromY := int64(2*y + 1 - o.Bounds.Min.Y - o.Bounds.Max.Y)
	if !o.turned() {
		if !image.Pt(x, y).In(o.Bounds) {
			return false
		}
		return withinEllipse(fromX, fromY, spanX, spanY)
	}
	sine, cosine := o.turn()
	alongX := float64(float64(fromX)*cosine) + float64(float64(fromY)*sine)
	alongY := float64(-float64(fromX)*sine) + float64(float64(fromY)*cosine)
	spanXf, spanYf := float64(spanX), float64(spanY)
	return float64(alongX*alongX*spanYf*spanYf)+float64(alongY*alongY*spanXf*spanXf) <= spanXf*spanXf*spanYf*spanYf
}

// withinEllipse is fromX²·spanY² + fromY²·spanX² <= spanX²·spanY², the upright
// test, for offsets no larger than their spans. Those products pass the range
// of an int64 once a box is some 55,000 pixels across, so they are worked in
// 128 bits whenever they could.
func withinEllipse(fromX, fromY, spanX, spanY int64) bool {
	if spanX < 1<<15 && spanY < 1<<15 {
		return fromX*fromX*spanY*spanY+fromY*fromY*spanX*spanX <= spanX*spanX*spanY*spanY
	}
	if spanX >= 1<<31 || spanY >= 1<<31 {
		x, y, sx, sy := float64(fromX), float64(fromY), float64(spanX), float64(spanY)
		return float64(x*x*sy*sy)+float64(y*y*sx*sx) <= sx*sx*sy*sy
	}
	square := func(value int64) uint64 { return uint64(value * value) }
	xHigh, xLow := bits.Mul64(square(fromX), square(spanY))
	yHigh, yLow := bits.Mul64(square(fromY), square(spanX))
	sumLow, carry := bits.Add64(xLow, yLow, 0)
	sumHigh, _ := bits.Add64(xHigh, yHigh, carry)
	limitHigh, limitLow := bits.Mul64(square(spanX), square(spanY))
	return sumHigh < limitHigh || (sumHigh == limitHigh && sumLow <= limitLow)
}

// Extent is the smallest box square to the page that holds the whole ellipse,
// which is the box itself until it is turned and larger after.
//
// A turned ellipse is widest where its two axes' contributions add, and that
// width is the square root of the sum of their squares — the same identity
// that says a sine and a cosine of the same angle make a circle.
func (o Oval) Extent() image.Rectangle {
	if !o.turned() || o.Bounds.Empty() {
		return o.Bounds
	}
	centreX, centreY := o.centre()
	radiusX, radiusY := o.radii()
	sine, cosine := o.turn()
	halfWidth := math.Hypot(float64(radiusX*cosine), float64(radiusY*sine))
	halfHeight := math.Hypot(float64(radiusX*sine), float64(radiusY*cosine))
	return image.Rect(
		int(math.Floor(centreX-halfWidth)),
		int(math.Floor(centreY-halfHeight)),
		int(math.Ceil(centreX+halfWidth))+1,
		int(math.Ceil(centreY+halfHeight))+1,
	)
}
