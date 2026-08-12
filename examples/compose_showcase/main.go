package main

import (
	"bytes"
	_ "embed"
	"flag"
	"fmt"
	"image"
	"image/png"
	"os"

	"github.com/xwvike/inkwire/internal/compose"
	"github.com/xwvike/inkwire/internal/display"
)

// portraitPNG is deliberately kept beside this example so the page can be
// rebuilt without depending on a path outside the package.
//
//go:embed portrait.png
var portraitPNG []byte

func main() {
	pngPath := flag.String("png", "compose_showcase.png", "PNG preview output path")
	payloadPath := flag.String("payload", "", "optional Gicisky payload output path")
	flag.Parse()

	frame, report, err := renderComposeShowcase()
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

	fmt.Printf("compose showcase: %v, drawn=%v\n", report.Bounds, frame.Bounds())
	if len(report.MissingRunes) > 0 {
		fmt.Printf("missing runes: %q\n", string(report.MissingRunes))
	}
	for _, warning := range report.Warnings {
		fmt.Printf("warning %s: %s\n", warning.Path, warning.Message)
	}
	for _, decision := range report.Images {
		fmt.Printf("image %s: dither=%d fit=%d sampling=%d threshold=%d red-disabled=%v\n",
			decision.Path, decision.Options.Dither, decision.Options.Fit, decision.Options.Sampling,
			decision.Options.Threshold, decision.Options.DisableRed)
	}
}

func renderComposeShowcase() (*display.Frame, compose.Report, error) {
	photo, err := png.Decode(bytes.NewReader(portraitPNG))
	if err != nil {
		return nil, compose.Report{}, fmt.Errorf("decode embedded portrait: %w", err)
	}
	document, err := buildDocument(photo)
	if err != nil {
		return nil, compose.Report{}, err
	}
	return compose.Render(document)
}

func buildDocument(photo image.Image) (compose.Document, error) {
	grid, err := display.NewPattern([]string{
		"x...", "....", "....", "....",
	}, map[rune]display.Ink{'x': display.InkBlack})
	if err != nil {
		return compose.Document{}, err
	}
	blueprint, err := display.NewPattern([]string{
		"x...", "..x.", "....", ".x..",
	}, map[rune]display.Ink{'x': display.InkRed})
	if err != nil {
		return compose.Document{}, err
	}

	fit := display.FitCover
	sampling := display.SampleBilinear
	dither := display.DitherFloydSteinberg
	disableRed := true

	return compose.Document{
		Orientation: display.OrientationLandscape,
		Background:  compose.Value(display.InkWhite),
		Root: compose.Absolute{
			Size: composeImagePoint(296, 128),
			Clip: true,
			Children: []compose.Placed{
				compose.PlaceAt(0, 0, 296, 20, compose.Rectangle{
					Radius: 4,
					Fill:   compose.Ink(display.InkBlack),
				}),
				compose.PlaceAt(7, 2, 282, 16, compose.Text{Runs: []display.TextRun{
					compose.Run("INKWIRE ", "monaco", 12, display.InkWhite),
					compose.Run("COMPOSE", "monaco", 12, display.InkRed),
					compose.Run(" / 296x128", "monaco", 12, display.InkWhite),
				}}),
				compose.Place(image.Rect(4, 24, 78, 126), leftPanel(photo, fit, sampling, dither, disableRed)),
				compose.Place(image.Rect(82, 24, 206, 126), centrePanel(grid, blueprint)),
				compose.Place(image.Rect(210, 24, 292, 126), rightPanel()),
			},
		},
	}, nil
}

