package compose

import (
	"fmt"
	"image"

	"github.com/xwvike/inkwire/internal/display"
)

// InlineVerticalAlign is the alignment of one inline formatting box against
// the line box that contains it. Baseline is the CSS initial value.
type InlineVerticalAlign uint8

const (
	InlineBaseline InlineVerticalAlign = iota
	InlineTop
	InlineMiddle
	InlineBottom
)

// InlineItem is either a run of text or an atomic inline node. Padding,
// background and border belong to the item rather than the parent line so an
// inline element keeps its own box when the line wraps.
type InlineItem struct {
	Runs    []display.TextRun
	Node    Node
	Break   bool
	Padding Insets
	Margin  Insets
	// PaddingLengths and MarginLengths retain percentages until the inline
	// containing block is known. Their integer counterparts keep the legacy
	// scene form for fixed pixel values.
	PaddingLengths [4]Length
	MarginLengths  [4]Length

	Background                                       *display.Ink
	Border                                           *display.StrokeStyle
	BorderTop, BorderRight, BorderBottom, BorderLeft *display.StrokeStyle
	Radius                                           int

	LineHeight               int
	VerticalAlign            InlineVerticalAlign
	Wrap                     display.WrapMode
	Top, Right, Bottom, Left Length
}

// Inline lays out text and atomic inline-level boxes into line boxes. It is a
// block-level node: its own width is the available line width unless Size
// states one, while its height is the sum of generated line boxes.
type Inline struct {
	Size       image.Point
	Items      []InlineItem
	Align      display.HorizontalAlign
	Wrap       display.WrapMode
	LineHeight int
}

func (Inline) composeNode() {}

type inlineFragment struct {
	item     InlineItem
	runs     []display.TextRun
	node     Node
	width    int
	height   int
	ascent   int
	descent  int
	contentW int
	contentH int
}

type inlineLine struct {
	fragments []inlineFragment
	width     int
	height    int
	ascent    int
	descent   int
}

func (i Inline) validate(path string) error {
	if !validSize(i.Size) {
		return fmt.Errorf("%s: inline size must not be negative, got %v", path, i.Size)
	}
	if i.Align > display.AlignEnd || i.Wrap > display.WrapRunes || i.LineHeight < 0 {
		return fmt.Errorf("%s: invalid inline alignment, wrap or line height", path)
	}
	for index, item := range i.Items {
		itemPath := childPath(path, "items", index)
		if item.Break {
			if item.Node != nil || len(item.Runs) != 0 {
				return fmt.Errorf("%s: a line break cannot carry content", itemPath)
			}
			continue
		}
		if nilNode(item.Node) && len(item.Runs) == 0 {
			return fmt.Errorf("%s: inline item needs text or a node", itemPath)
		}
		if item.Node != nil && len(item.Runs) != 0 {
			return fmt.Errorf("%s: inline item cannot carry both text and a node", itemPath)
		}
		if !item.Padding.valid() || !lengthsValid(item.PaddingLengths) {
			return fmt.Errorf("%s: padding and margin must not be negative", itemPath)
		}
		if item.LineHeight < 0 || item.VerticalAlign > InlineBottom || item.Wrap > display.WrapRunes || item.Radius < 0 {
			return fmt.Errorf("%s: invalid inline item style", itemPath)
		}
		if item.Border != nil {
			if item.Border.Width < 0 {
				return fmt.Errorf("%s.border: width must not be negative", itemPath)
			}
			for _, dash := range item.Border.Dash {
				if dash <= 0 {
					return fmt.Errorf("%s.border: dash lengths must be positive", itemPath)
				}
			}
		}
		for name, stroke := range map[string]*display.StrokeStyle{"borderTop": item.BorderTop, "borderRight": item.BorderRight, "borderBottom": item.BorderBottom, "borderLeft": item.BorderLeft} {
			if stroke != nil {
				if err := validateStroke(itemPath+"."+name, *stroke); err != nil {
					return err
				}
			}
		}
		for name, length := range map[string]Length{
			"top": item.Top, "right": item.Right, "bottom": item.Bottom, "left": item.Left,
		} {
			tenths, _ := length.Parts()
			if length.IsSet() && tenths < 0 {
				return fmt.Errorf("%s.%s: percentage must not be negative", itemPath, name)
			}
		}
	}
	return nil
}

