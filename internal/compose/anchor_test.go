package compose

import (
	"image"
	"testing"

	"github.com/xwvike/inkwire/internal/display"
)

func TestAnchoredAutoSizeUsesMeasuredContent(t *testing.T) {
	tests := []struct {
		name   string
		anchor Anchor
		want   image.Rectangle
	}{
		{
			name: "left and top",
			anchor: Anchor{
				Left: Pixels(5), Top: Pixels(6),
				Node: Rectangle{Size: image.Pt(20, 10), Fill: Ink(display.InkRed)},
			},
			want: image.Rect(5, 6, 25, 16),
		},
		{
			name: "right and bottom",
			anchor: Anchor{
				Right: Pixels(7), Bottom: Pixels(8),
				Node: Rectangle{Size: image.Pt(20, 10), Fill: Ink(display.InkRed)},
			},
			want: image.Rect(73, 32, 93, 42),
		},
		{
			name: "no insets",
			anchor: Anchor{
				Node: Rectangle{Size: image.Pt(20, 10), Fill: Ink(display.InkRed)},
			},
			want: image.Rect(0, 0, 20, 10),
		},
		{
			name: "opposite edges stretch",
			anchor: Anchor{
				Left: Pixels(5), Right: Pixels(7), Top: Pixels(6),
				Node: Rectangle{Size: image.Pt(20, 10), Fill: Ink(display.InkRed)},
			},
			want: image.Rect(5, 6, 93, 16),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			frame, _ := compileAndRender(t, Document{
				Size:       image.Pt(100, 50),
				Background: Ink(display.InkWhite),
				Root: Anchored{Children: []Anchor{
					test.anchor,
				}},
			})
			if got := boundsOfInk(frame, display.InkRed); got != test.want {
				t.Fatalf("red bounds = %v, want %v", got, test.want)
			}
		})
	}
}

func TestAnchoredAutoSizeInsetPercentagesUseContainingBlock(t *testing.T) {
	anchor := Anchor{
		Left: Tenths(100), Right: Tenths(100),
		Node: Rectangle{Size: image.Pt(20, 10), Fill: Ink(display.InkRed)},
	}
	if got := anchor.maximum(image.Pt(100, 50)); got != image.Pt(80, 50) {
		t.Fatalf("available size = %v, want (80,50)", got)
	}
}
