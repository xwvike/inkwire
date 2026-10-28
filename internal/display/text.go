package display

import (
	"fmt"
	"image"
	"slices"
	"strconv"
	"strings"
)

type HorizontalAlign uint8

const (
	AlignStart HorizontalAlign = iota
	AlignCenter
	AlignEnd
)

type VerticalAlign uint8

const (
	AlignTop VerticalAlign = iota
	AlignMiddle
	AlignBottom
)

type WrapMode uint8

const (
	NoWrap WrapMode = iota
	// WrapRunes ends a line at whichever character would not fit, which is
	// CSS's word-break: break-all.
	WrapRunes
	// WrapWords ends a line where BreakBetween allows, as a browser wraps
	// ordinary text. A word wider than the whole line is broken where it
	// stops fitting, as overflow-wrap: break-word does, rather than run out of
	// its box. Spaces at the end of a line hang past it and are not counted.
	WrapWords
)

type TextStyle struct {
	Font     string
	Fallback []string
	Size     int
	Ink      Ink
	// Bold draws each glyph a second time a pixel to its right, which is
	// how a bitmap face with no bold strike is made heavier. Every glyph is
	// then a pixel wider and a pixel further on.
	Bold bool
	// LetterSpacing is added to the advance of every character, and may be
	// negative.
	LetterSpacing int
	Decoration    TextDecoration
}

// TextDecoration is the lines drawn with text: under it, through it, or both.
type TextDecoration uint8

const (
	Underline TextDecoration = 1 << iota
	LineThrough
	Overline
)

type TextRun struct {
	Text  string
	Style TextStyle
}

type TextBox struct {
	Bounds        image.Rectangle
	Runs          []TextRun
	Align         HorizontalAlign
	VerticalAlign VerticalAlign
	Wrap          WrapMode
	LineHeight    int
	// Ellipsis ends a line too long for the box with "…" where it stops
	// fitting, as text-overflow: ellipsis does, instead of cutting it off.
	Ellipsis bool
}

type glyphPlacement struct {
	glyph      Glyph
	metrics    FontMetrics
	ink        Ink
	r          rune
	missing    bool
	decoration TextDecoration
	// style and set are what the glyph was found with, so that an ellipsis
	// put in its place is drawn in the same face.
	style TextStyle
	set   *FontSet
}

type textLine struct {
	glyphs  []glyphPlacement
	width   int
	ascent  int
	descent int
	lineGap int
	height  int
}

// TextLineMetrics exposes the measurements a caller needs to align an
// individual inline fragment without exposing glyph placement internals.
// Width is the advance of the line; ascent and descent describe its baseline.
type TextLineMetrics struct {
	Width   int
	Ascent  int
	Descent int
	LineGap int
	Height  int
}

// halfLeading is the room above this line's letters when the line box is
// taller than they are.
//
// CSS calls the difference leading and puts half of it above the letters and
// half below, which is why a line-height larger than the text centres the text
// in the line rather than pushing it down. All of it used to hang below, so a
// block of lines was as far above its own middle as the leading of its last
// line — and which line was last changed where the whole block sat, because
// two fonts leave different amounts of it.
//
// The half that cannot be split goes below, so a line is never pushed past the
// bottom of its own box by rounding.
func (l textLine) halfLeading() int {
	leading := l.height - l.ascent - l.descent
	if leading <= 0 {
		return 0
	}
	return leading / 2
}

// TextLayout is the measured, immutable result of resolving a TextBox.
type TextLayout struct {
	box     TextBox
	lines   []textLine
	width   int
	height  int
	missing []rune
	// missingIn names the families that were asked for a glyph they do not
	// have, so a warning can say which font to stop using.
	missingIn []string
}

