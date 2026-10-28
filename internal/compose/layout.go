package compose

import (
	"fmt"
	"image"
	"math"
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
	// MainSpaceBetween and the two after it are justify-content's
	// space-between, space-around and space-evenly: the line's free space
	// goes between the children rather than at its ends. Like the others
	// they only share out space nothing grew into.
	MainSpaceBetween
	MainSpaceAround
	MainSpaceEvenly
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
	// Grow divides positive free space by weight, after every base size and
	// gap has been allocated. Shrink divides negative free space by the CSS
	// scaled flex base size (shrink * base size).
	Grow   float64
	Shrink float64
	// Margin is resolved against the containing block's inline size. It is
	// included in the flex line's occupied space but not in the child's box.
	Margin [4]Length
	// MinMain and the rest bound the resolved sizes on each axis.
	MinMain, MaxMain   Length
	MinCross, MaxCross Length
	// AutoMargin marks the sides, top, right, bottom and left, whose margin
	// is auto. Along the line they share the free space before any
	// alignment does, so margin-left: auto pushes a child to the far end;
	// across it they place the child instead of stretching it, so a margin
	// of auto either side centres it.
	AutoMargin [4]bool
	// MinContent keeps the child from shrinking below its content along the
	// axis: its widest word in a row, the height of what it holds in a
	// column, or Stated if that is smaller. It is CSS's min-width: auto (or
	// min-height) on a flex item, which is why a label next to a long
	// paragraph keeps its word whole.
	MinContent bool
	// Stated is the size the child gives itself along the axis, its width
	// or height, which Basis does not always say: flex-basis replaces it as
	// the size to start from, but it is still what caps MinContent.
	Stated Length
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

// ratioMain derives the main size from the cross one when a ratio was given
// and no main size was, and there is a cross size to work from: one that was
// stated, or the one a stretched child of a column fills. The second is how a
// block with nothing but aspect-ratio takes its height from the width it is
// given, as it does in a browser.
func (c LayoutChild) ratioMain(crossAvailable int, horizontal bool, crossAlign CrossAlignment) (int, bool) {
	if c.Ratio <= 0 || c.Basis.IsSet() {
		return 0, false
	}
	cross, ok := c.Cross.Resolve(crossAvailable)
	if !ok {
		if horizontal || c.alignment(crossAlign) != CrossStretch || crossAvailable >= unboundedMeasure {
			return 0, false
		}
		cross = crossAvailable
	}
	cross = clamp(cross, c.MinCross, c.MaxCross, crossAvailable)
	if horizontal {
		return int(float64(float64(cross)*c.Ratio) + 0.5), true
	}
	return int(float64(cross)/c.Ratio + 0.5), true
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
	return int(float64(float64(mainSize)*c.Ratio) + 0.5), true
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
	return c.Basis.valid() && c.Cross.valid() && c.Grow >= 0 && c.Shrink >= 0 &&
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
		if err := ctx.paintWithContaining(child.Node, list, childBounds, bounds, nodePath); err != nil {
			return err
		}
	}
	return nil
}

// Row lays children out from left to right. It never reorders them.
type Row struct {
	Size       image.Point
	Gap        int
	GapLength  Length
	MainAlign  MainAlignment
	CrossAlign CrossAlignment
	Children   []LayoutChild
	Wrap       Wrapping
}

func (Row) composeNode() {}

func (r Row) measure(ctx *compileContext, maximum image.Point, path string) (image.Point, error) {
	if r.Wrap.Wrap {
		return measureWrapped(ctx, maximum, r.Size, r.Gap, r.GapLength, r.CrossAlign, r.Wrap, true, r.Children, path)
	}
	return measureFlow(ctx, maximum, r.Size, r.Gap, r.GapLength, r.CrossAlign, true, r.Children, path)
}

func (r Row) paint(ctx *compileContext, list *display.DisplayList, bounds image.Rectangle, path string) error {
	if r.Wrap.Wrap {
		return paintWrapped(ctx, list, bounds, r.Gap, r.GapLength, r.MainAlign, r.CrossAlign, r.Wrap, true, r.Children, path)
	}
	return paintFlow(ctx, list, bounds, r.Gap, r.GapLength, r.MainAlign, r.CrossAlign, true, r.Children, path)
}

