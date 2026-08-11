package display_test

import (
	"fmt"
	"image"

	"inkwire/internal/display"
)

func ExampleLayoutText() {
	fonts, _ := display.NewBuiltinFontRegistry()
	frame, _ := display.NewFrame(296, 128, display.InkWhite)
	box := display.TextBox{
		Bounds:     image.Rect(4, 4, 292, 18),
		LineHeight: 14,
		Runs: []display.TextRun{
			{Text: "今天 ", Style: display.TextStyle{Font: "ui", Size: 12}},
			{Text: "23.5℃", Style: display.TextStyle{Font: "ui", Size: 12, Ink: display.InkRed}},
		},
	}
	layout, _ := display.LayoutText(fonts, box)
	layout.Draw(display.NewCanvas(frame))
	payload, _ := display.EncodeGicisky(frame)

	size := layout.Size()
	fmt.Printf("text=%dx%d payload=%d\n", size.X, size.Y, len(payload))
	// Output: text=66x14 payload=9472
}