func LayoutText(registry *FontRegistry, box TextBox) (*TextLayout, error) {
	if registry == nil {
		return nil, fmt.Errorf("font registry must not be nil")
	}
	if box.Bounds.Empty() {
		return nil, fmt.Errorf("text bounds must not be empty")
	}
	if box.LineHeight < 0 {
		return nil, fmt.Errorf("line height must not be negative")
	}

	layout := &TextLayout{box: box}
	current := textLine{}
	var placeWord func(glyphPlacement)
	lineHasContent := false
	lastMetrics := FontMetrics{Ascent: 10, Descent: 2, LineGap: 2}

	finishLine := func(force bool) {
		if !force && !lineHasContent {
			return
		}
		if current.ascent == 0 && current.descent == 0 {
			current.ascent = lastMetrics.Ascent
			current.descent = lastMetrics.Descent
			current.lineGap = lastMetrics.LineGap
		}
		current.height = current.ascent + current.descent + current.lineGap
		if box.LineHeight > 0 {
			current.height = box.LineHeight
		}
		layout.width = max(layout.width, current.width)
		layout.height += current.height
		layout.lines = append(layout.lines, current)
		current = textLine{}
		lineHasContent = false
	}

	// Word wrapping keeps where the line could last have ended, so that a
	// word that does not fit can be carried to the next line whole.
	opening := 0
	finishWords := func() {
		current.glyphs = trimTrailingSpaces(current.glyphs)
		measureLine(&current)
		finishLine(false)
		opening = 0
	}
	placeWord = func(placement glyphPlacement) {
		if count := len(current.glyphs); count > 0 {
			before := rune(0)
			if count > 1 {
				before = current.glyphs[count-2].r
			}
			if BreakBetween(before, current.glyphs[count-1].r, placement.r) {
				opening = count
			}
		}
		if !isSpace(placement.r) && lineHasContent && current.width+placement.glyph.Advance > box.Bounds.Dx() {
			if opening > 0 {
				carried := slices.Clone(current.glyphs[opening:])
				current.glyphs = current.glyphs[:opening]
				finishWords()
				current.glyphs = carried
				measureLine(&current)
				lineHasContent = len(carried) > 0
			}
			// What is left is one word; if it still does not fit with this
			// character on the end, the word is broken here.
			if lineHasContent && current.width+placement.glyph.Advance > box.Bounds.Dx() {
				finishWords()
			}
		}
		current.glyphs = append(current.glyphs, placement)
		measureLine(&current)
		lineHasContent = true
	}

	bold := map[*byte]Glyph{}
	appendRune := func(r rune, style TextStyle, set *FontSet) {
		placement, ok := place(r, style, set, bold)
		if !ok {
			layout.missing = append(layout.missing, r)
			fontName := style.Font
			if len(style.Fallback) != 0 {
				fontName += ", " + strings.Join(style.Fallback, ", ")
			}
			if !slices.Contains(layout.missingIn, fontName) {
				layout.missingIn = append(layout.missingIn, fontName)
			}
		}
		lastMetrics = placement.metrics
		if box.Wrap == WrapRunes && lineHasContent && current.width+placement.glyph.Advance > box.Bounds.Dx() {
			finishLine(false)
		}
		if box.Wrap == WrapWords {
			placeWord(placement)
			return
		}
		current.glyphs = append(current.glyphs, placement)
		current.width += placement.glyph.Advance
		current.ascent = max(current.ascent, placement.metrics.Ascent)
		current.descent = max(current.descent, placement.metrics.Descent)
		current.lineGap = max(current.lineGap, placement.metrics.LineGap)
		lineHasContent = true
	}

	for _, run := range box.Runs {
		style := run.Style
		if style.Font == "" {
			style.Font = DefaultFontFamily
		}
		if style.Size == 0 && style.Font == DefaultFontFamily {
			style.Size = DefaultFontSize
		}
		if !style.Ink.valid() {
			return nil, fmt.Errorf("invalid text ink %d", style.Ink)
		}
		set, ok := registry.Match(style.Font, style.Size)
		if len(style.Fallback) != 0 {
			set, ok = registry.MatchStack(append([]string{style.Font}, style.Fallback...), style.Size)
		}
		if !ok {
			// A size is one of a list rather than a number in a range, so the
			// list is the answer to the question being asked here.
			if sizes := registry.Sizes(style.Font); len(sizes) != 0 {
				return nil, fmt.Errorf("font %q has no %dpx strike; it has %s",
					style.Font, style.Size, joinSizes(sizes))
			}
			return nil, fmt.Errorf("unknown font %q", style.Font)
		}
		lastMetrics = set.Metrics()
		for _, r := range run.Text {
			switch r {
			case '\n':
				if box.Wrap == WrapWords {
					current.glyphs = trimTrailingSpaces(current.glyphs)
					measureLine(&current)
					opening = 0
				}
				finishLine(true)
			case '\t':
				for range 4 {
					appendRune(' ', style, set)
				}
			default:
				appendRune(r, style, set)
			}
		}
	}
	if box.Wrap == WrapWords {
		finishWords()
	} else {
		finishLine(false)
	}
	if box.Ellipsis {
		layout.ellipsize(bold)
	}
	return layout, nil
}

