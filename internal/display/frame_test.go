package display

import (
	"image"
	"strings"
	"testing"
)

func TestCanvasClipsPrimitives(t *testing.T) {
	frame, err := NewFrame(8, 6, InkWhite)
	if err != nil {
		t.Fatal(err)
	}
	canvas := NewCanvas(frame).Clip(image.Rect(2, 1, 6, 5))
	canvas.FillRect(image.Rect(0, 0, 4, 3), InkRed)
	canvas.DrawLine(image.Pt(0, 0), image.Pt(7, 5), StrokeStyle{Ink: InkBlack, Width: 1})

	assertInk(t, frame, 1, 1, InkWhite)
	assertInk(t, frame, 3, 1, InkRed)
	assertInk(t, frame, 3, 2, InkBlack)
	assertInk(t, frame, 6, 5, InkWhite)
}

func TestFrameRejectsInvalidDimensions(t *testing.T) {
	if _, err := NewFrame(0, 10, InkWhite); err == nil {
		t.Fatal("NewFrame accepted a zero width")
	}
	if _, err := NewFrame(4097, 4096, InkWhite); err == nil || !strings.Contains(err.Error(), "pixel limit") {
		t.Fatalf("oversized frame error = %v", err)
	}
	maxInt := int(^uint(0) >> 1)
	if _, err := NewFrame(maxInt, 2, InkWhite); err == nil || !strings.Contains(err.Error(), "pixel limit") {
		t.Fatalf("overflowing frame error = %v", err)
	}
}