// Column lays children out from top to bottom. It never reorders them.
type Column struct {
	Size       image.Point
	Gap        int
	GapLength  Length
	MainAlign  MainAlignment
	CrossAlign CrossAlignment
	Children   []LayoutChild
	Wrap       Wrapping
}

func (Column) composeNode() {}

func (c Column) measure(ctx *compileContext, maximum image.Point, path string) (image.Point, error) {
	if c.Wrap.Wrap {
		return measureWrapped(ctx, maximum, c.Size, c.Gap, c.GapLength, c.CrossAlign, c.Wrap, false, c.Children, path)
	}
	return measureFlow(ctx, maximum, c.Size, c.Gap, c.GapLength, c.CrossAlign, false, c.Children, path)
}

func (c Column) paint(ctx *compileContext, list *display.DisplayList, bounds image.Rectangle, path string) error {
	if c.Wrap.Wrap {
		return paintWrapped(ctx, list, bounds, c.Gap, c.GapLength, c.MainAlign, c.CrossAlign, c.Wrap, false, c.Children, path)
	}
	return paintFlow(ctx, list, bounds, c.Gap, c.GapLength, c.MainAlign, c.CrossAlign, false, c.Children, path)
}

// Wrapping is flex-wrap: children that do not fit on one line go on to the
// next, each line laid out as a line of its own, and the lines stacked across
// the box with CrossGap between them. AlignContent shares out the room the
// lines leave across the box, as align-content does; nil stretches the lines
// into it, which is what a browser does with normal.
type Wrapping struct {
	Wrap           bool
	CrossGap       int
	CrossGapLength Length
	AlignContent   *MainAlignment
}

func measureFlow(ctx *compileContext, maximum, preferred image.Point, gap int, gapLength Length, crossAlign CrossAlignment, horizontal bool, children []LayoutChild, path string) (image.Point, error) {
	restoreInline := ctx.pushMeasureInline(maximum.X)
	defer restoreInline()
	gap = resolvedGap(gap, gapLength, mainOf(maximum, horizontal))
	if gap < 0 || !gapLength.valid() {
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
		size, err := child.Node.measure(ctx, flowMeasureMaximum(maximum, horizontal), nodePath)
		if err != nil {
			return image.Point{}, err
		}
		childMain, childCross := axes(size, horizontal)
		if derived, ok := child.ratioMain(mainOf(maximum, !horizontal), horizontal, crossAlign); ok {
			childMain = derived
		}
		childMain = child.intrinsicMain(childMain)
		main += childMain + measuredMargin(child.Margin, horizontal, maximum.X)
		cross = max(cross, childCross+measuredMargin(child.Margin, !horizontal, maximum.X))
	}
	if len(children) > 1 {
		main += gap * (len(children) - 1)
	}
	// A row too wide for its box is shrunk into it when it is painted, and
	// what it holds is then laid out narrower: a paragraph wraps onto more
	// lines and the row is as tall as that, not as the one long line it
	// would be with room for it.
	if available := maximum.X; horizontal && available < unboundedMeasure && main > available {
		if preferred.X > 0 {
			available = min(available, preferred.X)
		}
		sizes, margins, err := flexLine(ctx, image.Pt(available, maximum.Y), gap, crossAlign, horizontal, children, path, 0)
		if err != nil {
			return image.Point{}, err
		}
		cross = 0
		for index, size := range sizes {
			cross = max(cross, size.Y+margins[index].vertical())
		}
	}
	natural := fromAxes(main, cross, horizontal)
	return preferredSize(natural, preferred, maximum)
}