// place finds the glyph for one character in the style it is written in, or
// a placeholder the size of the face if the face has none, which is false.
func place(r rune, style TextStyle, set *FontSet, bold map[*byte]Glyph) (glyphPlacement, bool) {
	resolved, ok := set.Resolve(r)
	placement := glyphPlacement{ink: style.Ink, r: r, decoration: style.Decoration, style: style, set: set}
	if ok {
		placement.glyph = resolved.Glyph
		placement.metrics = resolved.Metrics
		if style.Bold {
			placement.glyph = emboldened(placement.glyph, bold)
		}
	} else {
		metrics := set.Metrics()
		size := max(1, metrics.Ascent+metrics.Descent)
		placement.glyph = Glyph{Width: size, Height: size, Advance: size}
		placement.metrics = metrics
		placement.missing = true
	}
	placement.glyph.Advance = max(0, placement.glyph.Advance+style.LetterSpacing)
	return placement, ok
}

// emboldened is a glyph drawn over itself a pixel to the right. Each one is
// made once a layout, however often it is used.
func emboldened(glyph Glyph, made map[*byte]Glyph) Glyph {
	if len(glyph.Data) == 0 {
		glyph.Advance++
		return glyph
	}
	if heavier, ok := made[&glyph.Data[0]]; ok {
		return heavier
	}
	heavier := Glyph{Width: glyph.Width + 1, Height: glyph.Height, Advance: glyph.Advance + 1}
	heavier.RowBytes = (heavier.Width + 7) / 8
	heavier.Data = make([]byte, heavier.RowBytes*heavier.Height)
	for y := 0; y < glyph.Height; y++ {
		for x := 0; x < heavier.Width; x++ {
			if glyph.On(x, y) || glyph.On(x-1, y) {
				heavier.Data[y*heavier.RowBytes+x/8] |= 0x80 >> uint(x%8)
			}
		}
	}
	made[&glyph.Data[0]] = heavier
	return heavier
}

// ellipsize ends every line too long for the box with an ellipsis, taking
// off as many characters as it needs to fit one. The ellipsis is drawn in
// the face of the last character kept, or as three full stops where that
// face has no "…": a set that has to reach into another face for it finds
// the one in a Chinese face, three heavy dots in the middle of a full-width
// cell, which ends a line of Latin text as nothing a browser would draw. A
// box too narrow for even the ellipsis keeps as much of it as fits, as a
// browser does.
func (l *TextLayout) ellipsize(bold map[*byte]Glyph) {
	available := l.box.Bounds.Dx()
	l.width = 0
	for index := range l.lines {
		line := &l.lines[index]
		if line.width > available && len(line.glyphs) > 0 {
			last := line.glyphs[len(line.glyphs)-1]
			mark := []glyphPlacement{}
			if dots, ok := place('…', last.style, last.set, bold); ok && sameFace(last.set, last.r, '…') {
				mark = append(mark, dots)
			} else {
				dot, _ := place('.', last.style, last.set, bold)
				mark = append(mark, dot, dot, dot)
			}
			markWidth := 0
			for _, placement := range mark {
				markWidth += placement.glyph.Advance
			}
			kept := line.glyphs
			for len(kept) > 0 && widthOf(kept)+markWidth > available {
				kept = kept[:len(kept)-1]
			}
			kept = trimTrailingSpaces(kept)
			line.glyphs = append(slices.Clone(kept), mark...)
			measureLine(line)
		}
		l.width = max(l.width, line.width)
	}
}

