package main

import (
	"flag"
	"fmt"
	"image"
	"os"

	"github.com/xwvike/inkwire/internal/compose"
	"github.com/xwvike/inkwire/internal/display"
)

func main() {
	pngPath := flag.String("png", "showcase.png", "PNG preview output path")
	payloadPath := flag.String("payload", "", "optional Gicisky payload output path")
	flag.Parse()

	frame, err := renderShowcase()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := writePNG(*pngPath, frame); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if *payloadPath != "" {
		payload, err := display.EncodeGicisky(frame)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		if err := os.WriteFile(*payloadPath, payload, 0o644); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
}

func renderShowcase() (*display.Frame, error) {
	compiler, err := compose.NewDefaultCompiler()
	if err != nil {
		return nil, err
	}
	compiled, _, err := compiler.Compile(compose.Document{
		Orientation: display.OrientationLandscape,
		Background:  compose.Value(display.InkWhite),
		Root: compose.Absolute{Size: image.Pt(296, 128), Clip: true, Children: []compose.Placed{
			{Bounds: image.Rect(1, 1, 295, 127), Node: compose.Rectangle{Size: image.Pt(294, 126), Radius: 6, Stroke: compose.Stroke(stroke(display.InkBlack, 1))}},
			{Bounds: image.Rect(3, 3, 293, 26), Node: compose.Rectangle{Size: image.Pt(290, 23), Radius: 5, Fill: compose.Ink(display.InkBlack)}},
			{Bounds: image.Rect(8, 5, 288, 24), Node: compose.Text{Size: image.Pt(280, 19), Runs: []display.TextRun{
				run("INKWIRE  ", "monaco", 14, display.InkWhite), run("图元与文字展示", "ui", 14, display.InkWhite), run("  RBW", "monaco", 14, display.InkRed),
			}}},
			{Bounds: image.Rect(4, 29, 99, 124), Node: basicShapes()},
			{Bounds: image.Rect(101, 29, 198, 124), Node: pathsAndArcs()},
			{Bounds: image.Rect(200, 29, 292, 124), Node: typeAndColor()},
		}},
	})
	if err != nil {
		return nil, err
	}
	return compiled.Render()
}

func basicShapes() compose.Node {
	return compose.Absolute{Size: image.Pt(95, 95), Clip: true, Children: []compose.Placed{
		{Bounds: image.Rect(0, 0, 95, 95), Node: compose.Rectangle{Size: image.Pt(95, 95), Radius: 5, Stroke: compose.Stroke(stroke(display.InkBlack, 1))}},
		{Bounds: image.Rect(5, 3, 90, 15), Node: label("BASIC / SHAPES")},
		{Bounds: image.Rect(5, 18, 40, 20), Node: compose.Line{Size: image.Pt(35, 2), From: image.Pt(0, 1), To: image.Pt(35, 1), Stroke: stroke(display.InkBlack, 1)}},
		{Bounds: image.Rect(48, 18, 90, 22), Node: compose.Line{Size: image.Pt(42, 4), From: image.Pt(0, 2), To: image.Pt(42, 2), Stroke: display.StrokeStyle{Ink: display.InkRed, Width: 2, Dash: []int{3, 2}, DashOffset: 1}}},
		{Bounds: image.Rect(5, 25, 30, 45), Node: compose.Rectangle{Size: image.Pt(25, 20), Stroke: compose.Stroke(stroke(display.InkBlack, 1))}},
		{Bounds: image.Rect(35, 25, 58, 45), Node: compose.Rectangle{Size: image.Pt(23, 20), Fill: compose.Ink(display.InkRed)}},
		{Bounds: image.Rect(63, 25, 90, 45), Node: compose.Rectangle{Size: image.Pt(27, 20), Radius: 4, Stroke: compose.Stroke(stroke(display.InkBlack, 2))}},
		{Bounds: image.Rect(5, 49, 23, 67), Node: compose.Circle{Center: image.Pt(9, 9), Radius: 7, Stroke: compose.Stroke(stroke(display.InkBlack, 1))}},
		{Bounds: image.Rect(28, 49, 44, 67), Node: compose.Circle{Center: image.Pt(8, 9), Radius: 7, Fill: compose.Ink(display.InkRed)}},
		{Bounds: image.Rect(49, 47, 69, 68), Node: compose.Ellipse{Size: image.Pt(20, 21), Stroke: compose.Stroke(stroke(display.InkRed, 1))}},
		{Bounds: image.Rect(74, 47, 94, 68), Node: compose.Ellipse{Size: image.Pt(20, 21), Fill: compose.Ink(display.InkBlack)}},
		{Bounds: image.Rect(5, 72, 45, 88), Node: compose.Polyline{Size: image.Pt(40, 16), Points: []image.Point{{X: 0, Y: 15}, {X: 9, Y: 6}, {X: 18, Y: 15}, {X: 27, Y: 6}, {X: 36, Y: 15}}, Stroke: stroke(display.InkRed, 1)}},
		{Bounds: image.Rect(50, 65, 78, 95), Node: compose.Polygon{Size: image.Pt(28, 30), Points: []image.Point{{X: 2, Y: 29}, {X: 8, Y: 9}, {X: 18, Y: 5}, {X: 27, Y: 17}, {X: 20, Y: 29}}, Stroke: compose.Stroke(stroke(display.InkBlack, 1))}},
		{Bounds: image.Rect(75, 65, 95, 95), Node: compose.Polygon{Size: image.Pt(20, 30), Points: []image.Point{{X: 2, Y: 29}, {X: 11, Y: 7}, {X: 19, Y: 29}}, Fill: compose.Ink(display.InkRed)}},
	}}
}

func pathsAndArcs() compose.Node {
	var wave display.Path
	wave.MoveTo(image.Pt(6, 23))
	wave.CubicTo(image.Pt(23, 6), image.Pt(42, 38), image.Pt(58, 22))
	wave.QuadraticTo(image.Pt(75, 9), image.Pt(91, 23))
	var landscape display.Path
	landscape.MoveTo(image.Pt(5, 89))
	landscape.LineTo(image.Pt(15, 69))
	landscape.LineTo(image.Pt(26, 81))
	landscape.QuadraticTo(image.Pt(38, 59), image.Pt(50, 81))
	landscape.CubicTo(image.Pt(61, 73), image.Pt(72, 62), image.Pt(82, 89))
	landscape.Close()
	return compose.Absolute{Size: image.Pt(97, 95), Clip: true, Children: []compose.Placed{
		{Bounds: image.Rect(0, 0, 97, 95), Node: compose.Rectangle{Size: image.Pt(97, 95), Radius: 5, Stroke: compose.Stroke(stroke(display.InkBlack, 1))}},
		{Bounds: image.Rect(5, 3, 92, 15), Node: label("PATH / ARC")},
		{Bounds: image.Rect(4, 18, 94, 45), Node: compose.Path{Size: image.Pt(90, 27), Path: wave, Stroke: compose.Stroke(stroke(display.InkBlack, 1))}},
		{Bounds: image.Rect(5, 46, 37, 76), Node: compose.Arc{Size: image.Pt(32, 30), Start: 140, Sweep: 260, Stroke: display.StrokeStyle{Ink: display.InkRed, Width: 2, Dash: []int{4, 2}}}},
		{Bounds: image.Rect(41, 48, 66, 73), Node: compose.Pie{Size: image.Pt(25, 25), Start: -90, Sweep: 125, Ink: display.InkRed}},
		{Bounds: image.Rect(70, 47, 94, 73), Node: compose.Chord{Size: image.Pt(24, 26), Start: 20, Sweep: 210, Ink: display.InkBlack}},
		{Bounds: image.Rect(4, 66, 82, 95), Node: compose.Path{Size: image.Pt(78, 29), Path: landscape, Fill: compose.Ink(display.InkBlack)}},
		{Bounds: image.Rect(77, 67, 97, 95), Node: compose.Circle{Center: image.Pt(10, 14), Radius: 10, Stroke: compose.Stroke(stroke(display.InkRed, 3))}},
	}}
}

func typeAndColor() compose.Node {
	return compose.Absolute{Size: image.Pt(92, 95), Clip: true, Children: []compose.Placed{
		{Bounds: image.Rect(0, 0, 92, 95), Node: compose.Rectangle{Size: image.Pt(92, 95), Radius: 5, Stroke: compose.Stroke(stroke(display.InkBlack, 1))}},
		{Bounds: image.Rect(5, 3, 87, 15), Node: label("TYPE / COLOR")},
		{Bounds: image.Rect(5, 15, 87, 34), Node: compose.Text{Size: image.Pt(82, 19), Runs: []display.TextRun{run("中文", "hzk", 16, display.InkBlack), run("16", "monaco", 16, display.InkRed)}}},
		{Bounds: image.Rect(5, 34, 87, 50), Node: compose.Text{Size: image.Pt(82, 16), Runs: []display.TextRun{run("图元", "hzk", 14, display.InkBlack), run("14", "monaco", 14, display.InkRed)}}},
		{Bounds: image.Rect(5, 50, 88, 65), Node: compose.Text{Size: image.Pt(83, 15), Runs: []display.TextRun{run("中文12 ", "ui", 12, display.InkBlack), run("ABC", "monaco", 12, display.InkRed)}}},
		{Bounds: image.Rect(5, 66, 87, 80), Node: compose.Text{Size: image.Pt(82, 14), Align: display.AlignCenter, Runs: []display.TextRun{run("MONACO10 23.5", "monaco", 10, display.InkBlack)}}},
		{Bounds: image.Rect(4, 81, 88, 93), Node: compose.Rectangle{Size: image.Pt(84, 12), Radius: 3, Fill: compose.Ink(display.InkBlack)}},
		{Bounds: image.Rect(7, 81, 85, 93), Node: compose.Text{Size: image.Pt(78, 12), Align: display.AlignCenter, Runs: []display.TextRun{run("WHITE", "monaco", 10, display.InkWhite), run(" / RED", "monaco", 10, display.InkRed)}}},
	}}
}

func label(value string) compose.Node {
	return compose.Text{Runs: []display.TextRun{run(value, "monaco", 10, display.InkBlack)}}
}

func stroke(ink display.Ink, width int) display.StrokeStyle {
	return display.StrokeStyle{Ink: ink, Width: width}
}

func run(text, font string, size int, ink display.Ink) display.TextRun {
	return display.TextRun{Text: text, Style: display.TextStyle{Font: font, Size: size, Ink: ink}}
}

func writePNG(path string, frame *display.Frame) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	if err := display.WritePNG(file, frame); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}
