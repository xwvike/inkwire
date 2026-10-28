package compose

import (
	"fmt"
	"image"

	"github.com/xwvike/inkwire/internal/display"
)

// flexWrapLine is one line of a wrapped flex container: the children from
// first up to end, and how much of the line they take before any of them
// grows or shrinks.
type flexWrapLine struct {
	first, end int
	used       int
}

// wrapLines breaks children into the lines a wrapping flex container puts
// them on, as CSS does: each child is put on the current line if its
// hypothetical size fits in what the line has left, and starts a new one if
// not. A child's hypothetical size is where it starts — its basis, or what it
// measures — held up to the least its content can be laid out in, so that a
// row of flex: 1 items wraps when their words no longer fit side by side and
// not before. A child too big for any line is put on a line of its own.
func wrapLines(ctx *compileContext, box image.Point, gap int, crossAlign CrossAlignment, horizontal bool, children []LayoutChild, path string) ([]flexWrapLine, error) {
	available := mainOf(box, horizontal)
	var lines []flexWrapLine
	line := flexWrapLine{}
	for index, child := range children {
		nodePath := childPath(path, "children", index)
		if nilNode(child.Node) {
			return nil, fmt.Errorf("%s: node must not be nil", nodePath)
		}
		if !child.valid() {
			return nil, fmt.Errorf("%s: sizes and grow must not be negative", nodePath)
		}
		size, err := child.Node.measure(ctx, flowMeasureMaximum(box, horizontal), nodePath)
		if err != nil {
			return nil, err
		}
		margin := child.resolvedMargin(box.X)
		marginMain, marginCross := margin.horizontal(), margin.vertical()
		if !horizontal {
			marginMain, marginCross = marginCross, marginMain
		}
		measured, _ := axes(size, horizontal)
		if derived, ok := child.ratioMain(max(0, mainOf(box, !horizontal)-marginCross), horizontal, crossAlign); ok {
			measured = derived
		}
		hypothetical := child.mainSize(measured, available)
		if child.MinContent && hypothetical < measured {
			floor := measured
			if horizontal {
				if floor, err = itemMinContent(ctx, child.Node, nodePath); err != nil {
					return nil, err
				}
			}
			if stated, ok := child.Stated.Resolve(available); ok {
				floor = min(floor, stated)
			}
			hypothetical = max(hypothetical, floor)
		}
		outer := hypothetical + marginMain
		if index > line.first && line.used+gap+outer > available {
			line.end = index
			lines = append(lines, line)
			line = flexWrapLine{first: index}
		}
		if index > line.first {
			line.used += gap
		}
		line.used += outer
	}
	line.end = len(children)
	return append(lines, line), nil
}

func measureWrapped(ctx *compileContext, maximum, preferred image.Point, gap int, gapLength Length, crossAlign CrossAlignment, wrap Wrapping, horizontal bool, children []LayoutChild, path string) (image.Point, error) {
	restoreInline := ctx.pushMeasureInline(maximum.X)
	defer restoreInline()
	if !validSize(preferred) {
		return image.Point{}, fmt.Errorf("%s: size must not be negative, got %v", path, preferred)
	}
	box := maximum
	if preferred.X > 0 {
		box.X = min(box.X, preferred.X)
	}
	if preferred.Y > 0 {
		box.Y = min(box.Y, preferred.Y)
	}
	gap, crossGap, err := wrapGaps(gap, gapLength, wrap, box, horizontal, path)
	if err != nil {
		return image.Point{}, err
	}
	lines, err := wrapLines(ctx, box, gap, crossAlign, horizontal, children, path)
	if err != nil {
		return image.Point{}, err
	}
	main, cross := 0, crossGap*max(0, len(lines)-1)
	for _, line := range lines {
		_, _, lineCross, err := wrapLineSizes(ctx, box, gap, crossAlign, horizontal, children, path, line)
		if err != nil {
			return image.Point{}, err
		}
		main = max(main, line.used)
		cross += lineCross
	}
	return preferredSize(fromAxes(main, cross, horizontal), preferred, maximum)
}