func (i Inline) measure(ctx *compileContext, maximum image.Point, path string) (image.Point, error) {
	if err := i.validate(path); err != nil {
		return image.Point{}, err
	}
	width := maximum.X
	if i.Size.X > 0 {
		width = min(i.Size.X, maximum.X)
	}
	if width <= 0 || maximum.Y <= 0 {
		return image.Point{}, nil
	}
	percentWidth := ctx.measureInlineWidth(maximum)
	restoreInline := ctx.pushMeasureInline(percentWidth)
	_, naturalWidth, naturalHeight, err := i.layout(ctx, width, percentWidth, path)
	restoreInline()
	if err != nil {
		return image.Point{}, err
	}
	size := image.Pt(naturalWidth, naturalHeight)
	if i.Size.X > 0 {
		size.X = i.Size.X
	}
	if i.Size.Y > 0 {
		size.Y = i.Size.Y
	}
	ctx.wants(path, size)
	return constrainSize(size, maximum), nil
}

func (i Inline) paint(ctx *compileContext, list *display.DisplayList, bounds image.Rectangle, path string) error {
	if bounds.Empty() {
		ctx.warn(path, "empty-layout", "inline content has no drawable area")
		return nil
	}
	lines, _, _, err := i.layout(ctx, bounds.Dx(), bounds.Dx(), path)
	if err != nil {
		return err
	}
	y := bounds.Min.Y
	for index, line := range lines {
		linePath := childPath(path, "lines", index)
		start := 0
		switch i.Align {
		case display.AlignCenter:
			start = (bounds.Dx() - line.width) / 2
		case display.AlignEnd:
			start = bounds.Dx() - line.width
		}
		lineY := y
		baseline := lineY + (line.height-(line.ascent+line.descent))/2 + line.ascent
		cursor := bounds.Min.X + start
		for fragmentIndex, fragment := range line.fragments {
			fragmentPath := childPath(linePath, "items", fragmentIndex)
			cursor += fragment.item.Margin.Left
			boxTop := lineY + line.height - fragment.height + fragment.item.Margin.Top
			switch fragment.item.VerticalAlign {
			case InlineBaseline:
				boxTop = baseline - fragment.ascent + fragment.item.Margin.Top
			case InlineTop:
				boxTop = lineY + fragment.item.Margin.Top
			case InlineMiddle:
				boxTop = lineY + (line.height-fragment.height)/2 + fragment.item.Margin.Top
			case InlineBottom:
				boxTop = lineY + line.height - fragment.height + fragment.item.Margin.Top
			}
			box := image.Rect(cursor, boxTop, cursor+fragment.width, boxTop+fragment.height)
			dx := relativeShift(fragment.item.Left, fragment.item.Right, bounds.Dx())
			dy := relativeShift(fragment.item.Top, fragment.item.Bottom, bounds.Dy())
			box = box.Add(image.Pt(dx, dy))
			content := inlineContentBox(box, fragment.item)
			if fragment.item.Background != nil {
				list.FillRoundRect(box, fragment.item.Radius, *fragment.item.Background)
			}
			if fragment.item.Border != nil {
				if fragment.item.Radius > 0 {
					list.StrokeRoundRect(box, fragment.item.Radius, *fragment.item.Border)
				} else {
					list.StrokeRect(box, *fragment.item.Border)
				}
			}
			if fragment.item.BorderTop != nil {
				strokeSide(list, box, 0, *fragment.item.BorderTop)
			}
			if fragment.item.BorderRight != nil {
				strokeSide(list, box, 1, *fragment.item.BorderRight)
			}
			if fragment.item.BorderBottom != nil {
				strokeSide(list, box, 2, *fragment.item.BorderBottom)
			}
			if fragment.item.BorderLeft != nil {
				strokeSide(list, box, 3, *fragment.item.BorderLeft)
			}
			if content.Empty() {
				ctx.warn(fragmentPath, "empty-layout", "inline item border leaves no content area")
				cursor += fragment.width + fragment.item.Margin.Right
				continue
			}
			content = image.Rectangle{
				Min: content.Min.Add(image.Pt(fragment.item.Padding.Left, fragment.item.Padding.Top)),
				Max: content.Max.Sub(image.Pt(fragment.item.Padding.Right, fragment.item.Padding.Bottom)),
			}
			if !content.Empty() {
				if fragment.node != nil {
					if err := ctx.paintWithContaining(fragment.node, list, content, bounds, fragmentPath); err != nil {
						return err
					}
				} else {
					textBox := display.TextBox{Bounds: content, Runs: fragment.runs,
						Align: display.AlignStart, VerticalAlign: display.AlignTop,
						Wrap: display.NoWrap, LineHeight: fragment.item.LineHeight}
					layout, err := list.DrawTextBox(ctx.compiler.Fonts, textBox)
					if err != nil {
						return fmt.Errorf("%s: %w", fragmentPath, err)
					}
					ctx.addMissing(fragmentPath, layout.MissingRunes(), layout.MissingFonts())
					if columns, lines, rows := layout.Clipped(); columns > 0 || lines > 0 || rows > 0 {
						ctx.warn(fragmentPath, "text-clipped", fmt.Sprintf("%q does not fit %dx%d: %s cut off",
							runText(fragment.runs), content.Dx(), content.Dy(), lostText(columns, lines, rows)))
					}
				}
			}
			cursor += fragment.width + fragment.item.Margin.Right
		}
		y += line.height
	}
	return nil
}

