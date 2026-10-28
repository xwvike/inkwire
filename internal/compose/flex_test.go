package compose

import (
	"image"
	"strings"
	"testing"

	"github.com/xwvike/inkwire/internal/display"
)

func monacoText(written string) Text {
	return Text{Runs: []display.TextRun{{Text: written, Style: monaco}}, Wrap: display.WrapWords}
}

// lineSizes lays children out in a row of the given width and says what each
// was given.
func lineSizes(t *testing.T, box image.Point, horizontal bool, children ...LayoutChild) []image.Point {
	t.Helper()
	compiler, err := NewDefaultCompiler()
	if err != nil {
		t.Fatal(err)
	}
	ctx := &compileContext{compiler: compiler}
	sizes, _, err := flexLine(ctx, box, 0, CrossStretch, horizontal, children, "row", 0)
	if err != nil {
		t.Fatal(err)
	}
	return sizes
}

const paragraph = "The living room has been warm all afternoon and the sensor reads above target."

// A label beside a long paragraph keeps its word whole, as it does in a
// browser, where min-width: auto keeps a flex item from shrinking below its
// longest word. The paragraph takes the rest and wraps in it.
func TestAFlexItemDoesNotShrinkBelowItsLongestWord(t *testing.T) {
	compiler, err := NewDefaultCompiler()
	if err != nil {
		t.Fatal(err)
	}
	character := monacoWidth(t, compiler, 1)
	label := LayoutChild{Node: monacoText("Temperature"), Shrink: 1, MinContent: true}
	body := LayoutChild{Node: monacoText(paragraph), Shrink: 1, MinContent: true}
	sizes := lineSizes(t, image.Pt(40*character, 400), true, label, body)
	if sizes[0].X != 11*character {
		t.Errorf("the label was given %dpx, want its word's %dpx", sizes[0].X, 11*character)
	}
	if sizes[1].X != 29*character {
		t.Errorf("the paragraph was given %dpx, want the other %dpx", sizes[1].X, 29*character)
	}
	if lines := sizes[1].Y / sizes[0].Y; lines < 3 {
		t.Errorf("the paragraph is %dpx tall, %d lines; at its width it wraps onto more", sizes[1].Y, lines)
	}

	// Without the floor it shrinks with everything else, which is what
	// min-width: 0 asks for.
	label.MinContent = false
	if sizes := lineSizes(t, image.Pt(40*character, 400), true, label, body); sizes[0].X >= 11*character {
		t.Errorf("a label free to shrink was given %dpx, its whole word", sizes[0].X)
	}

	// Its padding is part of what it cannot do without.
	padded := LayoutChild{Node: Padding{Insets: Insets{Left: 3, Right: 4}, Child: monacoText("Temperature")},
		Shrink: 1, MinContent: true}
	if sizes := lineSizes(t, image.Pt(40*character, 400), true, padded, body); sizes[0].X != 11*character+7 {
		t.Errorf("the padded label was given %dpx, want %dpx", sizes[0].X, 11*character+7)
	}

	// Nor does one that starts from nothing stay there, as flex: 0 1 0 would.
	empty := LayoutChild{Node: monacoText("Temperature"), Shrink: 1, MinContent: true, Basis: Pixels(0)}
	if sizes := lineSizes(t, image.Pt(40*character, 400), true, empty); sizes[0].X != 11*character {
		t.Errorf("a label with no basis was given %dpx, want its word's %dpx", sizes[0].X, 11*character)
	}

	// A label that states a width narrower than its word is held there, not
	// at the word.
	label = LayoutChild{Node: monacoText("Temperature"), Shrink: 1, MinContent: true,
		Basis: Pixels(5 * character), Stated: Pixels(5 * character)}
	if sizes := lineSizes(t, image.Pt(40*character, 400), true, label, body); sizes[0].X != 5*character {
		t.Errorf("a label stating %dpx was given %dpx", 5*character, sizes[0].X)
	}

	// flex-basis is only where it starts. One that starts narrower than its
	// word and states no width is still held at the word.
	label.Stated = Length{}
	if sizes := lineSizes(t, image.Pt(40*character, 400), true, label, body); sizes[0].X != 11*character {
		t.Errorf("a label with a basis of %dpx was given %dpx, want its word's %dpx", 5*character, sizes[0].X, 11*character)
	}
}

// When one item stops at its least size, the others share what is left of
// the shrinking from their own base sizes; taking it from where the first
// round left them shrinks them twice.
func TestShrinkingPastAFrozenItemIsTakenOnce(t *testing.T) {
	fixed := func(width int) Node { return Rectangle{Size: image.Pt(width, 10)} }
	sizes := lineSizes(t, image.Pt(300, 10), true,
		LayoutChild{Node: fixed(100), Shrink: 1, MinMain: Pixels(90)},
		LayoutChild{Node: fixed(300), Shrink: 1})
	if sizes[0].X != 90 || sizes[1].X != 210 {
		t.Fatalf("sizes = %d and %d, want 90 and 210", sizes[0].X, sizes[1].X)
	}
}

