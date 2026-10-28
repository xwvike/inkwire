package display

import (
	"image"
	"strings"
	"testing"
)

// drawnText draws one run into a frame of its own and hands back the frame
// and the layout.
func drawnText(t *testing.T, style TextStyle, text string, box TextBox) (*Frame, *TextLayout) {
	t.Helper()
	registry := builtinRegistry(t)
	if box.Bounds.Empty() {
		box.Bounds = image.Rect(0, 0, 200, 40)
	}
	box.Runs = []TextRun{{Text: text, Style: style}}
	frame, err := NewFrame(box.Bounds.Max.X, box.Bounds.Max.Y, InkWhite)
	if err != nil {
		t.Fatal(err)
	}
	layout, err := NewCanvas(frame).DrawTextBox(registry, box)
	if err != nil {
		t.Fatal(err)
	}
	return frame, layout
}

func inkIn(frame *Frame, area image.Rectangle) int {
	count := 0
	for y := area.Min.Y; y < area.Max.Y; y++ {
		for x := area.Min.X; x < area.Max.X; x++ {
			if ink, _ := frame.InkAt(x, y); ink == InkBlack {
				count++
			}
		}
	}
	return count
}

func TestBoldIsEachGlyphDrawnTwiceAPixelApart(t *testing.T) {
	plain, plainLayout := drawnText(t, TextStyle{}, "Hello", TextBox{})
	bold, boldLayout := drawnText(t, TextStyle{Bold: true}, "Hello", TextBox{})
	if got, want := boldLayout.Size().X, plainLayout.Size().X+5; got != want {
		t.Errorf("bold Hello is %dpx wide; want %d, a pixel more for each letter", got, want)
	}
	if inkIn(bold, bold.Bounds()) <= inkIn(plain, plain.Bounds()) {
		t.Error("bold text has no more ink than plain text")
	}
	// Every pixel of the first letter is still there, and so is the one to its
	// right.
	first := plainLayout.lines[0].glyphs[0].glyph
	for y := 0; y < first.Height; y++ {
		for x := 0; x < first.Width; x++ {
			if !first.On(x, y) {
				continue
			}
			for _, dx := range []int{0, 1} {
				if ink, _ := bold.InkAt(x+dx, y+plainLayout.lines[0].halfLeading()); ink != InkBlack {
					t.Fatalf("bold H is missing the pixel at %d,%d", x+dx, y)
				}
			}
		}
	}
}

func TestLetterSpacingIsAddedAfterEveryCharacter(t *testing.T) {
	_, plain := drawnText(t, TextStyle{}, "abcd", TextBox{})
	_, wide := drawnText(t, TextStyle{LetterSpacing: 3}, "abcd", TextBox{})
	if got, want := wide.Size().X, plain.Size().X+12; got != want {
		t.Errorf("abcd with 3px letter-spacing is %dpx; want %d", got, want)
	}
	_, tight := drawnText(t, TextStyle{LetterSpacing: -100}, "abcd", TextBox{})
	if tight.Size().X != 0 {
		t.Errorf("a letter-spacing larger than the letters left them %dpx wide; want them on top of each other", tight.Size().X)
	}
	for _, placement := range tight.lines[0].glyphs {
		if placement.glyph.Advance < 0 {
			t.Fatalf("%q steps back %dpx; a letter is never placed before the one ahead of it", placement.r, -placement.glyph.Advance)
		}
	}
	// It is part of where the line can break, so it is part of how narrow a
	// box can be too.
	registry := builtinRegistry(t)
	narrow, err := MinContentWidth(registry, TextBox{Wrap: WrapWords, Runs: []TextRun{{Text: "ab cd", Style: TextStyle{LetterSpacing: 2}}}})
	if err != nil {
		t.Fatal(err)
	}
	_, word := drawnText(t, TextStyle{LetterSpacing: 2}, "ab", TextBox{})
	if narrow != word.Size().X {
		t.Errorf("the min-content of spaced text is %d; want %d, one spaced word", narrow, word.Size().X)
	}
}

func TestDecorationsAreLinesAcrossTheText(t *testing.T) {
	for _, test := range []struct {
		name       string
		decoration TextDecoration
	}{{"underline", Underline}, {"line-through", LineThrough}, {"overline", Overline}} {
		t.Run(test.name, func(t *testing.T) {
			plain, layout := drawnText(t, TextStyle{}, "ab cd", TextBox{})
			decorated, _ := drawnText(t, TextStyle{Decoration: test.decoration}, "ab cd", TextBox{})
			width := layout.Size().X
			// Some row is inked from the first pixel of the text to the last,
			// across the space, which no plain row is.
			full := func(frame *Frame) int {
				rows := 0
				for y := 0; y < frame.Height(); y++ {
					if inkIn(frame, image.Rect(0, y, width, y+1)) == width {
						rows++
					}
				}
				return rows
			}
			if full(plain) != 0 || full(decorated) != 1 {
				t.Errorf("%s drew %d rows across the text; want one", test.name, full(decorated))
			}
			if inkIn(decorated, image.Rect(width, 0, decorated.Width(), decorated.Height())) != 0 {
				t.Errorf("%s ran past the end of the text", test.name)
			}
		})
	}
}