// sameFace says whether a set finds two characters in the same face. A
// character the set has nowhere is in no face, and so in nobody's.
func sameFace(set *FontSet, a, b rune) bool {
	first, ok := set.Resolve(a)
	if !ok {
		return false
	}
	second, ok := set.Resolve(b)
	return ok && first.Face == second.Face
}

func widthOf(glyphs []glyphPlacement) int {
	width := 0
	for _, placement := range glyphs {
		width += placement.glyph.Advance
	}
	return width
}

// measureLine works a line's width and extent out from the glyphs on it, for
// a line that has had glyphs taken off the end.
func measureLine(line *textLine) {
	line.width, line.ascent, line.descent, line.lineGap = 0, 0, 0, 0
	for _, placement := range line.glyphs {
		line.width += placement.glyph.Advance
		line.ascent = max(line.ascent, placement.metrics.Ascent)
		line.descent = max(line.descent, placement.metrics.Descent)
		line.lineGap = max(line.lineGap, placement.metrics.LineGap)
	}
}

// trimTrailingSpaces takes the spaces off the end of a line, which hang past
// its edge in CSS and so are neither drawn nor counted towards its width.
func trimTrailingSpaces(glyphs []glyphPlacement) []glyphPlacement {
	for len(glyphs) > 0 && isSpace(glyphs[len(glyphs)-1].r) {
		glyphs = glyphs[:len(glyphs)-1]
	}
	return glyphs
}

func (l *TextLayout) Size() image.Point {
	return image.Pt(l.width, l.height)
}

func (l *TextLayout) LineCount() int {
	return len(l.lines)
}

// Lines returns a snapshot of the measured line metrics. The layout remains
// immutable, so callers can use these values while arranging neighbouring
// fragments without being able to mutate the drawing result.
func (l *TextLayout) Lines() []TextLineMetrics {
	if l == nil {
		return nil
	}
	lines := make([]TextLineMetrics, len(l.lines))
	for index, line := range l.lines {
		lines[index] = TextLineMetrics{
			Width: line.width, Ascent: line.ascent, Descent: line.descent,
			LineGap: line.lineGap, Height: line.height,
		}
	}
	return lines
}

func (l *TextLayout) MissingRunes() []rune {
	return slices.Clone(l.missing)
}

// MissingFonts names the families that were asked for a glyph they do not
// have. Wanting a character a font does not carry is nearly always a matter of
// having reached for the wrong font, so the name of that font is half the
// answer.
func (l *TextLayout) MissingFonts() []string {
	return slices.Clone(l.missingIn)
}

// Clipped reports what the box cut off: pixels of overrun along the line, whole
// lines that did not start, and rows of ink lost off the top or bottom.
//
// The third is measured from the glyph bitmaps rather than from the line
// height, and the difference is the whole point. Sixty-six labels across the
// examples lay out a line taller than the box holding it, which is how a label
// is written on a panel this size — the box is sized to the letters, not to the
// descender and line gap under them. Comparing heights calls all sixty-six
// clipped. Asking which rows actually carry ink called one, and that one had
// been losing the bottom row of every character in it since it was written,
// with the render clean and the tests green.
func (l *TextLayout) Clipped() (columns, lines, rows int) {
	available := l.box.Bounds.Size()
	if l.width > available.X {
		columns = l.width - available.X
	}
	// A line counts as lost only when its ascent will not fit, because the
	// ascent is where the glyph bodies are. Measuring against the full line
	// height instead reports a loss whenever a box is sized to the letters
	// rather than to the descent and line gap below them.
	used := 0
	for index, line := range l.lines {
		if used+line.ascent > available.Y {
			lines = len(l.lines) - index
			break
		}
		used += line.height
	}

	// Only glyphs hanging outside the box are opened up. Inside it there is
	// nothing to lose, and reading every bitmap on every render to find that
	// out would be paid for by every page.
	box := l.box.Bounds
	above, below := 0, 0
	l.eachGlyph(func(at image.Rectangle, p glyphPlacement) {
		if at.Min.Y >= box.Min.Y && at.Max.Y <= box.Max.Y {
			return
		}
		top, bottom := p.inkRows()
		if top < 0 {
			return
		}
		if lost := box.Min.Y - (at.Min.Y + top); lost > above {
			above = lost
		}
		if lost := (at.Min.Y + bottom) - (box.Max.Y - 1); lost > below {
			below = lost
		}
	})
	rows = max(0, above) + max(0, below)
	return columns, lines, rows
}

