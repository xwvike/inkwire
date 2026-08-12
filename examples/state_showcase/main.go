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
	pngPath := flag.String("png", "state_showcase.png", "PNG preview output path")
	payloadPath := flag.String("payload", "", "optional Gicisky payload output path")
	flag.Parse()

	frame, err := renderStateShowcase()
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

func renderStateShowcase() (*display.Frame, error) {
	black := display.StrokeStyle{Ink: display.InkBlack, Width: 1}
	red := display.StrokeStyle{Ink: display.InkRed, Width: 1}
	compiler, err := compose.NewDefaultCompiler()
	if err != nil {
		return nil, err
	}
	compiled, report, err := compiler.Compile(compose.Document{
		Orientation: display.OrientationLandscape,
		Background:  compose.Value(display.InkWhite),
		Root: compose.Absolute{Size: image.Pt(296, 128), Clip: true, Children: []compose.Placed{
			{Bounds: image.Rect(0, 0, 296, 21), Node: compose.Rectangle{Size: image.Pt(296, 21), Fill: compose.Ink(display.InkBlack)}},
			{Bounds: image.Rect(6, 2, 213, 20), Node: compose.Text{Size: image.Pt(207, 18), Runs: []display.TextRun{run("DISPLAY LIST / ", "monaco", 14, display.InkWhite), run("状态栈", "hzk", 14, display.InkWhite)}}},
			{Bounds: image.Rect(220, 3, 291, 19), Node: compose.Text{Size: image.Pt(71, 16), Align: display.AlignEnd, Runs: []display.TextRun{run("COMPOSE", "monaco", 10, display.InkRed)}}},
			{Bounds: image.Rect(5, 23, 205, 36), Node: compose.Text{Size: image.Pt(200, 13), Runs: []display.TextRun{run("ABSOLUTE > STACK > CLIP", "monaco", 10, display.InkBlack)}}},
			{Bounds: image.Rect(3, 38, 98, 105), Node: stateClipPanel(black, red)},
			{Bounds: image.Rect(101, 38, 198, 105), Node: stateLayoutPanel(black)},
			{Bounds: image.Rect(201, 38, 293, 105), Node: stateStackPanel(black, red)},
			{Bounds: image.Rect(0, 108, 296, 128), Node: compose.Rectangle{Size: image.Pt(296, 20), Fill: compose.Ink(display.InkBlack)}},
			{Bounds: image.Rect(5, 110, 291, 126), Node: compose.Text{Size: image.Pt(286, 16), Align: display.AlignCenter, Runs: []display.TextRun{run("ABSOLUTE", "monaco", 12, display.InkWhite), run(" > CLIP > ", "monaco", 12, display.InkRed), run("ROW / COLUMN", "monaco", 12, display.InkWhite), run(" > STACK", "monaco", 12, display.InkRed)}}},
		}},
	})
	if err != nil {
		return nil, err
	}
	if len(report.MissingRunes) != 0 || len(report.Warnings) != 0 {
		return nil, fmt.Errorf("state compose report: missing=%q warnings=%v", string(report.MissingRunes), report.Warnings)
	}
	return compiled.Render()
}

func stateClipPanel(black, red display.StrokeStyle) compose.Node {
	children := []compose.Placed{{Bounds: image.Rect(0, 0, 95, 67), Node: compose.Rectangle{Size: image.Pt(95, 67), Radius: 4, Stroke: compose.Stroke(black)}}, {Bounds: image.Rect(4, 2, 91, 14), Node: compose.Rectangle{Size: image.Pt(87, 12), Fill: compose.Ink(display.InkBlack)}}, {Bounds: image.Rect(4, 2, 91, 14), Node: compose.Text{Size: image.Pt(87, 12), Align: display.AlignCenter, Runs: []display.TextRun{run("CLIP RECT", "monaco", 10, display.InkWhite)}}}}
	for i, x := 0, -32; x < 116; i, x = i+1, x+8 {
		ink := display.InkBlack
		if i%3 == 0 {
			ink = display.InkRed
		}
		children = append(children, compose.Placed{Bounds: image.Rect(0, 0, 95, 67), Node: compose.Line{Size: image.Pt(95, 67), From: image.Pt(x, 66), To: image.Pt(x+52, 13), Stroke: display.StrokeStyle{Ink: ink, Width: 2}}})
	}
	children = append(children, compose.Placed{Bounds: image.Rect(6, 20, 89, 61), Node: compose.ClipRect{Size: image.Pt(83, 41), Rect: image.Rect(0, 0, 83, 41), Child: compose.Rectangle{Size: image.Pt(83, 41), Fill: compose.Ink(display.InkWhite)}}})
	children = append(children, compose.Placed{Bounds: image.Rect(6, 50, 89, 61), Node: compose.Rectangle{Size: image.Pt(83, 11), Fill: compose.Ink(display.InkBlack)}})
	children = append(children, compose.Placed{Bounds: image.Rect(9, 50, 86, 61), Node: compose.Text{Size: image.Pt(77, 11), Align: display.AlignCenter, Runs: []display.TextRun{run("OUTSIDE CUT", "monaco", 10, display.InkWhite)}}})
	return compose.Absolute{Size: image.Pt(95, 67), Clip: true, Children: children}
}