func TestAnEllipsisEndsALineTooLongForItsBox(t *testing.T) {
	_, whole := drawnText(t, TextStyle{}, "Quarterly results", TextBox{})
	box := image.Rect(0, 0, whole.Size().X/2, 20)
	frame, layout := drawnText(t, TextStyle{}, "Quarterly results", TextBox{Bounds: box, Ellipsis: true})
	var written strings.Builder
	for _, placement := range layout.lines[0].glyphs {
		written.WriteRune(placement.r)
	}
	text := written.String()
	if !strings.HasSuffix(text, "…") && !strings.HasSuffix(text, "...") {
		t.Errorf("the line reads %q; want it to end in an ellipsis", text)
	}
	if !strings.HasPrefix("Quarterly results", strings.TrimRight(text, ".…")) {
		t.Errorf("the line reads %q; want the start of the text", text)
	}
	if layout.Size().X > box.Dx() || layout.Size().X != widthOf(layout.lines[0].glyphs) {
		t.Errorf("the ellipsised line is %dpx, its glyphs %dpx, in a %dpx box", layout.Size().X, widthOf(layout.lines[0].glyphs), box.Dx())
	}
	if columns, _, _ := layout.Clipped(); columns != 0 {
		t.Errorf("an ellipsised line still reports %d pixels cut off", columns)
	}
	if inkIn(frame, box) == 0 {
		t.Error("nothing was drawn")
	}
	// A line that fits is left alone.
	_, fits := drawnText(t, TextStyle{}, "Short", TextBox{Ellipsis: true})
	if got := len(fits.lines[0].glyphs); got != 5 {
		t.Errorf("a line that fits has %d glyphs; want its 5", got)
	}
	// The space before the cut is not left hanging in front of the ellipsis,
	// even where the room left would hold it.
	_, start := drawnText(t, TextStyle{}, "ab ", TextBox{})
	mark := widthOf(layout.lines[0].glyphs[len(layout.lines[0].glyphs)-1:])
	if layout.lines[0].glyphs[len(layout.lines[0].glyphs)-1].r == '.' {
		mark *= 3
	}
	_, cut := drawnText(t, TextStyle{}, "ab cdefghijklmnop", TextBox{Bounds: image.Rect(0, 0, start.Size().X+mark, 20), Ellipsis: true})
	for _, placement := range cut.lines[0].glyphs {
		if placement.r == ' ' {
			t.Error("the ellipsis follows a space")
		}
	}
	// How narrow the text can be is a question about its words, which an
	// ellipsis does not change.
	registry := builtinRegistry(t)
	for _, ellipsis := range []bool{false, true} {
		narrow, err := MinContentWidth(registry, TextBox{Wrap: WrapWords, Ellipsis: ellipsis,
			Runs: []TextRun{{Text: "Quarterly results"}}})
		if err != nil {
			t.Fatal(err)
		}
		_, word := drawnText(t, TextStyle{}, "Quarterly", TextBox{})
		if narrow != word.Size().X {
			t.Errorf("min-content with ellipsis %v is %d; want %d", ellipsis, narrow, word.Size().X)
		}
	}
}

// An ellipsis comes from the face the line ends in. The ui sets have "…" only
// in their Chinese face, where it is centred in a full-width cell; borrowed
// for a line of Latin text it is three beads at the height of the x, which is
// not what a browser ends that line with.
func TestAnEllipsisIsDrawnInTheFaceTheLineEndsIn(t *testing.T) {
	for _, test := range []struct {
		text, ends string
	}{
		{"Quarterly results", "..."},
		{"这是一段很长的中文文本内容", "…"},
	} {
		style := TextStyle{Font: "ui", Size: 16}
		_, whole := drawnText(t, style, test.text, TextBox{})
		_, layout := drawnText(t, style, test.text, TextBox{Bounds: image.Rect(0, 0, whole.Size().X/2, 20), Ellipsis: true})
		var written strings.Builder
		for _, placement := range layout.lines[0].glyphs {
			written.WriteRune(placement.r)
		}
		if !strings.HasSuffix(written.String(), test.ends) {
			t.Errorf("%q is cut to %q; want it to end in %q", test.text, written.String(), test.ends)
		}
	}
}
