package main

import (
	"bytes"
	_ "embed"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"

	"github.com/xwvike/inkwire/internal/compose"
	"github.com/xwvike/inkwire/internal/display"
)

//go:embed portrait.png
var portraitPNG []byte

const batteryLevel = 0.72

func main() {
	pngPath := flag.String("png", "card_showcase.png", "PNG preview output path")
	payloadPath := flag.String("payload", "", "optional Gicisky payload output path")
	flag.Parse()

	frame, err := renderCardShowcase()
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

func renderCardShowcase() (*display.Frame, error) {
	photo, err := png.Decode(bytes.NewReader(portraitPNG))
	if err != nil {
		return nil, fmt.Errorf("decode portrait: %w", err)
	}
	photo = preparePortrait(photo)
	fit, sampling, dither, disableRed := display.FitCover, display.SampleBilinear, display.DitherFloydSteinberg, true
	var portraitClip display.Path
	portraitClip.Arc(image.Rect(1, 1, 84, 84), 0, 360)

	hatch, err := display.NewPattern([]string{"xx..", ".xx.", "..xx", "x..x"}, map[rune]display.Ink{'x': display.InkBlack})
	if err != nil {
		return nil, err
	}

	compiler, err := compose.NewDefaultCompiler()
	if err != nil {
		return nil, err
	}
	compiled, report, err := compiler.Compile(compose.Document{
		Orientation: display.OrientationLandscape,
		Background:  compose.Value(display.InkWhite),
		Root: compose.Absolute{Size: image.Pt(296, 128), Clip: true, Children: []compose.Placed{
			{Bounds: image.Rect(0, 0, 296, 20), Node: compose.Rectangle{Size: image.Pt(296, 20), Fill: compose.Ink(display.InkBlack)}},
			{Bounds: image.Rect(6, 2, 200, 19), Node: compose.Text{Size: image.Pt(194, 17), Runs: []display.TextRun{run("INKWIRE ", "monaco", 12, display.InkWhite), run("墨水屏名片", "hzk", 14, display.InkWhite)}}},
			{Bounds: image.Rect(205, 3, 220, 18), Node: compose.Polygon{Size: image.Pt(15, 15), Points: starPoints(image.Pt(7, 7), 7, 3), Fill: compose.Ink(display.InkRed)}},
			{Bounds: image.Rect(222, 3, 292, 18), Node: compose.Text{Size: image.Pt(70, 15), Align: display.AlignEnd, Runs: []display.TextRun{run("296x128 BWR", "monaco", 10, display.InkRed)}}},
			{Bounds: image.Rect(4, 24, 98, 128), Node: portraitCard(photo, portraitClip, fit, sampling, dither, disableRed)},
			{Bounds: image.Rect(101, 24, 296, 128), Node: detailsCard(hatch)},
		}},
	})
	if err != nil {
		return nil, err
	}
	if len(report.MissingRunes) != 0 || len(report.Warnings) != 0 {
		return nil, fmt.Errorf("card compose report: missing=%q warnings=%v", string(report.MissingRunes), report.Warnings)
	}
	return compiled.Render()
}

func portraitCard(photo image.Image, clip display.Path, fit display.ImageFit, sampling display.SamplingMode, dither display.DitherMode, disableRed bool) compose.Node {
	return compose.Absolute{Size: image.Pt(94, 104), Children: []compose.Placed{
		{Bounds: image.Rect(5, 1, 90, 86), Node: compose.ClipPath{Size: image.Pt(85, 85), Path: clip, Child: compose.Image{Size: image.Pt(85, 85), Source: photo, Processing: compose.ImageManual, Options: display.ImageOptions{Fit: fit, Sampling: sampling, Dither: dither, DisableRed: disableRed}}}},
		{Bounds: image.Rect(0, 0, 94, 90), Node: compose.Circle{Center: image.Pt(47, 43), Radius: 41, Stroke: compose.Stroke(display.StrokeStyle{Ink: display.InkBlack, Width: 1})}},
		{Bounds: image.Rect(2, -2, 93, 89), Node: compose.Arc{Size: image.Pt(91, 91), Start: 0, Sweep: 360, Stroke: display.StrokeStyle{Ink: display.InkBlack, Width: 1, Dash: []int{2, 4}}}},
		{Bounds: image.Rect(0, 90, 94, 104), Node: compose.Text{Size: image.Pt(94, 14), Align: display.AlignCenter, Runs: []display.TextRun{run("PET ", "monaco", 10, display.InkBlack), run("宠物", "hzk", 12, display.InkBlack)}}},
		{Bounds: image.Rect(2, -2, 93, 89), Node: compose.Arc{Size: image.Pt(91, 91), Start: -90, Sweep: 360 * batteryLevel, Stroke: display.StrokeStyle{Ink: display.InkRed, Width: 2}}},
	}}
}

func detailsCard(hatch *display.Pattern) compose.Node {
	return compose.Absolute{Size: image.Pt(195, 104), Clip: true, Children: []compose.Placed{
		{Bounds: image.Rect(0, 0, 1, 98), Node: compose.Line{Size: image.Pt(1, 98), From: image.Pt(0, 0), To: image.Pt(0, 98), Stroke: display.StrokeStyle{Ink: display.InkBlack, Width: 1}}},
		{Bounds: image.Rect(7, 0, 99, 20), Node: compose.Text{Size: image.Pt(92, 20), Runs: []display.TextRun{run("小黑", "hzk", 16, display.InkBlack)}}},
		{Bounds: image.Rect(45, 3, 75, 17), Node: compose.Rectangle{Size: image.Pt(30, 14), Radius: 3, Fill: compose.Ink(display.InkRed)}},
		{Bounds: image.Rect(46, 4, 74, 16), Node: compose.Text{Size: image.Pt(28, 12), Align: display.AlignCenter, Runs: []display.TextRun{run("PRO", "monaco", 10, display.InkWhite)}}},
		{Bounds: image.Rect(81, 4, 191, 18), Node: compose.Text{Size: image.Pt(110, 14), Align: display.AlignEnd, Runs: []display.TextRun{run("ID 92943861", "monaco", 10, display.InkBlack)}}},
		{Bounds: image.Rect(7, 22, 191, 24), Node: compose.Line{Size: image.Pt(184, 2), From: image.Pt(0, 1), To: image.Pt(184, 1), Stroke: display.StrokeStyle{Ink: display.InkBlack, Width: 1, Dash: []int{3, 3}}}},
		{Bounds: image.Rect(7, 28, 191, 45), Node: levelRow("电量", batteryLevel, hatch)},
		{Bounds: image.Rect(7, 46, 191, 63), Node: valueRow("温度", "23.5", "℃")},
		{Bounds: image.Rect(7, 64, 191, 81), Node: valueRow("更新", "08-12 15:52", "")},
		{Bounds: image.Rect(7, 84, 191, 99), Node: footerNode()},
	}}
}

func levelRow(label string, fraction float64, hatch *display.Pattern) compose.Node {
	track := image.Rect(36, 2, 128, 13)
	filled := image.Rect(track.Min.X, track.Min.Y, track.Min.X+int(float64(track.Dx())*fraction), track.Max.Y)
	return compose.Absolute{Size: image.Pt(184, 17), Children: []compose.Placed{
		{Bounds: image.Rect(0, 0, 32, 14), Node: compose.Text{Size: image.Pt(32, 14), Runs: []display.TextRun{run(label, "hzk", 12, display.InkBlack)}}},
		{Bounds: image.Rect(36, 2, 36+filled.Dx(), 13), Node: compose.ClipPath{Size: filled.Size(), Path: roundRectLocalPath(filled.Dx(), filled.Dy(), 3), Child: compose.Pattern{Size: filled.Size(), Pattern: hatch}}},
		{Bounds: track, Node: compose.Rectangle{Size: track.Size(), Radius: 3, Stroke: compose.Stroke(display.StrokeStyle{Ink: display.InkBlack, Width: 1})}},
		{Bounds: image.Rect(132, 0, 184, 14), Node: compose.Text{Size: image.Pt(52, 14), Align: display.AlignEnd, Runs: []display.TextRun{run(fmt.Sprintf("%d%%", int(fraction*100)), "monaco", 12, display.InkRed)}}},
	}}
}

func valueRow(label, value, unit string) compose.Node {
	runs := []display.TextRun{run(value, "monaco", 12, display.InkBlack)}
	if unit != "" {
		runs = append(runs, run(unit, "hzk", 12, display.InkRed))
	}
	return compose.Absolute{Size: image.Pt(184, 17), Children: []compose.Placed{
		{Bounds: image.Rect(0, 0, 32, 14), Node: compose.Text{Size: image.Pt(32, 14), Runs: []display.TextRun{run(label, "hzk", 12, display.InkBlack)}}},
		{Bounds: image.Rect(36, 0, 136, 14), Node: compose.Text{Size: image.Pt(100, 14), Runs: runs}},
	}}
}

func footerNode() compose.Node {
	strip := image.Rect(0, 0, 184, 15)
	pattern, _ := display.NewPattern([]string{"x.", ".."}, map[rune]display.Ink{'x': display.InkBlack})
	return compose.Absolute{Size: strip.Size(), Children: []compose.Placed{
		{Bounds: strip, Node: compose.Pattern{Size: strip.Size(), Pattern: pattern}},
		{Bounds: strip, Node: compose.Rectangle{Size: strip.Size(), Stroke: compose.Stroke(display.StrokeStyle{Ink: display.InkBlack, Width: 1})}},
		{Bounds: image.Rect(26, 2, 158, 13), Node: compose.Rectangle{Size: image.Pt(132, 11), Fill: compose.Ink(display.InkWhite)}},
		{Bounds: image.Rect(26, 2, 158, 13), Node: compose.Text{Size: image.Pt(132, 11), Align: display.AlignCenter, Runs: []display.TextRun{run("BLUETOOTH ", "monaco", 10, display.InkBlack), run("蓝牙远程屏", "hzk", 12, display.InkRed)}}},
	}}
}

func preparePortrait(source image.Image) image.Image {
	if cropper, ok := source.(interface {
		SubImage(image.Rectangle) image.Image
	}); ok {
		bounds := source.Bounds()
		source = cropper.SubImage(image.Rect(bounds.Min.X+bounds.Dx()*31/100, bounds.Min.Y+bounds.Dy()*6/100, bounds.Min.X+bounds.Dx()*81/100, bounds.Min.Y+bounds.Dy()*56/100))
	}
	return unsharpMask(source, 10, 2.4)
}

func unsharpMask(source image.Image, radius int, amount float64) image.Image {
	bounds := source.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	gray := make([]float64, width*height)
	for y := range height {
		for x := range width {
			c := color.NRGBAModel.Convert(source.At(bounds.Min.X+x, bounds.Min.Y+y)).(color.NRGBA)
			gray[y*width+x] = 0.2126*float64(c.R) + 0.7152*float64(c.G) + 0.0722*float64(c.B)
		}
	}
	blurred := blurAxis(blurAxis(gray, width, height, radius, true), width, height, radius, false)
	out := image.NewNRGBA(image.Rect(0, 0, width, height))
	for index, value := range gray {
		level := uint8(min(255, max(0, int(value+amount*(value-blurred[index])))))
		out.SetNRGBA(index%width, index/width, color.NRGBA{R: level, G: level, B: level, A: 0xff})
	}
	return out
}

func blurAxis(values []float64, width, height, radius int, horizontal bool) []float64 {
	out := make([]float64, len(values))
	for y := range height {
		for x := range width {
			sum, count := 0.0, 0
			for offset := -radius; offset <= radius; offset++ {
				sampleX, sampleY := x, y
				if horizontal {
					sampleX += offset
				} else {
					sampleY += offset
				}
				if sampleX >= 0 && sampleX < width && sampleY >= 0 && sampleY < height {
					sum += values[sampleY*width+sampleX]
					count++
				}
			}
			out[y*width+x] = sum / float64(count)
		}
	}
	return out
}

func starPoints(center image.Point, outer, inner int) []image.Point {
	points := make([]image.Point, 0, 10)
	for i := range 10 {
		radius := float64(outer)
		if i%2 == 1 {
			radius = float64(inner)
		}
		angle := -math.Pi/2 + float64(i)*math.Pi/5
		points = append(points, image.Pt(center.X+int(math.Round(radius*math.Cos(angle))), center.Y+int(math.Round(radius*math.Sin(angle)))))
	}
	return points
}

func roundRectLocalPath(width, height, radius int) display.Path {
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
