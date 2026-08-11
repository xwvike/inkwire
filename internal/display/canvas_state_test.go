package display

import (
	"image"
	"image/color"
	"testing"
)

func TestCanvasSaveRestoreTranslationAndClip(t *testing.T) {
	frame := newTestFrame(t, 12, 9)
	canvas := NewCanvas(frame)

	canvas.Save()
	canvas.Translate(image.Pt(3, 2))
	canvas.ClipRect(image.Rect(0, 0, 4, 3))
	canvas.FillRect(image.Rect(-2, -2, 8, 8), InkBlack)
	if !canvas.Restore() {
		t.Fatal("Restore did not pop the saved state")
	}
	canvas.FillRect(image.Rect(0, 0, 1, 1), InkRed)

	assertInk(t, frame, 0, 0, InkRed)
	assertInk(t, frame, 3, 2, InkBlack)
	assertInk(t, frame, 6, 4, InkBlack)
	assertInk(t, frame, 7, 4, InkWhite)
	assertInk(t, frame, 3, 5, InkWhite)
	if canvas.Restore() {
		t.Fatal("Restore popped past the initial state")
	}
}

func TestCanvasNestedClipUsesCurrentCoordinates(t *testing.T) {
	frame := newTestFrame(t, 12, 8)
	canvas := NewCanvas(frame)
	canvas.ClipRect(image.Rect(1, 1, 10, 7))
	canvas.Translate(image.Pt(3, 2))
	canvas.ClipRect(image.Rect(0, 0, 4, 3))
	canvas.FillRect(image.Rect(-10, -10, 10, 10), InkRed)

	assertInk(t, frame, 3, 2, InkRed)
	assertInk(t, frame, 6, 4, InkRed)
	assertInk(t, frame, 2, 2, InkWhite)
	assertInk(t, frame, 7, 4, InkWhite)
}

func TestChildCanvasStateDoesNotPolluteParent(t *testing.T) {
	frame := newTestFrame(t, 12, 8)
	parent := NewCanvas(frame)
	parent.Translate(image.Pt(2, 1))
	child := parent.Clip(image.Rect(0, 0, 3, 3))
	child.Translate(image.Pt(4, 0))
	child.ClipRect(image.Rect(0, 0, 1, 1))
	child.FillRect(image.Rect(0, 0, 4, 4), InkBlack)

	parent.FillRect(image.Rect(0, 0, 2, 2), InkRed)
	assertInk(t, frame, 2, 1, InkRed)
	assertInk(t, frame, 3, 2, InkRed)
	assertInk(t, frame, 6, 1, InkWhite)
}

func TestCanvasTranslationMovesShapesTextAndImagesTogether(t *testing.T) {
	const width, height = 48, 28
	offset := image.Pt(7, 5)
	base := newTestFrame(t, width, height)
	translated := newTestFrame(t, width, height)
	registry := builtinRegistry(t)
	source := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	source.SetNRGBA(0, 0, color.NRGBA{R: 0xff, A: 0xff})
	source.SetNRGBA(1, 0, color.NRGBA{A: 0xff})
	source.SetNRGBA(0, 1, color.NRGBA{A: 0xff})
	source.SetNRGBA(1, 1, color.NRGBA{R: 0xff, A: 0xff})

	drawContent := func(canvas *Canvas) {
		canvas.FillCircle(image.Pt(3, 3), 2, InkBlack)
		canvas.DrawLine(image.Pt(8, 1), image.Pt(13, 4), StrokeStyle{Ink: InkRed, Width: 2})
		if _, err := canvas.DrawTextBox(registry, TextBox{
			Bounds: image.Rect(1, 7, 24, 21),
			Runs:   []TextRun{{Text: "A中", Style: TextStyle{Font: "ui", Size: 12, Ink: InkBlack}}},
		}); err != nil {
			t.Fatal(err)
		}
		if err := canvas.DrawImage(source, image.Rect(25, 2, 29, 6), ImageOptions{}); err != nil {
			t.Fatal(err)
		}
	}

	drawContent(NewCanvas(base))
	shifted := NewCanvas(translated)
	shifted.Translate(offset)
	drawContent(shifted)

	for y := 0; y < height-offset.Y; y++ {
		for x := 0; x < width-offset.X; x++ {
			want, _ := base.InkAt(x, y)
			got, _ := translated.InkAt(x+offset.X, y+offset.Y)
			if got != want {
				t.Fatalf("translated pixel (%d,%d) = %d, want %d from (%d,%d)", x+offset.X, y+offset.Y, got, want, x, y)
			}
		}
	}
}