func borderWidth(stroke *display.StrokeStyle) int {
	if stroke == nil || stroke.Width <= 0 {
		return 0
	}
	return stroke.Width
}

func inlineBorderInsets(item InlineItem) Insets {
	if item.Border != nil {
		width := borderWidth(item.Border)
		return Insets{Top: width, Right: width, Bottom: width, Left: width}
	}
	result := Insets{}
	if item.BorderTop != nil {
		result.Top = borderWidth(item.BorderTop)
	}
	if item.BorderRight != nil {
		result.Right = borderWidth(item.BorderRight)
	}
	if item.BorderBottom != nil {
		result.Bottom = borderWidth(item.BorderBottom)
	}
	if item.BorderLeft != nil {
		result.Left = borderWidth(item.BorderLeft)
	}
	return result
}

func inlineContentBox(box image.Rectangle, item InlineItem) image.Rectangle {
	border := inlineBorderInsets(item)
	return image.Rectangle{Min: box.Min.Add(image.Pt(border.Left, border.Top)), Max: box.Max.Sub(image.Pt(border.Right, border.Bottom))}
}

func (i Inline) layout(ctx *compileContext, width, percentWidth int, path string) ([]inlineLine, int, int, error) {
	lines := []inlineLine{{}}
	maxWidth := max(1, width)
	for index, item := range i.Items {
		itemPath := childPath(path, "items", index)
		if item.Break {
			if len(lines[len(lines)-1].fragments) != 0 || len(lines) == 1 {
				finishInlineLine(&lines[len(lines)-1], i.LineHeight)
				lines = append(lines, inlineLine{})
			}
			continue
		}
		fragments, err := i.fragments(ctx, item, maxWidth, percentWidth, itemPath)
		if err != nil {
			return nil, 0, 0, err
		}
		for _, fragment := range fragments {
			line := &lines[len(lines)-1]
			if i.Wrap == display.WrapRunes && item.Wrap != display.NoWrap && len(line.fragments) != 0 && line.width+fragment.width > maxWidth {
				finishInlineLine(line, i.LineHeight)
				lines = append(lines, inlineLine{})
				line = &lines[len(lines)-1]
			}
			line.fragments = append(line.fragments, fragment)
			line.width += fragment.width
			line.ascent = max(line.ascent, fragment.ascent)
			line.descent = max(line.descent, fragment.descent)
			line.height = max(line.height, fragment.height)
		}
	}
	if len(lines) > 0 && len(lines[len(lines)-1].fragments) == 0 && len(lines) > 1 {
		lines = lines[:len(lines)-1]
	}
	for index := range lines {
		finishInlineLine(&lines[index], i.LineHeight)
	}
	naturalWidth := 0
	naturalHeight := 0
	for _, line := range lines {
		naturalWidth = max(naturalWidth, line.width)
		naturalHeight += line.height
	}
	// A parent may deliberately give an inline box a shorter cross-axis size
	// (for example, a flex item). Painting clips to that allocation just like a
	// browser viewport; measuring it is not itself a failed layout.
	return lines, naturalWidth, naturalHeight, nil
}

func finishInlineLine(line *inlineLine, lineHeight int) {
	line.height = max(line.height, line.ascent+line.descent)
	if line.height < lineHeight {
		line.height = lineHeight
	}
}