// flexLine works out the size of each child of a line in a box of the given
// size, the way the browser's flex algorithm does: each starts at its base
// size, the line's free space is shared out by grow or shrink, and each is
// then measured across the line at the size it was given along it.
//
// The children are the ones from first on in the node path names, so a line
// of a wrapped container still names each child by where it is in the whole.
func flexLine(ctx *compileContext, box image.Point, gap int, crossAlign CrossAlignment, horizontal bool, children []LayoutChild, path string, first int) ([]image.Point, []Insets, error) {
	sizes := make([]image.Point, len(children))
	measuredMains := make([]int, len(children))
	margins := make([]Insets, len(children))
	availableMain := mainOf(box, horizontal)
	baseMain, marginMain := 0, 0
	for index, child := range children {
		nodePath := childPath(path, "children", first+index)
		size, err := child.Node.measure(ctx, flowMeasureMaximum(box, horizontal), nodePath)
		if err != nil {
			return nil, nil, err
		}
		margin := child.resolvedMargin(box.X)
		margins[index] = margin
		marginSize, crossMargin := margin.horizontal(), margin.vertical()
		if !horizontal {
			marginSize, crossMargin = crossMargin, marginSize
		}
		measuredMains[index], _ = axes(size, horizontal)
		if derived, ok := child.ratioMain(max(0, mainOf(box, !horizontal)-crossMargin), horizontal, crossAlign); ok {
			measuredMains[index] = derived
		}
		setMain(&size, horizontal, child.mainSize(measuredMains[index], availableMain))
		sizes[index] = size
		marginMain += marginSize
		baseMain += mainOf(size, horizontal) + marginSize
	}
	baseMain += gap * max(0, len(children)-1)
	// The least each child may shrink to, worked out only where it can
	// matter: on a line that has to shrink, or for a child that starts out
	// smaller than its content.
	floors := make([]int, len(children))
	for index, child := range children {
		base := mainOf(sizes[index], horizontal)
		if !child.MinContent || (baseMain <= availableMain && base >= measuredMains[index]) {
			continue
		}
		floor := measuredMains[index]
		if horizontal {
			width, err := itemMinContent(ctx, child.Node, childPath(path, "children", first+index))
			if err != nil {
				return nil, nil, err
			}
			floor = width
		}
		// A child that states its size is held to the smaller of that and
		// its content, as CSS holds an item with a width.
		if stated, ok := child.Stated.Resolve(availableMain); ok {
			floor = min(floor, stated)
		}
		floors[index] = floor
		if base < floor {
			setMain(&sizes[index], horizontal, floor)
		}
	}
	resolved := resolveFlexMainSizes(children, sizes, max(0, availableMain-marginMain), gap, horizontal, floors)
	for index := range resolved {
		if horizontal && resolved[index] < measuredMains[index] {
			// Narrower than it measured, so it is measured again at the
			// width it has, for the height that gives it.
			size, err := children[index].Node.measure(ctx, image.Pt(resolved[index], unboundedMeasure), childPath(path, "children", first+index))
			if err != nil {
				return nil, nil, err
			}
			sizes[index].Y = size.Y
		}
		setMain(&sizes[index], horizontal, resolved[index])
	}
	return sizes, margins, nil
}

func paintFlow(ctx *compileContext, list *display.DisplayList, bounds image.Rectangle, gap int, gapLength Length, mainAlign MainAlignment, crossAlign CrossAlignment, horizontal bool, children []LayoutChild, path string) error {
	if mainAlign > MainSpaceEvenly {
		return fmt.Errorf("%s: invalid main alignment %d", path, mainAlign)
	}
	if crossAlign > CrossEnd {
		return fmt.Errorf("%s: invalid cross alignment %d", path, crossAlign)
	}
	maximum := bounds.Size()
	restoreInline := ctx.pushMeasureInline(maximum.X)
	defer restoreInline()
	gap = resolvedGap(gap, gapLength, mainOf(bounds.Size(), horizontal))
	if gap < 0 || !gapLength.valid() {
		return fmt.Errorf("%s: gap must not be negative", path)
	}
	sizes, margins, err := flexLine(ctx, maximum, gap, crossAlign, horizontal, children, path, 0)
	if err != nil {
		return err
	}
	return paintLine(ctx, list, bounds, gap, mainAlign, crossAlign, horizontal, children, path, 0, sizes, margins)
}

