package compose

import (
	"fmt"
	"image"

	"github.com/xwvike/inkwire/internal/display"
)

// Ink returns an addressable ink value for optional fill fields.
func Ink(value display.Ink) *display.Ink { return &value }

// Stroke returns an independent stroke value for optional stroke fields.
func Stroke(value display.StrokeStyle) *display.StrokeStyle {
	value.Dash = append([]int(nil), value.Dash...)
	return &value
}

type Pixel struct {
	At   image.Point
	Ink  display.Ink
	Size image.Point
}

func (Pixel) composeNode() {}
func (p Pixel) measure(_ *compileContext, maximum image.Point, path string) (image.Point, error) {
	if err := validateInk(path, p.Ink); err != nil {
		return image.Point{}, err
	}
	natural := image.Pt(max(0, p.At.X+1), max(0, p.At.Y+1))
	return preferredSize(natural, p.Size, maximum)
}
func (p Pixel) paint(_ *compileContext, list *display.DisplayList, bounds image.Rectangle, _ string) error {
	point := bounds.Min.Add(p.At)
	list.Set(point.X, point.Y, p.Ink)
	return nil
}

// Rectangle paints its allocated bounds. Radius zero selects a plain rectangle.
type Rectangle struct {
	Size   image.Point
	Radius int
	Fill   *display.Ink
	Stroke *display.StrokeStyle
}

func (Rectangle) composeNode() {}
func (r Rectangle) measure(_ *compileContext, maximum image.Point, path string) (image.Point, error) {
	if err := validateBoxPaint(path, r.Size, r.Radius, r.Fill, r.Stroke); err != nil {
		return image.Point{}, err
	}
	return preferredSize(r.Size, r.Size, maximum)
}
func (r Rectangle) paint(_ *compileContext, list *display.DisplayList, bounds image.Rectangle, _ string) error {
	if r.Fill != nil {
		if r.Radius > 0 {
			list.FillRoundRect(bounds, r.Radius, *r.Fill)
		} else {
			list.FillRect(bounds, *r.Fill)
		}
	}
	if r.Stroke != nil {
		if r.Radius > 0 {
			list.StrokeRoundRect(bounds, r.Radius, *r.Stroke)
		} else {
			list.StrokeRect(bounds, *r.Stroke)
		}
	}
	return nil
}

type Line struct {
	Size     image.Point
	From, To image.Point
	Stroke   display.StrokeStyle
}

func (Line) composeNode() {}
func (l Line) measure(_ *compileContext, maximum image.Point, path string) (image.Point, error) {
	if err := validateStroke(path, l.Stroke); err != nil {
		return image.Point{}, err
	}
	natural := pointsSize([]image.Point{l.From, l.To})
	return preferredSize(natural, l.Size, maximum)
}
func (l Line) paint(_ *compileContext, list *display.DisplayList, bounds image.Rectangle, _ string) error {
	list.DrawLine(bounds.Min.Add(l.From), bounds.Min.Add(l.To), l.Stroke)
	return nil
}

type Polyline struct {
	Size   image.Point
	Points []image.Point
	Stroke display.StrokeStyle
}

func (Polyline) composeNode() {}
func (p Polyline) measure(_ *compileContext, maximum image.Point, path string) (image.Point, error) {
	if len(p.Points) < 2 {
		return image.Point{}, fmt.Errorf("%s: polyline needs at least two points", path)
	}
	if err := validateStroke(path, p.Stroke); err != nil {
		return image.Point{}, err
	}
	return preferredSize(pointsSize(p.Points), p.Size, maximum)
}
func (p Polyline) paint(_ *compileContext, list *display.DisplayList, bounds image.Rectangle, _ string) error {
	list.DrawPolyline(offsetPoints(p.Points, bounds.Min), p.Stroke)
	return nil
}

type Polygon struct {
	Size   image.Point
	Points []image.Point
	Fill   *display.Ink
	Stroke *display.StrokeStyle
}

