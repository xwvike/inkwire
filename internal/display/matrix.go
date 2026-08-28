package display

import (
	"image"
	"math"
)

// Matrix is a two-by-three affine transform, which is the shape every graphics
// stack states one in: the two interesting rows of a three-by-three whose last
// row is always (0, 0, 1).
//
//	x' = A·x + C·y + E
//	y' = B·x + D·y + F
//
// The letters are the ones PostScript chose, PDF kept, and Canvas takes in
// setTransform(a, b, c, d, e, f). They are used here rather than better names
// because anybody checking this against another implementation will be reading
// those letters, and a translation of the names is one more thing to get wrong.
//
// # Why a matrix and not an angle
//
// A rotation on its own does not compose. Turning by thirty degrees about one
// point and then by twenty about another is a third transform, and working out
// which one from two angles and two centres is arithmetic nobody should be
// writing twice. Multiplying two matrices is the same arithmetic, written once,
// in the form every other implementation checked it in.
type Matrix struct {
	A, B, C, D, E, F float64
}

// Identity leaves every point where it is.
func Identity() Matrix { return Matrix{A: 1, D: 1} }

// Translate moves by a whole number of pixels, which is the only transform
// this drawing model had until now.
func Translate(offset image.Point) Matrix {
	return Matrix{A: 1, D: 1, E: float64(offset.X), F: float64(offset.Y)}
}

// Rotate turns clockwise by degrees about a point.
//
// Clockwise because y grows downwards here, so a positive angle turns the way
// a positive angle turns on a screen — the same convention DrawArc uses.
func Rotate(degrees float64, aboutX, aboutY float64) Matrix {
	sine, cosine := math.Sincos(degrees * math.Pi / 180)
	// Move the centre to the origin, turn, move it back. Written out rather
	// than composed from three matrices so that the identity case is exact:
	// a quarter turn has a sine of exactly one and a cosine of exactly zero
	// only if nothing else has been multiplied through it first.
	return Matrix{
		A: cosine, B: sine,
		C: -sine, D: cosine,
		E: aboutX - aboutX*cosine + aboutY*sine,
		F: aboutY - aboutX*sine - aboutY*cosine,
	}
}

// Scale magnifies about a point.
func Scale(byX, byY float64, aboutX, aboutY float64) Matrix {
	return Matrix{
		A: byX, D: byY,
		E: aboutX - aboutX*byX,
		F: aboutY - aboutY*byY,
	}
}

// Then is m followed by next: the point is moved by m first and by next after,
// which is the order a nested transform is read in.
func (m Matrix) Then(next Matrix) Matrix {
	return Matrix{
		A: m.A*next.A + m.B*next.C,
		B: m.A*next.B + m.B*next.D,
		C: m.C*next.A + m.D*next.C,
		D: m.C*next.B + m.D*next.D,
		E: m.E*next.A + m.F*next.C + next.E,
		F: m.E*next.B + m.F*next.D + next.F,
	}
}

// Apply moves one point.
func (m Matrix) Apply(x, y float64) (float64, float64) {
	return m.A*x + m.C*y + m.E, m.B*x + m.D*y + m.F
}

// ApplyPoint moves a pixel, sampling at its centre.
//
// A pixel is a square and its coordinate names its corner, so moving the corner
// and rounding drifts everything half a pixel up and left of where it belongs.
// Every implementation that gets this wrong gets it wrong the same way.
func (m Matrix) ApplyPoint(point image.Point) image.Point {
	x, y := m.Apply(float64(point.X)+0.5, float64(point.Y)+0.5)
	return image.Pt(int(math.Floor(x)), int(math.Floor(y)))
}

// Determinant is zero when the transform flattens the plane onto a line, which
// is the one case that cannot be undone.
func (m Matrix) Determinant() float64 { return m.A*m.D - m.B*m.C }

// Invert undoes the transform, reporting false when there is nothing to undo
// it to.
func (m Matrix) Invert() (Matrix, bool) {
	determinant := m.Determinant()
	if determinant == 0 || math.IsNaN(determinant) || math.IsInf(determinant, 0) {
		return Identity(), false
	}
	return Matrix{
		A: m.D / determinant,
		B: -m.B / determinant,
		C: -m.C / determinant,
		D: m.A / determinant,
		E: (m.C*m.F - m.D*m.E) / determinant,
		F: (m.B*m.E - m.A*m.F) / determinant,
	}, true
}

// MapRect is the smallest box square to the page that holds a box after the
// transform, worked out from its four corners because a turned box's corners
// are the only points that can be furthest out.
func (m Matrix) MapRect(rect image.Rectangle) image.Rectangle {
	if rect.Empty() {
		return image.Rectangle{}
	}
	if offset, whole := m.Offset(); whole {
		return rect.Add(offset)
	}
	minX, minY := math.Inf(1), math.Inf(1)
	maxX, maxY := math.Inf(-1), math.Inf(-1)
	for _, corner := range [4][2]float64{
		{float64(rect.Min.X), float64(rect.Min.Y)},
		{float64(rect.Max.X), float64(rect.Min.Y)},
		{float64(rect.Min.X), float64(rect.Max.Y)},
		{float64(rect.Max.X), float64(rect.Max.Y)},
	} {
		x, y := m.Apply(corner[0], corner[1])
		minX, minY = math.Min(minX, x), math.Min(minY, y)
		maxX, maxY = math.Max(maxX, x), math.Max(maxY, y)
	}
	// Out one pixel on every side, because a corner that lands mid-pixel still
	// paints the pixel it landed in and rounding the other way would clip it.
	return image.Rect(
		int(math.Floor(minX))-1, int(math.Floor(minY))-1,
		int(math.Ceil(maxX))+1, int(math.Ceil(maxY))+1,
	)
}

// Offset reports the whole number of pixels this moves by, and whether that is
// all it does.
//
// It is the fast path and it is also the promise: every page drawn before
// there was anything but a translation still goes down the arithmetic it went
// down then, so not one of them moves by a pixel. A test holds the reference
// images to that.
func (m Matrix) Offset() (image.Point, bool) {
	if m.A != 1 || m.B != 0 || m.C != 0 || m.D != 1 {
		return image.Point{}, false
	}
	x, y := math.Round(m.E), math.Round(m.F)
	if x != m.E || y != m.F {
		return image.Point{}, false
	}
	return image.Pt(int(x), int(y)), true
}

// Quarters reports the number of quarter turns this is, and whether it is one:
// a whole number of quarter turns and a whole number of pixels, which together
// move every pixel onto another pixel and so need nothing resampled.
//
// It is what tells a glyph or a picture whether it can be turned exactly or has
// to be sampled.
func (m Matrix) Quarters() (int, bool) {
	const tolerance = 1e-9
	for turns := 0; turns < 4; turns++ {
		sine, cosine := math.Sincos(float64(turns) * math.Pi / 2)
		if math.Abs(m.A-cosine) < tolerance && math.Abs(m.B-sine) < tolerance &&
			math.Abs(m.C+sine) < tolerance && math.Abs(m.D-cosine) < tolerance {
			return turns, true
		}
	}
	return 0, false
}