// A column's items do not shrink below the height of what they hold.
func TestAColumnItemKeepsItsContentHeight(t *testing.T) {
	compiler, err := NewDefaultCompiler()
	if err != nil {
		t.Fatal(err)
	}
	width := monacoWidth(t, compiler, 20)
	text := monacoText(strings.Repeat("word ", 12))
	sizes := lineSizes(t, image.Pt(width, 20), false,
		LayoutChild{Node: text, Shrink: 1, MinContent: true},
		LayoutChild{Node: Rectangle{Size: image.Pt(10, 40)}, Shrink: 1, MinContent: true})
	tall, err := text.measure(&compileContext{compiler: compiler}, image.Pt(width, unboundedMeasure), "text")
	if err != nil {
		t.Fatal(err)
	}
	if sizes[0].Y != tall.Y || sizes[1].Y != 40 {
		t.Fatalf("heights = %d and %d, want %d and 40", sizes[0].Y, sizes[1].Y, tall.Y)
	}
}

// A row too wide for its box is as tall as its children once they are
// shrunk into it, not as tall as their one long line.
func TestARowIsMeasuredAtTheWidthsItWillHave(t *testing.T) {
	compiler, err := NewDefaultCompiler()
	if err != nil {
		t.Fatal(err)
	}
	character := monacoWidth(t, compiler, 1)
	row := Row{Children: []LayoutChild{
		{Node: monacoText("Temperature"), Shrink: 1, MinContent: true},
		{Node: monacoText(paragraph), Shrink: 1, MinContent: true},
	}}
	ctx := &compileContext{compiler: compiler}
	size, err := row.measure(ctx, image.Pt(40*character, unboundedMeasure), "row")
	if err != nil {
		t.Fatal(err)
	}
	line, err := monacoText("x").measure(ctx, image.Pt(unboundedMeasure, unboundedMeasure), "x")
	if err != nil {
		t.Fatal(err)
	}
	if size.Y < 3*line.Y {
		t.Fatalf("the row measured %dpx tall, one line is %dpx; its paragraph wraps onto three or more", size.Y, line.Y)
	}
}

// A wrapping row breaks its lines at the width it states, not at the room
// around it, and is as wide as its longest line when it states none.
func TestAWrappingRowBreaksAtTheWidthItStates(t *testing.T) {
	compiler, err := NewDefaultCompiler()
	if err != nil {
		t.Fatal(err)
	}
	ctx := &compileContext{compiler: compiler}
	item := LayoutChild{Node: Rectangle{Size: image.Pt(40, 10)}}
	for stated, want := range map[int]image.Point{50: image.Pt(50, 30), 0: image.Pt(120, 10)} {
		row := Row{Size: image.Pt(stated, 0), Wrap: Wrapping{Wrap: true}, Children: []LayoutChild{item, item, item}}
		size, err := row.measure(ctx, image.Pt(200, 200), "row")
		if err != nil {
			t.Fatal(err)
		}
		if size != want {
			t.Errorf("a wrapping row %dpx wide measured %v; want %v", stated, size, want)
		}
	}
}

// A margin above or below that is a share of the width counts toward the
// height a column measures, as it does when the column is painted.
func TestAColumnMeasuresAPercentageMarginAboveItsChild(t *testing.T) {
	compiler, err := NewDefaultCompiler()
	if err != nil {
		t.Fatal(err)
	}
	ctx := &compileContext{compiler: compiler}
	column := Column{Children: []LayoutChild{{Node: Rectangle{Size: image.Pt(10, 10)}, Margin: [4]Length{Tenths(100), {}, Tenths(50), {}}}}}
	size, err := column.measure(ctx, image.Pt(60, 200), "column")
	if err != nil {
		t.Fatal(err)
	}
	if size.Y != 19 {
		t.Errorf("a 10px child with 10%% above and 5%% below in a 60px column measured %dpx tall; want 19", size.Y)
	}
	row := Row{Children: column.Children}
	if size, _ := row.measure(ctx, image.Pt(60, 200), "row"); size.Y != 19 {
		t.Errorf("the same child in a 60px row measured %dpx tall; want 19", size.Y)
	}
	// With no width to take a share of, it counts for nothing.
	if size, _ := column.measure(ctx, image.Pt(unboundedMeasure, 200), "column"); size.Y != 10 {
		t.Errorf("with no width the column measured %dpx tall; want 10", size.Y)
	}
}