func (Polygon) composeNode() {}
func (p Polygon) measure(_ *compileContext, maximum image.Point, path string) (image.Point, error) {
	if len(p.Points) < 3 {
		return image.Point{}, fmt.Errorf("%s: polygon needs at least three points", path)
	}
	if err := validatePaint(path, p.Fill, p.Stroke); err != nil {
		return image.Point{}, err
	}
	return preferredSize(pointsSize(p.Points), p.Size, maximum)
}
func (p Polygon) paint(_ *compileContext, list *display.DisplayList, bounds image.Rectangle, _ string) error {
	points := offsetPoints(p.Points, bounds.Min)
	if p.Fill != nil {
		list.FillPolygon(points, *p.Fill)
	}
	if p.Stroke != nil {
		list.StrokePolygon(points, *p.Stroke)
	}
	return nil
}

type Circle struct {
	Size   image.Point
	Center image.Point
	Radius int
	Fill   *display.Ink
	Stroke *display.StrokeStyle
}

func (Circle) composeNode() {}
func (c Circle) measure(_ *compileContext, maximum image.Point, path string) (image.Point, error) {
	if c.Radius < 0 {
		return image.Point{}, fmt.Errorf("%s: radius must not be negative", path)
	}
	if err := validatePaint(path, c.Fill, c.Stroke); err != nil {
		return image.Point{}, err
	}
	natural := image.Pt(c.Center.X+c.Radius+1, c.Center.Y+c.Radius+1)
	return preferredSize(natural, c.Size, maximum)
}
func (c Circle) paint(_ *compileContext, list *display.DisplayList, bounds image.Rectangle, _ string) error {
	center := bounds.Min.Add(c.Center)
	if c.Fill != nil {
		list.FillCircle(center, c.Radius, *c.Fill)
	}
	if c.Stroke != nil {
		list.StrokeCircle(center, c.Radius, *c.Stroke)
	}
	return nil
}

type Ellipse struct {
	Size   image.Point
	Fill   *display.Ink
	Stroke *display.StrokeStyle
}

func (Ellipse) composeNode() {}
func (e Ellipse) measure(_ *compileContext, maximum image.Point, path string) (image.Point, error) {
	if err := validatePaint(path, e.Fill, e.Stroke); err != nil {
		return image.Point{}, err
	}
	return preferredSize(e.Size, e.Size, maximum)
}
func (e Ellipse) paint(_ *compileContext, list *display.DisplayList, bounds image.Rectangle, _ string) error {
	if e.Fill != nil {
		list.FillEllipse(display.Upright(bounds), *e.Fill)
	}
	if e.Stroke != nil {
		list.StrokeEllipse(display.Upright(bounds), *e.Stroke)
	}
	return nil
}

type Arc struct {
	Size         image.Point
	Start, Sweep float64
	Stroke       display.StrokeStyle
}

func (Arc) composeNode() {}
func (a Arc) measure(_ *compileContext, maximum image.Point, path string) (image.Point, error) {
	if err := validateStroke(path, a.Stroke); err != nil {
		return image.Point{}, err
	}
	return preferredSize(a.Size, a.Size, maximum)
}
func (a Arc) paint(_ *compileContext, list *display.DisplayList, bounds image.Rectangle, _ string) error {
	list.DrawArc(display.Upright(bounds), a.Start, a.Sweep, a.Stroke)
	return nil
}

type Pie struct {
	Size         image.Point
	Start, Sweep float64
	Ink          display.Ink
}

func (Pie) composeNode() {}
func (p Pie) measure(_ *compileContext, maximum image.Point, path string) (image.Point, error) {
	if err := validateInk(path, p.Ink); err != nil {
		return image.Point{}, err
	}
	return preferredSize(p.Size, p.Size, maximum)
}
func (p Pie) paint(_ *compileContext, list *display.DisplayList, bounds image.Rectangle, _ string) error {
	list.FillPie(display.Upright(bounds), p.Start, p.Sweep, p.Ink)
	return nil
}

type Chord struct {
	Size         image.Point
	Start, Sweep float64
	Ink          display.Ink
}

func (Chord) composeNode() {}
func (c Chord) measure(_ *compileContext, maximum image.Point, path string) (image.Point, error) {
	if err := validateInk(path, c.Ink); err != nil {
		return image.Point{}, err
	}
	return preferredSize(c.Size, c.Size, maximum)
}
func (c Chord) paint(_ *compileContext, list *display.DisplayList, bounds image.Rectangle, _ string) error {
	list.FillChord(display.Upright(bounds), c.Start, c.Sweep, c.Ink)
	return nil
}