// wrapLineSizes lays one line out on its own and says how far across the
// box it reaches.
func wrapLineSizes(ctx *compileContext, box image.Point, gap int, crossAlign CrossAlignment, horizontal bool, children []LayoutChild, path string, line flexWrapLine) ([]image.Point, []Insets, int, error) {
	sizes, margins, err := flexLine(ctx, box, gap, crossAlign, horizontal, children[line.first:line.end], path, line.first)
	if err != nil {
		return nil, nil, 0, err
	}
	cross := 0
	for index, size := range sizes {
		marginCross := margins[index].vertical()
		if !horizontal {
			marginCross = margins[index].horizontal()
		}
		_, childCross := axes(size, horizontal)
		cross = max(cross, childCross+marginCross)
	}
	return sizes, margins, cross, nil
}

func paintWrapped(ctx *compileContext, list *display.DisplayList, bounds image.Rectangle, gap int, gapLength Length, mainAlign MainAlignment, crossAlign CrossAlignment, wrap Wrapping, horizontal bool, children []LayoutChild, path string) error {
	if mainAlign > MainSpaceEvenly {
		return fmt.Errorf("%s: invalid main alignment %d", path, mainAlign)
	}
	if crossAlign > CrossEnd {
		return fmt.Errorf("%s: invalid cross alignment %d", path, crossAlign)
	}
	if wrap.AlignContent != nil && *wrap.AlignContent > MainSpaceEvenly {
		return fmt.Errorf("%s: invalid content alignment %d", path, *wrap.AlignContent)
	}
	box := bounds.Size()
	restoreInline := ctx.pushMeasureInline(box.X)
	defer restoreInline()
	gap, crossGap, err := wrapGaps(gap, gapLength, wrap, box, horizontal, path)
	if err != nil {
		return err
	}
	lines, err := wrapLines(ctx, box, gap, crossAlign, horizontal, children, path)
	if err != nil {
		return err
	}
	sizes := make([][]image.Point, len(lines))
	crosses := make([]int, len(lines))
	margins := make([][]Insets, len(lines))
	for index, line := range lines {
		if sizes[index], margins[index], crosses[index], err = wrapLineSizes(ctx, box, gap, crossAlign, horizontal, children, path, line); err != nil {
			return err
		}
	}
	availableMain, availableCross := axes(box, horizontal)
	if free := availableCross - sum(crosses) - crossGap*max(0, len(lines)-1); free > 0 && wrap.AlignContent == nil {
		// Stretched lines share the room they leave equally, the first ones
		// taking the pixels that do not divide.
		for index := range crosses {
			crosses[index] += free / len(crosses)
			if index < free%len(crosses) {
				crosses[index]++
			}
		}
	}
	starts := offsets(crosses, crossGap, availableCross, wrap.AlignContent)
	if total := starts[len(lines)] - crossGap; total > availableCross {
		ctx.warn(path, "layout-overflow", fmt.Sprintf("wrapped lines need %d pixels across, only %d are available", total, availableCross))
	}
	for index, line := range lines {
		lineBounds := rectFromAxes(bounds.Min, 0, starts[index], availableMain, crosses[index], horizontal)
		if err := paintLine(ctx, list, lineBounds, gap, mainAlign, crossAlign, horizontal,
			children[line.first:line.end], path, line.first, sizes[index], margins[index]); err != nil {
			return err
		}
	}
	return nil
}

// wrapGaps resolves the gap along a line and the one between lines.
func wrapGaps(gap int, gapLength Length, wrap Wrapping, box image.Point, horizontal bool, path string) (int, int, error) {
	gap = resolvedGap(gap, gapLength, mainOf(box, horizontal))
	crossGap := resolvedGap(wrap.CrossGap, wrap.CrossGapLength, mainOf(box, !horizontal))
	if gap < 0 || crossGap < 0 || !gapLength.valid() || !wrap.CrossGapLength.valid() {
		return 0, 0, fmt.Errorf("%s: gap must not be negative", path)
	}
	return gap, crossGap, nil
}