func stateLayoutPanel(black display.StrokeStyle) compose.Node {
	children := []compose.Node{}
	for _, ink := range []display.Ink{display.InkBlack, display.InkRed, display.InkBlack} {
		children = append(children, compose.Rectangle{Size: image.Pt(35, 20), Radius: 3, Stroke: compose.Stroke(display.StrokeStyle{Ink: ink, Width: 1})})
	}
	return compose.Absolute{Size: image.Pt(97, 67), Clip: true, Children: []compose.Placed{
		{Bounds: image.Rect(0, 0, 97, 67), Node: compose.Rectangle{Size: image.Pt(97, 67), Radius: 4, Stroke: compose.Stroke(black)}},
		{Bounds: image.Rect(4, 2, 93, 14), Node: compose.Rectangle{Size: image.Pt(89, 12), Fill: compose.Ink(display.InkBlack)}},
		{Bounds: image.Rect(4, 2, 93, 14), Node: compose.Text{Size: image.Pt(89, 12), Align: display.AlignCenter, Runs: []display.TextRun{run("ROW / COLUMN", "monaco", 10, display.InkWhite)}}},
		{Bounds: image.Rect(8, 22, 89, 44), Node: compose.Row{Size: image.Pt(81, 22), Gap: 5, CrossAlign: compose.CrossCenter, Children: []compose.LayoutChild{{Node: children[0], Basis: 22}, {Node: children[1], Basis: 22}, {Node: children[2], Basis: 22}}}},
		{Bounds: image.Rect(8, 49, 89, 62), Node: compose.Text{Size: image.Pt(81, 13), Align: display.AlignCenter, Runs: []display.TextRun{run("GAP 7px", "monaco", 10, display.InkBlack)}}},
	}}
}

func stateStackPanel(black, red display.StrokeStyle) compose.Node {
	return compose.Absolute{Size: image.Pt(92, 67), Clip: true, Children: []compose.Placed{
		{Bounds: image.Rect(0, 0, 92, 67), Node: compose.Rectangle{Size: image.Pt(92, 67), Radius: 4, Stroke: compose.Stroke(black)}},
		{Bounds: image.Rect(4, 2, 88, 14), Node: compose.Rectangle{Size: image.Pt(84, 12), Fill: compose.Ink(display.InkBlack)}},
		{Bounds: image.Rect(4, 2, 88, 14), Node: compose.Text{Size: image.Pt(84, 12), Align: display.AlignCenter, Runs: []display.TextRun{run("STACK ORDER", "monaco", 10, display.InkWhite)}}},
		{Bounds: image.Rect(8, 20, 29, 35), Node: compose.Rectangle{Size: image.Pt(21, 15), Radius: 2, Stroke: compose.Stroke(black)}},
		{Bounds: image.Rect(22, 27, 43, 42), Node: compose.Rectangle{Size: image.Pt(21, 15), Radius: 2, Fill: compose.Ink(display.InkRed)}},
		{Bounds: image.Rect(36, 34, 57, 49), Node: compose.Rectangle{Size: image.Pt(21, 15), Radius: 2, Stroke: compose.Stroke(black)}},
		{Bounds: image.Rect(61, 21, 84, 48), Node: compose.Polygon{Size: image.Pt(23, 27), Points: []image.Point{{X: 11, Y: 0}, {X: 22, Y: 13}, {X: 11, Y: 26}, {X: 0, Y: 13}}, Fill: compose.Ink(display.InkBlack)}},
		{Bounds: image.Rect(8, 53, 84, 64), Node: compose.Text{Size: image.Pt(76, 11), Align: display.AlignCenter, Runs: []display.TextRun{run("LATER OVER EARLIER", "monaco", 10, display.InkBlack)}}},
	}}
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