type Path struct {
	Size   image.Point
	Path   display.Path
	Fill   *display.Ink
	Stroke *display.StrokeStyle
}

func (Path) composeNode() {}
func (p Path) measure(_ *compileContext, maximum image.Point, path string) (image.Point, error) {
	if p.Path.Empty() {
		return image.Point{}, fmt.Errorf("%s: path must not be empty", path)
	}
	if err := validatePaint(path, p.Fill, p.Stroke); err != nil {
		return image.Point{}, err
	}
	pathBounds := p.Path.Bounds()
	natural := image.Pt(max(0, pathBounds.Max.X), max(0, pathBounds.Max.Y))
	return preferredSize(natural, p.Size, maximum)
}
func (p Path) paint(_ *compileContext, list *display.DisplayList, bounds image.Rectangle, _ string) error {
	list.Save()
	list.Translate(bounds.Min)
	if p.Fill != nil {
		list.FillPath(p.Path, *p.Fill)
	}
	if p.Stroke != nil {
		list.StrokePath(p.Path, *p.Stroke)
	}
	list.Restore()
	return nil
}

type Pattern struct {
	Size    image.Point
	Pattern *display.Pattern
}

func (Pattern) composeNode() {}
func (p Pattern) measure(_ *compileContext, maximum image.Point, path string) (image.Point, error) {
	if p.Pattern == nil {
		return image.Point{}, fmt.Errorf("%s: pattern must not be nil", path)
	}
	return preferredSize(p.Pattern.Size(), p.Size, maximum)
}
func (p Pattern) paint(_ *compileContext, list *display.DisplayList, bounds image.Rectangle, _ string) error {
	list.FillPattern(bounds, p.Pattern)
	return nil
}

// ClipPath clips Child to Path in coordinates relative to its allocated box.
type ClipPath struct {
	Size  image.Point
	Path  display.Path
	Child Node
}

func (ClipPath) composeNode() {}
func (c ClipPath) measure(ctx *compileContext, maximum image.Point, path string) (image.Point, error) {
	if c.Path.Empty() {
		return image.Point{}, fmt.Errorf("%s: clip path must not be empty", path)
	}
	if nilNode(c.Child) {
		return image.Point{}, fmt.Errorf("%s.child: node must not be nil", path)
	}
	size, err := c.Child.measure(ctx, maximum, path+".child")
	if err != nil {
		return image.Point{}, err
	}
	return preferredSize(size, c.Size, maximum)
}
func (c ClipPath) paint(ctx *compileContext, list *display.DisplayList, bounds image.Rectangle, path string) error {
	list.Save()
	list.Translate(bounds.Min)
	list.ClipPath(c.Path)
	list.Translate(image.Pt(-bounds.Min.X, -bounds.Min.Y))
	err := ctx.paintWithContaining(c.Child, list, bounds, bounds, path+".child")
	list.Restore()
	return err
}

// ClipRect clips Child to Rect in coordinates relative to its allocated box.
// Clip confines its child to whatever rectangle the layout gives it. ClipRect
// needs the rectangle stated, which suits a document that names coordinates;
// this suits one that does not know its own box until it has been laid out.
type Clip struct {
	Size  image.Point
	Child Node
}

func (Clip) composeNode() {}

func (c Clip) measure(ctx *compileContext, maximum image.Point, path string) (image.Point, error) {
	if nilNode(c.Child) {
		return image.Point{}, fmt.Errorf("%s.child: node must not be nil", path)
	}
	size, err := c.Child.measure(ctx, maximum, path+".child")
	if err != nil {
		return image.Point{}, err
	}
	return preferredSize(size, c.Size, maximum)
}

func (c Clip) paint(ctx *compileContext, list *display.DisplayList, bounds image.Rectangle, path string) error {
	list.Save()
	list.ClipRect(bounds)
	err := ctx.paintWithContaining(c.Child, list, bounds, bounds, path+".child")
	list.Restore()
	return err
}

type ClipRect struct {
	Size  image.Point
	Rect  image.Rectangle
	Child Node
}

