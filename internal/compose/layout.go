package compose

import (
	"fmt"
	"image"

	"github.com/xwvike/inkwire/internal/display"
)

// Insets are integer distances from a box's four edges.
type Insets struct {
	Top, Right, Bottom, Left int
}

func (i Insets) horizontal() int { return i.Left + i.Right }
func (i Insets) vertical() int   { return i.Top + i.Bottom }

func (i Insets) valid() bool {
	return i.Top >= 0 && i.Right >= 0 && i.Bottom >= 0 && i.Left >= 0
}

// CrossAlignment positions a child on the axis perpendicular to a Row or
// Column. The zero value stretches it to the available cross-axis size.
type CrossAlignment uint8

const (
	CrossStretch CrossAlignment = iota
	CrossStart
	CrossCenter
	CrossEnd
)

// MainAlignment positions a non-growing group along a Row or Column's main
// axis. It never inserts or changes content.
type MainAlignment uint8

const (
	MainStart MainAlignment = iota
	MainCenter
	MainEnd
)

// LayoutChild wraps one child in a Row or Column. Basis overrides the child's
// measured main-axis size when positive. Grow divides remaining pixels by
// integer weight after every base size and Gap has been allocated.
type LayoutChild struct {
	Node  Node
	Basis int
	Grow  int
}

type Placed struct {
	Bounds image.Rectangle
	Node   Node
}

// Absolute places children at explicit rectangles relative to its own origin.
// Size is its preferred size in a flow layout; zero axes are inferred from the
// placed rectangles. Clip explicitly clips children to the allocated box.
type Absolute struct {
	Size     image.Point
	Clip     bool
	Children []Placed
}

func (Absolute) composeNode() {}

func (a Absolute) measure(ctx *compileContext, maximum image.Point, path string) (image.Point, error) {
	if !validSize(a.Size) {
		return image.Point{}, fmt.Errorf("%s: size must not be negative, got %v", path, a.Size)
	}
	natural := image.Point{}
	for index, child := range a.Children {
		nodePath := childPath(path, "children", index)
		if nilNode(child.Node) {
			return image.Point{}, fmt.Errorf("%s: node must not be nil", nodePath)
		}
		if child.Bounds.Empty() {
			return image.Point{}, fmt.Errorf("%s: bounds must not be empty, got %v", nodePath, child.Bounds)
		}
		if _, err := child.Node.measure(ctx, child.Bounds.Size(), nodePath); err != nil {
			return image.Point{}, err
		}
		natural.X = max(natural.X, child.Bounds.Max.X)
		natural.Y = max(natural.Y, child.Bounds.Max.Y)
	}
	return preferredSize(natural, a.Size, maximum)
}

func (a Absolute) paint(ctx *compileContext, list *display.DisplayList, bounds image.Rectangle, path string) error {
	if a.Clip {
		list.Save()
		list.ClipRect(bounds)
		defer list.Restore()
	}
	for index, child := range a.Children {
		nodePath := childPath(path, "children", index)
		childBounds := child.Bounds.Add(bounds.Min)
		if err := child.Node.paint(ctx, list, childBounds, nodePath); err != nil {
			return err
		}
	}
	return nil
}

// Row lays children out from left to right. It never reorders them.
type Row struct {
	Size       image.Point
	Gap        int
	MainAlign  MainAlignment
	CrossAlign CrossAlignment
	Children   []LayoutChild
}

func (Row) composeNode() {}

func (r Row) measure(ctx *compileContext, maximum image.Point, path string) (image.Point, error) {
	return measureFlow(ctx, maximum, r.Size, r.Gap, true, r.Children, path)
}

func (r Row) paint(ctx *compileContext, list *display.DisplayList, bounds image.Rectangle, path string) error {
	return paintFlow(ctx, list, bounds, r.Gap, r.MainAlign, r.CrossAlign, true, r.Children, path)
}

// Column lays children out from top to bottom. It never reorders them.
type Column struct {
	Size       image.Point
	Gap        int
	MainAlign  MainAlignment
	CrossAlign CrossAlignment
	Children   []LayoutChild
}

func (Column) composeNode() {}

func (c Column) measure(ctx *compileContext, maximum image.Point, path string) (image.Point, error) {
	return measureFlow(ctx, maximum, c.Size, c.Gap, false, c.Children, path)
}

