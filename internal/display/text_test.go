package display

import (
	"image"
	"testing"
)

func TestTextLayoutDrawsMixedColorsAndReportsMissingGlyphs(t *testing.T) {
	registry := builtinRegistry(t)
	frame, err := NewFrame(96, 20, InkWhite)
	if err != nil {
		t.Fatal(err)
	}
	box := TextBox{
		Bounds: image.Rect(0, 0, 96, 14),
		Runs: []TextRun{
			{Text: "中A", Style: TextStyle{Font: DefaultFont, Ink: InkBlack}},
			{Text: "23", Style: TextStyle{Font: DefaultFont, Ink: InkRed}},
			{Text: "😀", Style: TextStyle{Font: DefaultFont, Ink: InkBlack}},
		},
		LineHeight: 14,
	}
	layout, err := NewCanvas(frame).DrawTextBox(registry, box)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := layout.LineCount(), 1; got != want {
		t.Fatalf("line count = %d, want %d", got, want)
	}
	if got, want := layout.MissingRunes(), []rune{'😀'}; len(got) != len(want) || got[0] != want[0] {
		t.Fatalf("missing runes = %q, want %q", string(got), string(want))
	}
	black, red := countInk(frame, InkBlack), countInk(frame, InkRed)
	if black == 0 || red == 0 {
		t.Fatalf("rendered black=%d red=%d pixels", black, red)
	}
}

func TestTextLayoutWrapsAtRuneBoundaries(t *testing.T) {
	registry := builtinRegistry(t)
	layout, err := LayoutText(registry, TextBox{
		Bounds: image.Rect(0, 0, 24, 64),
		Runs: []TextRun{
			{Text: "中文测试", Style: TextStyle{Font: DefaultFont}},
		},
		Wrap:       WrapRunes,
		LineHeight: 14,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := layout.LineCount(), 2; got != want {
		t.Fatalf("line count = %d, want %d", got, want)
	}
	if got, want := layout.Size(), image.Pt(24, 28); got != want {
		t.Fatalf("layout size = %v, want %v", got, want)
	}
}

func TestTextStyleSelectsFamilyByPixelSize(t *testing.T) {
	registry := builtinRegistry(t)
	layout, err := LayoutText(registry, TextBox{
		Bounds: image.Rect(0, 0, 80, 24),
		Runs: []TextRun{
			{Text: "中文A1", Style: TextStyle{Font: "ui", Size: 14}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if layout.Size().X != 42 {
		t.Fatalf("layout width = %d, want 42 (14+14+7+7)", layout.Size().X)
	}
	if _, err := LayoutText(registry, TextBox{
		Bounds: image.Rect(0, 0, 80, 24),
		Runs:   []TextRun{{Text: "test", Style: TextStyle{Font: "ui", Size: 15}}},
	}); err == nil {
		t.Fatal("layout accepted unavailable ui 15px")
	}
}

func countInk(frame *Frame, target Ink) int {
	count := 0
	for y := 0; y < frame.Height(); y++ {
		for x := 0; x < frame.Width(); x++ {
			ink, _ := frame.InkAt(x, y)
			if ink == target {
				count++
			}
		}
	}
	return count
}