func leftPanel(photo image.Image, fit display.ImageFit, sampling display.SamplingMode, dither display.DitherMode, disableRed bool) compose.Node {
	var clip display.Path
	// ClipPath coordinates are local to the node bounds.
	clip.Arc(image.Rect(0, 0, 64, 64), 0, 360)
	clip.Close()
	return compose.Absolute{
		Size: composeImagePoint(74, 102), Clip: true,
		Children: []compose.Placed{
			{Bounds: image.Rect(0, 0, 74, 102), Node: compose.Rectangle{
				Size: composeImagePoint(74, 102), Radius: 4,
				Stroke: compose.Stroke(display.StrokeStyle{Ink: display.InkBlack, Width: 1}),
			}},
			{Bounds: image.Rect(5, 5, 69, 69), Node: compose.ClipPath{
				Size: composeImagePoint(64, 64), Path: clip,
				Child: compose.Image{Size: composeImagePoint(64, 64), Source: photo, Processing: compose.ImageAuto,
					Overrides: compose.ImageOverrides{Fit: &fit, Sampling: &sampling, Dither: &dither, DisableRed: &disableRed},
					Contrast:  &compose.Contrast{Radius: 4, Amount: 1.3}},
			}},
			{Bounds: image.Rect(5, 5, 69, 69), Node: compose.Circle{Center: image.Pt(32, 32), Radius: 32,
				Stroke: compose.Stroke(display.StrokeStyle{Ink: display.InkRed, Width: 1})}},
			{Bounds: image.Rect(7, 74, 67, 88), Node: compose.Text{Size: composeImagePoint(60, 14), Runs: []display.TextRun{
				run("PHOTO / ", "monaco", 10, display.InkBlack), run("人像", "hzk", 12, display.InkRed),
			}}},
			{Bounds: image.Rect(7, 89, 67, 101), Node: compose.Text{Size: composeImagePoint(60, 12), Runs: []display.TextRun{
				run("AUTO IMAGE", "monaco", 10, display.InkBlack),
			}}},
		},
	}
}

func centrePanel(grid, blueprint *display.Pattern) compose.Node {
	var wave display.Path
	wave.MoveTo(image.Pt(5, 41))
	wave.CubicTo(image.Pt(20, 10), image.Pt(30, 61), image.Pt(45, 31))
	wave.QuadraticTo(image.Pt(58, 9), image.Pt(73, 34))
	wave.CubicTo(image.Pt(88, 58), image.Pt(97, 15), image.Pt(111, 27))
	var mountain display.Path
	mountain.MoveTo(image.Pt(6, 55))
	mountain.LineTo(image.Pt(21, 35))
	mountain.LineTo(image.Pt(34, 50))
	mountain.QuadraticTo(image.Pt(48, 24), image.Pt(61, 49))
	mountain.LineTo(image.Pt(75, 34))
	mountain.LineTo(image.Pt(91, 55))
	mountain.Close()

	graph := compose.Stack{Size: composeImagePoint(116, 57), Children: []compose.Node{
		compose.Rectangle{Size: composeImagePoint(116, 57), Radius: 3, Fill: compose.Ink(display.InkWhite), Stroke: compose.Stroke(display.StrokeStyle{Ink: display.InkBlack, Width: 1})},
		compose.Pattern{Size: composeImagePoint(116, 57), Pattern: grid},
		compose.Path{Size: composeImagePoint(116, 57), Path: wave, Stroke: compose.Stroke(display.StrokeStyle{Ink: display.InkBlack, Width: 1, Dash: []int{4, 2}})},
		compose.Path{Size: composeImagePoint(116, 57), Path: mountain, Fill: compose.Ink(display.InkBlack)},
		compose.Circle{Center: image.Pt(101, 15), Radius: 7, Fill: compose.Ink(display.InkRed), Stroke: compose.Stroke(display.StrokeStyle{Ink: display.InkRed, Width: 1})},
	}}

	return compose.Absolute{Size: composeImagePoint(124, 102), Clip: true, Children: []compose.Placed{
		{Bounds: image.Rect(0, 0, 124, 102), Node: compose.Rectangle{Size: composeImagePoint(124, 102), Radius: 4, Stroke: compose.Stroke(display.StrokeStyle{Ink: display.InkBlack, Width: 1})}},
		{Bounds: image.Rect(4, 3, 120, 16), Node: compose.Text{Size: composeImagePoint(116, 13), Runs: []display.TextRun{
			run("DISPLAY LIST  ", "monaco", 10, display.InkBlack), run("图形", "hzk", 12, display.InkRed),
		}}},
		{Bounds: image.Rect(4, 18, 120, 75), Node: graph},
		{Bounds: image.Rect(4, 78, 120, 98), Node: compose.Row{Size: composeImagePoint(116, 20), Gap: 2, CrossAlign: compose.CrossCenter, Children: []compose.LayoutChild{
			{Node: compose.Text{Size: composeImagePoint(42, 14), Runs: []display.TextRun{run("R 87%", "monaco", 10, display.InkBlack)}}},
			{Node: compose.Text{Size: composeImagePoint(70, 14), Runs: []display.TextRun{run("TEMP 23.5", "monaco", 10, display.InkRed)}}},
		}}},
		{Bounds: image.Rect(4, 98, 120, 102), Node: compose.ClipPath{Size: composeImagePoint(116, 4), Path: horizontalClip(), Child: compose.Pattern{Size: composeImagePoint(116, 4), Pattern: blueprint}}},
	}}
}