func (c Column) paint(ctx *compileContext, list *display.DisplayList, bounds image.Rectangle, path string) error {
	return paintFlow(ctx, list, bounds, c.Gap, c.MainAlign, c.CrossAlign, false, c.Children, path)
}

func measureFlow(ctx *compileContext, maximum, preferred image.Point, gap int, horizontal bool, children []LayoutChild, path string) (image.Point, error) {
	if gap < 0 {
		return image.Point{}, fmt.Errorf("%s: gap must not be negative, got %d", path, gap)
	}
	if !validSize(preferred) {
		return image.Point{}, fmt.Errorf("%s: size must not be negative, got %v", path, preferred)
	}
	main, cross := 0, 0
	for index, child := range children {
		nodePath := childPath(path, "children", index)
		if nilNode(child.Node) {
			return image.Point{}, fmt.Errorf("%s: node must not be nil", nodePath)
		}
		if child.Basis < 0 || child.Grow < 0 {
			return image.Point{}, fmt.Errorf("%s: basis and grow must not be negative", nodePath)
		}
		size, err := child.Node.measure(ctx, maximum, nodePath)
		if err != nil {
			return image.Point{}, err
		}
		childMain, childCross := axes(size, horizontal)
		if child.Basis > 0 {
			childMain = child.Basis
		}
		main += childMain
		cross = max(cross, childCross)
	}
	if len(children) > 1 {
		main += gap * (len(children) - 1)
	}
	natural := fromAxes(main, cross, horizontal)
	return preferredSize(natural, preferred, maximum)
}

func paintFlow(ctx *compileContext, list *display.DisplayList, bounds image.Rectangle, gap int, mainAlign MainAlignment, crossAlign CrossAlignment, horizontal bool, children []LayoutChild, path string) error {
	if mainAlign > MainEnd {
		return fmt.Errorf("%s: invalid main alignment %d", path, mainAlign)
	}
	if crossAlign > CrossEnd {
		return fmt.Errorf("%s: invalid cross alignment %d", path, crossAlign)
	}
	maximum := bounds.Size()
	sizes := make([]image.Point, len(children))
	baseMain, growTotal := 0, 0
	for index, child := range children {
		nodePath := childPath(path, "children", index)
		size, err := child.Node.measure(ctx, maximum, nodePath)
		if err != nil {
			return err
		}
		if child.Basis > 0 {
			setMain(&size, horizontal, child.Basis)
		}
		sizes[index] = size
		childMain, _ := axes(size, horizontal)
		baseMain += childMain
		growTotal += child.Grow
	}
	if len(children) > 1 {
		baseMain += gap * (len(children) - 1)
	}
	availableMain, availableCross := axes(bounds.Size(), horizontal)
	if baseMain > availableMain {
		ctx.warn(path, "layout-overflow", fmt.Sprintf("children need %d pixels on the main axis, only %d are available", baseMain, availableMain))
	}
	remaining := max(0, availableMain-baseMain)
	if growTotal > 0 && remaining > 0 {
		used := 0
		for index, child := range children {
			if child.Grow == 0 {
				continue
			}
			extra := remaining * child.Grow / growTotal
			used += extra
			setMain(&sizes[index], horizontal, mainOf(sizes[index], horizontal)+extra)
		}
		for index := range children {
			if used == remaining {
				break
			}
			if children[index].Grow > 0 {
				setMain(&sizes[index], horizontal, mainOf(sizes[index], horizontal)+1)
				used++
			}
		}
		baseMain += remaining
	}
	start := 0
	if growTotal == 0 && baseMain < availableMain {
		switch mainAlign {
		case MainCenter:
			start = (availableMain - baseMain) / 2
		case MainEnd:
			start = availableMain - baseMain
		}
	}
	cursor := start
	for index, child := range children {
		nodePath := childPath(path, "children", index)
		childMain, childCross := axes(sizes[index], horizontal)
		crossStart := 0
		switch crossAlign {
		case CrossStretch:
			childCross = availableCross
		case CrossCenter:
			crossStart = (availableCross - childCross) / 2
		case CrossEnd:
			crossStart = availableCross - childCross
		}
		childBounds := rectFromAxes(bounds.Min, cursor, crossStart, childMain, childCross, horizontal)
		if err := child.Node.paint(ctx, list, childBounds, nodePath); err != nil {
			return err
		}
		cursor += childMain + gap
	}
	return nil
}

