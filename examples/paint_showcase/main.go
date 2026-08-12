package main

import (
	"flag"
	"fmt"
	"image"
	"image/color"
	"math"
	"os"

	"github.com/xwvike/inkwire/internal/compose"
	"github.com/xwvike/inkwire/internal/display"
)

func main() {
	pngPath := flag.String("png", "paint_showcase.png", "PNG preview output path")
	payloadPath := flag.String("payload", "", "optional Gicisky payload output path")
	flag.Parse()

	frame, err := renderPaintShowcase()
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

func renderPaintShowcase() (*display.Frame, error) {
	photo := syntheticPhoto(240, 128)
	grid, err := display.NewPattern([]string{"x.", ".x"}, map[rune]display.Ink{'x': display.InkBlack})
	if err != nil {
		return nil, err
	}
	diagonal, err := display.NewPattern([]string{"x...", ".x..", "..x.", "...x"}, map[rune]display.Ink{'x': display.InkBlack})
	if err != nil {
		return nil, err
	}
	dots, err := display.NewPattern([]string{"x..", "...", ".x."}, map[rune]display.Ink{'x': display.InkRed})
	if err != nil {
		return nil, err
	}
	var clip display.Path
	clip.Arc(image.Rect(0, 0, 128, 84), 0, 360)
	clip.Close()

	compiler, err := compose.NewDefaultCompiler()
	if err != nil {
		return nil, err
	}
	compiled, _, err := compiler.Compile(compose.Document{
		Orientation: display.OrientationLandscape,
		Background:  compose.Value(display.InkWhite),
		Root: compose.Absolute{Size: image.Pt(296, 128), Clip: true, Children: []compose.Placed{
			{Bounds: image.Rect(0, 0, 296, 19), Node: compose.Rectangle{Size: image.Pt(296, 19), Fill: compose.Ink(display.InkBlack)}},
			{Bounds: image.Rect(5, 2, 291, 18), Node: compose.Text{Size: image.Pt(286, 16), Runs: []display.TextRun{
				run("CLIP ", "monaco", 12, display.InkWhite), run("裁剪", "hzk", 12, display.InkWhite), run("  PATTERN ", "monaco", 12, display.InkRed), run("图案", "hzk", 12, display.InkRed), run("  DASH ", "monaco", 12, display.InkWhite), run("虚线", "hzk", 12, display.InkWhite),
			}}},
			{Bounds: image.Rect(3, 22, 146, 126), Node: photoCard(photo)},
			{Bounds: image.Rect(149, 22, 293, 73), Node: patternCard(grid, diagonal, dots)},
			{Bounds: image.Rect(149, 76, 293, 126), Node: dashCard()},
		}},
	})
	if err != nil {
		return nil, err
	}
	return compiled.Render()
}

func photoCard(photo image.Image) compose.Node {
	return compose.Absolute{Size: image.Pt(143, 104), Clip: true, Children: []compose.Placed{
		{Bounds: image.Rect(0, 0, 143, 104), Node: compose.Rectangle{Size: image.Pt(143, 104), Radius: 4, Stroke: compose.Stroke(display.StrokeStyle{Ink: display.InkBlack, Width: 1})}},
		{Bounds: image.Rect(7, 20, 136, 96), Node: compose.ClipPath{Size: image.Pt(129, 76), Path: roundedLocalPath(129, 76, 12), Child: compose.Image{Size: image.Pt(129, 76), Source: photo, Processing: compose.ImageManual, Options: display.ImageOptions{Fit: display.FitCover, Sampling: display.SampleBilinear, Dither: display.DitherFloydSteinberg}}}},
		{Bounds: image.Rect(7, 20, 136, 96), Node: compose.Rectangle{Size: image.Pt(129, 76), Radius: 12, Stroke: compose.Stroke(display.StrokeStyle{Ink: display.InkRed, Width: 1})}},
		{Bounds: image.Rect(6, 3, 137, 16), Node: compose.Text{Size: image.Pt(131, 13), Runs: []display.TextRun{run("CLIP PATH + IMAGE", "monaco", 10, display.InkBlack)}}},
	}}
}

func patternCard(grid, diagonal, dots *display.Pattern) compose.Node {
	return compose.Absolute{Size: image.Pt(144, 51), Clip: true, Children: []compose.Placed{
		{Bounds: image.Rect(0, 0, 144, 51), Node: compose.Rectangle{Size: image.Pt(144, 51), Radius: 4, Stroke: compose.Stroke(display.StrokeStyle{Ink: display.InkBlack, Width: 1})}},
		{Bounds: image.Rect(6, 3, 138, 16), Node: compose.Text{Size: image.Pt(132, 13), Runs: []display.TextRun{run("PATTERN + CLIP", "monaco", 10, display.InkBlack)}}},
		{Bounds: image.Rect(6, 18, 45, 44), Node: compose.Stack{Size: image.Pt(39, 26), Children: []compose.Node{compose.Pattern{Size: image.Pt(39, 26), Pattern: grid}, compose.Rectangle{Size: image.Pt(39, 26), Stroke: compose.Stroke(display.StrokeStyle{Ink: display.InkBlack, Width: 1})}}}},
		{Bounds: image.Rect(49, 18, 88, 44), Node: compose.Stack{Size: image.Pt(39, 26), Children: []compose.Node{compose.Pattern{Size: image.Pt(39, 26), Pattern: diagonal}, compose.Rectangle{Size: image.Pt(39, 26), Stroke: compose.Stroke(display.StrokeStyle{Ink: display.InkBlack, Width: 1})}}}},
		{Bounds: image.Rect(92, 18, 118, 44), Node: compose.Stack{Size: image.Pt(26, 26), Children: []compose.Node{compose.ClipPath{Size: image.Pt(26, 26), Path: circleBoxPath(26, 26), Child: compose.Pattern{Size: image.Pt(26, 26), Pattern: dots}}, compose.Ellipse{Size: image.Pt(26, 26), Stroke: compose.Stroke(display.StrokeStyle{Ink: display.InkBlack, Width: 1})}}}},
	}}
}

func dashCard() compose.Node {
	children := make([]compose.Placed, 0, 7)
	children = append(children, compose.Placed{Bounds: image.Rect(0, 0, 144, 50), Node: compose.Rectangle{Size: image.Pt(144, 50), Radius: 4, Stroke: compose.Stroke(display.StrokeStyle{Ink: display.InkBlack, Width: 1})}})
	children = append(children, compose.Placed{Bounds: image.Rect(6, 3, 138, 16), Node: compose.Text{Size: image.Pt(132, 13), Runs: []display.TextRun{run("DASH AT ANY ANGLE", "monaco", 10, display.InkBlack)}}})
	for index, degrees := range []float64{0, 22.5, 45, 67.5, 90} {
		radians := degrees * math.Pi / 180
		from := image.Pt(8+index*28, 42)
		to := image.Pt(from.X+int(math.Round(26*math.Cos(radians))), from.Y-int(math.Round(26*math.Sin(radians))))
		ink := display.InkBlack
		if index%2 == 1 {
			ink = display.InkRed
		}
		children = append(children, compose.Placed{Bounds: image.Rect(0, 0, 144, 50), Node: compose.Line{Size: image.Pt(144, 50), From: from, To: to, Stroke: display.StrokeStyle{Ink: ink, Width: 1, Dash: []int{4, 3}}}})
	}
	return compose.Absolute{Size: image.Pt(144, 50), Clip: true, Children: children}
}

func roundedLocalPath(width, height, radius int) display.Path {
	var path display.Path
	path.Arc(image.Rect(0, 0, 2*radius, 2*radius), 180, 90)
	path.Arc(image.Rect(width-2*radius, 0, width, 2*radius), 270, 90)
	path.Arc(image.Rect(width-2*radius, height-2*radius, width, height), 0, 90)
	path.Arc(image.Rect(0, height-2*radius, 2*radius, height), 90, 90)
	path.Close()
	return path
}

func circleLocalPath(cx, cy, radius int) display.Path {
	var path display.Path
	path.Arc(image.Rect(cx-radius, cy-radius, cx+radius+1, cy+radius+1), 0, 360)
	return path
}

func circleBoxPath(width, height int) display.Path {
	var path display.Path
	path.Arc(image.Rect(0, 0, width, height), 0, 360)
	return path
}

func syntheticPhoto(width, height int) image.Image {
	photo := image.NewNRGBA(image.Rect(0, 0, width, height))
	sun := image.Pt(width*2/3, height*7/20)
	horizon := height * 7 / 10
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			value := 30 + 150*y/height
			distance := math.Hypot(float64(x-sun.X), float64(y-sun.Y)) / float64(height/3)
			if distance < 1 {
				value += int(200 * (1 - distance*distance))
			}
			if y > horizon {
				ridge := 10 * math.Sin(float64(x)/17)
				if float64(y) > float64(horizon)+ridge {
					value = value/4 + 15
				}
			}
			level := uint8(min(255, max(0, value)))
			photo.SetNRGBA(x, y, color.NRGBA{R: level, G: level, B: level, A: 0xff})
		}
	}
	return photo
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