func rightPanel() compose.Node {
	return compose.Absolute{Size: composeImagePoint(82, 102), Clip: true, Children: []compose.Placed{
		{Bounds: image.Rect(0, 0, 82, 102), Node: compose.Rectangle{Size: composeImagePoint(82, 102), Radius: 4, Stroke: compose.Stroke(display.StrokeStyle{Ink: display.InkBlack, Width: 1})}},
		{Bounds: image.Rect(5, 4, 77, 32), Node: compose.Column{Size: composeImagePoint(72, 28), Gap: 0, CrossAlign: compose.CrossStretch, Children: []compose.LayoutChild{
			{Node: compose.Text{Size: composeImagePoint(72, 13), Runs: []display.TextRun{run("PRIMITIVES", "monaco", 10, display.InkBlack)}}},
			{Node: compose.Text{Size: composeImagePoint(72, 13), Runs: []display.TextRun{
				run("图元", "hzk", 12, display.InkRed), run(" / COLOR", "monaco", 10, display.InkRed),
			}}},
		}}},
		{Bounds: image.Rect(6, 36, 76, 62), Node: compose.Absolute{Size: composeImagePoint(70, 26), Children: []compose.Placed{
			{Bounds: image.Rect(0, 0, 22, 22), Node: compose.Arc{Size: composeImagePoint(22, 22), Start: 25, Sweep: 290, Stroke: display.StrokeStyle{Ink: display.InkRed, Width: 2}}},
			{Bounds: image.Rect(25, 1, 46, 22), Node: compose.Pie{Size: composeImagePoint(21, 21), Start: -90, Sweep: 120, Ink: display.InkRed}},
			{Bounds: image.Rect(49, 1, 70, 22), Node: compose.Chord{Size: composeImagePoint(21, 21), Start: 20, Sweep: 220, Ink: display.InkBlack}},
		}}},
		{Bounds: image.Rect(6, 64, 76, 82), Node: compose.Absolute{Size: composeImagePoint(70, 18), Children: []compose.Placed{
			{Bounds: image.Rect(1, 1, 23, 17), Node: compose.Ellipse{Size: composeImagePoint(22, 16), Stroke: compose.Stroke(display.StrokeStyle{Ink: display.InkBlack, Width: 3})}},
			{Bounds: image.Rect(28, 1, 49, 17), Node: compose.Polygon{Size: composeImagePoint(21, 16), Points: []image.Point{image.Pt(10, 0), image.Pt(20, 8), image.Pt(10, 16), image.Pt(0, 8)}, Fill: compose.Ink(display.InkRed)}},
			{Bounds: image.Rect(55, 2, 69, 16), Node: compose.Pixel{At: image.Pt(3, 3), Ink: display.InkBlack, Size: composeImagePoint(14, 14)}},
		}}},
		{Bounds: image.Rect(6, 85, 76, 98), Node: compose.Absolute{Size: composeImagePoint(70, 13), Children: []compose.Placed{
			{Bounds: image.Rect(0, 0, 70, 13), Node: compose.Line{Size: composeImagePoint(70, 13), From: image.Pt(0, 12), To: image.Pt(70, 0), Stroke: display.StrokeStyle{Ink: display.InkRed, Width: 2, Dash: []int{3, 2}}}},
			{Bounds: image.Rect(2, 2, 18, 11), Node: compose.Rectangle{Size: composeImagePoint(16, 9), Fill: compose.Ink(display.InkBlack), Stroke: compose.Stroke(display.StrokeStyle{Ink: display.InkRed, Width: 1})}},
			{Bounds: image.Rect(44, 1, 68, 12), Node: compose.Polyline{Size: composeImagePoint(24, 11), Points: []image.Point{
				image.Pt(0, 8), image.Pt(6, 2), image.Pt(12, 8), image.Pt(18, 3), image.Pt(24, 8),
			}, Stroke: display.StrokeStyle{Ink: display.InkBlack, Width: 1}}},
		}}},
	}}
}

func horizontalClip() display.Path {
	var path display.Path
	path.MoveTo(image.Pt(0, 0))
	path.LineTo(image.Pt(116, 0))
	path.LineTo(image.Pt(116, 4))
	path.LineTo(image.Pt(0, 4))
	path.Close()
	return path
}

func composeImagePoint(x, y int) image.Point { return image.Pt(x, y) }

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
