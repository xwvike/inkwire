package compose

import (
	"fmt"
	"image"
	"slices"

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

// LayoutChild wraps one child in a Row or Column.
//
// The sizes here are lengths rather than pixel counts because a percentage
// only becomes a number once the container has one, and that is not until the
// layout runs. Resolving them here rather than earlier is what lets Basis,
// Cross and the four limits all be stated the same way and mean the same
// thing, whichever axis they land on.
type LayoutChild struct {
	Node Node
	// Basis overrides the child's measured size along the container's axis.
	Basis Length
	// Cross overrides it across the container's axis. A child with one is
	// never stretched, because stretching is what fills a size nobody stated.
	Cross Length
	// Grow divides what is left over by integer weight, after every base size
	// and Gap has been allocated.
	Grow int
	// MinMain and the rest bound the resolved sizes on each axis.
	MinMain, MaxMain   Length
	MinCross, MaxCross Length
	// AlignSelf places this child across the axis regardless of what the
	// container asked for.
	AlignSelf *CrossAlignment
	// Ratio ties the two axes together as width divided by height, so a box
	// that states one size takes the other from it. Zero leaves them free.
	Ratio float64
}

// mainSize resolves the child's size along the container's axis, falling back
// to what it measured.
func (c LayoutChild) mainSize(measured, available int) int {
	size := measured
	if resolved, ok := c.Basis.Resolve(available); ok {
		size = resolved
	}
	return clamp(size, c.MinMain, c.MaxMain, available)
}

// intrinsicMain is the same question asked before a container exists, where a
// percentage has nothing to be a percentage of.
func (c LayoutChild) intrinsicMain(measured int) int {
	size := measured
	if stated, ok := c.Basis.intrinsic(); ok {
		size = stated
	}
	if low, ok := c.MinMain.intrinsic(); ok && size < low {
		size = low
	}
	if high, ok := c.MaxMain.intrinsic(); ok && size > high {
		size = high
	}
	return size
}

// crossSizeOf resolves the child's size across the container's axis. stretched
// says whether the container would otherwise fill it.
func (c LayoutChild) crossSizeOf(measured, available int, stretched bool) int {
	size := measured
	if stretched {
		size = available
	}
	if resolved, ok := c.Cross.Resolve(available); ok {
		size = resolved
	}
	return clamp(size, c.MinCross, c.MaxCross, available)
}

// ratioCross derives the cross size from the main one when a ratio was given
// and no cross size was, which is the case aspect-ratio exists for.
func (c LayoutChild) ratioCross(mainSize int, horizontal bool) (int, bool) {
	if c.Ratio <= 0 || c.Cross.IsSet() {
		return 0, false
	}
	if horizontal {
		return int(float64(mainSize)/c.Ratio + 0.5), true
	}
	return int(float64(mainSize)*c.Ratio + 0.5), true
}

func (c LayoutChild) alignment(container CrossAlignment) CrossAlignment {
	if c.AlignSelf != nil {
		return *c.AlignSelf
	}
	// A child that states its cross size is never stretched; CSS treats
	// stretch as filling a size nobody gave, and this one was given.
	if container == CrossStretch && c.Cross.IsSet() {
		return CrossStart
	}
	return container
}

func (c LayoutChild) valid() bool {
	return c.Basis.valid() && c.Cross.valid() && c.Grow >= 0 &&
		c.MinMain.valid() && c.MaxMain.valid() && c.MinCross.valid() && c.MaxCross.valid()
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
		if !child.valid() {
			return image.Point{}, fmt.Errorf("%s: sizes and grow must not be negative", nodePath)
		}
		size, err := child.Node.measure(ctx, maximum, nodePath)
		if err != nil {
			return image.Point{}, err
		}
		childMain, childCross := axes(size, horizontal)
		childMain = child.intrinsicMain(childMain)
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
		measuredMain, _ := axes(size, horizontal)
		setMain(&size, horizontal, child.mainSize(measuredMain, mainOf(bounds.Size(), horizontal)))
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
		// A maximum bounds the size however it was arrived at, so growing is
		// clamped after the fact rather than being allowed to pass it.
		for index, child := range children {
			grown := mainOf(sizes[index], horizontal)
			if capped := clamp(grown, child.MinMain, child.MaxMain, availableMain); capped != grown {
				setMain(&sizes[index], horizontal, capped)
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
		childMain, measuredCross := axes(sizes[index], horizontal)
		alignment := child.alignment(crossAlign)
		childCross := child.crossSizeOf(measuredCross, availableCross, alignment == CrossStretch)
		if derived, ok := child.ratioCross(childMain, horizontal); ok {
			childCross = clamp(derived, child.MinCross, child.MaxCross, availableCross)
			if alignment == CrossStretch {
				alignment = CrossStart
			}
		}
		crossStart := 0
		switch alignment {
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

// Anchor places one child against the edges of its container. Which edges were
// named decides how it is sized: opposite edges stretch it between them, a
// single edge holds it at its own size, and neither leaves it at the origin.
type Anchor struct {
	Top, Right, Bottom, Left Length
	Width, Height            Length
	Node                     Node
	// Layer orders overlapping children. Higher is painted later, so it
	// appears over lower ones; equal layers keep their document order.
	Layer int
}

// Anchored resolves its children's insets against the box it is finally given,
// which is why it exists alongside Absolute: an absolute rectangle has to be
// known when the document is written, and an inset from the far edge cannot be
// until the container has been laid out.
type Anchored struct {
	Size     image.Point
	Children []Anchor
}

func (Anchored) composeNode() {}

func (a Anchored) measure(ctx *compileContext, maximum image.Point, path string) (image.Point, error) {
	for index, child := range a.Children {
		nodePath := childPath(path, "children", index)
		if nilNode(child.Node) {
			return image.Point{}, fmt.Errorf("%s: node must not be nil", nodePath)
		}
		if !child.Width.valid() || !child.Height.valid() {
			return image.Point{}, fmt.Errorf("%s: size must not be negative", nodePath)
		}
		if _, err := child.Node.measure(ctx, maximum, nodePath); err != nil {
			return image.Point{}, err
		}
	}
	// Anchored children are placed against their container and contribute
	// nothing to how large it wants to be, which is what taking them out of
	// the flow means.
	return constrainSize(a.Size, maximum), nil
}

func (a Anchored) paint(ctx *compileContext, list *display.DisplayList, bounds image.Rectangle, path string) error {
	ordered := make([]int, len(a.Children))
	for index := range ordered {
		ordered[index] = index
	}
	slices.SortStableFunc(ordered, func(i, j int) int { return a.Children[i].Layer - a.Children[j].Layer })
	for _, index := range ordered {
		child := a.Children[index]
		nodePath := childPath(path, "children", index)
		placed := child.resolve(bounds)
		if placed.Empty() {
			ctx.warn(nodePath, "empty-layout", "the anchored box resolved to no area")
			continue
		}
		if err := child.Node.paint(ctx, list, placed, nodePath); err != nil {
			return err
		}
	}
	return nil
}

// resolve turns the insets into a rectangle inside the container, following
// the same rules CSS does for an absolutely positioned box.
func (a Anchor) resolve(bounds image.Rectangle) image.Rectangle {
	span := func(startLen, endLen, sizeLen Length, low, high int) (int, int) {
		available := high - low
		// The insets are distances and may be negative, so that a box can be
		// hung off the edge of its container. The size between them cannot.
		start, hasStart := startLen.Offset(available)
		end, hasEnd := endLen.Offset(available)
		size, hasSize := sizeLen.Resolve(available)
		switch {
		case hasStart && hasEnd:
			return low + start, high - end
		case hasStart:
			if hasSize {
				return low + start, low + start + size
			}
			return low + start, high
		case hasEnd:
			if hasSize {
				return high - end - size, high - end
			}
			return low, high - end
		case hasSize:
			return low, low + size
		}
		return low, high
	}
	left, right := span(a.Left, a.Right, a.Width, bounds.Min.X, bounds.Max.X)
	top, bottom := span(a.Top, a.Bottom, a.Height, bounds.Min.Y, bounds.Max.Y)
	return image.Rect(left, top, right, bottom)
}