// paintLine places one line's children, at the sizes flexLine gave them,
// across the whole of bounds.
func paintLine(ctx *compileContext, list *display.DisplayList, bounds image.Rectangle, gap int, mainAlign MainAlignment, crossAlign CrossAlignment, horizontal bool, children []LayoutChild, path string, first int, sizes []image.Point, margins []Insets) error {
	availableMain, availableCross := axes(bounds.Size(), horizontal)
	baseMain := gap * max(0, len(children)-1)
	for index, size := range sizes {
		marginSize := margins[index].horizontal()
		if !horizontal {
			marginSize = margins[index].vertical()
		}
		baseMain += mainOf(size, horizontal) + marginSize
	}
	if baseMain > availableMain {
		ctx.warn(path, "layout-overflow", fmt.Sprintf("children need %d pixels on the main axis, only %d are available", baseMain, availableMain))
	}
	lead := mainLead(children, availableMain-baseMain, mainAlign, horizontal)
	cursor := 0
	for index, child := range children {
		nodePath := childPath(path, "children", first+index)
		childMain, measuredCross := axes(sizes[index], horizontal)
		alignment := child.alignment(crossAlign)
		margin := margins[index]
		crossMarginStart, crossMarginEnd := margin.Top, margin.Bottom
		if !horizontal {
			crossMarginStart, crossMarginEnd = margin.Left, margin.Right
		}
		availableChildCross := max(0, availableCross-crossMarginStart-crossMarginEnd)
		childCross := child.crossSizeOf(measuredCross, availableChildCross, alignment == CrossStretch)
		// A main size that came from the cross one leaves the cross alone.
		_, fromCross := child.ratioMain(availableChildCross, horizontal, crossAlign)
		if derived, ok := child.ratioCross(childMain, horizontal); ok && !fromCross {
			childCross = clamp(derived, child.MinCross, child.MaxCross, availableCross)
			if alignment == CrossStretch {
				alignment = CrossStart
			}
		}
		// Auto margins across the line place the child in what is left of
		// it, and so it is not stretched to fill it.
		autoStart, autoEnd := child.AutoMargin[0], child.AutoMargin[2]
		if !horizontal {
			autoStart, autoEnd = child.AutoMargin[3], child.AutoMargin[1]
		}
		if autoStart || autoEnd {
			if alignment == CrossStretch && !child.Cross.IsSet() {
				childCross = child.crossSizeOf(measuredCross, availableChildCross, false)
			}
			alignment = autoPlaced(alignment, autoStart, autoEnd)
		}
		crossStart := 0
		switch alignment {
		case CrossCenter:
			crossStart = (availableChildCross - childCross) / 2
		case CrossEnd:
			crossStart = availableChildCross - childCross
		}
		crossStart += crossMarginStart
		mainMarginStart, mainMarginEnd := margin.Left, margin.Right
		if !horizontal {
			mainMarginStart, mainMarginEnd = margin.Top, margin.Bottom
		}
		cursor += mainMarginStart + lead[index]
		childBounds := rectFromAxes(bounds.Min, cursor, crossStart, childMain, childCross, horizontal)
		if err := ctx.paintWithContaining(child.Node, list, childBounds, bounds, nodePath); err != nil {
			return err
		}
		cursor += childMain + mainMarginEnd + gap
	}
	return nil
}

func (c LayoutChild) resolvedMargin(availableWidth int) Insets {
	return Insets{Top: resolveMargin(c.Margin[0], availableWidth), Right: resolveMargin(c.Margin[1], availableWidth), Bottom: resolveMargin(c.Margin[2], availableWidth), Left: resolveMargin(c.Margin[3], availableWidth)}
}

func intrinsicMargin(margin [4]Length, horizontal bool) int {
	if horizontal {
		left, lok := intrinsicOrZero(margin[3])
		right, rok := intrinsicOrZero(margin[1])
		if lok && rok {
			return left + right
		}
		return 0
	}
	top, tok := intrinsicOrZero(margin[0])
	bottom, bok := intrinsicOrZero(margin[2])
	if tok && bok {
		return top + bottom
	}
	return 0
}

// measuredMargin is a child's margin along one axis as a measure counts it.
// A percentage is a share of the containing block's width, which a measure
// knows when it is bounded; the margins above and below then count as they
// will when painted. Across, the width is the thing being worked out, so a
// share of it counts for nothing, as in CSS.
func measuredMargin(margin [4]Length, horizontal bool, width int) int {
	if horizontal || width >= unboundedMeasure {
		return intrinsicMargin(margin, horizontal)
	}
	return resolveMargin(margin[0], width) + resolveMargin(margin[2], width)
}

func intrinsicOrZero(length Length) (int, bool) {
	if !length.IsSet() {
		return 0, true
	}
	return length.intrinsic()
}

func resolvedGap(fallback int, length Length, available int) int {
	if value, ok := length.Resolve(available); ok {
		return value
	}
	return fallback
}

