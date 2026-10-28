package compose

import (
	"image"
	"slices"
	"strings"
	"testing"

	"github.com/xwvike/inkwire/internal/display"
)

var monaco = display.TextStyle{Font: "monaco", Size: 10}

// monacoWidth is how wide n characters of monaco are, so the cases can give
// their widths in characters.
func monacoWidth(t *testing.T, compiler *Compiler, n int) int {
	t.Helper()
	layout, err := display.LayoutText(compiler.Fonts, display.TextBox{Bounds: image.Rect(0, 0, 1, 1),
		Runs: []display.TextRun{{Text: strings.Repeat("x", n), Style: monaco}}})
	if err != nil {
		t.Fatal(err)
	}
	return layout.Size().X
}

func words(written string) InlineItem {
	return InlineItem{Runs: []display.TextRun{{Text: written, Style: monaco}}, Wrap: display.WrapWords}
}

// inlineLines lays inline out in a width of characters and writes each line
// as its fragments, a box written [].
func inlineLines(t *testing.T, inline Inline, characters int) []string {
	t.Helper()
	compiler, err := NewDefaultCompiler()
	if err != nil {
		t.Fatal(err)
	}
	ctx := &compileContext{compiler: compiler}
	lines, _, _, err := inline.layout(ctx, monacoWidth(t, compiler, characters), 0, "inline")
	if err != nil {
		t.Fatal(err)
	}
	var written []string
	for _, line := range lines {
		var fragments []string
		for _, fragment := range line.fragments {
			if fragment.node != nil {
				fragments = append(fragments, "[]")
				continue
			}
			var joined strings.Builder
			for _, run := range fragment.runs {
				joined.WriteString(run.Text)
			}
			fragments = append(fragments, joined.String())
		}
		written = append(written, strings.Join(fragments, "|"))
	}
	return written
}

func TestInlineTextWrapsAtWordsAcrossItems(t *testing.T) {
	box := InlineItem{Node: Rectangle{Size: image.Pt(1, 1)}, Wrap: display.WrapWords}
	nowrap := words("cd ef")
	nowrap.Wrap = display.NoWrap
	nowrap2 := words("de")
	nowrap2.Wrap = display.NoWrap
	tests := []struct {
		name       string
		items      []InlineItem
		characters int
		want       []string
	}{
		{"between words", []InlineItem{words("hello world")}, 9, []string{"hello", "world"}},
		{"a word in two styles is one word", []InlineItem{words("ab foo"), words("bar baz")}, 7,
			[]string{"ab", "foo|bar", "baz"}},
		{"the space may come first", []InlineItem{words("ab"), words(" cd")}, 3, []string{"ab", "cd"}},
		{"a long word breaks", []InlineItem{words("a abcdefgh")}, 4, []string{"a", "abcd", "efgh"}},
		{"a box is a place to break", []InlineItem{words("abc"), box, words("def")}, 3,
			[]string{"abc", "[]", "def"}},
		{"a space at the end does not count", []InlineItem{words("a bcd ef")}, 5, []string{"a bcd", "ef"}},
		{"the block decides between items", []InlineItem{words("x abc "), nowrap2}, 5,
			[]string{"x abc", "de"}},
		{"nowrap text holds", []InlineItem{words("ab "), nowrap, words(" gh")}, 4,
			[]string{"ab", "cd ef", "gh"}},
		{"a forced break", []InlineItem{words("ab"), {Break: true}, {Break: true}, words("cd")}, 9,
			[]string{"ab", "", "cd"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := inlineLines(t, Inline{Items: test.items, Wrap: display.WrapWords}, test.characters)
			if !slices.Equal(got, test.want) {
				t.Fatalf("lines = %q, want %q", got, test.want)
			}
		})
	}
}

// An inline box that wraps is cut where the line ends: its left padding and
// border go with its first line and its right ones with its last, and its
// background is no wider than the text and padding it is behind.
func TestAWrappedInlineBoxIsCutAtTheLineEnd(t *testing.T) {
	compiler, err := NewDefaultCompiler()
	if err != nil {
		t.Fatal(err)
	}
	character := monacoWidth(t, compiler, 1)
	red := display.InkRed
	item := words("aaa bbb")
	item.Background = &red
	item.Padding = Insets{Left: 5, Right: 7}
	item.Margin = Insets{Left: 3, Right: 11}
	inline := Inline{Items: []InlineItem{item}, Wrap: display.WrapWords, LineHeight: 20}
	width := 3*character + 20
	document := Document{Size: image.Pt(width, 40), Root: inline}
	compiled, _, err := compiler.Compile(document)
	if err != nil {
		t.Fatal(err)
	}
	frame, err := compiled.Render()
	if err != nil {
		t.Fatal(err)
	}
	// Each line's red, from the background showing between and around the
	// letters.
	spans := map[int][2]int{}
	for _, line := range []int{0, 20} {
		first, last := frame.Width(), -1
		for y := line; y < line+20; y++ {
			for x := 0; x < frame.Width(); x++ {
				if ink, _ := frame.InkAt(x, y); ink == display.InkRed {
					first, last = min(first, x), max(last, x)
				}
			}
		}
		spans[line] = [2]int{first, last + 1}
	}
	if want := [2]int{3, 3 + 5 + 3*character}; spans[0] != want {
		t.Errorf("first line's background spans %v, want %v", spans[0], want)
	}
	if want := [2]int{0, 3*character + 7}; spans[20] != want {
		t.Errorf("second line's background spans %v, want %v", spans[20], want)
	}
}

// A box cut at a line's end has no corner there to round, and a share of the
// box for a radius is no different from a number of pixels in that.
func TestACutInlineBoxHasSquareCorners(t *testing.T) {
	compiler, err := NewDefaultCompiler()
	if err != nil {
		t.Fatal(err)
	}
	character := monacoWidth(t, compiler, 1)
	red := display.InkRed
	item := words("aaa bbb")
	item.Background = &red
	item.RadiusLength = Tenths(500)
	inline := Inline{Items: []InlineItem{item}, Wrap: display.WrapWords, LineHeight: 20}
	compiled, _, err := compiler.Compile(Document{Size: image.Pt(3*character+4, 40), Root: inline})
	if err != nil {
		t.Fatal(err)
	}
	frame, err := compiled.Render()
	if err != nil {
		t.Fatal(err)
	}
	box := image.Rectangle{Min: image.Pt(frame.Width(), 20)}
	for y := 0; y < 20; y++ {
		for x := 0; x < frame.Width(); x++ {
			if ink, _ := frame.InkAt(x, y); ink == display.InkRed {
				box = box.Union(image.Rect(x, y, x+1, y+1))
			}
		}
	}
	if ink, _ := frame.InkAt(box.Max.X-1, box.Min.Y); ink != display.InkRed {
		t.Errorf("the first line's box %v is rounded where it was cut", box)
	}
}
