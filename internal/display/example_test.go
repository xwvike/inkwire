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

func ExamplePath() {
	frame, _ := display.NewFrame(296, 128, display.InkWhite)
	canvas := display.NewCanvas(frame)

	var path display.Path
	path.MoveTo(image.Pt(10, 10))
	path.LineTo(image.Pt(40, 10))
	path.LineTo(image.Pt(40, 30))
	path.LineTo(image.Pt(10, 30))
	path.Close()
	canvas.FillPath(path, display.InkBlack)
	canvas.DrawArc(image.Rect(50, 10, 81, 41), 180, 270, display.StrokeStyle{
		Ink: display.InkRed, Width: 2, Dash: []int{4, 2},
	})

	payload, _ := display.EncodeGicisky(frame)
	fmt.Printf("path=%v payload=%d\n", path.Bounds(), len(payload))
	// Output: path=(10,10)-(41,31) payload=9472
}