func flowMeasureMaximum(maximum image.Point, horizontal bool) image.Point {
	if horizontal {
		maximum.X = unboundedMeasure
		// A flex row's cross size is not a clip. An auto-sized item may be
		// taller than the line and overflow it when its contents are visible.
		// Keep the natural cross size available for align-items to position.
		maximum.Y = unboundedMeasure
	} else {
		maximum.Y = unboundedMeasure
	}
	return maximum
}

// resolveFlexMainSizes applies the one-dimensional part of the flex sizing
// algorithm. The layout model is pixel based, so fractional results are
// rounded only after all items have participated; the final correction keeps
// the line's total equal to the available space whenever its constraints allow
// it. Min/max constraints freeze items and redistribute the remaining free
// space among the rest, matching the browser's shrink/grow loop.
func resolveFlexMainSizes(children []LayoutChild, sizes []image.Point, available, gap int, horizontal bool, floors []int) []int {
	result := make([]int, len(sizes))
	base := make([]float64, len(sizes))
	for index, size := range sizes {
		result[index] = mainOf(size, horizontal)
		base[index] = float64(result[index])
	}
	if len(sizes) == 0 {
		return result
	}
	free := float64(available - gap*max(0, len(sizes)-1))
	for _, value := range result {
		free -= float64(value)
	}
	if free > 0 {
		flexDistribute(children, result, base, free, true, available, floors)
	} else if free < 0 {
		flexDistribute(children, result, base, -free, false, available, floors)
	}
	return result
}

// mainLead is how much further along the line each child starts than it
// would packed at the start, given the free space the line has once its
// children have grown and shrunk. Auto margins take it first; justify-content
// shares out what is left, which is nothing if they took it. Each lead counts
// everything before that child, so the shares are rounded once each rather
// than adding up their rounding.
func mainLead(children []LayoutChild, free int, align MainAlignment, horizontal bool) []int {
	lead := make([]int, len(children))
	if free <= 0 || len(children) == 0 {
		return lead
	}
	startSide, endSide := 3, 1
	if !horizontal {
		startSide, endSide = 0, 2
	}
	autos := 0
	for _, child := range children {
		if child.AutoMargin[startSide] {
			autos++
		}
		if child.AutoMargin[endSide] {
			autos++
		}
	}
	share := float64(free)
	if autos > 0 {
		taken := 0
		for index, child := range children {
			if child.AutoMargin[startSide] {
				taken++
			}
			lead[index] = int(math.Round(share * float64(taken) / float64(autos)))
			if child.AutoMargin[endSide] {
				taken++
			}
		}
		return relative(lead)
	}
	return relative(distribute(free, len(children), align))
}

// distribute is how far each of count things starts past where it would be
// packed at the start, when free is shared out as align says. A share that
// is negative is not shared, so what overflows starts where it would.
func distribute(free, count int, align MainAlignment) []int {
	lead := make([]int, count)
	if free <= 0 {
		return lead
	}
	n, share := float64(count), float64(free)
	for index := range lead {
		before := 0.0
		switch align {
		case MainCenter:
			before = share / 2
		case MainEnd:
			before = share
		case MainSpaceBetween:
			if count > 1 {
				before = share * float64(index) / (n - 1)
			}
		case MainSpaceAround:
			before = share * (float64(index) + 0.5) / n
		case MainSpaceEvenly:
			before = share * float64(index+1) / (n + 1)
		}
		lead[index] = int(math.Round(before))
	}
	return lead
}

// relative turns how far each child is from where it would be packed into how
// much further each is than the one before it, which is what a cursor adds.
func relative(lead []int) []int {
	previous := 0
	for index, total := range lead {
		lead[index], previous = total-previous, total
	}
	return lead
}

