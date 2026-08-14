package compose

import (
	"fmt"
	"image"
	"math"

	"github.com/xwvike/inkwire/internal/display"
)

// ShapeKind names the outlines a clip can be described by.
type ShapeKind uint8

const (
	ShapeNone ShapeKind = iota
	ShapeInset
	ShapeCircle
	ShapeEllipse
	ShapePolygon
)

// Shape is an outline stated in lengths rather than pixels, because the box it
// is an outline of does not have a size until the layout has run. A circle of
// half the width means nothing until there is a width.
type Shape struct {
	Kind ShapeKind
	// Insets are the four edges for an inset shape, and Corner rounds it.
	Insets [4]Length
	Corner Length
	// Radius sizes a circle; RadiusX and RadiusY size an ellipse.
	Radius           Length
	RadiusX, RadiusY Length
	// Centre is where a circle or ellipse sits, defaulting to the middle.
	Centre [2]Length
	// Points are the corners of a polygon, in order.
	Points [][2]Length
}

func (s Shape) empty() bool { return s.Kind == ShapeNone }

// Path builds the outline for a box of this size. Coordinates come out
// relative to the box's own origin, which is what ClipShape then translates.
func (s Shape) Path(size image.Point) display.Path {
	var path display.Path
	switch s.Kind {
	case ShapeInset:
		top := resolveOr(s.Insets[0], size.Y, 0)
		right := resolveOr(s.Insets[1], size.X, 0)
		bottom := resolveOr(s.Insets[2], size.Y, 0)
		left := resolveOr(s.Insets[3], size.X, 0)
		box := image.Rect(left, top, size.X-right, size.Y-bottom)
		corner := resolveOr(s.Corner, min(box.Dx(), box.Dy()), 0)
		appendRoundRect(&path, box, corner)
	case ShapeCircle:
		// A percentage radius is a share of the box's diagonal over root two,
		// which is what makes circle(50%) touch all four edges of a square.
		reference := int(math.Round(math.Hypot(float64(size.X), float64(size.Y)) / math.Sqrt2))
		radius := resolveOr(s.Radius, reference, reference/2)
		centre := s.centreOf(size)
		appendEllipse(&path, image.Rect(centre.X-radius, centre.Y-radius, centre.X+radius, centre.Y+radius))
	case ShapeEllipse:
		radiusX := resolveOr(s.RadiusX, size.X, size.X/2)
		radiusY := resolveOr(s.RadiusY, size.Y, size.Y/2)
		centre := s.centreOf(size)
		appendEllipse(&path, image.Rect(centre.X-radiusX, centre.Y-radiusY, centre.X+radiusX, centre.Y+radiusY))
	case ShapePolygon:
		for index, point := range s.Points {
			at := image.Pt(resolveOr(point[0], size.X, 0), resolveOr(point[1], size.Y, 0))
			if index == 0 {
				path.MoveTo(at)
				continue
			}
			path.LineTo(at)
		}
		path.Close()
	}
	return path
}

func (s Shape) centreOf(size image.Point) image.Point {
	return image.Pt(
		resolveOr(s.Centre[0], size.X, size.X/2),
		resolveOr(s.Centre[1], size.Y, size.Y/2),
	)
}

func resolveOr(length Length, available, fallback int) int {
	if value, ok := length.Resolve(available); ok {
		return value
	}
	return fallback
}

// appendRoundRect traces the outline of box.
//
// A path names pixels and image.Rectangle names the space between them: the
// last pixel of a box is at Max-1, not Max. Tracing to Max instead puts the
// outline a pixel past the box on each far edge, which is a whole extra row
// and column of a clip that is supposed to be exactly the size it was given.
// display.roundRectPoints has the same corners and gets this right.
func appendRoundRect(path *display.Path, box image.Rectangle, corner int) {
	right, bottom := box.Max.X-1, box.Max.Y-1
	if corner <= 0 {
		path.MoveTo(box.Min)
		path.LineTo(image.Pt(right, box.Min.Y))
		path.LineTo(image.Pt(right, bottom))
		path.LineTo(image.Pt(box.Min.X, bottom))
		path.Close()
		return
	}
	corner = min(corner, min(box.Dx(), box.Dy())/2)
	// The arcs take half-open rectangles, which display already reads the same
	// way; only the straight runs between them are stated as pixels.
	path.MoveTo(image.Pt(box.Min.X+corner, box.Min.Y))
	path.LineTo(image.Pt(right-corner, box.Min.Y))
	path.Arc(image.Rect(box.Max.X-2*corner, box.Min.Y, box.Max.X, box.Min.Y+2*corner), -90, 90)
	path.LineTo(image.Pt(right, bottom-corner))
	path.Arc(image.Rect(box.Max.X-2*corner, box.Max.Y-2*corner, box.Max.X, box.Max.Y), 0, 90)
	path.LineTo(image.Pt(box.Min.X+corner, bottom))
	path.Arc(image.Rect(box.Min.X, box.Max.Y-2*corner, box.Min.X+2*corner, box.Max.Y), 90, 90)
	path.LineTo(image.Pt(box.Min.X, box.Min.Y+corner))
	path.Arc(image.Rect(box.Min.X, box.Min.Y, box.Min.X+2*corner, box.Min.Y+2*corner), 180, 90)
	path.Close()
}

func appendEllipse(path *display.Path, bounds image.Rectangle) {
	path.Arc(bounds, 0, 360)
	path.Close()
}

// ClipShape confines its child to an outline resolved against the box the
// layout gives it.
//
// ClipPath takes a path already in pixels, which suits a document that names
// coordinates. A stylesheet says circle(50%), and half of what is only settled
// once the box is.
type ClipShape struct {
	Size  image.Point
	Shape Shape
	Child Node
}

func (ClipShape) composeNode() {}

func (c ClipShape) measure(ctx *compileContext, maximum image.Point, path string) (image.Point, error) {
	if nilNode(c.Child) {
		return image.Point{}, fmt.Errorf("%s.child: node must not be nil", path)
	}
	size, err := c.Child.measure(ctx, maximum, path+".child")
	if err != nil {
		return image.Point{}, err
	}
	return preferredSize(size, c.Size, maximum)
}

func (c ClipShape) paint(ctx *compileContext, list *display.DisplayList, bounds image.Rectangle, path string) error {
	if c.Shape.empty() {
		return c.Child.paint(ctx, list, bounds, path+".child")
	}
	outline := c.Shape.Path(bounds.Size())
	list.Save()
	list.Translate(bounds.Min)
	list.ClipPath(outline)
	list.Translate(image.Pt(-bounds.Min.X, -bounds.Min.Y))
	err := c.Child.paint(ctx, list, bounds, path+".child")
	list.Restore()
	return err
}