// inkRows is the first and last row of the glyph that has any ink in it, or
// -1 for a glyph that is blank. A space is not clipped by being outside the
// box, and neither is the empty band a font leaves above its capitals.
func (p glyphPlacement) inkRows() (top, bottom int) {
	top, bottom = -1, -1
	if p.missing {
		// The placeholder is a drawn box, so all of it is ink.
		return 0, p.glyph.Height - 1
	}
	for y := 0; y < p.glyph.Height; y++ {
		for x := 0; x < p.glyph.Width; x++ {
			if p.glyph.On(x, y) {
				if top < 0 {
					top = y
				}
				bottom = y
				break
			}
		}
	}
	return top, bottom
}

// eachGlyph walks the laid-out glyphs where they will be drawn.
//
// Drawing and measuring what was lost have to agree about where every glyph
// went, and they did not: one placed the glyphs and the other compared heights,
// so a label whose last row fell outside its box was drawn short and reported
// as fitting. Both go through here now, and a position can only be wrong in one
// place.
func (l *TextLayout) eachGlyph(visit func(at image.Rectangle, p glyphPlacement)) {
	// Every line carries its own leading, half above its letters and half
	// below, so the laid-out height is symmetric about the letters in it and
	// centring on that height centres the letters. It was not: the leading
	// hung below, and the correction subtracted the last line's share of it,
	// which made where a block sat depend on which font ended it.
	y := l.box.Bounds.Min.Y
	switch l.box.VerticalAlign {
	case AlignMiddle:
		y += (l.box.Bounds.Dy() - l.height) / 2
	case AlignBottom:
		y += l.box.Bounds.Dy() - l.height
	}

	for _, line := range l.lines {
		x := l.box.Bounds.Min.X
		switch l.box.Align {
		case AlignCenter:
			x += (l.box.Bounds.Dx() - line.width) / 2
		case AlignEnd:
			x += l.box.Bounds.Dx() - line.width
		}
		baseline := y + line.halfLeading() + line.ascent
		for _, placement := range line.glyphs {
			top := baseline - placement.metrics.Ascent
			visit(image.Rect(x, top, x+placement.glyph.Width, top+placement.glyph.Height), placement)
			x += placement.glyph.Advance
		}
		y += line.height
	}
}

func (l *TextLayout) Draw(canvas *Canvas) {
	if canvas == nil || len(l.lines) == 0 {
		return
	}
	clipped := canvas.Clip(l.box.Bounds)
	// Every halo goes down before any letter does, so that one glyph's ring
	// cannot clear a stroke of the glyph beside it.
	if canvas.frame.toned != nil {
		l.eachGlyph(func(at image.Rectangle, placement glyphPlacement) {
			if placement.missing {
				return
			}
			clipped.haloWhere(at, func(x, y int) bool {
				return placement.glyph.On(x-at.Min.X, y-at.Min.Y)
			}, placement.ink)
		})
	}
	l.eachGlyph(func(at image.Rectangle, placement glyphPlacement) {
		if placement.missing {
			drawMissingGlyph(clipped, at, placement.ink)
		} else {
			drawGlyph(clipped, at.Min, placement.glyph, placement.ink)
		}
		if placement.decoration != 0 {
			drawDecoration(clipped, at, placement)
		}
	})
}

// drawDecoration draws a glyph's share of the lines its text is decorated
// with, across the whole of its advance so that the shares join up. An
// underline sits a pixel below the baseline when the face leaves room there,
// a line through sits across the middle of a lower-case letter, and a line
// over sits along the top of the line's ascent. Each is a pixel thick for
// every sixteen of the face's height, and never less than one.
func drawDecoration(canvas *Canvas, at image.Rectangle, placement glyphPlacement) {
	metrics := placement.metrics
	baseline := at.Min.Y + metrics.Ascent
	thickness := max(1, (metrics.Ascent+metrics.Descent)/16)
	span := func(top int) {
		canvas.FillRect(image.Rect(at.Min.X, top, at.Min.X+placement.glyph.Advance, top+thickness), placement.ink)
	}
	if placement.decoration&Underline != 0 {
		span(baseline + min(1, max(0, metrics.Descent-thickness)))
	}
	if placement.decoration&LineThrough != 0 {
		span(baseline - (metrics.Ascent*3+5)/10 - thickness/2)
	}
	if placement.decoration&Overline != 0 {
		span(at.Min.Y)
	}
}

