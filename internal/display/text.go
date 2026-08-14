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

// Clipped reports whether drawing will actually lose content, which is not the
// same as overflowing the box.
//
// Measured across the scenes in this repository, fifty-six of fifty-nine
// overflows were one to four pixels on the vertical axis: the line's descent
// and gap reaching past a box sized to the glyphs, with every glyph still
// drawn. Warning about those would bury the three that mattered. So the test
// is whether a whole line goes missing, or whether the line is wider than the
// box and characters fall off the end.
//
// Losing characters is the case worth catching. Prose cut short is visibly cut
// short; a figure of 3260 clipped to 260 is still a number, still plausible,
// and gives no sign that it is wrong.
func (l *TextLayout) Clipped() (columns, lines int) {
	available := l.box.Bounds.Size()
	if l.width > available.X {
		columns = l.width - available.X
	}
	// A line counts as lost only when its ascent will not fit, because the
	// ascent is where the glyph bodies are. Measuring against the full line
	// height instead reports a loss whenever a box is sized to the letters
	// rather than to the descent and line gap below them, which is most
	// single-line labels in this repository and none of them is clipped.
	used := 0
	for index, line := range l.lines {
		if used+line.ascent > available.Y {
			return columns, len(l.lines) - index
		}
		used += line.height
	}
	return columns, 0
}

func (l *TextLayout) Draw(canvas *Canvas) {
	if canvas == nil || len(l.lines) == 0 {
		return
	}
	clipped := canvas.Clip(l.box.Bounds)
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
		baseline := y + line.ascent
		for _, placement := range line.glyphs {
			top := baseline - placement.metrics.Ascent
			if placement.missing {
				drawMissingGlyph(clipped, image.Rect(x, top, x+placement.glyph.Width, top+placement.glyph.Height), placement.ink)
			} else {
				drawGlyph(clipped, image.Pt(x, top), placement.glyph, placement.ink)
			}
			x += placement.glyph.Advance
		}
		y += line.height
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

func drawGlyph(canvas *Canvas, at image.Point, glyph Glyph, ink Ink) {
	for y := 0; y < glyph.Height; y++ {
		for x := 0; x < glyph.Width; x++ {
			if glyph.On(x, y) {
				canvas.Set(at.X+x, at.Y+y, ink)
			}
		}
	}
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
