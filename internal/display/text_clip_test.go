package display

import (
	"image"
	"testing"
)

func layout(t *testing.T, box image.Rectangle, wrap WrapMode, runs ...TextRun) *TextLayout {
	t.Helper()
	fonts, err := NewBuiltinFontRegistry()
	if err != nil {
		t.Fatal(err)
	}
	result, err := LayoutText(fonts, TextBox{Bounds: box, Runs: runs, Wrap: wrap})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func mono(text string, size int) TextRun {
	return TextRun{Text: text, Style: TextStyle{Font: "monaco", Size: size, Ink: InkBlack}}
}

// The case this whole check exists for. A figure that loses its leading digit
// is still a figure, and nothing on the panel suggests it is the wrong one.
func TestAFigureTooWideForItsBoxIsReported(t *testing.T) {
	// Monaco 12 advances 7 pixels, so ten characters need seventy.
	narrow := layout(t, image.Rect(0, 0, 60, 15), NoWrap, mono("3260/3720G", 12))
	columns, lines, _ := narrow.Clipped()
	if columns != 10 {
		t.Errorf("columns = %d, want the ten pixels that do not fit", columns)
	}
	if lines != 0 {
		t.Errorf("lines = %d, want none: it is one line", lines)
	}

	wide := layout(t, image.Rect(0, 0, 70, 15), NoWrap, mono("3260/3720G", 12))
	if columns, lines, _ := wide.Clipped(); columns != 0 || lines != 0 {
		t.Errorf("a box that fits reported %d columns and %d lines", columns, lines)
	}
}

// Measured across this repository, fifty-six of fifty-nine overflows were a
// box sized to the letters rather than to the descent and line gap below them.
// Every glyph was drawn. Reporting those would bury the three that mattered.
func TestABoxSizedToTheLettersIsNotClipped(t *testing.T) {
	fonts, err := NewBuiltinFontRegistry()
	if err != nil {
		t.Fatal(err)
	}
	set, ok := fonts.Match("monaco", 12)
	if !ok {
		t.Fatal("monaco 12 is missing")
	}
	metrics := set.Metrics()
	full := metrics.Ascent + metrics.Descent + metrics.LineGap
	if full <= metrics.Ascent {
		t.Fatalf("metrics %+v leave no descent or gap to test with", metrics)
	}

	// A box exactly as tall as the ascent: the glyph bodies fit, the gap
	// below them does not.
	tight := layout(t, image.Rect(0, 0, 200, metrics.Ascent), NoWrap, mono("/backup", 12))
	if _, lines, _ := tight.Clipped(); lines != 0 {
		t.Errorf("a box of %d pixels reported %d lines lost, but the ascent is %d",
			metrics.Ascent, lines, metrics.Ascent)
	}
	// One pixel short of the ascent is a real loss.
	shorter := layout(t, image.Rect(0, 0, 200, metrics.Ascent-1), NoWrap, mono("/backup", 12))
	if _, lines, _ := shorter.Clipped(); lines != 1 {
		t.Errorf("a box one pixel under the ascent reported %d lines lost, want 1", lines)
	}
}

// Wrapped text in a box too short does lose whole lines, and that has to be
// reported: the missing lines are content the author asked for.
func TestWrappedLinesThatDoNotFitAreCounted(t *testing.T) {
	const sentence = "the quick brown fox jumps over the lazy dog again and again"
	tall := layout(t, image.Rect(0, 0, 100, 200), WrapRunes, mono(sentence, 12))
	if tall.LineCount() < 4 {
		t.Fatalf("the sample wrapped to %d lines, too few to test with", tall.LineCount())
	}
	if _, lines, _ := tall.Clipped(); lines != 0 {
		t.Fatalf("a box with room to spare reported %d lines lost", lines)
	}

	short := layout(t, image.Rect(0, 0, 100, 34), WrapRunes, mono(sentence, 12))
	_, lines, _ := short.Clipped()
	if lines == 0 {
		t.Fatal("a box holding two of the lines reported nothing lost")
	}
	if shown := short.LineCount() - lines; shown < 1 {
		t.Errorf("reported %d of %d lines lost, which leaves nothing drawn",
			lines, short.LineCount())
	}
}

// A box shorter than the full line still shows the line, which is what
// TestABoxSizedToTheLettersIsNotClipped asserts. Measuring the raw difference
// between content and box would report those, so no such accessor is exported:
// the only question worth answering from outside is whether content was lost.
func TestTheLayoutExposesLossRatherThanOverflow(t *testing.T) {
	fonts, err := NewBuiltinFontRegistry()
	if err != nil {
		t.Fatal(err)
	}
	set, _ := fonts.Match("monaco", 12)
	metrics := set.Metrics()
	result := layout(t, image.Rect(0, 0, 200, metrics.Ascent), NoWrap, mono("ok", 12))
	if result.Size().Y <= metrics.Ascent {
		t.Fatal("the sample does not exceed its box, so it cannot show the difference")
	}
	if _, lines, _ := result.Clipped(); lines != 0 {
		t.Errorf("a box shorter than the line box reported %d lines lost", lines)
	}
}

// A line that fits by its own height can still lose the bottom of its letters.
//
// This is the shape of a bug that lived in examples/card_showcase from the day
// it was written: hzk at 12 is ten of ascent and two of descent and fills all
// twelve rows, so a box of eleven cut the last row off every character. The
// line was there, the render was clean, the tests were green, and the only way
// to find it was to look at the picture.
//
// Measuring heights cannot catch it and must not be made to try: a box sized to
// the letters rather than to the descender under them is how most labels here
// are written, and sixty-six of them would report a loss they do not have. What
// separates the two is whether the rows falling outside the box carry any ink.
func TestABoxShorterThanItsLettersReportsTheRowsItTakes(t *testing.T) {
	cjk := TextRun{Text: "蓝牙", Style: TextStyle{Font: "hzk", Size: 12, Ink: InkBlack}}

	// hzk 12 is ten of ascent, two of descent and two of line gap, and CSS
	// puts half that gap above the letters — so the twelve rows of ink start
	// one row down and an eleven row box loses two of them.
	short := layout(t, image.Rect(0, 0, 60, 11), NoWrap, cjk)
	columns, lines, rows := short.Clipped()
	if rows != 2 {
		t.Fatalf("rows = %d, want 2 rows of ink lost", rows)
	}
	if columns != 0 || lines != 0 {
		t.Fatalf("columns = %d and lines = %d; neither is what this box takes", columns, lines)
	}

	// The whole line box and the same text keeps everything.
	exact := layout(t, image.Rect(0, 0, 60, 14), NoWrap, cjk)
	if columns, lines, rows := exact.Clipped(); columns != 0 || lines != 0 || rows != 0 {
		t.Fatalf("a box of twelve reported %d, %d, %d; it holds the whole glyph", columns, lines, rows)
	}
}

// The whitespace a font leaves under its letters is not ink, so overflowing
// into it is not a loss. Every label sized to its letters relies on this.
func TestOverflowingIntoTheLineGapIsNotClipping(t *testing.T) {
	// Monaco 12 lays out a line of seventeen: twelve of ascent, three of
	// descent, two of gap. Digits reach neither the gap nor the descender.
	tight := layout(t, image.Rect(0, 0, 80, 14), NoWrap, mono("0.0428", 12))
	if _, _, rows := tight.Clipped(); rows != 0 {
		t.Fatalf("rows = %d, want 0: the three pixels of overflow are empty", rows)
	}
}

// Where a block of text sits must not depend on which font ends it.
//
// The leading a line-height adds used to hang entirely below the letters, and
// the correction for that subtracted the last line's share of it — so a block
// of Chinese then Latin and the same block the other way round sat about two
// pixels apart, and every line in it moved. CSS puts half the leading above the
// letters and half below, which makes each line self-contained: the same line
// lands in the same place whatever is above or below it.
func TestALineSitsInTheSamePlaceWhateverFollowsIt(t *testing.T) {
	cjk := TextRun{Text: "中文", Style: TextStyle{Font: "hzk", Size: 12, Ink: InkBlack}}
	latin := TextRun{Text: "ABC", Style: TextStyle{Font: "monaco", Size: 12, Ink: InkBlack}}
	newline := TextRun{Text: "\n", Style: TextStyle{Font: "hzk", Size: 12, Ink: InkBlack}}

	// The first line is the same in both; only what follows it differs.
	inkTopOf := func(t *testing.T, second TextRun) int {
		t.Helper()
		box := TextBox{
			Bounds: image.Rect(0, 0, 180, 40), LineHeight: 16, VerticalAlign: AlignMiddle,
			Runs: []TextRun{cjk, newline, second},
		}
		fonts, err := NewBuiltinFontRegistry()
		if err != nil {
			t.Fatal(err)
		}
		laid, err := LayoutText(fonts, box)
		if err != nil {
			t.Fatal(err)
		}
		top := -1
		laid.eachGlyph(func(at image.Rectangle, p glyphPlacement) {
			if top >= 0 || at.Min.Y > 20 {
				return
			}
			for y := 0; y < p.glyph.Height; y++ {
				for x := 0; x < p.glyph.Width; x++ {
					if p.glyph.On(x, y) {
						top = at.Min.Y + y
						return
					}
				}
			}
		})
		if top < 0 {
			t.Fatal("the first line drew no ink")
		}
		return top
	}

	after := inkTopOf(t, cjk)
	if other := inkTopOf(t, latin); other != after {
		t.Errorf("the first line's ink starts at %d when Chinese follows it and %d when Latin does",
			after, other)
	}
}
