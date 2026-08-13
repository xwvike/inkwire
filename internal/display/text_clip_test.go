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
	columns, lines := narrow.Clipped()
	if columns != 10 {
		t.Errorf("columns = %d, want the ten pixels that do not fit", columns)
	}
	if lines != 0 {
		t.Errorf("lines = %d, want none: it is one line", lines)
	}

	wide := layout(t, image.Rect(0, 0, 70, 15), NoWrap, mono("3260/3720G", 12))
	if columns, lines := wide.Clipped(); columns != 0 || lines != 0 {
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
	if _, lines := tight.Clipped(); lines != 0 {
		t.Errorf("a box of %d pixels reported %d lines lost, but the ascent is %d",
			metrics.Ascent, lines, metrics.Ascent)
	}
	// One pixel short of the ascent is a real loss.
	shorter := layout(t, image.Rect(0, 0, 200, metrics.Ascent-1), NoWrap, mono("/backup", 12))
	if _, lines := shorter.Clipped(); lines != 1 {
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
	if _, lines := tall.Clipped(); lines != 0 {
		t.Fatalf("a box with room to spare reported %d lines lost", lines)
	}

	short := layout(t, image.Rect(0, 0, 100, 34), WrapRunes, mono(sentence, 12))
	_, lines := short.Clipped()
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
	if _, lines := result.Clipped(); lines != 0 {
		t.Errorf("a box shorter than the line box reported %d lines lost", lines)
	}
}