func flexDistribute(children []LayoutChild, result []int, base []float64, free float64, growing bool, available int, floors []int) {
	active := make([]bool, len(children))
	for index, child := range children {
		active[index] = (growing && child.Grow > 0) || (!growing && child.Shrink > 0)
	}
	target := append([]float64(nil), base...)
	for {
		weight := 0.0
		for index, child := range children {
			if !active[index] {
				continue
			}
			if growing {
				weight += child.Grow
			} else {
				weight += float64(child.Shrink * base[index])
			}
		}
		if weight <= 0 {
			break
		}
		// Once a candidate is frozen by a min/max constraint, only its
		// clamped delta is removed from the free space. Unfrozen candidates
		// are recomputed from their original flex base size each round; their
		// previous proposal is not already committed.
		frozenDelta := 0.0
		for index := range children {
			if active[index] {
				continue
			}
			if growing {
				frozenDelta += target[index] - base[index]
			} else {
				frozenDelta += base[index] - target[index]
			}
		}
		remaining := free - frozenDelta
		if remaining <= 0.001 {
			break
		}
		frozen := false
		proposal := append([]float64(nil), target...)
		for index, child := range children {
			if !active[index] {
				continue
			}
			factor := child.Grow
			if !growing {
				factor = float64(child.Shrink * base[index])
			}
			delta := remaining * factor / weight
			// Recompute active candidates from the original flex base size. Frozen
			// items retain their clamped target while the remaining free space is
			// redistributed among the unfrozen items.
			candidate := base[index] + delta
			if !growing {
				candidate = base[index] - delta
			}
			clamped := max(clampFloat(candidate, child, available), float64(floors[index]))
			if math.Abs(clamped-candidate) > 0.001 {
				proposal[index] = clamped
				active[index] = false
				frozen = true
			} else {
				proposal[index] = candidate
			}
		}
		target = proposal
		if !frozen {
			break
		}
	}
	targetSum := int(math.Round(sumFloat(target)))
	got := 0
	for index, value := range target {
		result[index] = max(0, int(math.Floor(value)))
		got += result[index]
	}
	// Keep integer rounding deterministic and preserve the available line
	// length. Earlier items receive the indivisible pixels, which is also how
	// the previous integer grow path behaved.
	// The pixels are moved a whole run at a time: one at a time, a line some
	// way past the frame would take as many steps as it has pixels.
	if delta := targetSum - got; delta > 0 {
		for index := range result {
			if active[index] {
				result[index] += delta
				break
			}
		}
	}
	for delta, index := got-targetSum, len(result)-1; delta > 0 && index >= 0; index-- {
		if !active[index] || result[index] <= 0 {
			continue
		}
		taken := min(delta, result[index])
		result[index] -= taken
		delta -= taken
	}
}

func sumFloat(values []float64) float64 {
	total := 0.0
	for _, value := range values {
		total += value
	}
	return total
}

func clampFloat(value float64, child LayoutChild, available int) float64 {
	if low, ok := child.MinMain.Resolve(available); ok && value < float64(low) {
		value = float64(low)
	}
	if high, ok := child.MaxMain.Resolve(available); ok && value > float64(high) {
		value = float64(high)
	}
	return value
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
		childMaximum := maximum
		if s.Size.X > 0 {
			childMaximum.X = min(s.Size.X, maximum.X)
		}
		if s.Size.Y > 0 {
			childMaximum.Y = min(s.Size.Y, maximum.Y)
		}
		restoreInline := ctx.pushMeasureInline(childMaximum.X)
		size, err := child.measure(ctx, childMaximum, nodePath)
		restoreInline()
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
		if err := ctx.paintWithContaining(child, list, bounds, bounds, childPath(path, "children", index)); err != nil {
			return err
		}
	}
	return nil
}

// Padding reduces the rectangle passed to Child by the declared insets.
type Padding struct {
	Insets  Insets
	Lengths [4]Length
	Child   Node
}

func (Padding) composeNode() {}

func (p Padding) measure(ctx *compileContext, maximum image.Point, path string) (image.Point, error) {
	if !p.Insets.valid() || !lengthsValid(p.Lengths) {
		return image.Point{}, fmt.Errorf("%s: padding must not be negative", path)
	}
	if nilNode(p.Child) {
		return image.Point{}, fmt.Errorf("%s.child: node must not be nil", path)
	}
	availableWidth := ctx.measureInlineWidth(maximum)
	insets := p.resolved(availableWidth)
	innerMax := image.Pt(max(0, maximum.X-insets.horizontal()), max(0, maximum.Y-insets.vertical()))
	restoreInline := ctx.pushMeasureInline(max(0, availableWidth-insets.horizontal()))
	size, err := p.Child.measure(ctx, innerMax, path+".child")
	restoreInline()
	if err != nil {
		return image.Point{}, err
	}
	return constrainSize(image.Pt(size.X+insets.horizontal(), size.Y+insets.vertical()), maximum), nil
}

