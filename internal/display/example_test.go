package display_test

import (
	"fmt"
	"image"

	"github.com/xwvike/inkwire/internal/display"
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

	size := layout.Size()
	fmt.Printf("text=%dx%d\n", size.X, size.Y)
	// Output: text=66x14
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

	fmt.Printf("path=%v\n", path.Bounds())
	// Output: path=(10,10)-(41,31)
}

func ExampleDisplayList() {
	frame, _ := display.NewFrame(32, 16, display.InkWhite)
	list := &display.DisplayList{}
	list.Save()
	list.ClipRect(image.Rect(2, 2, 18, 14))
	list.Translate(image.Pt(4, 3))
	list.FillRoundRect(image.Rect(0, 0, 12, 8), 2, display.InkBlack)
	list.StrokeCircle(image.Pt(16, 4), 4, display.StrokeStyle{Ink: display.InkRed, Width: 2})
	list.Restore()
	_ = list.Replay(display.NewCanvas(frame))

	fmt.Printf("commands=%d bounds=%v\n", list.Len(), list.Bounds())
	// Output: commands=6 bounds=(4,3)-(18,12)
}
