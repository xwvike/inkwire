package compose

import (
	"fmt"
	"image"
	"slices"

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
	Gradient                                         *display.Gradient
	Border                                           *display.StrokeStyle
	BorderTop, BorderRight, BorderBottom, BorderLeft *display.StrokeStyle
	Radius                                           int
	// RadiusLength replaces Radius when set, as it does on a Rectangle. An
	// inline box is drawn with round corners even where a percentage would
	// make it an ellipse.
	RadiusLength Length

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
	// first and last say whether this fragment starts and ends its item. An
	// item cut across lines has its left padding, border and margin on its
	// first fragment only and its right ones on its last, as CSS slices an
	// inline box that wraps.
	first, last bool
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
	if i.Align > display.AlignEnd || i.Wrap > display.WrapWords || i.LineHeight < 0 {
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
		if item.LineHeight < 0 || item.VerticalAlign > InlineBottom || item.Wrap > display.WrapWords || item.Radius < 0 {
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
			// The fragment's advance and height take in its margins; its box
			// is what is inside them.
			margin := fragment.item.Margin
			box := image.Rect(cursor+margin.Left, boxTop,
				cursor+fragment.width-margin.Right, boxTop+fragment.height-margin.Top-margin.Bottom)
			advance := fragment.width
			dx := relativeShift(fragment.item.Left, fragment.item.Right, bounds.Dx())
			dy := relativeShift(fragment.item.Top, fragment.item.Bottom, bounds.Dy())
			box = box.Add(image.Pt(dx, dy))
			content := inlineContentBox(box, fragment.item)
			radius, _ := cornerOf(ctx, fragment.item.Radius, fragment.item.RadiusLength, box.Size(), path)
			if fragment.item.Background != nil {
				list.FillRoundRect(box, radius, *fragment.item.Background)
			}
			if fragment.item.Gradient != nil {
				list.FillGradient(box, radius, fragment.item.Gradient)
			}
			if fragment.item.Border != nil {
				if radius > 0 {
					list.StrokeRoundRect(box, radius, *fragment.item.Border)
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
				cursor += advance
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
			cursor += advance
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
	maxWidth := max(1, width)
	pieces, err := i.pieces(ctx, maxWidth, percentWidth, path)
	if err != nil {
		return nil, 0, 0, err
	}
	var lines [][]inlinePiece
	var current []inlinePiece
	used := 0
	end := func() {
		lines = append(lines, trimHangingSpaces(current))
		current, used = nil, 0
	}
	for index := 0; index < len(pieces); {
		if pieces[index].forced {
			end()
			index++
			continue
		}
		// A group is what has to stay on one line: the pieces up to the next
		// place a line may end. Spaces at its end hang, so they are not
		// counted when asking whether it fits.
		next := index + 1
		for next < len(pieces) && !pieces[next-1].breakAfter && !pieces[next].forced {
			next++
		}
		group := pieces[index:next]
		total, fits := groupWidth(group)
		switch {
		case used+fits <= maxWidth || len(current) == 0 && fits <= maxWidth:
			current = append(current, group...)
			used += total
			index = next
		case len(current) != 0:
			end()
		default:
			// One word wider than the whole line: it is broken where it
			// stops fitting, rather than let out of its box.
			split, err := i.splitRunes(ctx, group, path)
			if err != nil {
				return nil, 0, 0, err
			}
			for _, piece := range split {
				if used+piece.width > maxWidth && len(current) != 0 && !piece.space &&
					!(piece.keep && current[len(current)-1].keep) {
					end()
				}
				current = append(current, piece)
				used += piece.width
			}
			index = next
		}
	}
	if len(current) != 0 || len(lines) == 0 {
		end()
	}

	result := make([]inlineLine, len(lines))
	naturalWidth, naturalHeight := 0, 0
	for index, pieces := range lines {
		line, err := i.assemble(ctx, pieces, path)
		if err != nil {
			return nil, 0, 0, err
		}
		finishInlineLine(&line, i.LineHeight)
		result[index] = line
		naturalWidth = max(naturalWidth, line.width)
		naturalHeight += line.height
	}
	// A parent may deliberately give an inline box a shorter cross-axis size
	// (for example, a flex item). Painting clips to that allocation just like a
	// browser viewport; measuring it is not itself a failed layout.
	return result, naturalWidth, naturalHeight, nil
}

func finishInlineLine(line *inlineLine, lineHeight int) {
	line.height = max(line.height, line.ascent+line.descent)
	if line.height < lineHeight {
		line.height = lineHeight
	}
}

// inlinePiece is the least that is laid out on a line: an atomic box, or a
// stretch of one item's text that a line may not end inside.
type inlinePiece struct {
	item   int
	runs   []display.TextRun
	node   Node
	size   image.Point
	forced bool
	// space is a stretch of spaces, which hangs at the end of a line.
	space bool
	// keep is text that must not wrap, which runs past the end of a line
	// rather than be broken.
	keep bool
	// breakAfter says a line may end after this piece.
	breakAfter bool
	// first and last say the piece starts or ends its item, which is where
	// the item's padding, border and margin go.
	first, last bool
	// width is the piece's advance along the line, with whatever of its
	// item's edges it carries.
	width int
	// resolved is the item with its percentages worked out.
	resolved InlineItem
}

// pieces cuts the items into the pieces lines are filled with, and marks
// where a line may end between them.
func (i Inline) pieces(ctx *compileContext, maxWidth, percentWidth int, path string) ([]inlinePiece, error) {
	var pieces []inlinePiece
	// The character before the next one, across items: a word written in
	// two styles is still one word.
	previous, beforePrevious := rune(0), rune(0)
	previousMode := display.NoWrap
	for index, item := range i.Items {
		itemPath := childPath(path, "items", index)
		mode := item.Wrap
		if i.Wrap == display.NoWrap {
			mode = display.NoWrap
		} else if i.Wrap == display.WrapRunes && mode != display.NoWrap {
			mode = display.WrapRunes
		}
		if item.Break {
			pieces = append(pieces, inlinePiece{item: index, forced: true})
			previous, beforePrevious = 0, 0
			continue
		}
		resolved := item.resolved(percentWidth)
		if item.Node != nil {
			size, err := item.Node.measure(ctx, image.Pt(maxWidth, 1<<30), itemPath+".node")
			if err != nil {
				return nil, err
			}
			if previous != 0 && breakable(beforePrevious, previous, atomic, i.joinedMode(previousMode, mode)) {
				pieces[len(pieces)-1].breakAfter = true
			}
			pieces = append(pieces, inlinePiece{item: index, node: item.Node, size: size,
				first: true, last: true, resolved: resolved})
			previous, beforePrevious, previousMode = atomic, 0, mode
			continue
		}
		start := len(pieces)
		var runs []display.TextRun
		var text []rune
		flush := func(breakAfter bool) {
			if len(runs) == 0 {
				return
			}
			pieces = append(pieces, inlinePiece{item: index, runs: runs, space: allSpaces(text),
				keep: mode == display.NoWrap, breakAfter: breakAfter, resolved: resolved})
			runs, text = nil, nil
		}
		for _, run := range item.Runs {
			for _, r := range run.Text {
				if previous != 0 {
					opening := breakable(beforePrevious, previous, r, i.joinedMode(previousMode, mode))
					switch {
					case len(runs) == 0:
						// The first character of this item: the opening, if
						// any, is between the last piece and this one.
						if len(pieces) != 0 && opening {
							pieces[len(pieces)-1].breakAfter = true
						}
					case opening || isSpaceRune(previous) != isSpaceRune(r):
						flush(opening)
					}
				}
				if len(runs) != 0 && sameTextStyle(runs[len(runs)-1].Style, run.Style) {
					runs[len(runs)-1].Text += string(r)
				} else {
					runs = append(runs, display.TextRun{Text: string(r), Style: run.Style})
				}
				text = append(text, r)
				beforePrevious, previous, previousMode = previous, r, mode
			}
		}
		flush(false)
		if len(pieces) > start {
			pieces[start].first = true
			pieces[len(pieces)-1].last = true
		}
	}
	for index := range pieces {
		piece := &pieces[index]
		if piece.forced {
			continue
		}
		if piece.node == nil {
			size, err := textAdvance(ctx, piece.runs, piece.resolved.LineHeight, path)
			if err != nil {
				return nil, err
			}
			piece.size = size
		}
		piece.width = piece.size.X + edgeAdvance(piece.resolved, piece.first, piece.last)
	}
	return pieces, nil
}

// atomic stands for an atomic box among the characters around it, as U+FFFC
// stands for an object in text.
const atomic = '\uFFFC'

// breakable says whether a line may end between a and b, written in mode.
// A line may end on either side of an atomic box, as it may on either side
// of a picture in a browser, but not before a space.
func breakable(before, a, b rune, mode display.WrapMode) bool {
	switch {
	case mode == display.NoWrap || isSpaceRune(b):
		return false
	case mode == display.WrapRunes || a == atomic || b == atomic:
		return true
	}
	return display.BreakBetween(before, a, b)
}

// joinedMode is how the place between two characters may break. CSS leaves
// that to the nearest element holding both of them: the item, when they are
// in one, or else something around the two items, which is taken to be the
// block unless both items agree.
func (i Inline) joinedMode(a, b display.WrapMode) display.WrapMode {
	if a == b {
		return a
	}
	return i.Wrap
}

func sameTextStyle(a, b display.TextStyle) bool {
	return a.Font == b.Font && a.Size == b.Size && a.Ink == b.Ink && slices.Equal(a.Fallback, b.Fallback) &&
		a.Bold == b.Bold && a.LetterSpacing == b.LetterSpacing && a.Decoration == b.Decoration
}

func isSpaceRune(r rune) bool { return r == ' ' || r == '\t' || r == '\u3000' }

func allSpaces(text []rune) bool {
	for _, r := range text {
		if !isSpaceRune(r) {
			return false
		}
	}
	return len(text) != 0
}

// edgeAdvance is how far an item's left and right padding, border and margin
// take a piece along the line, for the sides the piece carries.
func edgeAdvance(item InlineItem, first, last bool) int {
	border := inlineBorderInsets(item)
	advance := 0
	if first {
		advance += item.Margin.Left + border.Left + item.Padding.Left
	}
	if last {
		advance += item.Margin.Right + border.Right + item.Padding.Right
	}
	return advance
}

func textAdvance(ctx *compileContext, runs []display.TextRun, lineHeight int, path string) (image.Point, error) {
	layout, err := display.LayoutText(ctx.compiler.Fonts, display.TextBox{
		Bounds: image.Rect(0, 0, 1, max(1, lineHeight)), Runs: runs,
		Wrap: display.NoWrap, LineHeight: lineHeight,
	})
	if err != nil {
		return image.Point{}, fmt.Errorf("%s: %w", path, err)
	}
	return layout.Size(), nil
}

// groupWidth is a group's advance, and its advance without the spaces it ends
// with, which hang past the end of a line rather than make it longer.
func groupWidth(group []inlinePiece) (total, fits int) {
	hanging := 0
	for _, piece := range group {
		total += piece.width
		if piece.space {
			hanging += piece.width
		} else {
			hanging = 0
		}
	}
	return total, total - hanging
}

// trimHangingSpaces drops the spaces a line ends with, which CSS neither
// draws nor counts once a line has ended there.
func trimHangingSpaces(pieces []inlinePiece) []inlinePiece {
	for len(pieces) > 0 && pieces[len(pieces)-1].space {
		last := pieces[len(pieces)-1]
		pieces = pieces[:len(pieces)-1]
		// The item still ends here, so the piece before takes its right edge.
		if last.last && len(pieces) > 0 && pieces[len(pieces)-1].item == last.item {
			kept := &pieces[len(pieces)-1]
			kept.last = true
			kept.width = kept.size.X + edgeAdvance(kept.resolved, kept.first, true)
		}
	}
	return pieces
}

// splitRunes cuts a group into one piece per character, for a word that has
// to be broken because no line is wide enough for it.
func (i Inline) splitRunes(ctx *compileContext, group []inlinePiece, path string) ([]inlinePiece, error) {
	var split []inlinePiece
	for _, piece := range group {
		if piece.node != nil || piece.space || piece.keep {
			split = append(split, piece)
			continue
		}
		var characters []display.TextRun
		for _, run := range piece.runs {
			for _, r := range run.Text {
				characters = append(characters, display.TextRun{Text: string(r), Style: run.Style})
			}
		}
		for index, character := range characters {
			part := inlinePiece{item: piece.item, runs: []display.TextRun{character}, resolved: piece.resolved,
				first: piece.first && index == 0, last: piece.last && index == len(characters)-1,
				breakAfter: index == len(characters)-1 && piece.breakAfter}
			size, err := textAdvance(ctx, part.runs, piece.resolved.LineHeight, path)
			if err != nil {
				return nil, err
			}
			part.size = size
			part.width = size.X + edgeAdvance(part.resolved, part.first, part.last)
			split = append(split, part)
		}
	}
	return split, nil
}

// assemble makes a line's fragments from its pieces: the pieces of one item
// that sit together on the line become one fragment, drawn as one box.
func (i Inline) assemble(ctx *compileContext, pieces []inlinePiece, path string) (inlineLine, error) {
	var line inlineLine
	for start := 0; start < len(pieces); {
		end := start + 1
		if pieces[start].node == nil {
			for end < len(pieces) && pieces[end].item == pieces[start].item && pieces[end].node == nil && !pieces[end].forced {
				end++
			}
		}
		first, last := pieces[start], pieces[end-1]
		var fragment inlineFragment
		if first.node != nil {
			fragment = makeInlineFragment(first.resolved, nil, first.node, first.size, nil, true, true)
		} else {
			var runs []display.TextRun
			for _, piece := range pieces[start:end] {
				for _, run := range piece.runs {
					if len(runs) != 0 && sameTextStyle(runs[len(runs)-1].Style, run.Style) {
						runs[len(runs)-1].Text += run.Text
					} else {
						runs = append(runs, run)
					}
				}
			}
			layout, err := display.LayoutText(ctx.compiler.Fonts, display.TextBox{
				Bounds: image.Rect(0, 0, 1, max(1, first.resolved.LineHeight)), Runs: runs,
				Wrap: display.NoWrap, LineHeight: first.resolved.LineHeight,
			})
			if err != nil {
				return inlineLine{}, fmt.Errorf("%s: %w", childPath(path, "items", first.item), err)
			}
			metrics := layout.Lines()
			fragment = makeInlineFragment(first.resolved, runs, nil, layout.Size(), &metrics[0], first.first, last.last)
		}
		line.fragments = append(line.fragments, fragment)
		line.width += fragment.width
		line.ascent = max(line.ascent, fragment.ascent)
		line.descent = max(line.descent, fragment.descent)
		line.height = max(line.height, fragment.height)
		start = end
	}
	return line, nil
}

// slice is the part of an item a fragment draws: an item cut across lines
// keeps its left edge on its first fragment and its right edge on its last,
// and its rounded corners only when it is not cut at all.
func (item InlineItem) slice(first, last bool) InlineItem {
	if first && last {
		return item
	}
	if item.Border != nil {
		item.BorderTop, item.BorderRight, item.BorderBottom, item.BorderLeft = item.Border, item.Border, item.Border, item.Border
		item.Border = nil
	}
	if !first {
		item.Margin.Left, item.Padding.Left, item.BorderLeft = 0, 0, nil
	}
	if !last {
		item.Margin.Right, item.Padding.Right, item.BorderRight = 0, 0, nil
	}
	item.Radius, item.RadiusLength = 0, Length{}
	return item
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

func makeInlineFragment(item InlineItem, runs []display.TextRun, node Node, size image.Point, metrics *display.TextLineMetrics, first, last bool) inlineFragment {
	item = item.slice(first, last)
	border := inlineBorderInsets(item)
	contentW, contentH := size.X, size.Y
	ascent, descent := contentH, 0
	if metrics != nil {
		ascent, descent = metrics.Ascent, metrics.Descent+metrics.LineGap
	}
	return inlineFragment{
		item: item, runs: runs, node: node, first: first, last: last,
		contentW: contentW, contentH: contentH,
		width:   contentW + edgeAdvance(item, true, true),
		height:  contentH + item.Padding.Top + item.Padding.Bottom + border.vertical() + item.Margin.Top + item.Margin.Bottom,
		ascent:  ascent + item.Padding.Top + border.Top + item.Margin.Top,
		descent: descent + item.Padding.Bottom + border.Bottom + item.Margin.Bottom,
	}
}