func (p Padding) paint(ctx *compileContext, list *display.DisplayList, bounds image.Rectangle, path string) error {
	insets := p.resolved(bounds.Dx())
	min := bounds.Min.Add(image.Pt(insets.Left, insets.Top))
	maxPoint := bounds.Max.Sub(image.Pt(insets.Right, insets.Bottom))
	if min.X >= maxPoint.X || min.Y >= maxPoint.Y {
		// Padding round nothing is how a box that is only its border comes
		// out, a rule drawn with border-top and no height, and there was
		// nothing inside it to lose.
		if _, nothing := p.Child.(Spacer); !nothing {
			ctx.warn(path, "empty-layout", "padding leaves no drawable area")
		}
		return nil
	}
	inner := image.Rectangle{Min: min, Max: maxPoint}
	return ctx.paintWithContaining(p.Child, list, inner, inner, path+".child")
}

func (p Padding) resolved(availableWidth int) Insets {
	if !lengthsSet(p.Lengths) {
		return p.Insets
	}
	return Insets{Top: resolveInset(p.Lengths[0], availableWidth), Right: resolveInset(p.Lengths[1], availableWidth), Bottom: resolveInset(p.Lengths[2], availableWidth), Left: resolveInset(p.Lengths[3], availableWidth)}
}

func resolveInset(length Length, available int) int {
	if value, ok := length.Resolve(available); ok {
		return value
	}
	return 0
}

func resolveMargin(length Length, available int) int {
	if value, ok := length.Offset(available); ok {
		return value
	}
	return 0
}

func lengthsSet(lengths [4]Length) bool {
	for _, length := range lengths {
		if length.IsSet() {
			return true
		}
	}
	return false
}

func lengthsValid(lengths [4]Length) bool {
	for _, length := range lengths {
		if !length.valid() {
			return false
		}
	}
	return true
}

// Relative keeps its child's measured flow box but paints that box at an
// offset. Unlike Anchored, it does not remove the child from flow or change
// the size its parent allocates. The four edges follow CSS precedence: the
// start edge wins when both edges on one axis are stated, and the end edge
// moves in the opposite direction when it stands alone.
type Relative struct {
	Top, Right, Bottom, Left Length
	Child                    Node
}

func (Relative) composeNode() {}

func (r Relative) measure(ctx *compileContext, maximum image.Point, path string) (image.Point, error) {
	if nilNode(r.Child) {
		return image.Point{}, fmt.Errorf("%s.child: node must not be nil", path)
	}
	return r.Child.measure(ctx, maximum, path+".child")
}

func (r Relative) paint(ctx *compileContext, list *display.DisplayList, bounds image.Rectangle, path string) error {
	if nilNode(r.Child) {
		return fmt.Errorf("%s.child: node must not be nil", path)
	}
	container := ctx.containing
	dx := relativeShift(r.Left, r.Right, container.Dx())
	dy := relativeShift(r.Top, r.Bottom, container.Dy())
	shifted := bounds.Add(image.Pt(dx, dy))
	return ctx.paintWithContaining(r.Child, list, shifted, shifted, path+".child")
}

func relativeShift(start, end Length, available int) int {
	if value, ok := start.Offset(available); ok {
		return value
	}
	if value, ok := end.Offset(available); ok {
		return -value
	}
	return 0
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
	// Replaced selects intrinsic sizing rather than ordinary auto-size
	// stretching. Ratio and the auto flags describe the SVG dimensions before
	// its insets are resolved.
	Ratio                 float64
	Replaced              bool
	AutoWidth, AutoHeight bool
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
		if child.Ratio < 0 || math.IsNaN(child.Ratio) || math.IsInf(child.Ratio, 0) {
			return image.Point{}, fmt.Errorf("%s: ratio must be finite and non-negative", nodePath)
		}
		if _, err := child.Node.measure(ctx, child.measureMaximum(maximum), nodePath); err != nil {
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
		// Auto-sized positioned boxes use their measured content size when one
		// edge, or neither edge, establishes their position. Measure against the
		// space left after the declared insets so wrapping still follows the
		// containing block rather than the whole page.
		natural, err := child.Node.measure(ctx, child.measureMaximum(bounds.Size()), nodePath)
		if err != nil {
			return err
		}
		placed := child.resolve(bounds, natural)
		if placed.Empty() {
			// A stated zero size is a valid, intentional state (for example an
			// empty progress fill). Keep the diagnostic for an auto-sized box
			// that unexpectedly resolved to no area.
			explicitZero := (child.Width.IsSet() && placed.Dx() == 0) ||
				(child.Height.IsSet() && placed.Dy() == 0)
			if !explicitZero {
				ctx.warn(nodePath, "empty-layout", "the anchored box resolved to no area")
			}
			continue
		}
		if err := ctx.paintWithContaining(child.Node, list, placed, bounds, nodePath); err != nil {
			return err
		}
	}
	return nil
}