// Stack paints children in slice order into the same allocated rectangle.
// Later children therefore appear over earlier children.
type Stack struct {
	Size     image.Point
	Children []Node
}

func (Stack) composeNode() {}

func (s Stack) measure(ctx *compileContext, maximum image.Point, path string) (image.Point, error) {
	if !validSize(s.Size) {
		return image.Point{}, fmt.Errorf("%s: size must not be negative, got %v", path, s.Size)
	}
	natural := image.Point{}
	for index, child := range s.Children {
		nodePath := childPath(path, "children", index)
		if nilNode(child) {
			return image.Point{}, fmt.Errorf("%s: node must not be nil", nodePath)
		}
		size, err := child.measure(ctx, maximum, nodePath)
		if err != nil {
			return image.Point{}, err
		}
		natural.X = max(natural.X, size.X)
		natural.Y = max(natural.Y, size.Y)
	}
	return preferredSize(natural, s.Size, maximum)
}

func (s Stack) paint(ctx *compileContext, list *display.DisplayList, bounds image.Rectangle, path string) error {
	for index, child := range s.Children {
		if err := child.paint(ctx, list, bounds, childPath(path, "children", index)); err != nil {
			return err
		}
	}
	return nil
}

// Padding reduces the rectangle passed to Child by the declared insets.
type Padding struct {
	Insets Insets
	Child  Node
}

func (Padding) composeNode() {}

func (p Padding) measure(ctx *compileContext, maximum image.Point, path string) (image.Point, error) {
	if !p.Insets.valid() {
		return image.Point{}, fmt.Errorf("%s: padding must not be negative", path)
	}
	if nilNode(p.Child) {
		return image.Point{}, fmt.Errorf("%s.child: node must not be nil", path)
	}
	innerMax := image.Pt(max(0, maximum.X-p.Insets.horizontal()), max(0, maximum.Y-p.Insets.vertical()))
	size, err := p.Child.measure(ctx, innerMax, path+".child")
	if err != nil {
		return image.Point{}, err
	}
	return constrainSize(image.Pt(size.X+p.Insets.horizontal(), size.Y+p.Insets.vertical()), maximum), nil
}

func (p Padding) paint(ctx *compileContext, list *display.DisplayList, bounds image.Rectangle, path string) error {
	min := bounds.Min.Add(image.Pt(p.Insets.Left, p.Insets.Top))
	maxPoint := bounds.Max.Sub(image.Pt(p.Insets.Right, p.Insets.Bottom))
	if min.X >= maxPoint.X || min.Y >= maxPoint.Y {
		ctx.warn(path, "empty-layout", "padding leaves no drawable area")
		return nil
	}
	inner := image.Rectangle{Min: min, Max: maxPoint}
	return p.Child.paint(ctx, list, inner, path+".child")
}

// Spacer reserves an explicit size and paints nothing.
type Spacer struct{ Size image.Point }

func (Spacer) composeNode() {}

func (s Spacer) measure(_ *compileContext, maximum image.Point, path string) (image.Point, error) {
	if !validSize(s.Size) {
		return image.Point{}, fmt.Errorf("%s: spacer size must not be negative, got %v", path, s.Size)
	}
	return constrainSize(s.Size, maximum), nil
}

func (Spacer) paint(*compileContext, *display.DisplayList, image.Rectangle, string) error { return nil }

func axes(size image.Point, horizontal bool) (main, cross int) {
	if horizontal {
		return size.X, size.Y
	}
	return size.Y, size.X
}

func fromAxes(main, cross int, horizontal bool) image.Point {
	if horizontal {
		return image.Pt(main, cross)
	}
	return image.Pt(cross, main)
}

func mainOf(size image.Point, horizontal bool) int {
	main, _ := axes(size, horizontal)
	return main
}

func setMain(size *image.Point, horizontal bool, value int) {
	if horizontal {
		size.X = value
	} else {
		size.Y = value
	}
}

func rectFromAxes(origin image.Point, mainStart, crossStart, mainSize, crossSize int, horizontal bool) image.Rectangle {
	if horizontal {
		min := origin.Add(image.Pt(mainStart, crossStart))
		return image.Rectangle{Min: min, Max: min.Add(image.Pt(mainSize, crossSize))}
	}
	min := origin.Add(image.Pt(crossStart, mainStart))
	return image.Rectangle{Min: min, Max: min.Add(image.Pt(crossSize, mainSize))}
}
