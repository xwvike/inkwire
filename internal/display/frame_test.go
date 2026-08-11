package display

import (
	"image"
	"testing"
)

func TestCanvasClipsPrimitives(t *testing.T) {
	frame, err := NewFrame(8, 6, InkWhite)
	if err != nil {
		t.Fatal(err)
	}
	canvas := NewCanvas(frame).Clip(image.Rect(2, 1, 6, 5))
	canvas.FillRect(image.Rect(0, 0, 4, 3), InkRed)
	canvas.DrawLine(image.Pt(0, 0), image.Pt(7, 5), InkBlack)

	assertInk(t, frame, 1, 1, InkWhite)
	assertInk(t, frame, 3, 1, InkRed)
	assertInk(t, frame, 3, 2, InkBlack)
	assertInk(t, frame, 6, 5, InkWhite)
}

func TestFrameRejectsInvalidDimensions(t *testing.T) {
	if _, err := NewFrame(0, 10, InkWhite); err == nil {
		t.Fatal("NewFrame accepted a zero width")
	}
}

func assertInk(t *testing.T, frame *Frame, x, y int, want Ink) {
	t.Helper()
	got, ok := frame.InkAt(x, y)
	if !ok {
		t.Fatalf("pixel (%d,%d) is outside frame", x, y)
	}
	if got != want {
		t.Fatalf("pixel (%d,%d) = %d, want %d", x, y, got, want)
	}
}
