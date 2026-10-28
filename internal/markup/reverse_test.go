package markup

import (
	"image"
	"testing"

	"github.com/xwvike/inkwire/internal/display"
)

func TestAReversedLineRunsFromItsFarEnd(t *testing.T) {
	body := `<i class="a"></i><i class="b"></i><i class="c"></i>`
	const items = `.a { width: 10px; height: 10px; background: black; }
		.b { width: 20px; height: 10px; background: red; }
		.c { width: 30px; height: 10px; background: yellow; } `
	tests := []struct {
		name, css          string
		black, red, yellow image.Rectangle
	}{
		{"row-reverse packs at the right", `.page { flex-direction: row-reverse; align-items: flex-start; }`,
			image.Rect(90, 0, 100, 10), image.Rect(70, 0, 90, 10), image.Rect(40, 0, 70, 10)},
		{"flex-end packs at the left", `.page { flex-direction: row-reverse; align-items: flex-start; justify-content: flex-end; }`,
			image.Rect(50, 0, 60, 10), image.Rect(30, 0, 50, 10), image.Rect(0, 0, 30, 10)},
		{"start is the line's own start", `.page { flex-direction: row-reverse; align-items: flex-start; justify-content: start; }`,
			image.Rect(50, 0, 60, 10), image.Rect(30, 0, 50, 10), image.Rect(0, 0, 30, 10)},
		{"a gap and a centre", `.page { flex-direction: row-reverse; align-items: flex-start; justify-content: center; gap: 5px; }`,
			image.Rect(75, 0, 85, 10), image.Rect(50, 0, 70, 10), image.Rect(15, 0, 45, 10)},
		{"column-reverse packs at the bottom", `.page { flex-direction: column-reverse; align-items: flex-start; }`,
			image.Rect(0, 40, 10, 50), image.Rect(0, 30, 20, 40), image.Rect(0, 20, 30, 30)},
		{"a margin stays on its side", `.page { flex-direction: row-reverse; align-items: flex-start; } .a { margin-right: 4px; }`,
			image.Rect(86, 0, 96, 10), image.Rect(66, 0, 86, 10), image.Rect(36, 0, 66, 10)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := boxes(t, body, items+test.css)
			expect(t, got, display.InkBlack, test.black, "the first item")
			expect(t, got, display.InkRed, test.red, "the second item")
			expect(t, got, display.InkYellow, test.yellow, "the third item")
		})
	}
}
