package main

import (
	"flag"
	"fmt"
	"image"
	"os"

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
	frame, err := display.NewPage(display.OrientationLandscape, display.InkWhite)
	if err != nil {
		return nil, err
	}
	fonts, err := display.NewBuiltinFontRegistry()
	if err != nil {
		return nil, err
	}
	list := &display.DisplayList{}
	view := showcase{list: list, fonts: fonts}
	if err := view.draw(); err != nil {
		return nil, err
	}
	if err := list.Replay(display.NewCanvas(frame)); err != nil {
		return nil, err
	}
	return frame, nil
}

type showcase struct {
	list  *display.DisplayList
	fonts *display.FontRegistry
}

func (s showcase) draw() error {
	black1 := display.StrokeStyle{Ink: display.InkBlack, Width: 1}
	black2 := display.StrokeStyle{Ink: display.InkBlack, Width: 2}
	red1 := display.StrokeStyle{Ink: display.InkRed, Width: 1}

	s.list.StrokeRoundRect(image.Rect(1, 1, 295, 127), 6, black1)
	s.list.FillRoundRect(image.Rect(3, 3, 293, 26), 5, display.InkBlack)
	if err := s.text(image.Rect(8, 5, 288, 24), 18, display.AlignStart,
		run("INKWIRE  ", "monaco", 14, display.InkWhite),
		run("图元与文字展示", "ui", 14, display.InkWhite),
		run("  RBW", "monaco", 14, display.InkRed),
	); err != nil {
		return err
	}

	left := image.Rect(4, 29, 99, 124)
	middle := image.Rect(101, 29, 198, 124)
	right := image.Rect(200, 29, 292, 124)
	s.list.StrokeRoundRect(left, 5, black1)
	s.list.StrokeRoundRect(middle, 5, black1)
	s.list.StrokeRoundRect(right, 5, black1)

	if err := s.label(image.Rect(9, 32, 94, 44), "BASIC / SHAPES"); err != nil {
		return err
	}
	s.list.DrawLine(image.Pt(9, 47), image.Pt(45, 47), black1)
	s.list.DrawLine(image.Pt(52, 47), image.Pt(92, 47), display.StrokeStyle{
		Ink: display.InkRed, Width: 2, Dash: []int{3, 2}, DashOffset: 1,
	})
	s.list.StrokeRect(image.Rect(9, 53, 34, 67), black1)
	s.list.FillRect(image.Rect(39, 53, 62, 67), display.InkRed)
	s.list.StrokeRoundRect(image.Rect(67, 53, 93, 67), 4, black2)
	s.list.StrokeCircle(image.Pt(17, 80), 7, black1)
	s.list.FillCircle(image.Pt(38, 80), 7, display.InkRed)
	s.list.StrokeEllipse(image.Rect(49, 73, 69, 88), red1)
	s.list.FillEllipse(image.Rect(74, 73, 94, 88), display.InkBlack)
	s.list.DrawPolyline([]image.Point{
		image.Pt(9, 101), image.Pt(18, 92), image.Pt(27, 101), image.Pt(36, 92), image.Pt(45, 101),
	}, red1)
	s.list.StrokePolygon([]image.Point{
		image.Pt(52, 118), image.Pt(58, 98), image.Pt(70, 94), image.Pt(79, 106), image.Pt(72, 118),
	}, black1)
	s.list.FillPolygon([]image.Point{
		image.Pt(77, 118), image.Pt(86, 96), image.Pt(95, 118),
	}, display.InkRed)

	if err := s.label(image.Rect(106, 32, 193, 44), "PATH / ARC"); err != nil {
		return err
	}
	var wave display.Path
	wave.MoveTo(image.Pt(107, 52))
	wave.CubicTo(image.Pt(124, 35), image.Pt(143, 67), image.Pt(159, 51))
	wave.QuadraticTo(image.Pt(176, 38), image.Pt(193, 52))
	s.list.StrokePath(wave, black1)
	s.list.DrawArc(image.Rect(106, 59, 137, 89), 140, 260, display.StrokeStyle{
		Ink: display.InkRed, Width: 2, Dash: []int{4, 2},
	})
	s.list.FillPie(image.Rect(141, 61, 166, 86), -90, 125, display.InkRed)
	s.list.FillChord(image.Rect(170, 60, 194, 86), 20, 210, display.InkBlack)

	var landscape display.Path
	landscape.MoveTo(image.Pt(106, 118))
	landscape.LineTo(image.Pt(116, 98))
	landscape.LineTo(image.Pt(127, 110))
	landscape.QuadraticTo(image.Pt(139, 88), image.Pt(151, 110))
	landscape.CubicTo(image.Pt(162, 102), image.Pt(172, 91), image.Pt(181, 118))
	landscape.Close()
	s.list.FillPath(landscape, display.InkBlack)
	s.list.StrokeCircle(image.Pt(188, 106), 10, display.StrokeStyle{Ink: display.InkRed, Width: 3})

	if err := s.label(image.Rect(205, 32, 287, 44), "TYPE / COLOR"); err != nil {
		return err
	}
	if err := s.text(image.Rect(205, 44, 287, 63), 18, display.AlignStart,
		run("中文", "hzk", 16, display.InkBlack),
		run("16", "monaco", 16, display.InkRed),
	); err != nil {
		return err
	}
	if err := s.text(image.Rect(205, 64, 287, 80), 16, display.AlignStart,
		run("图元", "hzk", 14, display.InkBlack),
		run("14", "monaco", 14, display.InkRed),
	); err != nil {
		return err
	}
	if err := s.text(image.Rect(205, 81, 288, 96), 14, display.AlignStart,
		run("中文12 ", "ui", 12, display.InkBlack),
		run("ABC", "monaco", 12, display.InkRed),
	); err != nil {
		return err
	}
	if err := s.text(image.Rect(205, 97, 287, 110), 12, display.AlignCenter,
		run("MONACO10 23.5", "monaco", 10, display.InkBlack),
	); err != nil {
		return err
	}
	s.list.FillRoundRect(image.Rect(204, 110, 288, 121), 3, display.InkBlack)
	if err := s.text(image.Rect(207, 110, 285, 121), 11, display.AlignCenter,
		run("WHITE", "monaco", 10, display.InkWhite),
		run(" / RED", "monaco", 10, display.InkRed),
	); err != nil {
		return err
	}
	return nil
}

func (s showcase) label(bounds image.Rectangle, value string) error {
	return s.text(bounds, 12, display.AlignStart, run(value, "monaco", 10, display.InkBlack))
}

func (s showcase) text(bounds image.Rectangle, lineHeight int, align display.HorizontalAlign, runs ...display.TextRun) error {
	layout, err := s.list.DrawTextBox(s.fonts, display.TextBox{
		Bounds: bounds, Runs: runs, Align: align, LineHeight: lineHeight,
	})
	if err != nil {
		return err
	}
	if missing := layout.MissingRunes(); len(missing) != 0 {
		return fmt.Errorf("missing showcase glyphs: %q", string(missing))
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