func (i Inline) fragments(ctx *compileContext, item InlineItem, maxWidth, percentWidth int, path string) ([]inlineFragment, error) {
	item = item.resolved(percentWidth)
	if item.Node != nil {
		size, err := item.Node.measure(ctx, image.Pt(maxWidth, 1<<30), path+".node")
		if err != nil {
			return nil, err
		}
		return []inlineFragment{makeInlineFragment(item, nil, item.Node, size, nil)}, nil
	}
	if len(item.Runs) == 0 {
		return nil, nil
	}
	// Text fragments are split at runes only when a line has to wrap. This
	// keeps a styled phrase together in the common case while retaining the
	// existing renderer's deterministic rune wrapping at narrow widths.
	var result []inlineFragment
	var runs []display.TextRun
	flush := func() error {
		if len(runs) == 0 {
			return nil
		}
		layout, err := display.LayoutText(ctx.compiler.Fonts, display.TextBox{
			Bounds: image.Rect(0, 0, max(1, maxWidth), max(1, item.LineHeight)), Runs: runs,
			Wrap: display.NoWrap, LineHeight: item.LineHeight,
		})
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		metrics := layout.Lines()
		if len(metrics) == 0 {
			return nil
		}
		result = append(result, makeInlineFragment(item, runs, nil, layout.Size(), &metrics[0]))
		runs = nil
		return nil
	}
	for _, run := range item.Runs {
		for _, r := range run.Text {
			piece := display.TextRun{Text: string(r), Style: run.Style}
			candidate := append(append([]display.TextRun{}, runs...), piece)
			layout, err := display.LayoutText(ctx.compiler.Fonts, display.TextBox{
				Bounds: image.Rect(0, 0, max(1, maxWidth), max(1, item.LineHeight)), Runs: candidate,
				Wrap: display.NoWrap, LineHeight: item.LineHeight,
			})
			if err != nil {
				return nil, fmt.Errorf("%s: %w", path, err)
			}
			border := inlineBorderInsets(item)
			if i.Wrap == display.WrapRunes && item.Wrap != display.NoWrap && len(runs) != 0 && layout.Size().X+item.Padding.horizontal()+border.horizontal()+item.Margin.horizontal() > maxWidth {
				if err := flush(); err != nil {
					return nil, err
				}
				runs = []display.TextRun{piece}
			} else {
				runs = candidate
			}
		}
	}
	if err := flush(); err != nil {
		return nil, err
	}
	return result, nil
}

func (item InlineItem) resolved(availableWidth int) InlineItem {
	if lengthsSet(item.PaddingLengths) {
		item.Padding = resolvedLengths(item.PaddingLengths, availableWidth, false)
	}
	if lengthsSet(item.MarginLengths) {
		item.Margin = resolvedLengths(item.MarginLengths, availableWidth, true)
	}
	return item
}

func resolvedLengths(lengths [4]Length, availableWidth int, allowNegative bool) Insets {
	result := Insets{}
	resolve := resolveInset
	if allowNegative {
		resolve = resolveMargin
	}
	result.Top = resolve(lengths[0], availableWidth)
	result.Right = resolve(lengths[1], availableWidth)
	result.Bottom = resolve(lengths[2], availableWidth)
	result.Left = resolve(lengths[3], availableWidth)
	return result
}

func makeInlineFragment(item InlineItem, runs []display.TextRun, node Node, size image.Point, metrics *display.TextLineMetrics) inlineFragment {
	border := inlineBorderInsets(item)
	contentW, contentH := size.X, size.Y
	ascent, descent := contentH, 0
	if metrics != nil {
		ascent, descent = metrics.Ascent, metrics.Descent+metrics.LineGap
	}
	return inlineFragment{
		item: item, runs: runs, node: node,
		contentW: contentW, contentH: contentH,
		width:   contentW + item.Padding.Left + item.Padding.Right + border.horizontal() + item.Margin.Left + item.Margin.Right,
		height:  contentH + item.Padding.Top + item.Padding.Bottom + border.vertical() + item.Margin.Top + item.Margin.Bottom,
		ascent:  ascent + item.Padding.Top + border.Top + item.Margin.Top,
		descent: descent + item.Padding.Bottom + border.Bottom + item.Margin.Bottom,
	}
}
