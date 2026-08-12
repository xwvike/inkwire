package main

import (
	"flag"
	"fmt"
	"image"
	"image/color"
	"math"
	"os"

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
	frame, err := display.NewPage(display.OrientationLandscape, display.InkWhite)
	if err != nil {
		return nil, err
	}
	fonts, err := display.NewBuiltinFontRegistry()
	if err != nil {
		return nil, err
	}
	view := paintShowcase{canvas: display.NewCanvas(frame), fonts: fonts}
	if err := view.draw(); err != nil {
		return nil, err
	}
	return frame, nil
}

type paintShowcase struct {
	canvas *display.Canvas
	fonts  *display.FontRegistry
}

func (s paintShowcase) draw() error {
	if err := s.header(); err != nil {
		return err
	}
	if err := s.clippedPhoto(image.Rect(3, 22, 146, 126)); err != nil {
		return err
	}
	if err := s.patterns(image.Rect(149, 22, 293, 73)); err != nil {
		return err
	}
	return s.dashAngles(image.Rect(149, 76, 293, 126))
}

func (s paintShowcase) header() error {
	s.canvas.FillRect(image.Rect(0, 0, 296, 19), display.InkBlack)
	return s.text(image.Rect(5, 2, 291, 18), 15, display.AlignStart,
		run("CLIP ", "monaco", 12, display.InkWhite),
		run("裁剪", "hzk", 12, display.InkWhite),
		run("  PATTERN ", "monaco", 12, display.InkRed),
		run("图案", "hzk", 12, display.InkRed),
		run("  DASH ", "monaco", 12, display.InkWhite),
		run("虚线", "hzk", 12, display.InkWhite),
	)
}

// A photograph dropped into a rounded card: the clip is a path, so the corners
// come off the image itself rather than being painted over afterwards.
func (s paintShowcase) clippedPhoto(panel image.Rectangle) error {
	s.canvas.StrokeRoundRect(panel, 4, display.StrokeStyle{Ink: display.InkBlack, Width: 1})
	if err := s.label(panel, "CLIP PATH + IMAGE"); err != nil {
		return err
	}

	card := image.Rect(panel.Min.X+7, panel.Min.Y+20, panel.Max.X-7, panel.Max.Y-8)
	s.canvas.Save()
	s.canvas.ClipPath(roundedPath(card, 12))
	if err := s.canvas.DrawImage(syntheticPhoto(240, 128), card, display.ImageOptions{
		Fit:      display.FitCover,
		Sampling: display.SampleBilinear,
		Dither:   display.DitherFloydSteinberg,
	}); err != nil {
		s.canvas.Restore()
		return err
	}
	s.canvas.Restore()
	s.canvas.StrokeRoundRect(card, 12, display.StrokeStyle{Ink: display.InkRed, Width: 1})
	return nil
}

// Three tiles that a panel with no grey can still tell apart, then the same
// idea shaped by a clip instead of by a rectangle.
func (s paintShowcase) patterns(panel image.Rectangle) error {
	s.canvas.StrokeRoundRect(panel, 4, display.StrokeStyle{Ink: display.InkBlack, Width: 1})
	if err := s.label(panel, "PATTERN + CLIP"); err != nil {
		return err
	}

	tiles := []struct {
		rect image.Rectangle
		rows []string
		inks map[rune]display.Ink
	}{
		{image.Rect(155, 40, 194, 66), []string{"x.", ".x"}, map[rune]display.Ink{'x': display.InkBlack}},
		{image.Rect(198, 40, 237, 66), []string{"x...", ".x..", "..x.", "...x"}, map[rune]display.Ink{'x': display.InkBlack}},
	}
	for _, tile := range tiles {
		pattern, err := display.NewPattern(tile.rows, tile.inks)
		if err != nil {
			return err
		}
		s.canvas.FillPattern(tile.rect, pattern)
		s.canvas.StrokeRect(tile.rect, display.StrokeStyle{Ink: display.InkBlack, Width: 1})
	}

	// The third slot is a disc rather than a rectangle: FillPattern only fills
	// a box, so the shape comes from the clip.
	circle := image.Rect(241, 40, 267, 66)
	dots, err := display.NewPattern([]string{"x..", "...", ".x."}, map[rune]display.Ink{'x': display.InkRed})
	if err != nil {
		return err
	}
	var disc display.Path
	disc.Arc(circle, 0, 360)
	s.canvas.Save()
	s.canvas.ClipPath(disc)
	s.canvas.FillPattern(circle, dots)
	s.canvas.Restore()
	s.canvas.StrokeEllipse(circle, display.StrokeStyle{Ink: display.InkBlack, Width: 1})
	return nil
}

// Every ray carries the same [4,3] dash. They should read at the same density
// whatever their angle, which is only true once the dash is measured as
// distance along the line rather than as a count of raster steps.
func (s paintShowcase) dashAngles(panel image.Rectangle) error {
	s.canvas.StrokeRoundRect(panel, 4, display.StrokeStyle{Ink: display.InkBlack, Width: 1})
	if err := s.label(panel, "DASH AT ANY ANGLE"); err != nil {
		return err
	}

	// Five segments of identical length, so the dashes can simply be counted:
	// each one carries four on-runs no matter which way it points.
	const length = 26
	baseline := panel.Max.Y - 8
	dash := display.StrokeStyle{Ink: display.InkBlack, Width: 1, Dash: []int{4, 3}}
	for index, degrees := range []float64{0, 22.5, 45, 67.5, 90} {
		radians := degrees * math.Pi / 180
		from := image.Pt(panel.Min.X+8+index*28, baseline)
		to := image.Pt(
			from.X+int(math.Round(length*math.Cos(radians))),
			from.Y-int(math.Round(length*math.Sin(radians))),
		)
		style := dash
		if index%2 == 1 {
			style.Ink = display.InkRed
		}
		s.canvas.DrawLine(from, to, style)
	}
	return nil
}

func roundedPath(rect image.Rectangle, radius int) display.Path {
	var path display.Path
	path.Arc(image.Rect(rect.Min.X, rect.Min.Y, rect.Min.X+2*radius, rect.Min.Y+2*radius), 180, 90)
	path.Arc(image.Rect(rect.Max.X-2*radius, rect.Min.Y, rect.Max.X, rect.Min.Y+2*radius), 270, 90)
	path.Arc(image.Rect(rect.Max.X-2*radius, rect.Max.Y-2*radius, rect.Max.X, rect.Max.Y), 0, 90)
	path.Arc(image.Rect(rect.Min.X, rect.Max.Y-2*radius, rect.Min.X+2*radius, rect.Max.Y), 90, 90)
	path.Close()
	return path
}

// syntheticPhoto keeps the example self-contained: a soft gradient with a
// bright disc and a dark foreground, which is the kind of content that makes
// error diffusion worth using in the first place.
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

func (s paintShowcase) label(panel image.Rectangle, value string) error {
	return s.text(image.Rect(panel.Min.X+6, panel.Min.Y+3, panel.Max.X-4, panel.Min.Y+16), 12,
		display.AlignStart, run(value, "monaco", 10, display.InkBlack))
}

func (s paintShowcase) text(bounds image.Rectangle, lineHeight int, align display.HorizontalAlign, runs ...display.TextRun) error {
	layout, err := s.canvas.DrawTextBox(s.fonts, display.TextBox{
		Bounds: bounds, Runs: runs, Align: align, LineHeight: lineHeight,
	})
	if err != nil {
		return err
	}
	if missing := layout.MissingRunes(); len(missing) != 0 {
		return fmt.Errorf("missing paint showcase glyphs: %q", string(missing))
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
