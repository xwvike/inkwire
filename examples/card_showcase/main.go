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

	"github.com/xwvike/inkwire/internal/display"
)

//go:embed portrait.png
var portraitPNG []byte

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
	frame, err := display.NewPage(display.OrientationLandscape, display.InkWhite)
	if err != nil {
		return nil, err
	}
	fonts, err := display.NewBuiltinFontRegistry()
	if err != nil {
		return nil, err
	}
	photo, err := png.Decode(bytes.NewReader(portraitPNG))
	if err != nil {
		return nil, fmt.Errorf("decode portrait: %w", err)
	}

	// Recorded first, then replayed, so the card exercises the display list on
	// the way to the panel exactly as a real screen would.
	list := &display.DisplayList{}
	card := cardShowcase{list: list, fonts: fonts, photo: photo}
	if err := card.draw(); err != nil {
		return nil, err
	}
	if err := list.Replay(display.NewCanvas(frame)); err != nil {
		return nil, err
	}
	return frame, nil
}

type cardShowcase struct {
	list  *display.DisplayList
	fonts *display.FontRegistry
	photo image.Image
}

var (
	black1 = display.StrokeStyle{Ink: display.InkBlack, Width: 1}
	red1   = display.StrokeStyle{Ink: display.InkRed, Width: 1}
)

const batteryLevel = 0.72

func (c cardShowcase) draw() error {
	if err := c.header(); err != nil {
		return err
	}
	if err := c.portrait(batteryLevel); err != nil {
		return err
	}
	return c.details()
}

func (c cardShowcase) header() error {
	c.list.FillRect(image.Rect(0, 0, 296, 20), display.InkBlack)
	if err := c.text(image.Rect(6, 2, 200, 19), 16, display.AlignStart,
		run("INKWIRE ", "monaco", 12, display.InkWhite),
		run("墨水屏名片", "hzk", 14, display.InkWhite),
	); err != nil {
		return err
	}
	// A filled polygon, kept as the one purely decorative mark on the card.
	c.list.FillPolygon(starPoints(image.Pt(212, 10), 7, 3), display.InkRed)
	return c.text(image.Rect(222, 3, 292, 18), 14, display.AlignEnd,
		run("296x128 BWR", "monaco", 10, display.InkRed),
	)
}

// The portrait is a photograph shaped by a clip, not a rectangle painted over
// afterwards: the circle comes from ClipPath and the image never leaves it.
// The ring around it doubles as the battery dial, drawn as a partial arc, which
// is an open curve and so stays centred on its own path.
func (c cardShowcase) portrait(fraction float64) error {
	center, radius := image.Pt(51, 67), 41
	c.list.Save()
	c.list.ClipPath(circlePath(center, radius))
	if err := c.list.DrawImage(preparePortrait(c.photo), circleRect(center, radius+1), display.ImageOptions{
		Fit:        display.FitCover,
		Sampling:   display.SampleBilinear,
		Dither:     display.DitherFloydSteinberg,
		DisableRed: true,
		// The room behind the subject is warm enough to reach the red plane,
		// which would scatter red through a photograph that has none in it.
	}); err != nil {
		c.list.Restore()
		return err
	}
	c.list.Restore()
	c.list.StrokeCircle(center, radius, black1)

	dial := circleRect(center, radius+4)
	c.list.DrawArc(dial, 0, 360, display.StrokeStyle{Ink: display.InkBlack, Width: 1, Dash: []int{2, 4}})
	c.list.DrawArc(dial, -90, 360*fraction, display.StrokeStyle{Ink: display.InkRed, Width: 2})

	return c.text(image.Rect(4, 114, 98, 127), 13, display.AlignCenter,
		run("PET ", "monaco", 10, display.InkBlack),
		run("宠物", "hzk", 12, display.InkBlack),
	)
}

// preparePortrait crops to the head and then lifts local contrast. A
// photograph of a dark subject is the hard case for this panel: at portrait
// size the head thresholds to one flat black mass and the eyes, which are
// mid-tone against dark fur, disappear into it. Boosting local contrast first
// pulls them back out. Line art needs none of this, which is why the encoder
// alone was enough for the previous artwork.
func preparePortrait(source image.Image) image.Image {
	return unsharpMask(cropToHead(source), 10, 2.4)
}

