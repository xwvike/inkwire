package compose

import (
	"fmt"
	"image"
	"math"

	"github.com/xwvike/inkwire/internal/display"
)

// SVGViewport maps a target-independent SVG drawing into the box layout gives
// it. Source is the reference viewport used by the drawing under Child;
// Natural is the CSS size it reports before a container allocates another one.
type SVGViewport struct {
	Natural image.Point
	Source  image.Point
	Stretch bool
	Map     bool
	Child   Node
}

func (SVGViewport) composeNode() {}

func (v SVGViewport) measure(ctx *compileContext, maximum image.Point, path string) (image.Point, error) {
	if nilNode(v.Child) {
		return image.Point{}, fmt.Errorf("%s.child: node must not be nil", path)
	}
	if !validSize(v.Natural) || v.Natural.X <= 0 || v.Natural.Y <= 0 {
		return image.Point{}, fmt.Errorf("%s: natural size must be positive, got %v", path, v.Natural)
	}
	if v.Map && (!validSize(v.Source) || v.Source.X <= 0 || v.Source.Y <= 0) {
		return image.Point{}, fmt.Errorf("%s: source viewport must be positive, got %v", path, v.Source)
	}
	natural := v.Natural
	if _, err := v.Child.measure(ctx, v.sourceBounds(natural).Size(), path+".child"); err != nil {
		return image.Point{}, err
	}
	return constrainSize(natural, maximum), nil
}

func (v SVGViewport) paint(ctx *compileContext, list *display.DisplayList, bounds image.Rectangle, path string) error {
	if bounds.Empty() {
		ctx.warn(path, "empty-layout", "the SVG viewport has no drawable area")
		return nil
	}
	list.Save()
	list.ClipRect(bounds)
	defer list.Restore()

	if !v.Map {
		list.Translate(bounds.Min)
		inner := image.Rectangle{Max: bounds.Size()}
		return ctx.paintWithContaining(v.Child, list, inner, inner, path+".child")
	}

	scaleX := float64(bounds.Dx()) / float64(v.Source.X)
	scaleY := float64(bounds.Dy()) / float64(v.Source.Y)
	offsetX, offsetY := 0.0, 0.0
	if !v.Stretch {
		scale := math.Min(scaleX, scaleY)
		scaleX, scaleY = scale, scale
		offsetX = (float64(bounds.Dx()) - float64(v.Source.X)*scale) / 2
		offsetY = (float64(bounds.Dy()) - float64(v.Source.Y)*scale) / 2
	}
	matrix := display.Scale(scaleX, scaleY, 0, 0).Then(display.Translate(image.Pt(bounds.Min.X, bounds.Min.Y)))
	matrix.E += offsetX
	matrix.F += offsetY
	list.Transform(matrix)
	inner := image.Rectangle{Max: v.Source}
	return ctx.paintWithContaining(v.Child, list, inner, inner, path+".child")
}

func (v SVGViewport) sourceBounds(fallback image.Point) image.Rectangle {
	if v.Map {
		return image.Rectangle{Max: v.Source}
	}
	return image.Rectangle{Max: fallback}
}

func ratioHeight(width int, ratio float64) int {
	if width <= 0 || ratio <= 0 {
		return 0
	}
	return max(1, int(math.Round(float64(width)/ratio)))
}

func ratioWidth(height int, ratio float64) int {
	if height <= 0 || ratio <= 0 {
		return 0
	}
	return max(1, int(math.Round(float64(height)*ratio)))
}