// maximum returns the available containing-block space after the insets. It is
// used for measuring an auto-sized positioned child, where the available width
// is the shrink-to-fit limit rather than the whole containing block.
func (a Anchor) maximum(available image.Point) image.Point {
	axis := func(startLen, endLen Length, size int) int {
		original := size
		if start, ok := startLen.Offset(original); ok {
			size -= start
		}
		if end, ok := endLen.Offset(original); ok {
			size -= end
		}
		return max(0, size)
	}
	return image.Pt(
		axis(a.Left, a.Right, available.X),
		axis(a.Top, a.Bottom, available.Y),
	)
}

func (a Anchor) measureMaximum(available image.Point) image.Point {
	if a.Replaced {
		return image.Pt(unboundedMeasure, unboundedMeasure)
	}
	return a.maximum(available)
}

// resolve turns the insets into a rectangle inside the container, following
// the same rules CSS does for an absolutely positioned box. An auto-sized
// child uses its measured natural size unless both edges on that axis are
// stated, in which case the edges stretch it between them.
func (a Anchor) resolve(bounds image.Rectangle, natural image.Point) image.Rectangle {
	if a.Replaced {
		return a.resolveReplaced(bounds, natural)
	}
	span := func(startLen, endLen, sizeLen Length, natural, low, high int) (int, int) {
		available := high - low
		// The insets are distances and may be negative, so that a box can be
		// hung off the edge of its container. The size between them cannot.
		start, hasStart := startLen.Offset(available)
		end, hasEnd := endLen.Offset(available)
		size, hasSize := sizeLen.Resolve(available)
		if !hasSize {
			size = max(0, natural)
		}
		switch {
		case hasStart && hasEnd:
			return low + start, high - end
		case hasStart:
			return low + start, low + start + size
		case hasEnd:
			return high - end - size, high - end
		}
		return low, low + size
	}
	left, right := span(a.Left, a.Right, a.Width, natural.X, bounds.Min.X, bounds.Max.X)
	top, bottom := span(a.Top, a.Bottom, a.Height, natural.Y, bounds.Min.Y, bounds.Max.Y)
	return image.Rect(left, top, right, bottom)
}

func (a Anchor) resolveReplaced(bounds image.Rectangle, natural image.Point) image.Rectangle {
	available := bounds.Size()
	width, hasWidth := a.Width.Resolve(available.X)
	height, hasHeight := a.Height.Resolve(available.Y)
	if !hasWidth {
		width = natural.X
	}
	if !hasHeight {
		height = natural.Y
	}

	if a.Ratio > 0 {
		switch {
		case !a.AutoWidth && a.AutoHeight:
			height = ratioHeight(width, a.Ratio)
		case a.AutoWidth && !a.AutoHeight:
			width = ratioWidth(height, a.Ratio)
		case a.AutoWidth && a.AutoHeight:
			left, _ := a.Left.Offset(available.X)
			right, _ := a.Right.Offset(available.X)
			width = max(0, available.X-left-right)
			height = ratioHeight(width, a.Ratio)
		}
	}
	width, height = max(0, width), max(0, height)

	place := func(startLen, endLen Length, size, low, high int) int {
		if start, ok := startLen.Offset(high - low); ok {
			return low + start
		}
		if end, ok := endLen.Offset(high - low); ok {
			return high - end - size
		}
		return low
	}
	left := place(a.Left, a.Right, width, bounds.Min.X, bounds.Max.X)
	top := place(a.Top, a.Bottom, height, bounds.Min.Y, bounds.Max.Y)
	return image.Rect(left, top, left+width, top+height)
}