func cropToHead(source image.Image) image.Image {
	cropper, ok := source.(interface {
		SubImage(image.Rectangle) image.Image
	})
	if !ok {
		return source
	}
	bounds := source.Bounds()
	crop := image.Rect(
		bounds.Min.X+bounds.Dx()*31/100,
		bounds.Min.Y+bounds.Dy()*6/100,
		bounds.Min.X+bounds.Dx()*81/100,
		bounds.Min.Y+bounds.Dy()*56/100,
	)
	return cropper.SubImage(crop)
}

// unsharpMask returns src + amount*(src - blur(src)) in grayscale. Flat areas
// keep their tone while edges and texture pull apart.
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
	blurred := boxBlur(boxBlurRows(gray, width, height, radius), width, height, radius)

	out := image.NewNRGBA(image.Rect(0, 0, width, height))
	for y := range height {
		for x := range width {
			value := gray[y*width+x] + amount*(gray[y*width+x]-blurred[y*width+x])
			level := uint8(min(255, max(0, int(value))))
			out.SetNRGBA(x, y, color.NRGBA{R: level, G: level, B: level, A: 0xff})
		}
	}
	return out
}

func boxBlurRows(values []float64, width, height, radius int) []float64 {
	out := make([]float64, len(values))
	for y := range height {
		for x := range width {
			sum, count := 0.0, 0
			for d := -radius; d <= radius; d++ {
				if x+d >= 0 && x+d < width {
					sum += values[y*width+x+d]
					count++
				}
			}
			out[y*width+x] = sum / float64(count)
		}
	}
	return out
}

func boxBlur(values []float64, width, height, radius int) []float64 {
	out := make([]float64, len(values))
	for y := range height {
		for x := range width {
			sum, count := 0.0, 0
			for d := -radius; d <= radius; d++ {
				if y+d >= 0 && y+d < height {
					sum += values[(y+d)*width+x]
					count++
				}
			}
			out[y*width+x] = sum / float64(count)
		}
	}
	return out
}

func (c cardShowcase) details() error {
	c.list.DrawLine(image.Pt(101, 24), image.Pt(101, 122), black1)

	if err := c.text(image.Rect(108, 24, 200, 44), 20, display.AlignStart,
		run("小黑", "hzk", 16, display.InkBlack),
	); err != nil {
		return err
	}
	badge := image.Rect(146, 27, 176, 41)
	c.list.FillRoundRect(badge, 3, display.InkRed)
	if err := c.text(badge.Inset(1), 12, display.AlignCenter,
		run("PRO", "monaco", 10, display.InkWhite),
	); err != nil {
		return err
	}
	if err := c.text(image.Rect(182, 28, 292, 42), 13, display.AlignEnd,
		run("ID 92943861", "monaco", 10, display.InkBlack),
	); err != nil {
		return err
	}

	c.list.DrawLine(image.Pt(108, 47), image.Pt(292, 47), display.StrokeStyle{
		Ink: display.InkBlack, Width: 1, Dash: []int{3, 3},
	})

	if err := c.levelRow(52, "电量", batteryLevel); err != nil {
		return err
	}
	if err := c.valueRow(70, "温度", "23.5", "℃"); err != nil {
		return err
	}
	if err := c.valueRow(88, "更新", "08-12 15:52", ""); err != nil {
		return err
	}
	return c.footer(image.Rect(108, 108, 292, 123))
}

// A pattern strip with the label knocked out of it, which only works because
// the tile is anchored to the frame: the two halves either side of the text
// stay in phase.
func (c cardShowcase) footer(strip image.Rectangle) error {
	dots, err := display.NewPattern([]string{"x.", ".."}, map[rune]display.Ink{'x': display.InkBlack})
	if err != nil {
		return err
	}
	c.list.FillPattern(strip, dots)
	c.list.StrokeRect(strip, black1)
	plate := image.Rect(strip.Min.X+26, strip.Min.Y+2, strip.Max.X-26, strip.Max.Y-2)
	c.list.FillRect(plate, display.InkWhite)
	return c.text(plate, 12, display.AlignCenter,
		run("BLUETOOTH ", "monaco", 10, display.InkBlack),
		run("蓝牙远程屏", "hzk", 12, display.InkRed),
	)
}