func (c *Canvas) DrawTextBox(registry *FontRegistry, box TextBox) (*TextLayout, error) {
	layout, err := LayoutText(registry, box)
	if err != nil {
		return nil, err
	}
	layout.Draw(c)
	return layout, nil
}

// drawGlyph puts one glyph on the canvas.
//
// A strike is a picture of a letter rather than an outline of one, so a glyph
// turned by anything but a quarter has to be sampled, and at twelve pixels
// that shows. It is sampled rather than refused because CSS turns a box and
// everything in it, and a page whose labels vanished at an angle would be a
// worse surprise than one whose labels are rough at an angle.
//
// Glyph.On is already the question this needs asked, so the whole of the
// difference between a glyph square to the page and one at an angle is which
// pixels get to ask it — which is what fillWhere decides.
func drawGlyph(canvas *Canvas, at image.Point, glyph Glyph, ink Ink) {
	box := image.Rectangle{Min: at, Max: at.Add(image.Pt(glyph.Width, glyph.Height))}
	canvas.fillWhere(box, func(x, y int) (Ink, bool) {
		return ink, glyph.On(x-at.X, y-at.Y)
	})
}

func drawMissingGlyph(canvas *Canvas, rect image.Rectangle, ink Ink) {
	if rect.Dx() < 3 || rect.Dy() < 3 {
		canvas.FillRect(rect, ink)
		return
	}
	inset := rect.Inset(1)
	stroke := StrokeStyle{Ink: ink, Width: 1}
	canvas.StrokeRect(inset, stroke)
	canvas.DrawLine(inset.Min.Add(image.Pt(1, 1)), inset.Max.Sub(image.Pt(2, 2)), stroke)
	canvas.DrawLine(image.Pt(inset.Max.X-2, inset.Min.Y+1), image.Pt(inset.Min.X+1, inset.Max.Y-2), stroke)
}

// joinSizes lists the strikes a family has, for an error that would otherwise
// leave the reader to go and look them up.
func joinSizes(sizes []int) string {
	parts := make([]string, len(sizes))
	for index, size := range sizes {
		parts[index] = strconv.Itoa(size)
	}
	return strings.Join(parts, ", ")
}

// MinContentWidth is the narrowest box's text can be laid out in without a
// line running past it: its widest stretch that no line may end inside, as
// CSS has it for min-content. A word too long for its line is broken when it
// is laid out, but it is not broken here; that is the last resort, and a box
// asking how narrow it can go should not count on it.
func MinContentWidth(registry *FontRegistry, box TextBox) (int, error) {
	wrap := box.Wrap
	box.Wrap, box.Bounds, box.Ellipsis = NoWrap, image.Rect(0, 0, 1, 1), false
	layout, err := LayoutText(registry, box)
	if err != nil {
		return 0, err
	}
	widest := 0
	for _, line := range layout.lines {
		if wrap == NoWrap {
			widest = max(widest, line.width)
			continue
		}
		// The stretch since the last place a line could end, and the spaces
		// at its end, which hang and so take no room.
		stretch, hanging := 0, 0
		for index, placement := range line.glyphs {
			if index > 0 {
				before := rune(0)
				if index > 1 {
					before = line.glyphs[index-2].r
				}
				previous := line.glyphs[index-1].r
				opening := BreakBetween(before, previous, placement.r)
				if wrap == WrapRunes {
					opening = !isSpace(placement.r)
				}
				if opening {
					stretch, hanging = 0, 0
				}
			}
			stretch += placement.glyph.Advance
			if isSpace(placement.r) {
				hanging += placement.glyph.Advance
			} else {
				hanging = 0
			}
			widest = max(widest, stretch-hanging)
		}
	}
	return widest, nil
}
