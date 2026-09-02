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
	WrapRunes
)

type TextStyle struct {
	Font string
	Size int
	Ink  Ink
}

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
}

type glyphPlacement struct {
	glyph   Glyph
	metrics FontMetrics
	ink     Ink
	r       rune
	missing bool
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

	appendRune := func(r rune, style TextStyle, set *FontSet) {
		resolved, ok := set.Resolve(r)
		placement := glyphPlacement{ink: style.Ink, r: r}
		if ok {
			placement.glyph = resolved.Glyph
			placement.metrics = resolved.Metrics
		} else {
			metrics := set.Metrics()
			size := max(1, metrics.Ascent+metrics.Descent)
			placement.glyph = Glyph{Width: size, Height: size, Advance: size}
			placement.metrics = metrics
			placement.missing = true
			layout.missing = append(layout.missing, r)
			if !slices.Contains(layout.missingIn, style.Font) {
				layout.missingIn = append(layout.missingIn, style.Font)
			}
		}
		lastMetrics = placement.metrics
		if box.Wrap == WrapRunes && lineHasContent && current.width+placement.glyph.Advance > box.Bounds.Dx() {
			finishLine(false)
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
	finishLine(false)
	return layout, nil
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
	l.eachGlyph(func(at image.Rectangle, placement glyphPlacement) {
		if placement.missing {
			drawMissingGlyph(clipped, at, placement.ink)
		} else {
			drawGlyph(clipped, at.Min, placement.glyph, placement.ink)
		}
	})
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