// A bar whose filled part is a pattern rather than solid ink, shaped by the
// rounded rectangle it sits in.
func (c cardShowcase) levelRow(top int, label string, fraction float64) error {
	if err := c.text(image.Rect(108, top, 140, top+14), 14, display.AlignStart,
		run(label, "hzk", 12, display.InkBlack),
	); err != nil {
		return err
	}
	track := image.Rect(144, top+2, 236, top+13)
	filled := image.Rect(track.Min.X, track.Min.Y, track.Min.X+int(float64(track.Dx())*fraction), track.Max.Y)

	hatch, err := display.NewPattern([]string{"xx..", ".xx.", "..xx", "x..x"}, map[rune]display.Ink{'x': display.InkBlack})
	if err != nil {
		return err
	}
	c.list.Save()
	c.list.ClipPath(roundRectPath(filled, 3))
	c.list.FillPattern(filled, hatch)
	c.list.Restore()
	c.list.StrokeRoundRect(track, 3, black1)

	return c.text(image.Rect(240, top, 292, top+14), 14, display.AlignEnd,
		run(fmt.Sprintf("%d%%", int(fraction*100)), "monaco", 12, display.InkRed),
	)
}

func (c cardShowcase) valueRow(top int, label, value, unit string) error {
	if err := c.text(image.Rect(108, top, 140, top+14), 14, display.AlignStart,
		run(label, "hzk", 12, display.InkBlack),
	); err != nil {
		return err
	}
	runs := []display.TextRun{run(value, "monaco", 12, display.InkBlack)}
	if unit != "" {
		runs = append(runs, run(unit, "hzk", 12, display.InkRed))
	}
	return c.text(image.Rect(144, top, 244, top+14), 14, display.AlignStart, runs...)
}

func starPoints(center image.Point, outer, inner int) []image.Point {
	points := make([]image.Point, 0, 10)
	for i := range 10 {
		radius := float64(outer)
		if i%2 == 1 {
			radius = float64(inner)
		}
		angle := -math.Pi/2 + float64(i)*math.Pi/5
		points = append(points, image.Pt(
			center.X+int(math.Round(radius*math.Cos(angle))),
			center.Y+int(math.Round(radius*math.Sin(angle))),
		))
	}
	return points
}

func circleRect(center image.Point, radius int) image.Rectangle {
	return image.Rect(center.X-radius, center.Y-radius, center.X+radius+1, center.Y+radius+1)
}

func circlePath(center image.Point, radius int) display.Path {
	var path display.Path
	path.Arc(circleRect(center, radius), 0, 360)
	return path
}

func roundRectPath(rect image.Rectangle, radius int) display.Path {
	var path display.Path
	path.Arc(image.Rect(rect.Min.X, rect.Min.Y, rect.Min.X+2*radius, rect.Min.Y+2*radius), 180, 90)
	path.Arc(image.Rect(rect.Max.X-2*radius, rect.Min.Y, rect.Max.X, rect.Min.Y+2*radius), 270, 90)
	path.Arc(image.Rect(rect.Max.X-2*radius, rect.Max.Y-2*radius, rect.Max.X, rect.Max.Y), 0, 90)
	path.Arc(image.Rect(rect.Min.X, rect.Max.Y-2*radius, rect.Min.X+2*radius, rect.Max.Y), 90, 90)
	path.Close()
	return path
}

func (c cardShowcase) text(bounds image.Rectangle, lineHeight int, align display.HorizontalAlign, runs ...display.TextRun) error {
	layout, err := c.list.DrawTextBox(c.fonts, display.TextBox{
		Bounds: bounds, Runs: runs, Align: align, LineHeight: lineHeight,
	})
	if err != nil {
		return err
	}
	if missing := layout.MissingRunes(); len(missing) != 0 {
		return fmt.Errorf("missing card showcase glyphs: %q", string(missing))
	}
	return nil
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
