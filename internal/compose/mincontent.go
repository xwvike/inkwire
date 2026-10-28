package compose

import (
	"fmt"
	"image"

	"github.com/xwvike/inkwire/internal/display"
)

// minContentWidth is how narrow a node can be laid out without its content
// running past it, which is what CSS calls its min-content width: the widest
// word of its text, the widest thing it holds that cannot be made narrower,
// and the width it states for itself.
//
// It is what a flex item with min-width: auto does not shrink below. A node
// this does not know how to ask answers zero, which leaves it free to shrink
// as it always could.
func minContentWidth(ctx *compileContext, node Node, path string) (int, error) {
	return minContent(ctx, node, path, false)
}

// itemMinContent is a flex item's content-based minimum: the min-content of
// what it holds, leaving aside the width the item states for itself, which
// CSS applies separately as a cap (LayoutChild.Stated).
func itemMinContent(ctx *compileContext, node Node, path string) (int, error) {
	return minContent(ctx, node, path, true)
}

// minContent answers minContentWidth, for the node's content alone when
// ownSize is set: the size the node states, and the wrappers it is drawn in,
// are passed over to what is inside.
func minContent(ctx *compileContext, node Node, path string, ownSize bool) (int, error) {
	stated := func(size image.Point) bool { return size.X > 0 && !ownSize }
	switch n := node.(type) {
	case Text:
		if stated(n.Size) {
			return n.Size.X, nil
		}
		width, err := display.MinContentWidth(ctx.compiler.Fonts, n.box(image.Rectangle{}))
		if err != nil {
			return 0, fmt.Errorf("%s: %w", path, err)
		}
		return width, nil
	case Inline:
		if stated(n.Size) {
			return n.Size.X, nil
		}
		return n.minContentWidth(ctx, path)
	case Padding:
		width, err := minContent(ctx, n.Child, path+".child", ownSize)
		if err != nil {
			return 0, err
		}
		return width + n.resolved(ctx.measureInline).horizontal(), nil
	case Row:
		if stated(n.Size) {
			return n.Size.X, nil
		}
		return rowMinContent(ctx, n, path)
	case Column:
		if stated(n.Size) {
			return n.Size.X, nil
		}
		widest := 0
		for index, child := range n.Children {
			width, err := columnChildMinContent(ctx, child, childPath(path, "children", index))
			if err != nil {
				return 0, err
			}
			widest = max(widest, width)
		}
		return widest, nil
	case Stack:
		if stated(n.Size) {
			return n.Size.X, nil
		}
		widest := 0
		for index, child := range n.Children {
			width, err := minContent(ctx, child, childPath(path, "children", index), ownSize)
			if err != nil {
				return 0, err
			}
			widest = max(widest, width)
		}
		return widest, nil
	case Relative:
		return minContent(ctx, n.Child, path+".child", ownSize)
	case Clip:
		if stated(n.Size) {
			return n.Size.X, nil
		}
		return minContent(ctx, n.Child, path+".child", ownSize)
	case Image:
		return n.Size.X, nil
	case Rectangle:
		if ownSize {
			return 0, nil
		}
		return n.Size.X, nil
	case Grid:
		return n.Size.X, nil
	case Absolute:
		return n.Size.X, nil
	case Anchored:
		return n.Size.X, nil
	}
	return 0, nil
}

// rowMinContent is a row's narrowest: every child at the least it can shrink
// to, with the gaps and margins between them.
func rowMinContent(ctx *compileContext, row Row, path string) (int, error) {
	gap := resolvedGap(row.Gap, row.GapLength, ctx.measureInline)
	total := gap * max(0, len(row.Children)-1)
	for index, child := range row.Children {
		nodePath := childPath(path, "children", index)
		least := 0
		switch {
		case child.Shrink <= 0:
			// It does not shrink, so its least is the size it starts at.
			size, err := child.Node.measure(ctx, image.Pt(unboundedMeasure, unboundedMeasure), nodePath)
			if err != nil {
				return 0, err
			}
			least = child.intrinsicMain(size.X)
		default:
			if low, ok := child.MinMain.intrinsic(); ok {
				least = low
			}
			if child.MinContent {
				width, err := minContentWidth(ctx, child.Node, nodePath)
				if err != nil {
					return 0, err
				}
				least = max(least, width)
			}
		}
		total += least + intrinsicMargin(child.Margin, true)
	}
	return total, nil
}

// columnChildMinContent is a column child's narrowest, which is the width it
// states across the column if it states one.
func columnChildMinContent(ctx *compileContext, child LayoutChild, path string) (int, error) {
	if stated, ok := child.Cross.intrinsic(); ok {
		return stated + intrinsicMargin(child.Margin, true), nil
	}
	width, err := minContentWidth(ctx, child.Node, path)
	if err != nil {
		return 0, err
	}
	if low, ok := child.MinCross.intrinsic(); ok {
		width = max(width, low)
	}
	return width + intrinsicMargin(child.Margin, true), nil
}

// minContentWidth is the widest stretch of the inline content that no line
// may end inside, with the edges of the items it crosses.
func (i Inline) minContentWidth(ctx *compileContext, path string) (int, error) {
	if i.Wrap == display.NoWrap {
		_, natural, _, err := i.layout(ctx, unboundedMeasure, ctx.measureInline, path)
		return natural, err
	}
	pieces, err := i.pieces(ctx, unboundedMeasure, ctx.measureInline, path)
	if err != nil {
		return 0, err
	}
	widest := 0
	for index := 0; index < len(pieces); {
		if pieces[index].forced {
			index++
			continue
		}
		next := index + 1
		for next < len(pieces) && !pieces[next-1].breakAfter && !pieces[next].forced {
			next++
		}
		group := pieces[index:next]
		for at := range group {
			if node := group[at].node; node != nil {
				// An atomic box can itself be narrower than it would like.
				width, err := minContentWidth(ctx, node, childPath(path, "items", group[at].item)+".node")
				if err != nil {
					return 0, err
				}
				group[at].width = width + edgeAdvance(group[at].resolved, true, true)
			}
		}
		_, fits := groupWidth(group)
		widest = max(widest, fits)
		index = next
	}
	return widest, nil
}