func (ClipRect) composeNode() {}
func (c ClipRect) measure(ctx *compileContext, maximum image.Point, path string) (image.Point, error) {
	if c.Rect.Empty() {
		return image.Point{}, fmt.Errorf("%s: clip rectangle must not be empty", path)
	}
	if nilNode(c.Child) {
		return image.Point{}, fmt.Errorf("%s.child: node must not be nil", path)
	}
	size, err := c.Child.measure(ctx, maximum, path+".child")
	if err != nil {
		return image.Point{}, err
	}
	return preferredSize(size, c.Size, maximum)
}
func (c ClipRect) paint(ctx *compileContext, list *display.DisplayList, bounds image.Rectangle, path string) error {
	list.Save()
	list.ClipRect(c.Rect.Add(bounds.Min))
	err := ctx.paintWithContaining(c.Child, list, bounds, bounds, path+".child")
	list.Restore()
	return err
}

func validateBoxPaint(path string, size image.Point, radius int, fill *display.Ink, stroke *display.StrokeStyle) error {
	if !validSize(size) {
		return fmt.Errorf("%s: size must not be negative, got %v", path, size)
	}
	if radius < 0 {
		return fmt.Errorf("%s: radius must not be negative", path)
	}
	return validatePaint(path, fill, stroke)
}

func validatePaint(path string, fill *display.Ink, stroke *display.StrokeStyle) error {
	if fill == nil && stroke == nil {
		return fmt.Errorf("%s: either fill or stroke is required", path)
	}
	if fill != nil {
		if err := validateInk(path, *fill); err != nil {
			return err
		}
	}
	if stroke != nil {
		return validateStroke(path, *stroke)
	}
	return nil
}

func validateInk(path string, ink display.Ink) error {
	if ink > display.InkYellow {
		return fmt.Errorf("%s: invalid ink %d", path, ink)
	}
	return nil
}

func validateStroke(path string, stroke display.StrokeStyle) error {
	if err := validateInk(path, stroke.Ink); err != nil {
		return err
	}
	if stroke.Width <= 0 {
		return fmt.Errorf("%s: stroke width must be positive", path)
	}
	for _, length := range stroke.Dash {
		if length <= 0 {
			return fmt.Errorf("%s: dash lengths must be positive", path)
		}
	}
	return nil
}

func pointsSize(points []image.Point) image.Point {
	maximum := image.Point{}
	for _, point := range points {
		maximum.X = max(maximum.X, point.X+1)
		maximum.Y = max(maximum.Y, point.Y+1)
	}
	return maximum
}

func offsetPoints(points []image.Point, offset image.Point) []image.Point {
	result := make([]image.Point, len(points))
	for index, point := range points {
		result[index] = point.Add(offset)
	}
	return result
}

// Transformed magnifies or turns whatever its child draws.
//
// Unlike everything else here it does not pass the drawing straight through.
// The child is drawn onto a surface of its own and that surface is copied over
// with the transform applied, because a magnified circle is not a circle drawn
// with a larger radius and a turned line is not a line with swapped endpoints:
// the whole subtree has to move together.
//
// The child is given the box the transform would need to fill this one, so a
// child under a doubling gets half the room and comes out at full size.
type Transformed struct {
	Size      image.Point
	Transform display.Transform
	Child     Node
}

func (Transformed) composeNode() {}

func (t Transformed) measure(ctx *compileContext, maximum image.Point, path string) (image.Point, error) {
	if nilNode(t.Child) {
		return image.Point{}, fmt.Errorf("%s.child: node must not be nil", path)
	}
	inner, err := t.Child.measure(ctx, t.Transform.Invert(maximum), path+".child")
	if err != nil {
		return image.Point{}, err
	}
	return preferredSize(t.Transform.Apply(inner), t.Size, maximum)
}

