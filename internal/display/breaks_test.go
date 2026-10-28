package display

import (
	"image"
	"slices"
	"strings"
	"testing"
)

// lineTexts lays text out word-wrapped at the width fit takes on one line,
// and says what each line holds.
func lineTexts(t *testing.T, registry *FontRegistry, style TextStyle, text, fit string, wrap WrapMode) []string {
	t.Helper()
	width := 1
	if fit != "" {
		measured, err := LayoutText(registry, TextBox{Bounds: image.Rect(0, 0, 1, 1), Wrap: NoWrap,
			Runs: []TextRun{{Text: fit, Style: style}}})
		if err != nil {
			t.Fatal(err)
		}
		width = measured.Size().X
	}
	layout, err := LayoutText(registry, TextBox{Bounds: image.Rect(0, 0, width, 400), Wrap: wrap,
		Runs: []TextRun{{Text: text, Style: style}}})
	if err != nil {
		t.Fatal(err)
	}
	var lines []string
	for _, line := range layout.lines {
		var written strings.Builder
		for _, placement := range line.glyphs {
			written.WriteRune(placement.r)
		}
		lines = append(lines, written.String())
		if line.width > width {
			t.Errorf("%q: line %q is %dpx, wider than its %dpx box", text, written.String(), line.width, width)
		}
	}
	return lines
}

// A line wrapping words ends where a browser would end it, and the box it is
// in is the width of some shorter text, so the cases read as what fits.
func TestWordsWrapWhereABrowserWrapsThem(t *testing.T) {
	registry := builtinRegistry(t)
	latin := TextStyle{Font: "monaco", Size: 10}
	chinese := TextStyle{Font: "ui", Size: 12}
	tests := []struct {
		name  string
		style TextStyle
		text  string
		fit   string
		want  []string
	}{
		{"between words", latin, "hello world", "hello wor", []string{"hello", "world"}},
		{"several words on a line", latin, "a bb ccc dddd", "a bb ccc", []string{"a bb ccc", "dddd"}},
		{"spaces hang", latin, "ab   cd", "ab", []string{"ab", "cd"}},
		{"after a hyphen", latin, "well-known fact", "well-know", []string{"well-", "known", "fact"}},
		{"not before a digit", latin, "on 2026-10-26", "on 2026-10-2", []string{"on", "2026-10-26"}},
		{"after an em dash", latin, "word—word", "word—wo", []string{"word—", "word"}},
		{"a word too long breaks", latin, "abcdefghij", "abcd", []string{"abcd", "efgh", "ij"}},
		{"a long word after a short one", latin, "a abcdefghij", "abcd", []string{"a", "abcd", "efgh", "ij"}},
		{"punctuation stays with its word", latin, "one, two", "one,", []string{"one,", "two"}},
		{"between Chinese characters", chinese, "你好世界", "你好世", []string{"你好世", "界"}},
		{"not before closing punctuation", chinese, "你好，世界", "你好", []string{"你", "好，", "世界"}},
		{"not after an opening bracket", chinese, "你《好》", "你《好", []string{"你", "《好》"}},
		{"Chinese after a Latin word", chinese, "inkwire显示", "inkwire显", []string{"inkwire显", "示"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := lineTexts(t, registry, test.style, test.text, test.fit, WrapWords)
			if !slices.Equal(got, test.want) {
				t.Fatalf("%q in the width of %q = %q, want %q", test.text, test.fit, got, test.want)
			}
		})
	}
}

func TestBreakingCharactersStillWorks(t *testing.T) {
	registry := builtinRegistry(t)
	got := lineTexts(t, registry, TextStyle{Font: "monaco", Size: 10}, "hello world", "hello wor", WrapRunes)
	if want := []string{"hello wor", "ld"}; !slices.Equal(got, want) {
		t.Fatalf("lines = %q, want %q", got, want)
	}
}

func TestMinContentIsTheWidestUnbreakableStretch(t *testing.T) {
	registry := builtinRegistry(t)
	style := TextStyle{Font: "monaco", Size: 10}
	width := func(text string, wrap WrapMode) int {
		t.Helper()
		got, err := MinContentWidth(registry, TextBox{Wrap: wrap, Runs: []TextRun{{Text: text, Style: style}}})
		if err != nil {
			t.Fatal(err)
		}
		return got
	}
	character := width("x", WrapWords)
	tests := []struct {
		text  string
		wrap  WrapMode
		chars int
	}{
		{"a temperature reading", WrapWords, 11},
		{"well-known", WrapWords, 5},
		{"ab   ", WrapWords, 2},
		{"short\nmuch longer", WrapWords, 6},
		{"a temperature", NoWrap, 13},
		{"a temperature", WrapRunes, 1},
	}
	for _, test := range tests {
		if got := width(test.text, test.wrap); got != test.chars*character {
			t.Errorf("%q in mode %d: min-content %d, want %d characters (%d)", test.text, test.wrap, got, test.chars, test.chars*character)
		}
	}
}