func (t Transformed) paint(ctx *compileContext, list *display.DisplayList, bounds image.Rectangle, path string) error {
	inner := t.Transform.Invert(bounds.Size())
	if inner.X <= 0 || inner.Y <= 0 {
		ctx.warn(path, "empty-layout", "the transformed box leaves its child no room")
		return nil
	}
	// The child draws at its own size and orientation into a list of its own,
	// starting at the origin so the surface it lands on is only as large as it
	// needs to be.
	sub := &display.DisplayList{}
	innerBounds := image.Rectangle{Max: inner}
	if err := ctx.paintWithContaining(t.Child, sub, innerBounds, innerBounds, path+".child"); err != nil {
		return err
	}
	// Twice, over opposite backgrounds. The surface has to be copied whole and
	// a frame has no transparent ink, so without this the background the child
	// happened to be drawn on goes over the top of whatever is underneath, and
	// a transform inside a stack wipes out the layers below it.
	surface, err := display.NewFrame(inner.X, inner.Y, display.InkWhite)
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	against, err := display.NewFrame(inner.X, inner.Y, display.InkBlack)
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	if err := sub.Replay(display.NewCanvas(surface)); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	if err := sub.Replay(display.NewCanvas(against)); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	covered, err := display.NewCoverage(surface, against)
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return list.DrawFrame(surface, bounds.Min, t.Transform, covered)
}

// Rotated turns its child about a point.
//
// It is not the same node as Transformed and does not do the same thing.
// Transformed draws its child onto a surface and moves the surface, which
// works for a whole number of quarter turns and a whole-number magnification
// because those move every pixel onto another pixel. Rotated moves nothing:
// it puts a turn into the drawing state, and everything under it works out
// where its own geometry goes before deciding which pixels that covers. A
// turned line is a line between turned ends, not a line drawn and then turned.
//
// So any angle at all is exact for anything drawn from geometry. What is
// already pixels — a glyph from a bitmap strike, a picture — is sampled, and
// at twelve pixels a glyph at an angle is rough. It is drawn anyway, because a
// stylesheet that turns a box turns what is written in it, and a label that
// vanished at an angle would surprise an author more than a rough one does.
//
// # Why it does not change the layout
//
// The child is measured and given the same box it would have had. That is what
// CSS does with a transform and it is the only answer that composes: a box
// whose size depended on its angle could not be laid out beside anything,
// because its neighbours would move as it turned. A turned child reaches
// outside its box, the way a circle reaches outside a box its centre sits at
// the edge of.
type Rotated struct {
	Size    image.Point
	Degrees float64
	// Origin is the point turned about, measured from the top left of the box.
	// Absent is the centre, which is what transform-origin defaults to and
	// what somebody who says "turn this" means by it.
	//
	// It is a pair of lengths rather than a pair of numbers because it is
	// written as a share of the box more often than as a distance into it —
	// the middle, a corner, half way down — and a share of a box that has not
	// been laid out yet is not a number anybody can work out.
	Origin *[2]Length
	Child  Node
}

func (Rotated) composeNode() {}

func (r Rotated) measure(ctx *compileContext, maximum image.Point, path string) (image.Point, error) {
	if nilNode(r.Child) {
		return image.Point{}, fmt.Errorf("%s.child: node must not be nil", path)
	}
	if !validSize(r.Size) {
		return image.Point{}, fmt.Errorf("%s: size must not be negative, got %v", path, r.Size)
	}
	inner, err := r.Child.measure(ctx, maximum, path+".child")
	if err != nil {
		return image.Point{}, err
	}
	return preferredSize(inner, r.Size, maximum)
}

func (r Rotated) paint(ctx *compileContext, list *display.DisplayList, bounds image.Rectangle, path string) error {
	if bounds.Empty() {
		ctx.warn(path, "empty-layout", "the turned box has no area")
		return nil
	}
	origin := image.Pt((bounds.Min.X+bounds.Max.X)/2, (bounds.Min.Y+bounds.Max.Y)/2)
	if r.Origin != nil {
		x, statedX := r.Origin[0].Offset(bounds.Dx())
		y, statedY := r.Origin[1].Offset(bounds.Dy())
		if statedX {
			origin.X = bounds.Min.X + x
		}
		if statedY {
			origin.Y = bounds.Min.Y + y
		}
	}
	list.Save()
	list.Transform(display.Rotate(r.Degrees, float64(origin.X), float64(origin.Y)))
	err := ctx.paintWithContaining(r.Child, list, bounds, bounds, path+".child")
	list.Restore()
	return err
}
